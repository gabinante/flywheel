package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// notifyChannel is the Postgres LISTEN/NOTIFY channel name.
	notifyChannel = "event_bus"

	// defaultPollInterval is how often the polling fallback checks for missed events.
	defaultPollInterval = 5 * time.Second

	// defaultRetentionDays is how long fully-acked events are kept before pruning.
	defaultRetentionDays = 7
)

// PostgresBusConfig configures the Postgres durable event bus.
type PostgresBusConfig struct {
	// PollInterval controls how often the fallback poller checks for missed events.
	// Default: 5 seconds.
	PollInterval time.Duration

	// RetentionDays controls how long fully-acked events are kept. Default: 7.
	RetentionDays int
}

// PostgresBus is a durable event bus backed by a Postgres outbox table.
// It uses LISTEN/NOTIFY for real-time delivery with polling fallback.
type PostgresBus struct {
	pool   *pgxpool.Pool
	config PostgresBusConfig

	mu          sync.RWMutex
	exact       map[string][]subscriberEntry
	patterns    []patternEntry
	subscribers map[string]struct{} // set of all subscriber IDs

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Last processed sequence for polling fallback. Accessed concurrently from
	// Start, pollUndelivered, and deliverByID.
	lastSeq atomic.Int64
}

// storeMaxSeq advances lastSeq to seq if seq is larger, atomically.
func (b *PostgresBus) storeMaxSeq(seq int64) {
	for {
		cur := b.lastSeq.Load()
		if seq <= cur {
			return
		}
		if b.lastSeq.CompareAndSwap(cur, seq) {
			return
		}
	}
}

type subscriberEntry struct {
	subscriberID string
	handler      HandlerFn
}

type patternEntry struct {
	pattern      string
	subscriberID string
	handler      HandlerFn
}

// NewPostgresBus creates a new Postgres-backed durable event bus.
func NewPostgresBus(pool *pgxpool.Pool, config PostgresBusConfig) *PostgresBus {
	if config.PollInterval <= 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.RetentionDays <= 0 {
		config.RetentionDays = defaultRetentionDays
	}
	return &PostgresBus{
		pool:        pool,
		config:      config,
		exact:       make(map[string][]subscriberEntry),
		subscribers: make(map[string]struct{}),
	}
}

// Publish persists the event and delivers to subscribers (satisfies Bus interface).
func (b *PostgresBus) Publish(ctx context.Context, event Event) error {
	_, err := b.PublishDurable(ctx, event)
	return err
}

// PublishDurable persists the event to the outbox and triggers delivery.
func (b *PostgresBus) PublishDurable(ctx context.Context, event Event) (string, error) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return "", fmt.Errorf("marshal event payload: %w", err)
	}

	var eventID string
	err = b.pool.QueryRow(ctx, `
		INSERT INTO event_outbox (event_type, entity_key, payload)
		VALUES ($1, $2, $3)
		RETURNING id
	`, event.Type, event.EntityKey, payload).Scan(&eventID)
	if err != nil {
		return "", fmt.Errorf("insert event outbox: %w", err)
	}

	// Notify listeners for real-time delivery.
	_, err = b.pool.Exec(ctx, "SELECT pg_notify($1, $2)", notifyChannel, eventID)
	if err != nil {
		// Non-fatal: polling fallback will pick it up.
		slog.Warn("pg_notify failed, polling will retry", "error", err)
	}

	// Deliver to in-process subscribers immediately (best-effort for latency).
	event.ID = eventID
	b.deliverToSubscribers(ctx, event)

	return eventID, nil
}

// Subscribe registers a handler for exact event type match (satisfies Bus interface).
func (b *PostgresBus) Subscribe(eventType string, handler HandlerFn) {
	// Generate a subscriber ID for backward compatibility.
	b.mu.Lock()
	defer b.mu.Unlock()
	id := fmt.Sprintf("legacy:%s:%d", eventType, len(b.exact[eventType]))
	b.exact[eventType] = append(b.exact[eventType], subscriberEntry{
		subscriberID: id,
		handler:      handler,
	})
	b.subscribers[id] = struct{}{}
}

// SubscribePattern registers a handler for events matching a glob pattern.
func (b *PostgresBus) SubscribePattern(pattern string, subscriberID string, handler HandlerFn) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.patterns = append(b.patterns, patternEntry{
		pattern:      pattern,
		subscriberID: subscriberID,
		handler:      handler,
	})
	b.subscribers[subscriberID] = struct{}{}
	return nil
}

// Ack marks an event as successfully processed by a subscriber.
func (b *PostgresBus) Ack(ctx context.Context, eventID string, subscriberID string) error {
	_, err := b.pool.Exec(ctx, `
		INSERT INTO event_deliveries (event_id, subscriber_id, acked_at)
		VALUES ($1, $2, now())
		ON CONFLICT (event_id, subscriber_id)
		DO UPDATE SET acked_at = now()
	`, eventID, subscriberID)
	return err
}

// Start begins the LISTEN/NOTIFY listener and polling fallback.
func (b *PostgresBus) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Initialize lastSeq from the database.
	var maxSeq *int64
	err := b.pool.QueryRow(ctx, "SELECT MAX(sequence) FROM event_outbox").Scan(&maxSeq)
	if err != nil {
		return fmt.Errorf("get max sequence: %w", err)
	}
	if maxSeq != nil {
		b.lastSeq.Store(*maxSeq)
	}

	// Start LISTEN/NOTIFY listener.
	b.wg.Add(1)
	go b.listenLoop()

	// Start polling fallback.
	b.wg.Add(1)
	go b.pollLoop()

	// Start retention pruner.
	b.wg.Add(1)
	go b.pruneLoop()

	return nil
}

// Stop gracefully shuts down the bus.
func (b *PostgresBus) Stop() error {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	return nil
}

// listenLoop uses Postgres LISTEN/NOTIFY for real-time event delivery.
func (b *PostgresBus) listenLoop() {
	defer b.wg.Done()

	for {
		if b.ctx.Err() != nil {
			return
		}

		conn, err := b.pool.Acquire(b.ctx)
		if err != nil {
			if b.ctx.Err() != nil {
				return
			}
			slog.Error("acquire conn for LISTEN failed", "error", err)
			time.Sleep(time.Second)
			continue
		}

		_, err = conn.Exec(b.ctx, "LISTEN "+notifyChannel)
		if err != nil {
			conn.Release()
			if b.ctx.Err() != nil {
				return
			}
			slog.Error("LISTEN failed", "error", err)
			time.Sleep(time.Second)
			continue
		}

		// Block waiting for notifications.
		for {
			notification, err := conn.Conn().WaitForNotification(b.ctx)
			if err != nil {
				conn.Release()
				if b.ctx.Err() != nil {
					return
				}
				slog.Error("wait notification failed", "error", err)
				time.Sleep(time.Second)
				break
			}

			// Notification payload is the event ID.
			eventID := notification.Payload
			b.deliverByID(b.ctx, eventID)
		}
	}
}

// pollLoop is the fallback that picks up events missed by LISTEN/NOTIFY.
func (b *PostgresBus) pollLoop() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.pollUndelivered()
		}
	}
}

// pollUndelivered fetches events newer than lastSeq and delivers them.
func (b *PostgresBus) pollUndelivered() {
	rows, err := b.pool.Query(b.ctx, `
		SELECT id, event_type, entity_key, payload, created_at, sequence
		FROM event_outbox
		WHERE sequence > $1
		ORDER BY entity_key, sequence
		LIMIT 100
	`, b.lastSeq.Load())
	if err != nil {
		if b.ctx.Err() == nil {
			slog.Error("poll failed", "error", err)
		}
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id        string
			eventType string
			entityKey string
			payload   []byte
			createdAt time.Time
			seq       int64
		)
		if err := rows.Scan(&id, &eventType, &entityKey, &payload, &createdAt, &seq); err != nil {
			slog.Error("poll scan failed", "error", err)
			continue
		}

		var payloadMap map[string]any
		if err := json.Unmarshal(payload, &payloadMap); err != nil {
			slog.Error("poll unmarshal failed", "error", err)
			continue
		}

		event := Event{
			ID:        id,
			Type:      eventType,
			EntityKey: entityKey,
			Payload:   payloadMap,
			Timestamp: createdAt,
		}

		b.deliverToSubscribers(b.ctx, event)

		b.storeMaxSeq(seq)
	}
}

// deliverByID loads an event by ID and delivers to matching subscribers.
func (b *PostgresBus) deliverByID(ctx context.Context, eventID string) {
	var (
		eventType string
		entityKey string
		payload   []byte
		createdAt time.Time
		seq       int64
	)
	err := b.pool.QueryRow(ctx, `
		SELECT event_type, entity_key, payload, created_at, sequence
		FROM event_outbox WHERE id = $1
	`, eventID).Scan(&eventType, &entityKey, &payload, &createdAt, &seq)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("deliver by id failed", "event_id", eventID, "error", err)
		}
		return
	}

	var payloadMap map[string]any
	if err := json.Unmarshal(payload, &payloadMap); err != nil {
		slog.Error("unmarshal failed", "event_id", eventID, "error", err)
		return
	}

	event := Event{
		ID:        eventID,
		Type:      eventType,
		EntityKey: entityKey,
		Payload:   payloadMap,
		Timestamp: createdAt,
	}

	b.deliverToSubscribers(ctx, event)

	// Update lastSeq for poll dedup.
	b.storeMaxSeq(seq)
}

// deliverToSubscribers fans out an event to all matching handlers.
func (b *PostgresBus) deliverToSubscribers(ctx context.Context, event Event) {
	b.mu.RLock()
	exactHandlers := b.exact[event.Type]
	patterns := b.patterns
	b.mu.RUnlock()

	for _, entry := range exactHandlers {
		b.deliverOne(ctx, event, entry.subscriberID, entry.handler)
	}

	for _, pe := range patterns {
		if MatchPattern(pe.pattern, event.Type) {
			b.deliverOne(ctx, event, pe.subscriberID, pe.handler)
		}
	}
}

// deliverOne delivers to a single subscriber with idempotency check.
func (b *PostgresBus) deliverOne(ctx context.Context, event Event, subscriberID string, handler HandlerFn) {
	// Skip if already acked (idempotency for at-least-once).
	if event.ID != "" {
		var acked bool
		err := b.pool.QueryRow(ctx, `
			SELECT acked_at IS NOT NULL FROM event_deliveries
			WHERE event_id = $1 AND subscriber_id = $2
		`, event.ID, subscriberID).Scan(&acked)
		if err == nil && acked {
			return // Already processed.
		}

		// Record delivery attempt.
		_, _ = b.pool.Exec(ctx, `
			INSERT INTO event_deliveries (event_id, subscriber_id)
			VALUES ($1, $2)
			ON CONFLICT (event_id, subscriber_id) DO NOTHING
		`, event.ID, subscriberID)
	}

	// Call the handler, recovering from panics so one bad handler cannot kill
	// the delivery goroutine (listenLoop/pollLoop). On panic we skip the ack so
	// the event is redelivered on a later poll.
	if panicked := b.callHandler(ctx, handler, event, subscriberID); panicked {
		return
	}

	// Auto-ack after successful delivery (handler didn't panic).
	if event.ID != "" {
		_ = b.Ack(ctx, event.ID, subscriberID)
	}
}

// callHandler invokes handler and reports whether it panicked.
func (b *PostgresBus) callHandler(ctx context.Context, handler HandlerFn, event Event, subscriberID string) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			slog.Error("event handler panicked",
				"event_id", event.ID, "event_type", event.Type,
				"subscriber", subscriberID, "panic", r)
		}
	}()
	handler(ctx, event)
	return false
}

// pruneLoop removes old fully-acked events periodically.
func (b *PostgresBus) pruneLoop() {
	defer b.wg.Done()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.pruneOldEvents()
		}
	}
}

func (b *PostgresBus) pruneOldEvents() {
	cutoff := time.Now().AddDate(0, 0, -b.config.RetentionDays)
	_, err := b.pool.Exec(b.ctx, `
		DELETE FROM event_outbox
		WHERE created_at < $1
		AND NOT EXISTS (
			SELECT 1 FROM event_deliveries
			WHERE event_deliveries.event_id = event_outbox.id
			AND event_deliveries.acked_at IS NULL
		)
	`, cutoff)
	if err != nil && b.ctx.Err() == nil {
		slog.Error("prune failed", "error", err)
	}
}
