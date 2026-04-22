package hooks

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/google/uuid"
)

// GapDetector watches state-change observations and compares them against
// published change events. When a state change has no corresponding change
// event within a configurable window, it generates an unattributed change
// event automatically.
//
// This ensures the change stream is always complete — even when callers
// forget to publish a change event, the gap is captured.
type GapDetector struct {
	bus    events.Bus
	client *Client

	mu              sync.Mutex
	stateChanges    map[string]*stateObservation // entity_id → latest unmatched observation
	recentChanges   map[string]time.Time         // entity_id → latest change event timestamp
	gapWindow       time.Duration                // how long to wait before declaring a gap
	sweepInterval   time.Duration                // how often to sweep for gaps
	stateEventTypes []string                     // event types considered "state observations"
}

// stateObservation records a state change that hasn't been matched to a
// change event yet.
type stateObservation struct {
	EntityID    string
	EntityType  string
	ProjectID   string
	Environment string
	ObservedAt  time.Time
	Details     map[string]any
}

// GapOption configures the GapDetector.
type GapOption func(*GapDetector)

// WithGapWindow sets the duration to wait before declaring a gap.
// Default: 5 minutes.
func WithGapWindow(d time.Duration) GapOption {
	return func(g *GapDetector) {
		g.gapWindow = d
	}
}

// WithSweepInterval sets how often the detector checks for gaps.
// Default: 1 minute.
func WithSweepInterval(d time.Duration) GapOption {
	return func(g *GapDetector) {
		g.sweepInterval = d
	}
}

// WithStateEventTypes sets which event types are treated as state observations.
// Default: ticket lifecycle events that imply external state changes.
func WithStateEventTypes(types ...string) GapOption {
	return func(g *GapDetector) {
		g.stateEventTypes = types
	}
}

// NewGapDetector creates a gap detector. It subscribes to both state
// observations and change events on the bus. Call Start to begin the
// background sweep loop.
func NewGapDetector(bus events.Bus, client *Client, opts ...GapOption) *GapDetector {
	g := &GapDetector{
		bus:           bus,
		client:        client,
		stateChanges:  make(map[string]*stateObservation),
		recentChanges: make(map[string]time.Time),
		gapWindow:     5 * time.Minute,
		sweepInterval: 1 * time.Minute,
		stateEventTypes: []string{
			// Default: ticket state transitions that imply real-world changes.
			events.EventTicketDeploying,
			events.EventTicketValidated,
			events.EventTicketClosed,
		},
	}
	for _, opt := range opts {
		opt(g)
	}

	// Subscribe to change events — these "explain" state changes.
	bus.Subscribe(events.EventChangePublished, g.handleChangeEvent)

	// Subscribe to state observation events — these are what need explaining.
	for _, eventType := range g.stateEventTypes {
		bus.Subscribe(eventType, g.handleStateObservation)
	}

	return g
}

// handleChangeEvent records that a change event was published for an entity,
// and removes any pending unmatched state observation for that entity.
func (g *GapDetector) handleChangeEvent(_ context.Context, ev events.Event) {
	entityID, _ := ev.Payload["entity_id"].(string)
	if entityID == "" {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	g.recentChanges[entityID] = time.Now().UTC()
	// This change explains any pending state observation.
	delete(g.stateChanges, entityID)
}

// handleStateObservation records that a state change was observed for an entity.
// If no matching change event arrives within the gap window, the sweep will
// generate an unattributed event.
func (g *GapDetector) handleStateObservation(_ context.Context, ev events.Event) {
	entityID, _ := ev.Payload["ticket_id"].(string)
	if entityID == "" {
		entityID, _ = ev.Payload["entity_id"].(string)
	}
	if entityID == "" {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// If we recently saw a change event for this entity, this state change
	// is already explained. Allow a small grace period.
	if lastChange, ok := g.recentChanges[entityID]; ok {
		if time.Since(lastChange) < g.gapWindow {
			return
		}
	}

	g.stateChanges[entityID] = &stateObservation{
		EntityID:   entityID,
		ObservedAt: time.Now().UTC(),
		Details: map[string]any{
			"event_type": ev.Type,
			"payload":    ev.Payload,
		},
	}
}

// Start begins the background gap detection sweep loop. It blocks until the
// context is cancelled.
func (g *GapDetector) Start(ctx context.Context) {
	ticker := time.NewTicker(g.sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.sweep(ctx)
		}
	}
}

// sweep checks for unmatched state observations whose gap window has expired,
// and generates unattributed change events for them.
func (g *GapDetector) sweep(ctx context.Context) {
	g.mu.Lock()
	now := time.Now().UTC()
	var gaps []*stateObservation
	for entityID, obs := range g.stateChanges {
		if now.Sub(obs.ObservedAt) >= g.gapWindow {
			gaps = append(gaps, obs)
			delete(g.stateChanges, entityID)
		}
	}

	// Prune old recentChanges entries (older than 2x gap window).
	cutoff := now.Add(-2 * g.gapWindow)
	for entityID, t := range g.recentChanges {
		if t.Before(cutoff) {
			delete(g.recentChanges, entityID)
		}
	}
	g.mu.Unlock()

	// Generate unattributed events outside the lock.
	for _, obs := range gaps {
		g.emitUnattributed(ctx, obs)
	}
}

// emitUnattributed publishes two events: a gap detection notification and an
// auto-generated unattributed change event.
func (g *GapDetector) emitUnattributed(ctx context.Context, obs *stateObservation) {
	// Emit gap detection event.
	_ = g.bus.Publish(ctx, events.Event{
		Type: events.EventStateGapDetected,
		Payload: map[string]any{
			"entity_id":   obs.EntityID,
			"observed_at": obs.ObservedAt.Format(time.RFC3339),
			"details":     obs.Details,
		},
	})

	// Auto-generate an unattributed change event.
	_ = g.bus.Publish(ctx, events.Event{
		Type: events.EventChangeUnattributed,
		Payload: map[string]any{
			"id":          uuid.New().String(),
			"change_type": "unattributed",
			"entity_id":   obs.EntityID,
			"initiator":   "gap-detector",
			"timestamp":   obs.ObservedAt.Format(time.RFC3339Nano),
			"metadata": map[string]any{
				"auto_generated": true,
				"gap_detected":   true,
				"source_event":   obs.Details,
			},
		},
	})

	log.Printf("hooks/gap: unattributed change event generated for entity %s (state change at %s)",
		obs.EntityID, obs.ObservedAt.Format(time.RFC3339))
}

// PendingGaps returns the number of state observations currently awaiting
// matching change events. Useful for metrics and testing.
func (g *GapDetector) PendingGaps() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.stateChanges)
}

// Sweep triggers a manual gap sweep. Useful for testing without waiting
// for the ticker.
func (g *GapDetector) Sweep(ctx context.Context) {
	g.sweep(ctx)
}
