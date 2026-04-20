package events

import "time"

// Event type constants for the ticket lifecycle (spec v0.2).
// Each state transition emits a typed event on the bus.
const (
	// --- Lifecycle events (happy path) ---
	EventTicketCreated   = "ticket.created"
	EventTicketSpecced   = "ticket.specced"   // draft → specced
	EventTicketPlanning  = "ticket.planning"  // specced/claim → planning
	EventTicketStarted   = "ticket.started"   // planning → executing
	EventTicketSubmitted = "ticket.submitted" // executing → awaiting_validation
	EventTicketValidated = "ticket.validated" // awaiting_validation → validated
	EventTicketDeploying = "ticket.deploying" // validated → deploying
	EventTicketObserving = "ticket.observing" // deploying → observing
	EventTicketClosed    = "ticket.closed"    // observing → closed (or cancel)

	// --- Input / escalation events ---
	EventTicketAwaitingInput = "ticket.awaiting_input" // → awaiting_input
	EventTicketInputProvided = "ticket.input_provided" // awaiting_input → planning/executing
	EventTicketEscalated     = "ticket.escalated"      // executing → awaiting_input (escalation)

	// --- Failure arc events ---
	EventTicketReplanned   = "ticket.replanned"   // executing → planning
	EventTicketInvalidated = "ticket.invalidated" // validated → planning

	// --- Review events ---
	EventTicketApproved = "ticket.approved" // awaiting_validation → validated
	EventTicketRejected = "ticket.rejected" // awaiting_validation → executing

	// --- Operational events ---
	EventTicketClaimed   = "ticket.claimed"   // draft → planning (via claim)
	EventTicketFailed    = "ticket.failed"    // executing → draft
	EventTicketCancelled = "ticket.cancelled" // → closed (via cancel)
	EventTicketReopened  = "ticket.reopened"  // closed → draft
	EventLeaseExpired    = "lease.expired"    // planning/executing → draft

	// --- Deprecated aliases (backward compat for existing subscribers) ---
	EventTicketDone      = EventTicketClosed // alias: done → closed
	EventTicketUnblocked = "ticket.unblocked"
)

// Event carries type and typed payload for the bus.
// ID and EntityKey are set by durable implementations; in-process bus leaves them empty.
type Event struct {
	// ID is a unique identifier assigned by the durable store (empty for in-process).
	ID string

	// Type is the event type (e.g., "ticket.created").
	Type string

	// EntityKey groups events for ordered delivery. Events with the same entity key
	// are delivered in order. Typically "ticket:<id>" or "project:<id>".
	EntityKey string

	// Payload carries event-specific data.
	Payload map[string]any

	// Timestamp is when the event was created (set by store for durable, time.Now for in-process).
	Timestamp time.Time
}

// NewEvent creates an event with the given type and payload.
// EntityKey defaults to empty (no ordering guarantee).
func NewEvent(eventType string, payload map[string]any) Event {
	return Event{
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}

// WithEntityKey returns a copy of the event with the given entity key set.
func (e Event) WithEntityKey(key string) Event {
	e.EntityKey = key
	return e
}
