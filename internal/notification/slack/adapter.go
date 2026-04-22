// Package slack implements the Slack channel adapter for the notification layer.
// It uses Slack incoming webhooks to deliver notifications.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/notification"
)

// Adapter delivers notifications via Slack incoming webhooks.
type Adapter struct {
	client *http.Client
}

// NewAdapter creates a Slack notification adapter.
func NewAdapter() *Adapter {
	return &Adapter{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name returns "slack".
func (a *Adapter) Name() string { return "slack" }

// Send delivers a single notification to Slack via incoming webhook.
func (a *Adapter) Send(ctx context.Context, n *notification.Notification, prefs *notification.Preferences) error {
	webhookURL := prefs.SlackWebhookURL
	if webhookURL == "" {
		return fmt.Errorf("slack webhook URL not configured for project %s", n.ProjectID)
	}

	msg := a.formatMessage(n)
	return a.post(ctx, webhookURL, msg)
}

// SendDigest delivers a batch of notifications as a single digest message.
func (a *Adapter) SendDigest(ctx context.Context, notifications []*notification.Notification, prefs *notification.Preferences) error {
	webhookURL := prefs.SlackWebhookURL
	if webhookURL == "" {
		return fmt.Errorf("slack webhook URL not configured")
	}

	msg := a.formatDigest(notifications)
	return a.post(ctx, webhookURL, msg)
}

// slackMessage represents a Slack incoming webhook payload.
type slackMessage struct {
	Text   string       `json:"text,omitempty"`
	Blocks []slackBlock `json:"blocks,omitempty"`
}

type slackBlock struct {
	Type string     `json:"type"`
	Text *blockText `json:"text,omitempty"`
}

type blockText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (a *Adapter) formatMessage(n *notification.Notification) slackMessage {
	emoji := urgencyEmoji(n.Urgency)
	categoryLabel := categoryLabel(n.Category)

	headerText := fmt.Sprintf("%s *%s* | %s", emoji, n.Title, categoryLabel)
	bodyParts := []string{headerText}

	if n.Body != "" {
		bodyParts = append(bodyParts, n.Body)
	}

	if n.TicketID != "" {
		bodyParts = append(bodyParts, fmt.Sprintf("_Ticket:_ `%s`", n.TicketID))
	}

	if n.DecisionMeta != nil {
		if reason, ok := n.DecisionMeta["reason"].(string); ok && reason != "" {
			bodyParts = append(bodyParts, fmt.Sprintf("_Reason:_ %s", reason))
		}
	}

	blocks := []slackBlock{
		{
			Type: "section",
			Text: &blockText{
				Type: "mrkdwn",
				Text: strings.Join(bodyParts, "\n"),
			},
		},
		{
			Type: "context",
			Text: &blockText{
				Type: "mrkdwn",
				Text: fmt.Sprintf("Urgency: *%s* | Project: `%s` | %s",
					string(n.Urgency), n.ProjectID, n.CreatedAt.Format(time.RFC3339)),
			},
		},
	}

	return slackMessage{
		Text:   fmt.Sprintf("[%s] %s", string(n.Urgency), n.Title),
		Blocks: blocks,
	}
}

func (a *Adapter) formatDigest(notifications []*notification.Notification) slackMessage {
	var lines []string
	lines = append(lines, fmt.Sprintf("*Notification Digest* (%d items)", len(notifications)))
	lines = append(lines, "")

	for i, n := range notifications {
		if i >= 20 {
			lines = append(lines, fmt.Sprintf("_...and %d more_", len(notifications)-20))
			break
		}
		emoji := urgencyEmoji(n.Urgency)
		line := fmt.Sprintf("%s `%s` %s", emoji, string(n.Urgency), n.Title)
		if n.TicketID != "" {
			line += fmt.Sprintf(" (`%s`)", n.TicketID)
		}
		lines = append(lines, line)
	}

	blocks := []slackBlock{
		{
			Type: "section",
			Text: &blockText{
				Type: "mrkdwn",
				Text: strings.Join(lines, "\n"),
			},
		},
	}

	return slackMessage{
		Text:   fmt.Sprintf("Notification digest: %d items", len(notifications)),
		Blocks: blocks,
	}
}

func (a *Adapter) post(ctx context.Context, webhookURL string, msg slackMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal slack message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}

	return nil
}

func urgencyEmoji(u notification.Urgency) string {
	switch u {
	case notification.UrgencyCritical:
		return "\U0001F6A8" // rotating light
	case notification.UrgencyHigh:
		return "\u26A0\uFE0F" // warning
	case notification.UrgencyMedium:
		return "\U0001F4E2" // loudspeaker
	case notification.UrgencyLow:
		return "\U0001F4AC" // speech bubble
	default:
		return "\U0001F514" // bell
	}
}

func categoryLabel(c notification.Category) string {
	switch c {
	case notification.CategoryUrgentDecision:
		return "Urgent Decision"
	case notification.CategoryAutonomousAction:
		return "Autonomous Action"
	case notification.CategoryCalibrationReview:
		return "Calibration Review"
	case notification.CategoryAnomalyAlert:
		return "Anomaly Alert"
	default:
		return "Notification"
	}
}
