package webhook_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events/hooks"
	"github.com/gabinante/flywheel/internal/notification"
	wh "github.com/gabinante/flywheel/internal/notification/webhook"
)

// mockSecretProvider implements webhook.SecretProvider for testing.
type mockSecretProvider struct {
	secrets map[string]string
}

func (m *mockSecretProvider) GetWebhookSecret(_ context.Context, projectID string) (string, error) {
	s, ok := m.secrets[projectID]
	if !ok {
		return "", fmt.Errorf("no secret for project %s", projectID)
	}
	return s, nil
}

func TestAdapter_Send_SignsPayload(t *testing.T) {
	secret, err := hooks.GenerateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	provider := &mockSecretProvider{secrets: map[string]string{"proj-1": secret}}

	var receivedHeaders http.Header
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		buf := make([]byte, 1<<20)
		n, _ := r.Body.Read(buf)
		receivedBody = buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := wh.NewAdapter(provider)
	n := &notification.Notification{
		ID:        "notif-1",
		ProjectID: "proj-1",
		TicketID:  "ticket-1",
		Category:  notification.CategoryUrgentDecision,
		Urgency:   notification.UrgencyCritical,
		Title:     "Test notification",
		Body:      "Something happened",
		CreatedAt: time.Now().UTC(),
	}
	prefs := &notification.Preferences{
		WebhookURL: server.URL,
	}

	err = adapter.Send(context.Background(), n, prefs)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Verify signature header is present.
	sigHeader := receivedHeaders.Get("X-GHP-Signature")
	if sigHeader == "" {
		t.Fatal("X-GHP-Signature header missing")
	}
	if !strings.HasPrefix(sigHeader, "sha256=") {
		t.Fatalf("X-GHP-Signature should start with 'sha256=', got %q", sigHeader)
	}
	signature := strings.TrimPrefix(sigHeader, "sha256=")

	// Verify timestamp header is present and recent.
	tsHeader := receivedHeaders.Get("X-GHP-Timestamp")
	if tsHeader == "" {
		t.Fatal("X-GHP-Timestamp header missing")
	}

	// Verify event type header.
	eventType := receivedHeaders.Get("X-GHP-Event-Type")
	if eventType != "urgent_decision" {
		t.Errorf("expected event type 'urgent_decision', got %q", eventType)
	}

	// Independently verify the signature.
	var ts int64
	fmt.Sscanf(tsHeader, "%d", &ts)
	ok, err := hooks.VerifyWebhook(secret, signature, ts, string(receivedBody))
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if !ok {
		t.Error("signature verification failed — signature does not match body")
	}

	// Verify payload structure.
	var payload wh.WebhookPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ID != "notif-1" {
		t.Errorf("payload ID = %q, want 'notif-1'", payload.ID)
	}
	if payload.ProjectID != "proj-1" {
		t.Errorf("payload ProjectID = %q, want 'proj-1'", payload.ProjectID)
	}
}

func TestAdapter_Send_NoWebhookURL(t *testing.T) {
	provider := &mockSecretProvider{secrets: map[string]string{"proj-1": "aa"}}
	adapter := wh.NewAdapter(provider)

	n := &notification.Notification{
		ID:        "notif-1",
		ProjectID: "proj-1",
		Category:  notification.CategoryAnomalyAlert,
		Urgency:   notification.UrgencyLow,
		Title:     "test",
		CreatedAt: time.Now().UTC(),
	}
	prefs := &notification.Preferences{WebhookURL: ""}

	err := adapter.Send(context.Background(), n, prefs)
	if err == nil {
		t.Fatal("expected error for missing webhook URL")
	}
}

func TestAdapter_Send_NoSecret(t *testing.T) {
	provider := &mockSecretProvider{secrets: map[string]string{}} // no secrets
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := wh.NewAdapter(provider)

	n := &notification.Notification{
		ID:        "notif-1",
		ProjectID: "proj-1",
		Category:  notification.CategoryAnomalyAlert,
		Urgency:   notification.UrgencyLow,
		Title:     "test",
		CreatedAt: time.Now().UTC(),
	}
	prefs := &notification.Preferences{WebhookURL: server.URL}

	err := adapter.Send(context.Background(), n, prefs)
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestAdapter_Send_EndpointFailure(t *testing.T) {
	secret, _ := hooks.GenerateWebhookSecret()
	provider := &mockSecretProvider{secrets: map[string]string{"proj-1": secret}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	adapter := wh.NewAdapter(provider)

	n := &notification.Notification{
		ID:        "notif-1",
		ProjectID: "proj-1",
		Category:  notification.CategoryAnomalyAlert,
		Urgency:   notification.UrgencyLow,
		Title:     "test",
		CreatedAt: time.Now().UTC(),
	}
	prefs := &notification.Preferences{WebhookURL: server.URL}

	err := adapter.Send(context.Background(), n, prefs)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestAdapter_Send_RotatedSecretInvalidatesOld(t *testing.T) {
	oldSecret, _ := hooks.GenerateWebhookSecret()
	newSecret, _ := hooks.GenerateWebhookSecret()
	provider := &mockSecretProvider{secrets: map[string]string{"proj-1": oldSecret}}

	var receivedHeaders http.Header
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		buf := make([]byte, 1<<20)
		n, _ := r.Body.Read(buf)
		receivedBody = buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := wh.NewAdapter(provider)
	n := &notification.Notification{
		ID:        "notif-1",
		ProjectID: "proj-1",
		Category:  notification.CategoryUrgentDecision,
		Urgency:   notification.UrgencyCritical,
		Title:     "Test",
		CreatedAt: time.Now().UTC(),
	}
	prefs := &notification.Preferences{WebhookURL: server.URL}

	// Send with old secret.
	err := adapter.Send(context.Background(), n, prefs)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Extract signature from old-secret delivery.
	sigHeader := receivedHeaders.Get("X-GHP-Signature")
	signature := strings.TrimPrefix(sigHeader, "sha256=")
	tsHeader := receivedHeaders.Get("X-GHP-Timestamp")
	var ts int64
	fmt.Sscanf(tsHeader, "%d", &ts)

	// Verify with old secret — should pass.
	ok, err := hooks.VerifyWebhook(oldSecret, signature, ts, string(receivedBody))
	if err != nil {
		t.Fatalf("VerifyWebhook(oldSecret) error = %v", err)
	}
	if !ok {
		t.Error("old secret should verify the signature")
	}

	// Verify with new secret — should fail.
	ok, err = hooks.VerifyWebhook(newSecret, signature, ts, string(receivedBody))
	if err != nil {
		t.Fatalf("VerifyWebhook(newSecret) error = %v", err)
	}
	if ok {
		t.Error("new secret should NOT verify the old signature")
	}

	// Now rotate the secret.
	provider.secrets["proj-1"] = newSecret

	// Send again.
	err = adapter.Send(context.Background(), n, prefs)
	if err != nil {
		t.Fatalf("Send() after rotation error = %v", err)
	}

	// Verify new delivery with new secret — should pass.
	sigHeader = receivedHeaders.Get("X-GHP-Signature")
	signature = strings.TrimPrefix(sigHeader, "sha256=")
	tsHeader = receivedHeaders.Get("X-GHP-Timestamp")
	fmt.Sscanf(tsHeader, "%d", &ts)

	ok, err = hooks.VerifyWebhook(newSecret, signature, ts, string(receivedBody))
	if err != nil {
		t.Fatalf("VerifyWebhook(newSecret) after rotation error = %v", err)
	}
	if !ok {
		t.Error("new secret should verify the new signature after rotation")
	}
}

func TestAdapter_SendDigest(t *testing.T) {
	secret, _ := hooks.GenerateWebhookSecret()
	provider := &mockSecretProvider{secrets: map[string]string{"proj-1": secret}}

	var receivedHeaders http.Header
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		buf := make([]byte, 1<<20)
		n, _ := r.Body.Read(buf)
		receivedBody = buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := wh.NewAdapter(provider)
	notifications := []*notification.Notification{
		{ID: "n1", ProjectID: "proj-1", Category: notification.CategoryAnomalyAlert, Urgency: notification.UrgencyLow, Title: "Alert 1", CreatedAt: time.Now().UTC()},
		{ID: "n2", ProjectID: "proj-1", Category: notification.CategoryCalibrationReview, Urgency: notification.UrgencyMedium, Title: "Review 1", CreatedAt: time.Now().UTC()},
	}
	prefs := &notification.Preferences{WebhookURL: server.URL}

	err := adapter.SendDigest(context.Background(), notifications, prefs)
	if err != nil {
		t.Fatalf("SendDigest() error = %v", err)
	}

	// Verify headers.
	if receivedHeaders.Get("X-GHP-Signature") == "" {
		t.Error("X-GHP-Signature header missing on digest")
	}
	if receivedHeaders.Get("X-GHP-Event-Type") != "notification.digest" {
		t.Errorf("expected event type 'notification.digest', got %q", receivedHeaders.Get("X-GHP-Event-Type"))
	}

	// Verify signature.
	sigHeader := receivedHeaders.Get("X-GHP-Signature")
	signature := strings.TrimPrefix(sigHeader, "sha256=")
	tsHeader := receivedHeaders.Get("X-GHP-Timestamp")
	var ts int64
	fmt.Sscanf(tsHeader, "%d", &ts)

	ok, err := hooks.VerifyWebhook(secret, signature, ts, string(receivedBody))
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if !ok {
		t.Error("digest signature verification failed")
	}
}

func TestAdapter_Name(t *testing.T) {
	adapter := wh.NewAdapter(nil)
	if adapter.Name() != "webhook" {
		t.Errorf("Name() = %q, want 'webhook'", adapter.Name())
	}
}
