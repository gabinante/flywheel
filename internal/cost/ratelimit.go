package cost

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RateLimitHandler manages rate-limit backoff behavior.
// When a rate limit is hit, it backs off rather than failing, surfaces the
// delay to the operator, and automatically resumes when the reset time passes.
type RateLimitHandler struct {
	store  Store
	notify RateLimitNotifier

	mu      sync.RWMutex
	blocked map[string]*RateLimitEvent // provider:model -> active limit
}

// RateLimitNotifier pushes rate-limit notifications to operators.
type RateLimitNotifier interface {
	NotifyRateLimit(ctx context.Context, event *RateLimitEvent) error
	NotifyRateLimitResume(ctx context.Context, event *RateLimitEvent) error
}

// NoopRateLimitNotifier discards notifications.
type NoopRateLimitNotifier struct{}

// NotifyRateLimit is a no-op.
func (n *NoopRateLimitNotifier) NotifyRateLimit(_ context.Context, _ *RateLimitEvent) error {
	return nil
}

// NotifyRateLimitResume is a no-op.
func (n *NoopRateLimitNotifier) NotifyRateLimitResume(_ context.Context, _ *RateLimitEvent) error {
	return nil
}

// NewRateLimitHandler creates a new rate limit handler.
func NewRateLimitHandler(store Store, notify RateLimitNotifier) *RateLimitHandler {
	if notify == nil {
		notify = &NoopRateLimitNotifier{}
	}
	return &RateLimitHandler{
		store:   store,
		notify:  notify,
		blocked: make(map[string]*RateLimitEvent),
	}
}

// HandleRateLimit is called when an LLM provider returns a rate-limit error.
// It records the event, notifies the operator, and returns the wait duration.
// The caller should sleep for the returned duration before retrying.
func (h *RateLimitHandler) HandleRateLimit(ctx context.Context, projectID, ticketID, provider, model string, retryAfter time.Duration) (time.Duration, error) {
	if retryAfter <= 0 {
		retryAfter = 60 * time.Second // default backoff
	}

	// Cap maximum backoff at 5 minutes.
	if retryAfter > 5*time.Minute {
		retryAfter = 5 * time.Minute
	}

	resetAt := time.Now().UTC().Add(retryAfter)

	event := &RateLimitEvent{
		ID:         uuid.New().String(),
		ProjectID:  projectID,
		TicketID:   ticketID,
		Provider:   provider,
		Model:      model,
		RetryAfter: retryAfter,
		ResetAt:    resetAt,
		Timestamp:  time.Now().UTC(),
	}

	// Track as blocked.
	key := provider + ":" + model
	h.mu.Lock()
	h.blocked[key] = event
	h.mu.Unlock()

	// Persist.
	if err := h.store.RecordRateLimitEvent(ctx, event); err != nil {
		log.Printf("cost: failed to record rate limit event: %v", err)
	}

	// Notify operator.
	if err := h.notify.NotifyRateLimit(ctx, event); err != nil {
		log.Printf("cost: failed to notify rate limit: %v", err)
	}

	log.Printf("cost: rate limit hit for %s/%s (ticket=%s), backing off %v until %v",
		provider, model, ticketID, retryAfter, resetAt)

	return retryAfter, nil
}

// WaitAndResume blocks until the rate limit resets, then marks as resumed.
// Call this in a goroutine — it handles the backoff sleep internally.
func (h *RateLimitHandler) WaitAndResume(ctx context.Context, provider, model string) error {
	key := provider + ":" + model

	h.mu.RLock()
	event, ok := h.blocked[key]
	h.mu.RUnlock()

	if !ok {
		return nil // not rate limited
	}

	waitDuration := time.Until(event.ResetAt)
	if waitDuration <= 0 {
		// Already past reset time.
		h.markResumed(ctx, key, event)
		return nil
	}

	// Wait for reset or context cancellation.
	select {
	case <-time.After(waitDuration):
		h.markResumed(ctx, key, event)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsRateLimited checks if a provider/model combination is currently rate-limited.
func (h *RateLimitHandler) IsRateLimited(provider, model string) (bool, time.Duration) {
	key := provider + ":" + model

	h.mu.RLock()
	event, ok := h.blocked[key]
	h.mu.RUnlock()

	if !ok {
		return false, 0
	}

	remaining := time.Until(event.ResetAt)
	if remaining <= 0 {
		// Expired — clean up lazily.
		h.mu.Lock()
		delete(h.blocked, key)
		h.mu.Unlock()
		return false, 0
	}

	return true, remaining
}

// GetActiveRateLimits returns all currently active rate limits.
func (h *RateLimitHandler) GetActiveRateLimits() []*RateLimitEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var active []*RateLimitEvent
	now := time.Now().UTC()
	for _, event := range h.blocked {
		if event.ResetAt.After(now) {
			active = append(active, event)
		}
	}
	return active
}

func (h *RateLimitHandler) markResumed(ctx context.Context, key string, event *RateLimitEvent) {
	h.mu.Lock()
	delete(h.blocked, key)
	h.mu.Unlock()

	event.Resumed = true
	event.ResumedAt = time.Now().UTC()

	if err := h.store.MarkRateLimitResumed(ctx, event.ID); err != nil {
		log.Printf("cost: failed to mark rate limit resumed: %v", err)
	}

	if err := h.notify.NotifyRateLimitResume(ctx, event); err != nil {
		log.Printf("cost: failed to notify rate limit resume: %v", err)
	}

	log.Printf("cost: rate limit reset for %s, resuming operations", key)
}

// FormatRateLimitStatus returns a human-readable status of active rate limits.
func FormatRateLimitStatus(events []*RateLimitEvent) string {
	if len(events) == 0 {
		return "No active rate limits"
	}
	msg := fmt.Sprintf("%d active rate limit(s):\n", len(events))
	for _, e := range events {
		remaining := time.Until(e.ResetAt)
		msg += fmt.Sprintf("  - %s/%s: resets in %v\n", e.Provider, e.Model, remaining.Round(time.Second))
	}
	return msg
}
