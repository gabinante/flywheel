package events

import (
	"context"
	"sync"
)

// HandlerFn is called for each published event of the subscribed type.
type HandlerFn func(ctx context.Context, event Event)

// Bus is the event bus interface (in-process now; NATS-compatible later).
// Retained for backward compatibility — existing code can use either Bus or DurableEventBus.
type Bus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(eventType string, handler HandlerFn)
}

// DurableEventBus extends Bus with at-least-once delivery, pattern-based subscriptions,
// and ordered delivery by entity key. Implementations must persist events to an outbox
// before delivery and support polling fallback for missed LISTEN/NOTIFY signals.
//
// Pluggable: default is Postgres LISTEN/NOTIFY + outbox table. Alternatives include
// Redis Streams and NATS JetStream.
type DurableEventBus interface {
	Bus

	// PublishDurable persists the event to the outbox and delivers to subscribers.
	// Returns the event ID assigned by the store. At-least-once: the event is
	// guaranteed to be delivered to all matching subscribers (possibly more than once).
	PublishDurable(ctx context.Context, event Event) (eventID string, err error)

	// SubscribePattern registers a handler for events matching a glob pattern.
	// Patterns support '*' (match any segment) and '**' (match any number of segments).
	// Examples: "ticket.*", "ticket.state_changed", "*.created"
	// The subscriberID is used for idempotency — duplicate deliveries to the same
	// subscriber are deduplicated within the retention window.
	SubscribePattern(pattern string, subscriberID string, handler HandlerFn) error

	// Ack acknowledges successful processing of an event by a subscriber.
	// This marks the event as delivered for idempotency purposes.
	Ack(ctx context.Context, eventID string, subscriberID string) error

	// Start begins background delivery (LISTEN/NOTIFY listener + polling fallback).
	// Must be called after all subscriptions are registered.
	Start(ctx context.Context) error

	// Stop gracefully shuts down delivery. In-flight handlers are allowed to complete.
	Stop() error
}

// InProcessBus is an in-memory pub/sub implementation.
// It satisfies both Bus and DurableEventBus for backward compatibility,
// but does NOT provide durability or at-least-once guarantees.
type InProcessBus struct {
	mu              sync.RWMutex
	handlers        map[string][]HandlerFn
	patternHandlers []patternSubscription
}

type patternSubscription struct {
	pattern      string
	subscriberID string
	handler      HandlerFn
}

// NewInProcessBus returns a new in-process event bus.
func NewInProcessBus() *InProcessBus {
	return &InProcessBus{
		handlers: make(map[string][]HandlerFn),
	}
}

// Publish delivers the event to all subscribers of that type (exact match + patterns).
func (b *InProcessBus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	fns := b.handlers[event.Type]
	patterns := b.patternHandlers
	b.mu.RUnlock()

	for _, fn := range fns {
		fn(ctx, event)
	}
	for _, ps := range patterns {
		if MatchPattern(ps.pattern, event.Type) {
			ps.handler(ctx, event)
		}
	}
	return nil
}

// PublishDurable for InProcessBus just delegates to Publish (no durability).
func (b *InProcessBus) PublishDurable(ctx context.Context, event Event) (string, error) {
	return "", b.Publish(ctx, event)
}

// Subscribe registers a handler for the given event type (exact match).
func (b *InProcessBus) Subscribe(eventType string, handler HandlerFn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// SubscribePattern registers a handler for events matching a glob pattern.
func (b *InProcessBus) SubscribePattern(pattern string, subscriberID string, handler HandlerFn) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.patternHandlers = append(b.patternHandlers, patternSubscription{
		pattern:      pattern,
		subscriberID: subscriberID,
		handler:      handler,
	})
	return nil
}

// Ack is a no-op for InProcessBus (no durability tracking).
func (b *InProcessBus) Ack(_ context.Context, _ string, _ string) error {
	return nil
}

// Start is a no-op for InProcessBus.
func (b *InProcessBus) Start(_ context.Context) error {
	return nil
}

// Stop is a no-op for InProcessBus.
func (b *InProcessBus) Stop() error {
	return nil
}
