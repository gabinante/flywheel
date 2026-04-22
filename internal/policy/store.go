package policy

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists policies, decisions, change events, and proposals.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// --- Policies ---

// CreatePolicy inserts a new policy.
func (s *Store) CreatePolicy(ctx context.Context, p *Policy) error {
	p.ID = uuid.Must(uuid.NewV7()).String()
	rulesJSON, err := json.Marshal(p.Rules)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO policies (id, project_id, name, description, rules, enabled, min_sample, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		p.ID, p.ProjectID, p.Name, p.Description, rulesJSON, p.Enabled, p.MinSample, p.CreatedAt, p.UpdatedAt)
	return err
}

// GetPolicy returns a policy by ID.
func (s *Store) GetPolicy(ctx context.Context, id string) (*Policy, error) {
	var p Policy
	var rulesJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, name, description, rules, enabled, min_sample, created_at, updated_at
		 FROM policies WHERE id = $1`, id).
		Scan(&p.ID, &p.ProjectID, &p.Name, &p.Description, &rulesJSON, &p.Enabled, &p.MinSample, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rulesJSON, &p.Rules); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListPolicies returns all policies for a project.
func (s *Store) ListPolicies(ctx context.Context, projectID string, enabledOnly bool) ([]Policy, error) {
	query := `SELECT id, project_id, name, description, rules, enabled, min_sample, created_at, updated_at
		 FROM policies WHERE project_id = $1`
	if enabledOnly {
		query += ` AND enabled = true`
	}
	query += ` ORDER BY created_at`
	rows, err := s.pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Policy
	for rows.Next() {
		var p Policy
		var rulesJSON []byte
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.Name, &p.Description, &rulesJSON, &p.Enabled, &p.MinSample, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rulesJSON, &p.Rules); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// UpdatePolicy updates a policy's name, description, rules, and enabled status.
func (s *Store) UpdatePolicy(ctx context.Context, id string, name, description string, rules Rules, enabled bool, minSample int) error {
	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE policies SET name = $1, description = $2, rules = $3, enabled = $4, min_sample = $5, updated_at = now()
		 WHERE id = $6`,
		name, description, rulesJSON, enabled, minSample, id)
	return err
}

// --- Policy Decisions ---

// RecordDecision inserts a policy decision.
func (s *Store) RecordDecision(ctx context.Context, d *PolicyDecision) error {
	d.ID = uuid.Must(uuid.NewV7()).String()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO policy_decisions (id, policy_id, ticket_id, decision, reason, decided_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		d.ID, d.PolicyID, d.TicketID, d.Decision, d.Reason, d.DecidedAt)
	return err
}

// RecordOutcome updates the outcome of a previously recorded decision.
func (s *Store) RecordOutcome(ctx context.Context, decisionID string, outcome Outcome) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`UPDATE policy_decisions SET outcome = $1, outcome_at = $2 WHERE id = $3`,
		outcome, now, decisionID)
	return err
}

// RecordOutcomeByTicket updates outcomes for all decisions on a ticket.
func (s *Store) RecordOutcomeByTicket(ctx context.Context, ticketID string, outcome Outcome) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`UPDATE policy_decisions SET outcome = $1, outcome_at = $2 WHERE ticket_id = $3 AND outcome IS NULL`,
		outcome, now, ticketID)
	return err
}

// ListDecisions returns decisions for a policy (most recent first).
func (s *Store) ListDecisions(ctx context.Context, policyID string, limit int) ([]PolicyDecision, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, policy_id, ticket_id, decision, outcome, reason, decided_at, outcome_at
		 FROM policy_decisions WHERE policy_id = $1 ORDER BY decided_at DESC LIMIT $2`,
		policyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PolicyDecision
	for rows.Next() {
		var d PolicyDecision
		var outcome *string
		var outcomeAt *time.Time
		if err := rows.Scan(&d.ID, &d.PolicyID, &d.TicketID, &d.Decision, &outcome, &d.Reason, &d.DecidedAt, &outcomeAt); err != nil {
			return nil, err
		}
		if outcome != nil {
			o := Outcome(*outcome)
			d.Outcome = &o
		}
		d.OutcomeAt = outcomeAt
		list = append(list, d)
	}
	return list, rows.Err()
}

// ListDecisionsByTicket returns all policy decisions for a ticket.
func (s *Store) ListDecisionsByTicket(ctx context.Context, ticketID string) ([]PolicyDecision, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, policy_id, ticket_id, decision, outcome, reason, decided_at, outcome_at
		 FROM policy_decisions WHERE ticket_id = $1 ORDER BY decided_at`,
		ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PolicyDecision
	for rows.Next() {
		var d PolicyDecision
		var outcome *string
		var outcomeAt *time.Time
		if err := rows.Scan(&d.ID, &d.PolicyID, &d.TicketID, &d.Decision, &outcome, &d.Reason, &d.DecidedAt, &outcomeAt); err != nil {
			return nil, err
		}
		if outcome != nil {
			o := Outcome(*outcome)
			d.Outcome = &o
		}
		d.OutcomeAt = outcomeAt
		list = append(list, d)
	}
	return list, rows.Err()
}

// ComputeMetrics computes rolling metrics for a policy.
func (s *Store) ComputeMetrics(ctx context.Context, policyID string, minSample int) (*Metrics, error) {
	m := &Metrics{PolicyID: policyID}
	var autoApproved, rollbacks, incidents, successes int
	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE decision = 'auto_approved') AS auto_approved,
			COUNT(*) FILTER (WHERE outcome = 'rollback') AS rollbacks,
			COUNT(*) FILTER (WHERE outcome = 'incident') AS incidents,
			COUNT(*) FILTER (WHERE outcome = 'success') AS successes,
			COUNT(*) FILTER (WHERE outcome IS NULL) AS pending
		FROM policy_decisions
		WHERE policy_id = $1`, policyID).
		Scan(&m.TotalDecisions, &autoApproved, &rollbacks, &incidents, &successes, &m.PendingOutcomes)
	if err != nil {
		if err == pgx.ErrNoRows {
			return m, nil
		}
		return nil, err
	}
	// Convert counts to rates
	total := float64(m.TotalDecisions)
	if total > 0 {
		m.AutoApprovalRate = float64(autoApproved) / total
		m.RollbackRate = float64(rollbacks) / total
		m.IncidentRate = float64(incidents) / total
		m.SuccessRate = float64(successes) / total
	}
	m.SampleSizeSufficient = m.TotalDecisions >= minSample
	return m, nil
}

// ListRecentDecisionsByProject returns decisions for all policies in a project within the last N days.
func (s *Store) ListRecentDecisionsByProject(ctx context.Context, projectID string, days int) ([]PolicyDecision, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT pd.id, pd.policy_id, pd.ticket_id, pd.decision, pd.outcome, pd.reason, pd.decided_at, pd.outcome_at
		FROM policy_decisions pd
		JOIN policies p ON p.id = pd.policy_id
		WHERE p.project_id = $1 AND pd.decided_at >= now() - ($2 || ' days')::interval
		ORDER BY pd.decided_at DESC`, projectID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PolicyDecision
	for rows.Next() {
		var d PolicyDecision
		var outcome *string
		var outcomeAt *time.Time
		if err := rows.Scan(&d.ID, &d.PolicyID, &d.TicketID, &d.Decision, &outcome, &d.Reason, &d.DecidedAt, &outcomeAt); err != nil {
			return nil, err
		}
		if outcome != nil {
			o := Outcome(*outcome)
			d.Outcome = &o
		}
		d.OutcomeAt = outcomeAt
		list = append(list, d)
	}
	return list, rows.Err()
}

// --- Change Events ---

// RecordChangeEvent logs a policy edit.
func (s *Store) RecordChangeEvent(ctx context.Context, e *PolicyChangeEvent) error {
	e.ID = uuid.Must(uuid.NewV7()).String()
	var prevJSON, newJSON []byte
	var err error
	if e.PrevRules != nil {
		prevJSON, err = json.Marshal(e.PrevRules)
		if err != nil {
			return err
		}
	}
	if e.NewRules != nil {
		newJSON, err = json.Marshal(e.NewRules)
		if err != nil {
			return err
		}
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO policy_change_events (id, policy_id, actor_id, change_type, prev_rules, new_rules, notes, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.ID, e.PolicyID, e.ActorID, e.ChangeType, prevJSON, newJSON, e.Notes, e.CreatedAt)
	return err
}

// ListChangeEvents returns change events for a policy (newest first).
func (s *Store) ListChangeEvents(ctx context.Context, policyID string, limit int) ([]PolicyChangeEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, policy_id, actor_id, change_type, prev_rules, new_rules, notes, created_at
		 FROM policy_change_events WHERE policy_id = $1 ORDER BY created_at DESC LIMIT $2`,
		policyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PolicyChangeEvent
	for rows.Next() {
		var e PolicyChangeEvent
		var prevJSON, newJSON []byte
		if err := rows.Scan(&e.ID, &e.PolicyID, &e.ActorID, &e.ChangeType, &prevJSON, &newJSON, &e.Notes, &e.CreatedAt); err != nil {
			return nil, err
		}
		if prevJSON != nil {
			var r Rules
			if err := json.Unmarshal(prevJSON, &r); err == nil {
				e.PrevRules = &r
			}
		}
		if newJSON != nil {
			var r Rules
			if err := json.Unmarshal(newJSON, &r); err == nil {
				e.NewRules = &r
			}
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// --- Proposals ---

// CreateProposal inserts a system proposal.
func (s *Store) CreateProposal(ctx context.Context, p *PolicyProposal) error {
	p.ID = uuid.Must(uuid.NewV7()).String()
	suggJSON, err := json.Marshal(p.Suggestion)
	if err != nil {
		return err
	}
	statsJSON, err := json.Marshal(p.Statistics)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO policy_proposals (id, policy_id, proposal_type, suggestion, statistics, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		p.ID, p.PolicyID, p.ProposalType, suggJSON, statsJSON, p.Status, p.CreatedAt)
	return err
}

// ListProposals returns proposals for a policy.
func (s *Store) ListProposals(ctx context.Context, policyID string, pendingOnly bool) ([]PolicyProposal, error) {
	query := `SELECT id, policy_id, proposal_type, suggestion, statistics, status, created_at, resolved_at, resolved_by
		 FROM policy_proposals WHERE policy_id = $1`
	if pendingOnly {
		query += ` AND status = 'pending'`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, query, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PolicyProposal
	for rows.Next() {
		var p PolicyProposal
		var suggJSON, statsJSON []byte
		var resolvedAt *time.Time
		var resolvedBy *string
		if err := rows.Scan(&p.ID, &p.PolicyID, &p.ProposalType, &suggJSON, &statsJSON, &p.Status, &p.CreatedAt, &resolvedAt, &resolvedBy); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(suggJSON, &p.Suggestion)
		_ = json.Unmarshal(statsJSON, &p.Statistics)
		if resolvedAt != nil {
			p.ResolvedAt = resolvedAt
		}
		if resolvedBy != nil {
			p.ResolvedBy = *resolvedBy
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// ListProposalsByProject returns all pending proposals for a project.
func (s *Store) ListProposalsByProject(ctx context.Context, projectID string) ([]PolicyProposal, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT pp.id, pp.policy_id, pp.proposal_type, pp.suggestion, pp.statistics, pp.status, pp.created_at, pp.resolved_at, pp.resolved_by
		FROM policy_proposals pp
		JOIN policies p ON p.id = pp.policy_id
		WHERE p.project_id = $1 AND pp.status = 'pending'
		ORDER BY pp.created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PolicyProposal
	for rows.Next() {
		var p PolicyProposal
		var suggJSON, statsJSON []byte
		var resolvedAt *time.Time
		var resolvedBy *string
		if err := rows.Scan(&p.ID, &p.PolicyID, &p.ProposalType, &suggJSON, &statsJSON, &p.Status, &p.CreatedAt, &resolvedAt, &resolvedBy); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(suggJSON, &p.Suggestion)
		_ = json.Unmarshal(statsJSON, &p.Statistics)
		if resolvedAt != nil {
			p.ResolvedAt = resolvedAt
		}
		if resolvedBy != nil {
			p.ResolvedBy = *resolvedBy
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// ResolveProposal updates a proposal's status.
func (s *Store) ResolveProposal(ctx context.Context, id string, status ProposalStatus, resolvedBy string) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`UPDATE policy_proposals SET status = $1, resolved_at = $2, resolved_by = $3 WHERE id = $4`,
		status, now, resolvedBy, id)
	return err
}
