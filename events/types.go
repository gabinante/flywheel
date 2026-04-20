package events

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

	// --- Policy events ---
	EventPolicyChanged   = "policy.changed"   // policy set created, activated, deactivated, updated, or deleted
	EventPolicyEvaluated = "policy.evaluated" // policy was evaluated for a transition (audit trail)

	// --- Deprecated aliases (backward compat for existing subscribers) ---
	EventTicketDone      = EventTicketClosed // alias: done → closed
	EventTicketUnblocked = "ticket.unblocked"
)

// Event carries type and typed payload for the bus.
type Event struct {
	Type    string
	Payload map[string]any
}
