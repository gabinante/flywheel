package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the interface for policy persistence.
type Store interface {
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

// PostgresStore implements Store using PostgreSQL.
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

// MemoryStore implements Store in-memory for testing.
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
