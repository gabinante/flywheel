package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// memoryStore is an in-memory implementation of Store for testing.
type memoryStore struct {
	mu          sync.Mutex
	events      map[string]*Event
	deliveries  []*Delivery
	configs     map[string]*WebhookConfig
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		events:  make(map[string]*Event),
		configs: make(map[string]*WebhookConfig),
	}
}

func (s *memoryStore) CreateEvent(_ context.Context, event *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[event.ID] = event
	return nil
}

func (s *memoryStore) GetEvent(_ context.Context, id string) (*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.events[id]
	if !ok {
		return nil, fmt.Errorf("event not found: %s", id)
	}
	// Return a copy.
	copy := *e
	return &copy, nil
}

func (s *memoryStore) GetWebhookConfig(_ context.Context, projectID string) (*WebhookConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, ok := s.configs[projectID]
	if !ok {
		return &WebhookConfig{}, nil
	}
	return cfg, nil
}

func (s *memoryStore) UpdateEventStatus(_ context.Context, id string, status EventStatus, attemptCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.events[id]; ok {
		e.Status = status
		e.AttemptCount = attemptCount
	}
	return nil
}

func (s *memoryStore) MarkEventDelivered(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.events[id]; ok {
		e.Status = EventStatusDelivered
		now := time.Now().UTC()
		e.DeliveredAt = &now
	}
	return nil
}

func (s *memoryStore) MarkEventFailed(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.events[id]; ok {
		e.Status = EventStatusFailed
		now := time.Now().UTC()
		e.FailedAt = &now
	}
	return nil
}

func (s *memoryStore) RecordDelivery(_ context.Context, delivery *Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliveries = append(s.deliveries, delivery)
	return nil
}

func (s *memoryStore) ListDeliveries(_ context.Context, webhookEventID string) ([]*Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*Delivery
	for _, d := range s.deliveries {
		if d.WebhookEventID == webhookEventID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (s *memoryStore) ListEvents(_ context.Context, projectID string, limit, offset int) ([]*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*Event
	for _, e := range s.events {
		if e.ProjectID == projectID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (s *memoryStore) ListPendingEvents(_ context.Context, limit int) ([]*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*Event
	for _, e := range s.events {
		if e.Status == EventStatusPending {
			result = append(result, e)
		}
	}
	return result, nil
}

func makeTestEvent(projectID string) *Event {
	return &Event{
		ID:           "evt_test_123",
		ProjectID:    projectID,
		EventType:    "payment.completed",
		Payload: map[string]any{
			"transaction_id": "txn_abc",
			"amount":         1000,
			"currency":       "USD",
			"status":         "completed",
			"payment_method": "card",
			"customer": map[string]any{
				"id":    "cust_1",
				"email": "test@example.com",
			},
		},
		Status:       EventStatusPending,
		AttemptCount: 0,
		MaxAttempts:  MaxAttempts,
		CreatedAt:    time.Now().UTC(),
	}
}

func TestWorker_SuccessfulDelivery(t *testing.T) {
	var receivedHeaders http.Header
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		receivedBody, _ = readBody(r)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	store := newMemoryStore()
	projectID := "proj-1"
	secret := "test-webhook-secret"

	store.configs[projectID] = &WebhookConfig{
		WebhookURL:    server.URL,
		WebhookSecret: secret,
	}

	event := makeTestEvent(projectID)
	store.events[event.ID] = event

	worker := NewWorker(store)
	err := worker.HandleDelivery(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("expected successful delivery, got error: %v", err)
	}

	// Verify event is marked as delivered.
	stored := store.events[event.ID]
	if stored.Status != EventStatusDelivered {
		t.Errorf("expected status %q, got %q", EventStatusDelivered, stored.Status)
	}

	// Verify delivery was recorded.
	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}
	delivery := store.deliveries[0]
	if !delivery.Success {
		t.Error("expected successful delivery")
	}
	if delivery.AttemptNumber != 1 {
		t.Errorf("expected attempt 1, got %d", delivery.AttemptNumber)
	}
	if delivery.StatusCode == nil || *delivery.StatusCode != 200 {
		t.Error("expected status code 200")
	}

	// Verify headers.
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Error("expected Content-Type: application/json")
	}
	if receivedHeaders.Get("X-GHP-Signature") == "" {
		t.Error("expected X-GHP-Signature header")
	}
	if !strings.HasPrefix(receivedHeaders.Get("X-GHP-Signature"), "sha256=") {
		t.Error("expected X-GHP-Signature to start with sha256=")
	}
	if receivedHeaders.Get("X-GHP-Timestamp") == "" {
		t.Error("expected X-GHP-Timestamp header")
	}
	if receivedHeaders.Get("X-GHP-Event-Type") != "payment.completed" {
		t.Errorf("expected X-GHP-Event-Type=payment.completed, got %q",
			receivedHeaders.Get("X-GHP-Event-Type"))
	}
	if receivedHeaders.Get("X-GHP-Event-Id") != event.ID {
		t.Errorf("expected X-GHP-Event-Id=%s, got %q",
			event.ID, receivedHeaders.Get("X-GHP-Event-Id"))
	}
	if receivedHeaders.Get("User-Agent") != "GoHighPayment-Webhook/1.0" {
		t.Errorf("expected User-Agent=GoHighPayment-Webhook/1.0, got %q",
			receivedHeaders.Get("User-Agent"))
	}

	// Verify payload format.
	var payload StandardPayload
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payload.ID != event.ID {
		t.Errorf("payload.id=%s, want %s", payload.ID, event.ID)
	}
	if payload.Type != "payment.completed" {
		t.Errorf("payload.type=%s, want payment.completed", payload.Type)
	}
	if payload.Data == nil {
		t.Error("payload.data should not be nil")
	}
}

func TestWorker_SignatureVerification(t *testing.T) {
	var receivedSignature string
	var receivedTimestamp string
	var receivedBodyBytes []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-GHP-Signature")
		receivedTimestamp = r.Header.Get("X-GHP-Timestamp")
		receivedBodyBytes, _ = readBody(r)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newMemoryStore()
	projectID := "proj-1"
	secret := "my-secret-key"

	store.configs[projectID] = &WebhookConfig{
		WebhookURL:    server.URL,
		WebhookSecret: secret,
	}

	event := makeTestEvent(projectID)
	store.events[event.ID] = event

	worker := NewWorker(store)
	_ = worker.HandleDelivery(context.Background(), event.ID)

	// Verify the signature is valid.
	ts, err := ParseTimestampHeader(receivedTimestamp)
	if err != nil {
		t.Fatalf("failed to parse timestamp: %v", err)
	}

	err = VerifySignature(secret, receivedSignature, ts, receivedBodyBytes)
	if err != nil {
		t.Fatalf("signature verification failed: %v", err)
	}
}

func TestWorker_ServerError_Retry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	store := newMemoryStore()
	projectID := "proj-1"
	store.configs[projectID] = &WebhookConfig{
		WebhookURL:    server.URL,
		WebhookSecret: "secret",
	}

	event := makeTestEvent(projectID)
	store.events[event.ID] = event

	worker := NewWorker(store)
	err := worker.HandleDelivery(context.Background(), event.ID)

	// Should return a RetryError.
	retryErr, ok := IsRetryError(err)
	if !ok {
		t.Fatalf("expected RetryError, got: %v", err)
	}
	if retryErr.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", retryErr.Attempt)
	}

	// Event should still be pending.
	stored := store.events[event.ID]
	if stored.Status != EventStatusPending {
		t.Errorf("expected status pending, got %q", stored.Status)
	}

	// Delivery should be recorded.
	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}
	d := store.deliveries[0]
	if d.Success {
		t.Error("expected unsuccessful delivery")
	}
	if d.StatusCode == nil || *d.StatusCode != 500 {
		t.Error("expected status code 500")
	}
}

func TestWorker_ClientError_PermanentFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	store := newMemoryStore()
	projectID := "proj-1"
	store.configs[projectID] = &WebhookConfig{
		WebhookURL:    server.URL,
		WebhookSecret: "secret",
	}

	event := makeTestEvent(projectID)
	store.events[event.ID] = event

	worker := NewWorker(store)
	err := worker.HandleDelivery(context.Background(), event.ID)

	// Should NOT return RetryError (permanent failure).
	if err != nil {
		t.Fatalf("expected nil error for permanent failure, got: %v", err)
	}

	// Event should be marked as failed.
	stored := store.events[event.ID]
	if stored.Status != EventStatusFailed {
		t.Errorf("expected status failed, got %q", stored.Status)
	}
}

func TestWorker_429_RetryWithHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer server.Close()

	store := newMemoryStore()
	projectID := "proj-1"
	store.configs[projectID] = &WebhookConfig{
		WebhookURL:    server.URL,
		WebhookSecret: "secret",
	}

	event := makeTestEvent(projectID)
	store.events[event.ID] = event

	worker := NewWorker(store)
	err := worker.HandleDelivery(context.Background(), event.ID)

	// Should return RetryError with Retry-After value.
	retryErr, ok := IsRetryError(err)
	if !ok {
		t.Fatalf("expected RetryError, got: %v", err)
	}
	if retryErr.Delay != 30*time.Second {
		t.Errorf("expected 30s retry delay, got %v", retryErr.Delay)
	}

	// Event should still be pending (not permanently failed).
	stored := store.events[event.ID]
	if stored.Status != EventStatusPending {
		t.Errorf("expected status pending, got %q", stored.Status)
	}
}

func TestWorker_MaxAttemptsReached_MarksFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := newMemoryStore()
	projectID := "proj-1"
	store.configs[projectID] = &WebhookConfig{
		WebhookURL:    server.URL,
		WebhookSecret: "secret",
	}

	event := makeTestEvent(projectID)
	event.AttemptCount = 3 // Already attempted 3 times, this will be attempt 4 (max).
	store.events[event.ID] = event

	worker := NewWorker(store)
	err := worker.HandleDelivery(context.Background(), event.ID)

	// Should return nil (not RetryError) since max attempts reached.
	if err != nil {
		t.Fatalf("expected nil error after max attempts, got: %v", err)
	}

	// Event should be marked as failed.
	stored := store.events[event.ID]
	if stored.Status != EventStatusFailed {
		t.Errorf("expected status failed, got %q", stored.Status)
	}
}

func TestWorker_NoWebhookURL_Skips(t *testing.T) {
	store := newMemoryStore()
	projectID := "proj-1"
	store.configs[projectID] = &WebhookConfig{
		WebhookURL:    "", // No URL configured.
		WebhookSecret: "secret",
	}

	event := makeTestEvent(projectID)
	store.events[event.ID] = event

	worker := NewWorker(store)
	err := worker.HandleDelivery(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	// Event should still be pending (skipped, not failed).
	stored := store.events[event.ID]
	if stored.Status != EventStatusPending {
		t.Errorf("expected status pending, got %q", stored.Status)
	}
}

func TestWorker_AlreadyDelivered_Skips(t *testing.T) {
	store := newMemoryStore()
	projectID := "proj-1"
	store.configs[projectID] = &WebhookConfig{
		WebhookURL:    "http://example.com/webhook",
		WebhookSecret: "secret",
	}

	event := makeTestEvent(projectID)
	event.Status = EventStatusDelivered
	store.events[event.ID] = event

	worker := NewWorker(store)
	err := worker.HandleDelivery(context.Background(), event.ID)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	// No deliveries should be recorded.
	if len(store.deliveries) != 0 {
		t.Errorf("expected 0 deliveries, got %d", len(store.deliveries))
	}
}

func TestWorker_NetworkError_Retry(t *testing.T) {
	store := newMemoryStore()
	projectID := "proj-1"
	store.configs[projectID] = &WebhookConfig{
		WebhookURL:    "http://192.0.2.1:1/webhook", // Non-routable address.
		WebhookSecret: "secret",
	}

	event := makeTestEvent(projectID)
	store.events[event.ID] = event

	// Use a client with very short timeout.
	client := &http.Client{Timeout: 100 * time.Millisecond}
	worker := NewWorker(store, WithHTTPClient(client))
	err := worker.HandleDelivery(context.Background(), event.ID)

	// Should return RetryError (network error is retryable).
	retryErr, ok := IsRetryError(err)
	if !ok {
		t.Fatalf("expected RetryError for network error, got: %v", err)
	}
	if retryErr.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", retryErr.Attempt)
	}

	// Delivery should be recorded with error.
	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}
	d := store.deliveries[0]
	if d.Success {
		t.Error("expected unsuccessful delivery")
	}
	if d.Error == "" {
		t.Error("expected error message")
	}
	if d.StatusCode != nil {
		t.Error("expected nil status code for network error")
	}
}

func TestWorker_RetryDelays(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := newMemoryStore()
	projectID := "proj-1"
	store.configs[projectID] = &WebhookConfig{
		WebhookURL:    server.URL,
		WebhookSecret: "secret",
	}

	worker := NewWorker(store)

	// Test retry delays for each attempt.
	expectedDelays := []time.Duration{
		10 * time.Second,  // After attempt 1
		60 * time.Second,  // After attempt 2
		300 * time.Second, // After attempt 3
	}

	for i, expectedDelay := range expectedDelays {
		event := makeTestEvent(projectID)
		event.ID = fmt.Sprintf("evt_retry_%d", i)
		event.AttemptCount = i
		store.events[event.ID] = event

		err := worker.HandleDelivery(context.Background(), event.ID)
		retryErr, ok := IsRetryError(err)
		if !ok {
			t.Fatalf("attempt %d: expected RetryError, got: %v", i+1, err)
		}
		if retryErr.Delay != expectedDelay {
			t.Errorf("attempt %d: expected delay %v, got %v", i+1, expectedDelay, retryErr.Delay)
		}
	}
}

func TestWorker_MultipleEventTypes(t *testing.T) {
	eventTypes := []struct {
		eventType string
		payload   map[string]any
	}{
		{
			"payment.completed",
			map[string]any{"transaction_id": "txn_1", "amount": 1000, "currency": "USD"},
		},
		{
			"payment.failed",
			map[string]any{"transaction_id": "txn_2", "decline_reason": "insufficient_funds"},
		},
		{
			"payment.refunded",
			map[string]any{"transaction_id": "txn_3", "refund_amount": 500, "refund_type": "partial"},
		},
		{
			"chargeback.created",
			map[string]any{"chargeback_id": "cb_1", "reason_code": "10.4", "status": "open"},
		},
		{
			"chargeback.updated",
			map[string]any{"chargeback_id": "cb_1", "old_status": "open", "new_status": "resolved"},
		},
	}

	for _, tc := range eventTypes {
		t.Run(tc.eventType, func(t *testing.T) {
			var receivedType string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedType = r.Header.Get("X-GHP-Event-Type")
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			store := newMemoryStore()
			store.configs["proj-1"] = &WebhookConfig{
				WebhookURL:    server.URL,
				WebhookSecret: "secret",
			}

			event := &Event{
				ID:           "evt_" + tc.eventType,
				ProjectID:    "proj-1",
				EventType:    tc.eventType,
				Payload:      tc.payload,
				Status:       EventStatusPending,
				MaxAttempts:  MaxAttempts,
				CreatedAt:    time.Now().UTC(),
			}
			store.events[event.ID] = event

			worker := NewWorker(store)
			err := worker.HandleDelivery(context.Background(), event.ID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if receivedType != tc.eventType {
				t.Errorf("expected X-GHP-Event-Type=%s, got %s", tc.eventType, receivedType)
			}
		})
	}
}

func TestWorker_ResponseBodyTruncation(t *testing.T) {
	// Server returns a body larger than 1KB.
	longBody := strings.Repeat("x", 2000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(longBody))
	}))
	defer server.Close()

	store := newMemoryStore()
	store.configs["proj-1"] = &WebhookConfig{
		WebhookURL:    server.URL,
		WebhookSecret: "secret",
	}

	event := makeTestEvent("proj-1")
	store.events[event.ID] = event

	worker := NewWorker(store)
	_ = worker.HandleDelivery(context.Background(), event.ID)

	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}
	d := store.deliveries[0]
	if len(d.ResponseBody) > MaxResponseBodySize {
		t.Errorf("response body not truncated: %d > %d", len(d.ResponseBody), MaxResponseBodySize)
	}
}

func TestWorker_DurationTracking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := newMemoryStore()
	store.configs["proj-1"] = &WebhookConfig{
		WebhookURL:    server.URL,
		WebhookSecret: "secret",
	}

	event := makeTestEvent("proj-1")
	store.events[event.ID] = event

	worker := NewWorker(store)
	_ = worker.HandleDelivery(context.Background(), event.ID)

	if len(store.deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(store.deliveries))
	}
	d := store.deliveries[0]
	if d.DurationMs < 50 {
		t.Errorf("expected duration >= 50ms, got %dms", d.DurationMs)
	}
}

func TestTruncateResponseBody(t *testing.T) {
	short := "hello"
	if got := TruncateResponseBody(short); got != short {
		t.Errorf("short body should not be truncated: got %q", got)
	}

	long := strings.Repeat("x", 2000)
	truncated := TruncateResponseBody(long)
	if len(truncated) != MaxResponseBodySize {
		t.Errorf("expected length %d, got %d", MaxResponseBodySize, len(truncated))
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		header   string
		expected time.Duration
	}{
		{"30", 30 * time.Second},
		{"120", 120 * time.Second},
		{"0", 0},
		{"", 0},
		{"invalid", 0},
	}
	for _, tc := range tests {
		got := parseRetryAfter(tc.header)
		if got != tc.expected {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.header, got, tc.expected)
		}
	}
}

func TestIsRetryError(t *testing.T) {
	re := &RetryError{EventID: "evt_1", Delay: 10 * time.Second, Attempt: 1}
	if got, ok := IsRetryError(re); !ok || got != re {
		t.Error("expected RetryError to be recognized")
	}

	if _, ok := IsRetryError(fmt.Errorf("regular error")); ok {
		t.Error("expected regular error to not be RetryError")
	}

	if _, ok := IsRetryError(nil); ok {
		t.Error("expected nil to not be RetryError")
	}
}

// readBody reads the full request body.
func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}
