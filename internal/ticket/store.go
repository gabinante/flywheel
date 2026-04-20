package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrVersionConflict = errors.New("ticket version conflict")

// Store persists tickets and manages per-project sequence.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// NextSequence atomically increments and returns the next ticket number (1-based) for the project.
func (s *Store) NextSequence(ctx context.Context, projectID string) (int64, error) {
	var next int64
	err := s.pool.QueryRow(ctx,
		`UPDATE ticket_sequences SET next_val = next_val + 1 WHERE project_id = $1 RETURNING next_val`,
		projectID).Scan(&next)
	return next, err
}

// Create inserts a ticket. ID must already be set (e.g. projectSlug-seq).
func (s *Store) Create(ctx context.Context, t *Ticket) error {
	objJSON, _ := json.Marshal(t.Objective)
	ctxJSON, _ := json.Marshal(t.Context)
	inJSON, _ := json.Marshal(t.Inputs)
	outJSON, _ := json.Marshal(t.Outputs)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO tickets (id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		t.ID, t.ProjectID, t.Title, string(t.Type), int(t.Priority), string(t.State), t.Version,
		objJSON, ctxJSON, inJSON, outJSON, t.DependsOn, nullIfEmpty(t.WorkStreamID), t.CreatedBy, t.CreatedAt, t.UpdatedAt)
	return err
}

// GetByID returns a ticket by ID.
func (s *Store) GetByID(ctx context.Context, id string) (*Ticket, error) {
	var t Ticket
	var objJSON, ctxJSON, inJSON, outJSON []byte
	var dependsOn []string
	var assignedTo *string
	var workStreamID *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, assigned_to, created_by, created_at, updated_at
		 FROM tickets WHERE id = $1`, id).
		Scan(&t.ID, &t.ProjectID, &t.Title, &t.Type, &t.Priority, &t.State, &t.Version,
			&objJSON, &ctxJSON, &inJSON, &outJSON, &dependsOn, &workStreamID, &assignedTo, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if assignedTo != nil {
		t.AssignedTo = *assignedTo
	}
	if workStreamID != nil {
		t.WorkStreamID = *workStreamID
	}
	_ = json.Unmarshal(objJSON, &t.Objective)
	_ = json.Unmarshal(ctxJSON, &t.Context)
	t.Inputs = make(map[string]any)
	_ = json.Unmarshal(inJSON, &t.Inputs)
	t.Outputs = make(map[string]any)
	_ = json.Unmarshal(outJSON, &t.Outputs)
	t.DependsOn = dependsOn
	return &t, nil
}

// GetByIDs returns tickets by IDs (for DAG). Missing IDs are skipped.
func (s *Store) GetByIDs(ctx context.Context, ids []string) ([]*Ticket, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, assigned_to, created_by, created_at, updated_at
		 FROM tickets WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Ticket
	for rows.Next() {
		var t Ticket
		var objJSON, ctxJSON, inJSON, outJSON []byte
		var dependsOn []string
		var workStreamID, assignedTo *string
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Type, &t.Priority, &t.State, &t.Version,
			&objJSON, &ctxJSON, &inJSON, &outJSON, &dependsOn, &workStreamID, &assignedTo, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if assignedTo != nil {
			t.AssignedTo = *assignedTo
		}
		if workStreamID != nil {
			t.WorkStreamID = *workStreamID
		}
		_ = json.Unmarshal(objJSON, &t.Objective)
		_ = json.Unmarshal(ctxJSON, &t.Context)
		t.Inputs = make(map[string]any)
		_ = json.Unmarshal(inJSON, &t.Inputs)
		t.Outputs = make(map[string]any)
		_ = json.Unmarshal(outJSON, &t.Outputs)
		t.DependsOn = dependsOn
		list = append(list, &t)
	}
	return list, rows.Err()
}

// GetByProject returns tickets for a project. If workStreamID is non-empty, filters by work_stream_id. If state is non-empty, filters by state.
func (s *Store) GetByProject(ctx context.Context, projectID string, workStreamID string, state State) ([]*Ticket, error) {
	q := `SELECT id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, assigned_to, created_by, created_at, updated_at
		 FROM tickets WHERE project_id = $1`
	args := []any{projectID}
	argNum := 2
	if workStreamID != "" {
		q += fmt.Sprintf(` AND work_stream_id = $%d`, argNum)
		args = append(args, workStreamID)
		argNum++
	}
	if state != "" {
		q += fmt.Sprintf(` AND state = $%d`, argNum)
		args = append(args, string(state))
	}
	q += ` ORDER BY priority, created_at`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanRows(rows)
}

// ListByState returns tickets in a given state for a project (for queue).
func (s *Store) ListByState(ctx context.Context, projectID string, state State) ([]*Ticket, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, assigned_to, created_by, created_at, updated_at
		 FROM tickets WHERE project_id = $1 AND state = $2 ORDER BY priority, created_at`, projectID, string(state))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanRows(rows)
}

// UpdateState updates state and version (optimistic lock). Returns error if version mismatch.
func (s *Store) UpdateState(ctx context.Context, id string, version int, newState State, assignedTo string) error {
	cmd, err := s.pool.Exec(ctx,
		`UPDATE tickets SET state = $1, version = version + 1, updated_at = now(), assigned_to = $2 WHERE id = $3 AND version = $4`,
		string(newState), nullIfEmpty(assignedTo), id, version)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

// UpdateOutputs sets outputs and updates state/version.
func (s *Store) UpdateOutputs(ctx context.Context, id string, version int, outputs map[string]any) error {
	outJSON, _ := json.Marshal(outputs)
	cmd, err := s.pool.Exec(ctx,
		`UPDATE tickets SET outputs = $1, version = version + 1, updated_at = now() WHERE id = $2 AND version = $3`,
		outJSON, id, version)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

// UpdateContext replaces the ticket_context JSONB (for appending prior attempts / human answer).
func (s *Store) UpdateContext(ctx context.Context, id string, ctxVal TicketContext) error {
	ctxJSON, _ := json.Marshal(ctxVal)
	_, err := s.pool.Exec(ctx, `UPDATE tickets SET ticket_context = $1, updated_at = now() WHERE id = $2`, ctxJSON, id)
	return err
}

// UpdateDependsOn sets the depends_on list for a ticket.
func (s *Store) UpdateDependsOn(ctx context.Context, id string, dependsOn []string) error {
	_, err := s.pool.Exec(ctx, `UPDATE tickets SET depends_on = $1, updated_at = now() WHERE id = $2`, dependsOn, id)
	return err
}

// UpdateWorkStreamID sets the work_stream_id for a ticket.
func (s *Store) UpdateWorkStreamID(ctx context.Context, id string, workStreamID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE tickets SET work_stream_id = $1, updated_at = now() WHERE id = $2`, nullIfEmpty(workStreamID), id)
	return err
}

// UpdateTitleAndObjective sets title and objective JSON (full objective replacement for stored column).
func (s *Store) UpdateTitleAndObjective(ctx context.Context, id string, title string, obj Objective) error {
	objJSON, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE tickets SET title = $1, objective = $2, updated_at = now() WHERE id = $3`, title, objJSON, id)
	return err
}

// CountByCreatedBy returns the number of tickets created by the given agent/user.
func (s *Store) CountByCreatedBy(ctx context.Context, createdBy string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tickets WHERE created_by = $1`, createdBy).Scan(&n)
	return n, err
}

// CountByCreatedByPerDay returns daily ticket counts for the given agent for the last days (UTC, oldest first).
// Slice length is always days; missing days have count 0.
func (s *Store) CountByCreatedByPerDay(ctx context.Context, createdBy string, days int) ([]int, error) {
	if days <= 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT (date_trunc('day', created_at AT TIME ZONE 'UTC')::date)::text AS d, COUNT(*)::int
		 FROM tickets WHERE created_by = $1 AND created_at >= ((now() AT TIME ZONE 'UTC')::date - $2)
		 GROUP BY 1 ORDER BY 1`, createdBy, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	countByDate := make(map[string]int)
	for rows.Next() {
		var d string
		var c int
		if err := rows.Scan(&d, &c); err != nil {
			return nil, err
		}
		countByDate[d] = c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Build slice for last N days (oldest first), 0 for missing days.
	out := make([]int, days)
	now := time.Now().UTC()
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -days+1+i).Format("2006-01-02")
		out[i] = countByDate[date]
	}
	return out, nil
}

// GetTicketIDByCreateIdempotency returns the ticket_id for (project_id, idempotency_key) if one was recorded.
func (s *Store) GetTicketIDByCreateIdempotency(ctx context.Context, projectID, idempotencyKey string) (string, error) {
	var ticketID string
	err := s.pool.QueryRow(ctx,
		`SELECT ticket_id FROM idempotency_creates WHERE project_id = $1 AND idempotency_key = $2`,
		projectID, idempotencyKey).Scan(&ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return ticketID, nil
}

// SetCreateIdempotency records (project_id, idempotency_key) -> ticket_id for create_ticket idempotency.
func (s *Store) SetCreateIdempotency(ctx context.Context, projectID, idempotencyKey, ticketID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO idempotency_creates (project_id, idempotency_key, ticket_id) VALUES ($1, $2, $3)
		 ON CONFLICT (project_id, idempotency_key) DO NOTHING`,
		projectID, idempotencyKey, ticketID)
	return err
}

// ListStaleTickets returns tickets in any of the given states whose updated_at
// is older than now - threshold. Scans across all projects.
func (s *Store) ListStaleTickets(ctx context.Context, states []State, threshold time.Duration) ([]*Ticket, error) {
	if len(states) == 0 {
		return nil, nil
	}
	stateStrings := make([]string, len(states))
	for i, st := range states {
		stateStrings[i] = string(st)
	}
	cutoff := time.Now().UTC().Add(-threshold)
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, assigned_to, created_by, created_at, updated_at
		 FROM tickets WHERE state = ANY($1) AND updated_at < $2 ORDER BY updated_at`, stateStrings, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanRows(rows)
}

func (s *Store) scanRows(rows pgx.Rows) ([]*Ticket, error) {
	var list []*Ticket
	for rows.Next() {
		var t Ticket
		var objJSON, ctxJSON, inJSON, outJSON []byte
		var dependsOn []string
		var workStreamID, assignedTo *string
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Type, &t.Priority, &t.State, &t.Version,
			&objJSON, &ctxJSON, &inJSON, &outJSON, &dependsOn, &workStreamID, &assignedTo, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if assignedTo != nil {
			t.AssignedTo = *assignedTo
		}
		if workStreamID != nil {
			t.WorkStreamID = *workStreamID
		}
		_ = json.Unmarshal(objJSON, &t.Objective)
		_ = json.Unmarshal(ctxJSON, &t.Context)
		t.Inputs = make(map[string]any)
		_ = json.Unmarshal(inJSON, &t.Inputs)
		t.Outputs = make(map[string]any)
		_ = json.Unmarshal(outJSON, &t.Outputs)
		t.DependsOn = dependsOn
		list = append(list, &t)
	}
	return list, rows.Err()
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
