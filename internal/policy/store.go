package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// =============================================================================
// Posture Store: policy sets, rules, credential scopes
// =============================================================================

// PostureStore is the interface for policy set persistence.
type PostureStore interface {
	CreatePolicySet(ctx context.Context, ps *PolicySet) error
	GetPolicySet(ctx context.Context, id string) (*PolicySet, error)
	GetActivePolicySet(ctx context.Context, projectID string) (*PolicySet, error)
	ListPolicySets(ctx context.Context, projectID string) ([]*PolicySet, error)
	SetActive(ctx context.Context, id string, active bool) error
	UpdateRules(ctx context.Context, id string, rules []Rule) error
	DeletePolicySet(ctx context.Context, id string) error
	RecordPolicyChange(ctx context.Context, change *PolicyChange) error
	ListPolicyChanges(ctx context.Context, projectID string, limit int) ([]*PolicyChange, error)
}

// PostgresStore implements PostureStore using PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore returns a new PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// CreatePolicySet inserts a new policy set.
func (s *PostgresStore) CreatePolicySet(ctx context.Context, ps *PolicySet) error {
	rulesJSON, err := json.Marshal(ps.Rules)
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}
	credsJSON, err := json.Marshal(ps.CredentialScopes)
	if err != nil {
		return fmt.Errorf("marshal credential_scopes: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO policy_sets (id, project_id, name, description, posture, rules, credential_scopes, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		ps.ID, ps.ProjectID, ps.Name, ps.Description,
		nullIfEmpty(ps.Posture), rulesJSON, credsJSON,
		ps.IsActive, ps.CreatedAt, ps.UpdatedAt)
	return err
}

// GetPolicySet returns a policy set by ID.
func (s *PostgresStore) GetPolicySet(ctx context.Context, id string) (*PolicySet, error) {
	ps, err := s.scanOne(ctx,
		`SELECT id, project_id, name, description, posture, rules, credential_scopes, is_active, created_at, updated_at
		 FROM policy_sets WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return ps, nil
}

// GetActivePolicySet returns the active policy set for a project, or nil if none.
func (s *PostgresStore) GetActivePolicySet(ctx context.Context, projectID string) (*PolicySet, error) {
	ps, err := s.scanOne(ctx,
		`SELECT id, project_id, name, description, posture, rules, credential_scopes, is_active, created_at, updated_at
		 FROM policy_sets WHERE project_id = $1 AND is_active = true`, projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return ps, err
}

// ListPolicySets returns all policy sets for a project.
func (s *PostgresStore) ListPolicySets(ctx context.Context, projectID string) ([]*PolicySet, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, name, description, posture, rules, credential_scopes, is_active, created_at, updated_at
		 FROM policy_sets WHERE project_id = $1 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanRows(rows)
}

// SetActive sets the is_active flag on a policy set.
func (s *PostgresStore) SetActive(ctx context.Context, id string, active bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE policy_sets SET is_active = $1, updated_at = now() WHERE id = $2`,
		active, id)
	return err
}

// UpdateRules replaces the rules on a policy set.
func (s *PostgresStore) UpdateRules(ctx context.Context, id string, rules []Rule) error {
	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE policy_sets SET rules = $1, updated_at = now() WHERE id = $2`,
		rulesJSON, id)
	return err
}

// DeletePolicySet removes a policy set.
func (s *PostgresStore) DeletePolicySet(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM policy_sets WHERE id = $1`, id)
	return err
}

// RecordPolicyChange inserts a policy change audit record.
func (s *PostgresStore) RecordPolicyChange(ctx context.Context, change *PolicyChange) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO policy_changes (id, project_id, policy_set_id, change_type, changed_by, old_rules, new_rules, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		change.ID, change.ProjectID, change.PolicySetID,
		string(change.ChangeType), change.ChangedBy,
		nullRawJSON(change.OldRules), nullRawJSON(change.NewRules),
		change.CreatedAt)
	return err
}

// ListPolicyChanges returns recent policy changes for a project.
func (s *PostgresStore) ListPolicyChanges(ctx context.Context, projectID string, limit int) ([]*PolicyChange, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, policy_set_id, change_type, changed_by, old_rules, new_rules, created_at
		 FROM policy_changes WHERE project_id = $1 ORDER BY created_at DESC LIMIT $2`,
		projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*PolicyChange
	for rows.Next() {
		var c PolicyChange
		var oldRules, newRules []byte
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.PolicySetID, &c.ChangeType, &c.ChangedBy, &oldRules, &newRules, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.OldRules = oldRules
		c.NewRules = newRules
		list = append(list, &c)
	}
	return list, rows.Err()
}

// --- Internal helpers ---

func (s *PostgresStore) scanOne(ctx context.Context, query string, args ...any) (*PolicySet, error) {
	var ps PolicySet
	var rulesJSON, credsJSON []byte
	var posture *string
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&ps.ID, &ps.ProjectID, &ps.Name, &ps.Description,
		&posture, &rulesJSON, &credsJSON,
		&ps.IsActive, &ps.CreatedAt, &ps.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if posture != nil {
		ps.Posture = *posture
	}
	_ = json.Unmarshal(rulesJSON, &ps.Rules)
	_ = json.Unmarshal(credsJSON, &ps.CredentialScopes)
	return &ps, nil
}

func (s *PostgresStore) scanRows(rows pgx.Rows) ([]*PolicySet, error) {
	var list []*PolicySet
	for rows.Next() {
		var ps PolicySet
		var rulesJSON, credsJSON []byte
		var posture *string
		if err := rows.Scan(
			&ps.ID, &ps.ProjectID, &ps.Name, &ps.Description,
			&posture, &rulesJSON, &credsJSON,
			&ps.IsActive, &ps.CreatedAt, &ps.UpdatedAt); err != nil {
			return nil, err
		}
		if posture != nil {
			ps.Posture = *posture
		}
		_ = json.Unmarshal(rulesJSON, &ps.Rules)
		_ = json.Unmarshal(credsJSON, &ps.CredentialScopes)
		list = append(list, &ps)
	}
	return list, rows.Err()
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullRawJSON(data json.RawMessage) []byte {
	if data == nil {
		return nil
	}
	return data
}

// --- In-Memory Store (for testing and initial bootstrap) ---

// MemoryStore implements PostureStore in-memory for testing.
type MemoryStore struct {
	policySets map[string]*PolicySet
	changes    []*PolicyChange
}

// NewMemoryStore returns a new in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		policySets: make(map[string]*PolicySet),
	}
}

func (m *MemoryStore) CreatePolicySet(_ context.Context, ps *PolicySet) error {
	cp := *ps
	cp.Rules = make([]Rule, len(ps.Rules))
	copy(cp.Rules, ps.Rules)
	m.policySets[ps.ID] = &cp
	return nil
}

func (m *MemoryStore) GetPolicySet(_ context.Context, id string) (*PolicySet, error) {
	ps, ok := m.policySets[id]
	if !ok {
		return nil, fmt.Errorf("policy set not found: %s", id)
	}
	return ps, nil
}

func (m *MemoryStore) GetActivePolicySet(_ context.Context, projectID string) (*PolicySet, error) {
	for _, ps := range m.policySets {
		if ps.ProjectID == projectID && ps.IsActive {
			return ps, nil
		}
	}
	return nil, nil
}

func (m *MemoryStore) ListPolicySets(_ context.Context, projectID string) ([]*PolicySet, error) {
	var list []*PolicySet
	for _, ps := range m.policySets {
		if ps.ProjectID == projectID {
			list = append(list, ps)
		}
	}
	return list, nil
}

func (m *MemoryStore) SetActive(_ context.Context, id string, active bool) error {
	ps, ok := m.policySets[id]
	if !ok {
		return fmt.Errorf("policy set not found: %s", id)
	}
	ps.IsActive = active
	ps.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryStore) UpdateRules(_ context.Context, id string, rules []Rule) error {
	ps, ok := m.policySets[id]
	if !ok {
		return fmt.Errorf("policy set not found: %s", id)
	}
	ps.Rules = make([]Rule, len(rules))
	copy(ps.Rules, rules)
	ps.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryStore) DeletePolicySet(_ context.Context, id string) error {
	delete(m.policySets, id)
	return nil
}

func (m *MemoryStore) RecordPolicyChange(_ context.Context, change *PolicyChange) error {
	m.changes = append(m.changes, change)
	return nil
}

func (m *MemoryStore) ListPolicyChanges(_ context.Context, projectID string, limit int) ([]*PolicyChange, error) {
	var list []*PolicyChange
	for _, c := range m.changes {
		if c.ProjectID == projectID {
			list = append(list, c)
		}
	}
	if limit > 0 && len(list) > limit {
		list = list[len(list)-limit:]
	}
	return list, nil
}

// =============================================================================
// Calibration Store: policies, decisions, change events, proposals
// =============================================================================

// CalibrationStore persists policies, decisions, change events, and proposals.
type CalibrationStore struct {
	pool *pgxpool.Pool
}

// NewCalibrationStore returns a new CalibrationStore.
func NewCalibrationStore(pool *pgxpool.Pool) *CalibrationStore {
	return &CalibrationStore{pool: pool}
}

// --- Policies ---

// CreatePolicy inserts a new policy.
func (s *CalibrationStore) CreatePolicy(ctx context.Context, p *CalibrationPolicy) error {
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
func (s *CalibrationStore) GetPolicy(ctx context.Context, id string) (*CalibrationPolicy, error) {
	var p CalibrationPolicy
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
func (s *CalibrationStore) ListPolicies(ctx context.Context, projectID string, enabledOnly bool) ([]CalibrationPolicy, error) {
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
	var list []CalibrationPolicy
	for rows.Next() {
		var p CalibrationPolicy
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
func (s *CalibrationStore) UpdatePolicy(ctx context.Context, id string, name, description string, rules CalibrationRules, enabled bool, minSample int) error {
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
func (s *CalibrationStore) RecordDecision(ctx context.Context, d *CalibrationDecision) error {
	d.ID = uuid.Must(uuid.NewV7()).String()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO policy_decisions (id, policy_id, ticket_id, decision, reason, decided_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		d.ID, d.PolicyID, d.TicketID, d.Decision, d.Reason, d.DecidedAt)
	return err
}

// RecordOutcome updates the outcome of a previously recorded decision.
func (s *CalibrationStore) RecordOutcome(ctx context.Context, decisionID string, outcome Outcome) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`UPDATE policy_decisions SET outcome = $1, outcome_at = $2 WHERE id = $3`,
		outcome, now, decisionID)
	return err
}

// RecordOutcomeByTicket updates outcomes for all decisions on a ticket.
func (s *CalibrationStore) RecordOutcomeByTicket(ctx context.Context, ticketID string, outcome Outcome) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`UPDATE policy_decisions SET outcome = $1, outcome_at = $2 WHERE ticket_id = $3 AND outcome IS NULL`,
		outcome, now, ticketID)
	return err
}

// ListDecisions returns decisions for a policy (most recent first).
func (s *CalibrationStore) ListDecisions(ctx context.Context, policyID string, limit int) ([]CalibrationDecision, error) {
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
	var list []CalibrationDecision
	for rows.Next() {
		var d CalibrationDecision
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
func (s *CalibrationStore) ListDecisionsByTicket(ctx context.Context, ticketID string) ([]CalibrationDecision, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, policy_id, ticket_id, decision, outcome, reason, decided_at, outcome_at
		 FROM policy_decisions WHERE ticket_id = $1 ORDER BY decided_at`,
		ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []CalibrationDecision
	for rows.Next() {
		var d CalibrationDecision
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
func (s *CalibrationStore) ComputeMetrics(ctx context.Context, policyID string, minSample int) (*Metrics, error) {
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
func (s *CalibrationStore) ListRecentDecisionsByProject(ctx context.Context, projectID string, days int) ([]CalibrationDecision, error) {
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
	var list []CalibrationDecision
	for rows.Next() {
		var d CalibrationDecision
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
func (s *CalibrationStore) RecordChangeEvent(ctx context.Context, e *PolicyChangeEvent) error {
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
func (s *CalibrationStore) ListChangeEvents(ctx context.Context, policyID string, limit int) ([]PolicyChangeEvent, error) {
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
			var r CalibrationRules
			if err := json.Unmarshal(prevJSON, &r); err == nil {
				e.PrevRules = &r
			}
		}
		if newJSON != nil {
			var r CalibrationRules
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
func (s *CalibrationStore) CreateProposal(ctx context.Context, p *PolicyProposal) error {
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
func (s *CalibrationStore) ListProposals(ctx context.Context, policyID string, pendingOnly bool) ([]PolicyProposal, error) {
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
func (s *CalibrationStore) ListProposalsByProject(ctx context.Context, projectID string) ([]PolicyProposal, error) {
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
func (s *CalibrationStore) ResolveProposal(ctx context.Context, id string, status ProposalStatus, resolvedBy string) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`UPDATE policy_proposals SET status = $1, resolved_at = $2, resolved_by = $3 WHERE id = $4`,
		status, now, resolvedBy, id)
	return err
}
