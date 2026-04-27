// Package sms implements the SMS channel adapter for the notification layer.
// This is a stub adapter — operators should plug in their own SMS provider
// (Twilio, AWS SNS, etc.) via the ChannelAdapter interface.
package sms

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gabinante/flywheel/internal/notification"
)

// Adapter is a stub SMS channel adapter.
// It logs notifications that would be sent via SMS.
// Replace with a real SMS API implementation for production use.
type Adapter struct{}

// NewAdapter creates an SMS notification adapter (stub).
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Name returns "sms".
func (a *Adapter) Name() string { return "sms" }

// Send logs the notification that would be sent via SMS.
func (a *Adapter) Send(_ context.Context, n *notification.Notification, prefs *notification.Preferences) error {
	if prefs.SMSNumber == "" {
		return fmt.Errorf("SMS number not configured for project %s", n.ProjectID)
	}
	slog.Info("sms stub: would send", "to", prefs.SMSNumber, "urgency", string(n.Urgency), "title", n.Title)
	return nil
}

// SendDigest logs the digest that would be sent via SMS.
func (a *Adapter) SendDigest(_ context.Context, notifications []*notification.Notification, prefs *notification.Preferences) error {
	if prefs.SMSNumber == "" {
		return fmt.Errorf("SMS number not configured")
	}
	slog.Info("sms stub: would send digest", "count", len(notifications), "to", prefs.SMSNumber)
	return nil
}
