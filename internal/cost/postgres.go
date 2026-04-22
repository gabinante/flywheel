package cost

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements Store using PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgreSQL-backed cost store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// RecordCall persists a single LLM call record.
func (s *PostgresStore) RecordCall(ctx context.Context, record *LLMCallRecord) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO llm_call_records (id, project_id, ticket_id, worker_role, provider, model, model_tier, operation_type, input_tokens, output_tokens, cost_millicent, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		record.ID, record.ProjectID, record.TicketID, record.WorkerRole,
		record.Provider, record.Model, string(record.ModelTier), string(record.OperationType),
		record.InputTokens, record.OutputTokens, int64(record.CostMillicent), record.Timestamp,
	)
	return err
}

// GetCallsByTicket returns all LLM call records for a ticket.
func (s *PostgresStore) GetCallsByTicket(ctx context.Context, ticketID string) ([]*LLMCallRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, ticket_id, worker_role, provider, model, model_tier, operation_type, input_tokens, output_tokens, cost_millicent, created_at
		FROM llm_call_records WHERE ticket_id = $1 ORDER BY created_at`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCallRecords(rows)
}

// GetCallsByProject returns all LLM call records for a project in a given month (YYYY-MM format).
func (s *PostgresStore) GetCallsByProject(ctx context.Context, projectID string, month string) ([]*LLMCallRecord, error) {
	var rows pgx.Rows
	var err error
	if month != "" {
		rows, err = s.pool.Query(ctx, `
			SELECT id, project_id, ticket_id, worker_role, provider, model, model_tier, operation_type, input_tokens, output_tokens, cost_millicent, created_at
			FROM llm_call_records
			WHERE project_id = $1 AND to_char(created_at, 'YYYY-MM') = $2
			ORDER BY created_at`, projectID, month)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT id, project_id, ticket_id, worker_role, provider, model, model_tier, operation_type, input_tokens, output_tokens, cost_millicent, created_at
			FROM llm_call_records
			WHERE project_id = $1
			ORDER BY created_at`, projectID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCallRecords(rows)
}

// SetBudget creates or updates a budget for the given scope.
func (s *PostgresStore) SetBudget(ctx context.Context, budget *Budget) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO budgets (id, project_id, ticket_id, month, limit_millicent, warn_at, hard_stop, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT ON CONSTRAINT idx_budgets_scope
		DO UPDATE SET limit_millicent = EXCLUDED.limit_millicent, warn_at = EXCLUDED.warn_at, hard_stop = EXCLUDED.hard_stop, updated_at = EXCLUDED.updated_at`,
		budget.ID, budget.ProjectID, budget.TicketID, budget.Month,
		int64(budget.LimitMilli), budget.WarnAt, budget.HardStop, budget.CreatedAt, budget.UpdatedAt,
	)
	return err
}

// GetBudget returns the budget for a specific scope.
func (s *PostgresStore) GetBudget(ctx context.Context, projectID, ticketID, month string) (*Budget, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, ticket_id, month, limit_millicent, warn_at, hard_stop, created_at, updated_at
		FROM budgets WHERE project_id = $1 AND ticket_id = $2 AND month = $3`,
		projectID, ticketID, month)
	return scanBudget(row)
}

// GetBudgetsByProject returns all budgets for a project.
func (s *PostgresStore) GetBudgetsByProject(ctx context.Context, projectID string) ([]*Budget, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, ticket_id, month, limit_millicent, warn_at, hard_stop, created_at, updated_at
		FROM budgets WHERE project_id = $1 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var budgets []*Budget
	for rows.Next() {
		b, err := scanBudgetRow(rows)
		if err != nil {
			return nil, err
		}
		budgets = append(budgets, b)
	}
	return budgets, rows.Err()
}

// SumByProject returns total spend for a project in a given month.
func (s *PostgresStore) SumByProject(ctx context.Context, projectID string, month string) (Unit, error) {
	var total int64
	var err error
	if month != "" {
		err = s.pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(cost_millicent), 0)
			FROM llm_call_records
			WHERE project_id = $1 AND to_char(created_at, 'YYYY-MM') = $2`,
			projectID, month).Scan(&total)
	} else {
		err = s.pool.QueryRow(ctx, `
			SELECT COALESCE(SUM(cost_millicent), 0)
			FROM llm_call_records WHERE project_id = $1`,
			projectID).Scan(&total)
	}
	return Unit(total), err
}

// SumByTicket returns total spend for a ticket.
func (s *PostgresStore) SumByTicket(ctx context.Context, ticketID string) (Unit, error) {
	var total int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(cost_millicent), 0)
		FROM llm_call_records WHERE ticket_id = $1`, ticketID).Scan(&total)
	return Unit(total), err
}

// SumByProjectGrouped returns a cost summary broken down by ticket, role, model, and provider.
func (s *PostgresStore) SumByProjectGrouped(ctx context.Context, projectID string, month string) (*CostSummary, error) {
	summary := &CostSummary{
		ProjectID:  projectID,
		Month:      month,
		ByTicket:   make(map[string]Unit),
		ByRole:     make(map[string]Unit),
		ByModel:    make(map[string]Unit),
		ByProvider: make(map[string]Unit),
	}

	// Query for grouped sums in one pass
	var query string
	var args []any
	if month != "" {
		query = `
			SELECT ticket_id, worker_role, model, provider, SUM(cost_millicent) as total, COUNT(*) as cnt
			FROM llm_call_records
			WHERE project_id = $1 AND to_char(created_at, 'YYYY-MM') = $2
			GROUP BY ticket_id, worker_role, model, provider`
		args = []any{projectID, month}
	} else {
		query = `
			SELECT ticket_id, worker_role, model, provider, SUM(cost_millicent) as total, COUNT(*) as cnt
			FROM llm_call_records
			WHERE project_id = $1
			GROUP BY ticket_id, worker_role, model, provider`
		args = []any{projectID}
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ticketID, role, model, provider string
		var total int64
		var cnt int
		if err := rows.Scan(&ticketID, &role, &model, &provider, &total, &cnt); err != nil {
			return nil, err
		}
		summary.ByTicket[ticketID] += Unit(total)
		summary.ByRole[role] += Unit(total)
		summary.ByModel[model] += Unit(total)
		summary.ByProvider[provider] += Unit(total)
		summary.TotalMilli += Unit(total)
		summary.CallCount += cnt
	}
	summary.TotalDollars = summary.TotalMilli.ToDollars()
	return summary, rows.Err()
}

// CreateAlert persists a budget alert.
func (s *PostgresStore) CreateAlert(ctx context.Context, alert *BudgetAlert) error {
	statusJSON, err := json.Marshal(alert.Status)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO budget_alerts (id, project_id, ticket_id, alert_type, message, status_json, acknowledged, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		alert.ID, alert.ProjectID, alert.TicketID, string(alert.AlertType),
		alert.Message, statusJSON, alert.Acknowledged, alert.CreatedAt,
	)
	return err
}

// GetActiveAlerts returns unacknowledged alerts for a project.
func (s *PostgresStore) GetActiveAlerts(ctx context.Context, projectID string) ([]*BudgetAlert, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, ticket_id, alert_type, message, status_json, acknowledged, created_at
		FROM budget_alerts
		WHERE project_id = $1 AND NOT acknowledged
		ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []*BudgetAlert
	for rows.Next() {
		a := &BudgetAlert{}
		var statusJSON []byte
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.TicketID, &a.AlertType, &a.Message, &statusJSON, &a.Acknowledged, &a.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(statusJSON, &a.Status)
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// AcknowledgeAlert marks an alert as acknowledged.
func (s *PostgresStore) AcknowledgeAlert(ctx context.Context, alertID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE budget_alerts SET acknowledged = true WHERE id = $1`, alertID)
	return err
}

// RecordRateLimitEvent persists a rate limit event.
func (s *PostgresStore) RecordRateLimitEvent(ctx context.Context, event *RateLimitEvent) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO rate_limit_events (id, project_id, ticket_id, provider, model, retry_after_ms, reset_at, created_at, resumed, resumed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		event.ID, event.ProjectID, event.TicketID, event.Provider, event.Model,
		event.RetryAfter.Milliseconds(), event.ResetAt, event.Timestamp, event.Resumed, nilTime(event.ResumedAt),
	)
	return err
}

// MarkRateLimitResumed marks a rate limit event as resumed.
func (s *PostgresStore) MarkRateLimitResumed(ctx context.Context, eventID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE rate_limit_events SET resumed = true, resumed_at = now() WHERE id = $1`, eventID)
	return err
}

// GetActiveRateLimits returns active (non-resumed) rate limit events for a project.
func (s *PostgresStore) GetActiveRateLimits(ctx context.Context, projectID string) ([]*RateLimitEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, ticket_id, provider, model, retry_after_ms, reset_at, created_at, resumed, resumed_at
		FROM rate_limit_events
		WHERE project_id = $1 AND NOT resumed
		ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*RateLimitEvent
	for rows.Next() {
		e := &RateLimitEvent{}
		var retryMs int64
		var resumedAt *time.Time
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.TicketID, &e.Provider, &e.Model,
			&retryMs, &e.ResetAt, &e.Timestamp, &e.Resumed, &resumedAt); err != nil {
			return nil, err
		}
		e.RetryAfter = time.Duration(retryMs) * time.Millisecond
		if resumedAt != nil {
			e.ResumedAt = *resumedAt
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetModelPricing returns pricing for a specific provider+model.
func (s *PostgresStore) GetModelPricing(ctx context.Context, provider, model string) (*ModelPricing, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT provider, model, tier, input_per_million, output_per_million
		FROM model_pricing WHERE provider = $1 AND model = $2`, provider, model)
	p := &ModelPricing{}
	var inputPM, outputPM int64
	if err := row.Scan(&p.Provider, &p.Model, &p.Tier, &inputPM, &outputPM); err != nil {
		return nil, err
	}
	p.InputPerMillion = Unit(inputPM)
	p.OutputPerMillion = Unit(outputPM)
	return p, nil
}

// GetAllModelPricing returns all model pricing entries.
func (s *PostgresStore) GetAllModelPricing(ctx context.Context) ([]*ModelPricing, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT provider, model, tier, input_per_million, output_per_million
		FROM model_pricing ORDER BY provider, model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var prices []*ModelPricing
	for rows.Next() {
		p := &ModelPricing{}
		var inputPM, outputPM int64
		if err := rows.Scan(&p.Provider, &p.Model, &p.Tier, &inputPM, &outputPM); err != nil {
			return nil, err
		}
		p.InputPerMillion = Unit(inputPM)
		p.OutputPerMillion = Unit(outputPM)
		prices = append(prices, p)
	}
	return prices, rows.Err()
}

// SetFallbackPolicy creates or updates a fallback policy for an operation type.
func (s *PostgresStore) SetFallbackPolicy(ctx context.Context, projectID string, policy *FallbackPolicy) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO fallback_policies (project_id, operation_type, preferred_tier, allow_fallback, max_tier)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (project_id, operation_type)
		DO UPDATE SET preferred_tier = EXCLUDED.preferred_tier, allow_fallback = EXCLUDED.allow_fallback, max_tier = EXCLUDED.max_tier`,
		projectID, string(policy.OperationType), string(policy.PreferredTier), policy.AllowFallback, string(policy.MaxTier),
	)
	return err
}

// GetFallbackPolicies returns all fallback policies for a project.
func (s *PostgresStore) GetFallbackPolicies(ctx context.Context, projectID string) ([]*FallbackPolicy, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT operation_type, preferred_tier, allow_fallback, max_tier
		FROM fallback_policies WHERE project_id = $1`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []*FallbackPolicy
	for rows.Next() {
		p := &FallbackPolicy{}
		var opType, prefTier, maxTier string
		if err := rows.Scan(&opType, &prefTier, &p.AllowFallback, &maxTier); err != nil {
			return nil, err
		}
		p.OperationType = OperationType(opType)
		p.PreferredTier = ModelTier(prefTier)
		p.MaxTier = ModelTier(maxTier)
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

// CountCallsInMonth returns the number of calls in a month for projection purposes.
func (s *PostgresStore) CountCallsInMonth(ctx context.Context, projectID, month string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM llm_call_records
		WHERE project_id = $1 AND to_char(created_at, 'YYYY-MM') = $2`,
		projectID, month).Scan(&count)
	return count, err
}

// helpers

func scanCallRecords(rows pgx.Rows) ([]*LLMCallRecord, error) {
	var records []*LLMCallRecord
	for rows.Next() {
		r := &LLMCallRecord{}
		var costMilli int64
		var tierStr, opStr string
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.TicketID, &r.WorkerRole,
			&r.Provider, &r.Model, &tierStr, &opStr,
			&r.InputTokens, &r.OutputTokens, &costMilli, &r.Timestamp); err != nil {
			return nil, err
		}
		r.CostMillicent = Unit(costMilli)
		r.ModelTier = ModelTier(tierStr)
		r.OperationType = OperationType(opStr)
		records = append(records, r)
	}
	return records, rows.Err()
}

func scanBudget(row pgx.Row) (*Budget, error) {
	b := &Budget{}
	var limitMilli int64
	if err := row.Scan(&b.ID, &b.ProjectID, &b.TicketID, &b.Month,
		&limitMilli, &b.WarnAt, &b.HardStop, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	b.LimitMilli = Unit(limitMilli)
	return b, nil
}

func scanBudgetRow(rows pgx.Rows) (*Budget, error) {
	b := &Budget{}
	var limitMilli int64
	if err := rows.Scan(&b.ID, &b.ProjectID, &b.TicketID, &b.Month,
		&limitMilli, &b.WarnAt, &b.HardStop, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	b.LimitMilli = Unit(limitMilli)
	return b, nil
}

func nilTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
