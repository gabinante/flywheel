package review

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists reviews and escalations.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateReview inserts a review.
func (s *Store) CreateReview(ctx context.Context, r *Review) error {
	r.ID = uuid.Must(uuid.NewV7()).String()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO reviews (id, ticket_id, reviewer_id, decision, notes, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		r.ID, r.TicketID, r.ReviewerID, r.Decision, r.Notes, r.CreatedAt)
	return err
}

// CreateEscalation inserts an escalation.
func (s *Store) CreateEscalation(ctx context.Context, e *Escalation) error {
	e.ID = uuid.Must(uuid.NewV7()).String()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO escalations (id, ticket_id, agent_id, reason, question, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		e.ID, e.TicketID, e.AgentID, e.Reason, e.Question, e.CreatedAt)
	return err
}

// UpdateEscalationResolved sets answer and resolved_by/at.
func (s *Store) UpdateEscalationResolved(ctx context.Context, id, answer, resolvedBy string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE escalations SET answer = $1, resolved_by = $2, resolved_at = now() WHERE id = $3`,
		answer, resolvedBy, id)
	return err
}

// ListReviewsByTicket returns reviews for a ticket.
func (s *Store) ListReviewsByTicket(ctx context.Context, ticketID string) ([]Review, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ticket_id, reviewer_id, decision, notes, created_at FROM reviews WHERE ticket_id = $1 ORDER BY created_at`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Review
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.ID, &r.TicketID, &r.ReviewerID, &r.Decision, &r.Notes, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// CountByReviewer returns how many reviews the reviewer made (approved and rejected).
func (s *Store) CountByReviewer(ctx context.Context, reviewerID string) (approved, rejected int, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE decision = 'approved'), COUNT(*) FILTER (WHERE decision = 'rejected') FROM reviews WHERE reviewer_id = $1`,
		reviewerID).Scan(&approved, &rejected)
	return approved, rejected, err
}

// CountByReviewerPerDay returns daily approved and rejected counts for the given reviewer for the last days (UTC, oldest first).
// Slices are always length days; missing days have 0.
func (s *Store) CountByReviewerPerDay(ctx context.Context, reviewerID string, days int) (approved, rejected []int, err error) {
	if days <= 0 {
		return nil, nil, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT (date_trunc('day', created_at AT TIME ZONE 'UTC')::date)::text AS d,
		 COUNT(*) FILTER (WHERE decision = 'approved')::int AS a,
		 COUNT(*) FILTER (WHERE decision = 'rejected')::int AS r
		 FROM reviews WHERE reviewer_id = $1 AND created_at >= ((now() AT TIME ZONE 'UTC')::date - $2)
		 GROUP BY 1 ORDER BY 1`, reviewerID, days)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	type dayCount struct{ date string; a, r int }
	byDate := make(map[string]dayCount)
	for rows.Next() {
		var d string
		var a, r int
		if err := rows.Scan(&d, &a, &r); err != nil {
			return nil, nil, err
		}
		byDate[d] = dayCount{d, a, r}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	approved = make([]int, days)
	rejected = make([]int, days)
	now := time.Now().UTC()
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -days+1+i).Format("2006-01-02")
		if v, ok := byDate[date]; ok {
			approved[i] = v.a
			rejected[i] = v.r
		}
	}
	return approved, rejected, nil
}

// ListPendingReviewTicketIDs returns ticket IDs in state awaiting_validation for a project.
func (s *Store) ListPendingReviewTicketIDs(ctx context.Context, projectID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM tickets WHERE project_id = $1 AND state = 'awaiting_validation' ORDER BY updated_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetLatestUnresolvedEscalation returns the most recent unresolved escalation for a ticket, or nil if none.
func (s *Store) GetLatestUnresolvedEscalation(ctx context.Context, ticketID string) (*Escalation, error) {
	var e Escalation
	var answer, resolvedBy *string
	var resolvedAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, ticket_id, agent_id, reason, question, answer, resolved_by, resolved_at, created_at
		 FROM escalations WHERE ticket_id = $1 AND resolved_at IS NULL ORDER BY created_at DESC LIMIT 1`, ticketID).
		Scan(&e.ID, &e.TicketID, &e.AgentID, &e.Reason, &e.Question, &answer, &resolvedBy, &resolvedAt, &e.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if answer != nil {
		e.Answer = *answer
	}
	if resolvedBy != nil {
		e.ResolvedBy = *resolvedBy
	}
	if resolvedAt != nil {
		e.ResolvedAt = resolvedAt
	}
	return &e, nil
}

// ListEscalationsByProject returns unresolved escalations for a project.
func (s *Store) ListEscalationsByProject(ctx context.Context, projectID string) ([]Escalation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT e.id, e.ticket_id, e.agent_id, e.reason, e.question, e.answer, e.resolved_by, e.resolved_at, e.created_at
		 FROM escalations e JOIN tickets t ON t.id = e.ticket_id WHERE t.project_id = $1 AND e.resolved_at IS NULL ORDER BY e.created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Escalation
	for rows.Next() {
		var e Escalation
		var answer, resolvedBy *string
		var resolvedAt *time.Time
		if err := rows.Scan(&e.ID, &e.TicketID, &e.AgentID, &e.Reason, &e.Question, &answer, &resolvedBy, &resolvedAt, &e.CreatedAt); err != nil {
			return nil, err
		}
		if answer != nil {
			e.Answer = *answer
		}
		if resolvedBy != nil {
			e.ResolvedBy = *resolvedBy
		}
		if resolvedAt != nil {
			e.ResolvedAt = resolvedAt
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// GetEscalationByID returns one escalation.
func (s *Store) GetEscalationByID(ctx context.Context, id string) (*Escalation, error) {
	var e Escalation
	var answer, resolvedBy *string
	var resolvedAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, ticket_id, agent_id, reason, question, answer, resolved_by, resolved_at, created_at FROM escalations WHERE id = $1`, id).
		Scan(&e.ID, &e.TicketID, &e.AgentID, &e.Reason, &e.Question, &answer, &resolvedBy, &resolvedAt, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	if answer != nil {
		e.Answer = *answer
	}
	if resolvedBy != nil {
		e.ResolvedBy = *resolvedBy
	}
	if resolvedAt != nil {
		e.ResolvedAt = resolvedAt
	}
	return &e, nil
}
