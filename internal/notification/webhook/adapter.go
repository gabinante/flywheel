// Package webhook implements the outbound signed webhook channel adapter
// for the notification layer. Every outbound delivery is signed with
// HMAC-SHA256 using the project's webhook_secret so recipients can
// verify authenticity and reject replay attacks.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gabinante/flywheel/events/hooks"
	"github.com/gabinante/flywheel/internal/notification"
)

// SecretProvider looks up the webhook signing secret for a project.
// Typically backed by the project store.
type SecretProvider interface {
	GetWebhookSecret(ctx context.Context, projectID string) (string, error)
}

// Adapter delivers notifications via outbound signed webhooks.
// Each POST includes:
//   - X-GHP-Signature: sha256={hex_hmac}
//   - X-GHP-Timestamp: {unix_epoch_seconds}
//   - X-GHP-Event-Type: {notification_category}
type Adapter struct {
	client   *http.Client
	provider SecretProvider
}

// NewAdapter creates a webhook notification adapter with HMAC signing.
func NewAdapter(provider SecretProvider) *Adapter {
	return &Adapter{
		client:   &http.Client{Timeout: 10 * time.Second},
		provider: provider,
	}
}

// Name returns "webhook".
func (a *Adapter) Name() string { return "webhook" }

// WebhookPayload is the JSON body sent to the webhook endpoint.
type WebhookPayload struct {
	ID           string         `json:"id"`
	ProjectID    string         `json:"project_id"`
	TicketID     string         `json:"ticket_id,omitempty"`
	Category     string         `json:"category"`
	Urgency      string         `json:"urgency"`
	Title        string         `json:"title"`
	Body         string         `json:"body,omitempty"`
	Context      map[string]any `json:"context,omitempty"`
	DecisionMeta map[string]any `json:"decision_meta,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// Send delivers a single notification via a signed webhook POST.
func (a *Adapter) Send(ctx context.Context, n *notification.Notification, prefs *notification.Preferences) error {
	webhookURL := prefs.WebhookURL
	if webhookURL == "" {
		return fmt.Errorf("webhook URL not configured for project %s", n.ProjectID)
	}

	payload := WebhookPayload{
		ID:           n.ID,
		ProjectID:    n.ProjectID,
		TicketID:     n.TicketID,
		Category:     string(n.Category),
		Urgency:      string(n.Urgency),
		Title:        n.Title,
		Body:         n.Body,
		Context:      n.Context,
		DecisionMeta: n.DecisionMeta,
		CreatedAt:    n.CreatedAt,
	}

	return a.signAndPost(ctx, webhookURL, n.ProjectID, string(n.Category), payload)
}

// SendDigest delivers a batch of notifications as a single signed webhook POST.
func (a *Adapter) SendDigest(ctx context.Context, notifications []*notification.Notification, prefs *notification.Preferences) error {
	webhookURL := prefs.WebhookURL
	if webhookURL == "" {
		return fmt.Errorf("webhook URL not configured")
	}
	if len(notifications) == 0 {
		return nil
	}

	var payloads []WebhookPayload
	for _, n := range notifications {
		payloads = append(payloads, WebhookPayload{
			ID:        n.ID,
			ProjectID: n.ProjectID,
			TicketID:  n.TicketID,
			Category:  string(n.Category),
			Urgency:   string(n.Urgency),
			Title:     n.Title,
			Body:      n.Body,
			CreatedAt: n.CreatedAt,
		})
	}

	projectID := notifications[0].ProjectID
	return a.signAndPost(ctx, webhookURL, projectID, "notification.digest", payloads)
}

// signAndPost marshals the payload, signs it, and sends the POST with signature headers.
func (a *Adapter) signAndPost(ctx context.Context, webhookURL, projectID, eventType string, payload any) error {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: marshal payload: %w", err)
	}

	secret, err := a.provider.GetWebhookSecret(ctx, projectID)
	if err != nil {
		return fmt.Errorf("webhook: get secret for project %s: %w", projectID, err)
	}
	if secret == "" {
		return fmt.Errorf("webhook: no webhook secret configured for project %s", projectID)
	}

	timestamp := time.Now().Unix()
	signature, err := hooks.SignWebhook(secret, timestamp, string(bodyBytes))
	if err != nil {
		return fmt.Errorf("webhook: sign: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("webhook: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GHP-Signature", fmt.Sprintf("sha256=%s", signature))
	req.Header.Set("X-GHP-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("X-GHP-Event-Type", eventType)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: endpoint returned %d", resp.StatusCode)
	}

	return nil
}
