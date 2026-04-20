package cost

import (
	"context"
	"testing"
	"time"
)

func TestRateLimitHandlerBasic(t *testing.T) {
	store := NewMemStore()

	notifications := make([]*RateLimitEvent, 0)
	notifier := &testRLNotifier{events: &notifications}
	handler := NewRateLimitHandler(store, notifier)

	ctx := context.Background()

	// Simulate rate limit hit.
	wait, err := handler.HandleRateLimit(ctx, "proj-1", "ticket-1", "anthropic", "claude-sonnet-4-20250514", 30*time.Second)
	if err != nil {
		t.Fatalf("HandleRateLimit: %v", err)
	}
	if wait != 30*time.Second {
		t.Errorf("expected 30s backoff, got %v", wait)
	}

	// Should be rate-limited.
	limited, remaining := handler.IsRateLimited("anthropic", "claude-sonnet-4-20250514")
	if !limited {
		t.Error("expected rate-limited")
	}
	if remaining <= 0 {
		t.Error("expected positive remaining time")
	}

	// Notification should have been sent.
	if len(notifications) == 0 {
		t.Error("expected notification to be sent")
	}
}

func TestRateLimitHandlerMaxBackoff(t *testing.T) {
	store := NewMemStore()
	handler := NewRateLimitHandler(store, nil)

	ctx := context.Background()

	// Request huge backoff — should be capped at 5 minutes.
	wait, err := handler.HandleRateLimit(ctx, "proj-1", "ticket-1", "anthropic", "claude-opus-4-20250514", 30*time.Minute)
	if err != nil {
		t.Fatalf("HandleRateLimit: %v", err)
	}
	if wait != 5*time.Minute {
		t.Errorf("expected 5m cap, got %v", wait)
	}
}

func TestRateLimitHandlerExpiry(t *testing.T) {
	store := NewMemStore()
	handler := NewRateLimitHandler(store, nil)

	ctx := context.Background()

	// Set a very short backoff.
	_, err := handler.HandleRateLimit(ctx, "proj-1", "ticket-1", "anthropic", "test-model", 1*time.Millisecond)
	if err != nil {
		t.Fatalf("HandleRateLimit: %v", err)
	}

	// Wait for it to expire.
	time.Sleep(5 * time.Millisecond)

	// Should no longer be rate-limited.
	limited, _ := handler.IsRateLimited("anthropic", "test-model")
	if limited {
		t.Error("expected rate limit to have expired")
	}
}

func TestRateLimitHandlerGetActive(t *testing.T) {
	store := NewMemStore()
	handler := NewRateLimitHandler(store, nil)

	ctx := context.Background()

	// Add multiple rate limits.
	_, _ = handler.HandleRateLimit(ctx, "proj-1", "t1", "anthropic", "model-a", 60*time.Second)
	_, _ = handler.HandleRateLimit(ctx, "proj-1", "t2", "openai", "model-b", 30*time.Second)

	active := handler.GetActiveRateLimits()
	if len(active) != 2 {
		t.Errorf("expected 2 active rate limits, got %d", len(active))
	}
}

func TestRateLimitDefaultBackoff(t *testing.T) {
	store := NewMemStore()
	handler := NewRateLimitHandler(store, nil)

	ctx := context.Background()

	// Zero retry-after should default to 60s.
	wait, err := handler.HandleRateLimit(ctx, "proj-1", "t1", "anthropic", "model-x", 0)
	if err != nil {
		t.Fatalf("HandleRateLimit: %v", err)
	}
	if wait != 60*time.Second {
		t.Errorf("expected 60s default, got %v", wait)
	}
}

// testRLNotifier captures rate limit notifications for testing.
type testRLNotifier struct {
	events  *[]*RateLimitEvent
	resumes *[]*RateLimitEvent
}

func (n *testRLNotifier) NotifyRateLimit(_ context.Context, event *RateLimitEvent) error {
	if n.events != nil {
		*n.events = append(*n.events, event)
	}
	return nil
}

func (n *testRLNotifier) NotifyRateLimitResume(_ context.Context, event *RateLimitEvent) error {
	if n.resumes != nil {
		*n.resumes = append(*n.resumes, event)
	}
	return nil
}
