package webhook

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements Store using PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a webhook store backed by Postgres.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) CreateEvent(ctx context.Context, event *Event) error {
	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		payloadJSON = []byte("{}")
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO webhook_events (
			id, project_id, event_type, payload, status,
			attempt_count, max_attempts, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		event.ID, event.ProjectID, event.EventType, payloadJSON,
		string(event.Status), event.AttemptCount, event.MaxAttempts, event.CreatedAt,
	)
	return err
}

func (s *PostgresStore) GetEvent(ctx context.Context, id string) (*Event, error) {
	event := &Event{}
	var payloadJSON []byte

	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, event_type, payload, status,
			attempt_count, max_attempts, created_at, delivered_at, failed_at
		FROM webhook_events WHERE id = $1`, id,
	).Scan(
		&event.ID, &event.ProjectID, &event.EventType, &payloadJSON,
		&event.Status, &event.AttemptCount, &event.MaxAttempts,
		&event.CreatedAt, &event.DeliveredAt, &event.FailedAt,
	)
	if err != nil {
		return nil, err
	}

	event.Payload = make(map[string]any)
	_ = json.Unmarshal(payloadJSON, &event.Payload)
	return event, nil
}

func (s *PostgresStore) GetWebhookConfig(ctx context.Context, projectID string) (*WebhookConfig, error) {
	cfg := &WebhookConfig{}
	err := s.pool.QueryRow(ctx, `
		SELECT webhook_url, webhook_secret
		FROM projects WHERE id = $1`, projectID,
	).Scan(&cfg.WebhookURL, &cfg.WebhookSecret)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *PostgresStore) UpdateEventStatus(ctx context.Context, id string, status EventStatus, attemptCount int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE webhook_events SET status = $2, attempt_count = $3
		WHERE id = $1`,
		id, string(status), attemptCount,
	)
	return err
}

func (s *PostgresStore) MarkEventDelivered(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		UPDATE webhook_events SET status = 'delivered', delivered_at = $2
		WHERE id = $1`,
		id, now,
	)
	return err
}

func (s *PostgresStore) MarkEventFailed(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		UPDATE webhook_events SET status = 'failed', failed_at = $2
		WHERE id = $1`,
		id, now,
	)
	return err
}

func (s *PostgresStore) RecordDelivery(ctx context.Context, delivery *Delivery) error {
	var statusCode *int
	if delivery.StatusCode != nil {
		statusCode = delivery.StatusCode
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhook_deliveries (
			id, webhook_event_id, project_id, attempt_number,
			status_code, response_body, duration_ms, error, success, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		delivery.ID, delivery.WebhookEventID, delivery.ProjectID,
		delivery.AttemptNumber, statusCode, delivery.ResponseBody,
		delivery.DurationMs, delivery.Error, delivery.Success, delivery.CreatedAt,
	)
	return err
}

func (s *PostgresStore) ListDeliveries(ctx context.Context, webhookEventID string) ([]*Delivery, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, webhook_event_id, project_id, attempt_number,
			status_code, response_body, duration_ms, error, success, created_at
		FROM webhook_deliveries
		WHERE webhook_event_id = $1
		ORDER BY attempt_number ASC`,
		webhookEventID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Delivery
	for rows.Next() {
		d := &Delivery{}
		if err := rows.Scan(
			&d.ID, &d.WebhookEventID, &d.ProjectID, &d.AttemptNumber,
			&d.StatusCode, &d.ResponseBody, &d.DurationMs, &d.Error,
			&d.Success, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

func (s *PostgresStore) ListEvents(ctx context.Context, projectID string, limit, offset int) ([]*Event, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, event_type, payload, status,
			attempt_count, max_attempts, created_at, delivered_at, failed_at
		FROM webhook_events
		WHERE project_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		projectID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Event
	for rows.Next() {
		event := &Event{}
		var payloadJSON []byte
		if err := rows.Scan(
			&event.ID, &event.ProjectID, &event.EventType, &payloadJSON,
			&event.Status, &event.AttemptCount, &event.MaxAttempts,
			&event.CreatedAt, &event.DeliveredAt, &event.FailedAt,
		); err != nil {
			return nil, err
		}
		event.Payload = make(map[string]any)
		_ = json.Unmarshal(payloadJSON, &event.Payload)
		results = append(results, event)
	}
	return results, rows.Err()
}

func (s *PostgresStore) ListPendingEvents(ctx context.Context, limit int) ([]*Event, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, event_type, payload, status,
			attempt_count, max_attempts, created_at, delivered_at, failed_at
		FROM webhook_events
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Event
	for rows.Next() {
		event := &Event{}
		var payloadJSON []byte
		if err := rows.Scan(
			&event.ID, &event.ProjectID, &event.EventType, &payloadJSON,
			&event.Status, &event.AttemptCount, &event.MaxAttempts,
			&event.CreatedAt, &event.DeliveredAt, &event.FailedAt,
		); err != nil {
			return nil, err
		}
		event.Payload = make(map[string]any)
		_ = json.Unmarshal(payloadJSON, &event.Payload)
		results = append(results, event)
	}
	return results, rows.Err()
}
