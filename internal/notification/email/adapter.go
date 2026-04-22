// Package email implements the email channel adapter for the notification layer.
// This is a stub adapter — operators should plug in their own email delivery
// provider (SendGrid, SES, SMTP, etc.) via the ChannelAdapter interface.
package email

import (
	"context"
	"fmt"
	"log"

	"github.com/gabinante/flywheel/internal/notification"
)

// Adapter is a stub email channel adapter.
// It logs notifications that would be sent via email.
// Replace with a real SMTP/API implementation for production use.
type Adapter struct{}

// NewAdapter creates an email notification adapter (stub).
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Name returns "email".
func (a *Adapter) Name() string { return "email" }

// Send logs the notification that would be emailed.
func (a *Adapter) Send(_ context.Context, n *notification.Notification, prefs *notification.Preferences) error {
	if prefs.EmailAddress == "" {
		return fmt.Errorf("email address not configured for project %s", n.ProjectID)
	}
	log.Printf("notification/email: [stub] would send to %s — [%s] %s: %s",
		prefs.EmailAddress, string(n.Urgency), n.Title, n.Body)
	return nil
}

// SendDigest logs the digest that would be emailed.
func (a *Adapter) SendDigest(_ context.Context, notifications []*notification.Notification, prefs *notification.Preferences) error {
	if prefs.EmailAddress == "" {
		return fmt.Errorf("email address not configured")
	}
	log.Printf("notification/email: [stub] would send digest (%d items) to %s",
		len(notifications), prefs.EmailAddress)
	return nil
}
