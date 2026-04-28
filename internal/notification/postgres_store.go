package notification

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

// NewPostgresStore creates a notification store backed by Postgres.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) CreateNotification(ctx context.Context, n *Notification) error {
	decisionJSON, err := json.Marshal(n.DecisionMeta)
	if err != nil {
		decisionJSON = []byte("{}")
	}
	contextJSON, err := json.Marshal(n.Context)
	if err != nil {
		contextJSON = []byte("{}")
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO notifications (
			id, project_id, ticket_id, category, urgency, title, body,
			channel, routing, decision_json, context_json, status,
			classifier, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		n.ID, n.ProjectID, n.TicketID, string(n.Category), string(n.Urgency),
		n.Title, n.Body, string(n.Channel), string(n.Routing),
		decisionJSON, contextJSON, string(n.Status),
		n.Classifier, n.CreatedAt,
	)
	return err
}

func (s *PostgresStore) GetNotification(ctx context.Context, id string) (*Notification, error) {
	n := &Notification{}
	var decisionJSON, contextJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, ticket_id, category, urgency, title, body,
			channel, routing, decision_json, context_json, status,
			sent_at, error_message, dismissed, dismissed_at,
			classifier, created_at
		FROM notifications WHERE id = $1`, id,
	).Scan(
		&n.ID, &n.ProjectID, &n.TicketID, &n.Category, &n.Urgency,
		&n.Title, &n.Body, &n.Channel, &n.Routing,
		&decisionJSON, &contextJSON, &n.Status,
		&n.SentAt, &n.ErrorMessage, &n.Dismissed, &n.DismissedAt,
		&n.Classifier, &n.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(decisionJSON, &n.DecisionMeta)
	_ = json.Unmarshal(contextJSON, &n.Context)
	return n, nil
}

func (s *PostgresStore) UpdateNotificationStatus(ctx context.Context, id string, status Status, errorMsg string) error {
	var sentAt *time.Time
	if status == StatusSent {
		now := time.Now().UTC()
		sentAt = &now
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications SET status = $2, sent_at = $3, error_message = $4
		WHERE id = $1`,
		id, string(status), sentAt, errorMsg,
	)
	return err
}

func (s *PostgresStore) ListNotifications(ctx context.Context, projectID string, limit, offset int) ([]*Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, ticket_id, category, urgency, title, body,
			channel, routing, decision_json, context_json, status,
			sent_at, error_message, dismissed, dismissed_at,
			classifier, created_at
		FROM notifications WHERE project_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		projectID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Notification
	for rows.Next() {
		n := &Notification{}
		var decisionJSON, contextJSON []byte
		if err := rows.Scan(
			&n.ID, &n.ProjectID, &n.TicketID, &n.Category, &n.Urgency,
			&n.Title, &n.Body, &n.Channel, &n.Routing,
			&decisionJSON, &contextJSON, &n.Status,
			&n.SentAt, &n.ErrorMessage, &n.Dismissed, &n.DismissedAt,
			&n.Classifier, &n.CreatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(decisionJSON, &n.DecisionMeta)
		_ = json.Unmarshal(contextJSON, &n.Context)
		results = append(results, n)
	}
	return results, rows.Err()
}

func (s *PostgresStore) ListPendingDigest(ctx context.Context, projectID string) ([]*Notification, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, ticket_id, category, urgency, title, body,
			channel, routing, decision_json, context_json, status,
			sent_at, error_message, dismissed, dismissed_at,
			classifier, created_at
		FROM notifications
		WHERE project_id = $1 AND routing = 'digest' AND status = 'pending'
		ORDER BY created_at ASC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Notification
	for rows.Next() {
		n := &Notification{}
		var decisionJSON, contextJSON []byte
		if err := rows.Scan(
			&n.ID, &n.ProjectID, &n.TicketID, &n.Category, &n.Urgency,
			&n.Title, &n.Body, &n.Channel, &n.Routing,
			&decisionJSON, &contextJSON, &n.Status,
			&n.SentAt, &n.ErrorMessage, &n.Dismissed, &n.DismissedAt,
			&n.Classifier, &n.CreatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(decisionJSON, &n.DecisionMeta)
		_ = json.Unmarshal(contextJSON, &n.Context)
		results = append(results, n)
	}
	return results, rows.Err()
}

func (s *PostgresStore) MarkDigested(ctx context.Context, ids []string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications SET status = 'digested', sent_at = now()
		WHERE id = ANY($1)`, ids,
	)
	return err
}

func (s *PostgresStore) DismissNotification(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := s.pool.Exec(ctx, `
		UPDATE notifications SET dismissed = true, dismissed_at = $2
		WHERE id = $1 AND NOT dismissed`,
		id, now,
	)
	if err != nil {
		return err
	}
	// If we actually dismissed it, update the classifier's dismissal rate.
	if result.RowsAffected() > 0 {
		// Get the notification to find its classifier.
		var classifier, projectID string
		err := s.pool.QueryRow(ctx, `
			SELECT classifier, project_id FROM notifications WHERE id = $1`, id,
		).Scan(&classifier, &projectID)
		if err == nil && classifier != "" {
			_ = s.IncrementDismissed(ctx, classifier, projectID)
		}
	}
	return nil
}

func (s *PostgresStore) GetPreferences(ctx context.Context, projectID string) (*Preferences, error) {
	p := &Preferences{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, critical_channel, high_channel, medium_channel, low_channel,
			digest_enabled, digest_interval, push_threshold,
			slack_webhook_url, email_address, sms_number, webhook_url,
			created_at, updated_at
		FROM notification_preferences WHERE project_id = $1`, projectID,
	).Scan(
		&p.ID, &p.ProjectID, &p.CriticalChannel, &p.HighChannel,
		&p.MediumChannel, &p.LowChannel,
		&p.DigestEnabled, &p.DigestInterval, &p.PushThreshold,
		&p.SlackWebhookURL, &p.EmailAddress, &p.SMSNumber, &p.WebhookURL,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *PostgresStore) UpsertPreferences(ctx context.Context, prefs *Preferences) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notification_preferences (
			id, project_id, critical_channel, high_channel, medium_channel, low_channel,
			digest_enabled, digest_interval, push_threshold,
			slack_webhook_url, email_address, sms_number, webhook_url,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (project_id) DO UPDATE SET
			critical_channel = EXCLUDED.critical_channel,
			high_channel = EXCLUDED.high_channel,
			medium_channel = EXCLUDED.medium_channel,
			low_channel = EXCLUDED.low_channel,
			digest_enabled = EXCLUDED.digest_enabled,
			digest_interval = EXCLUDED.digest_interval,
			push_threshold = EXCLUDED.push_threshold,
			slack_webhook_url = EXCLUDED.slack_webhook_url,
			email_address = EXCLUDED.email_address,
			sms_number = EXCLUDED.sms_number,
			webhook_url = EXCLUDED.webhook_url,
			updated_at = EXCLUDED.updated_at`,
		prefs.ID, prefs.ProjectID,
		string(prefs.CriticalChannel), string(prefs.HighChannel),
		string(prefs.MediumChannel), string(prefs.LowChannel),
		prefs.DigestEnabled, prefs.DigestInterval, string(prefs.PushThreshold),
		prefs.SlackWebhookURL, prefs.EmailAddress, prefs.SMSNumber, prefs.WebhookURL,
		prefs.CreatedAt, prefs.UpdatedAt,
	)
	return err
}

func (s *PostgresStore) IncrementSent(ctx context.Context, classifier, projectID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notification_dismissal_rates (classifier, project_id, total_sent, window_sent, last_updated)
		VALUES ($1, $2, 1, 1, now())
		ON CONFLICT (classifier, project_id) DO UPDATE SET
			total_sent = notification_dismissal_rates.total_sent + 1,
			window_sent = notification_dismissal_rates.window_sent + 1,
			last_updated = now()`,
		classifier, projectID,
	)
	return err
}

func (s *PostgresStore) IncrementDismissed(ctx context.Context, classifier, projectID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notification_dismissal_rates (classifier, project_id, total_dismissed, window_dismissed, last_updated)
		VALUES ($1, $2, 1, 1, now())
		ON CONFLICT (classifier, project_id) DO UPDATE SET
			total_dismissed = notification_dismissal_rates.total_dismissed + 1,
			window_dismissed = notification_dismissal_rates.window_dismissed + 1,
			last_updated = now()`,
		classifier, projectID,
	)
	return err
}

func (s *PostgresStore) GetDismissalRate(ctx context.Context, classifier, projectID string) (*DismissalRate, error) {
	d := &DismissalRate{}
	err := s.pool.QueryRow(ctx, `
		SELECT classifier, project_id, total_sent, total_dismissed,
			window_sent, window_dismissed, last_updated
		FROM notification_dismissal_rates
		WHERE classifier = $1 AND project_id = $2`,
		classifier, projectID,
	).Scan(
		&d.Classifier, &d.ProjectID, &d.TotalSent, &d.TotalDismissed,
		&d.WindowSent, &d.WindowDismissed, &d.LastUpdated,
	)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (s *PostgresStore) ListDismissalRates(ctx context.Context, projectID string) ([]*DismissalRate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT classifier, project_id, total_sent, total_dismissed,
			window_sent, window_dismissed, last_updated
		FROM notification_dismissal_rates
		WHERE project_id = $1 ORDER BY last_updated DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*DismissalRate
	for rows.Next() {
		d := &DismissalRate{}
		if err := rows.Scan(
			&d.Classifier, &d.ProjectID, &d.TotalSent, &d.TotalDismissed,
			&d.WindowSent, &d.WindowDismissed, &d.LastUpdated,
		); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, rows.Err()
}
