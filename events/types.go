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

	// --- Rollback events ---
	EventTicketRolledBack = "ticket.rolled_back"       // stage-specific rollback → draft
	EventRollbackIncident = "ticket.rollback_incident"  // incident ticket auto-created on prod rollback

	// --- Review events ---
	EventTicketApproved = "ticket.approved" // awaiting_validation → validated
	EventTicketRejected = "ticket.rejected" // awaiting_validation → executing

	// --- Operational events ---
	EventTicketClaimed   = "ticket.claimed"   // draft → planning (via claim)
	EventTicketFailed    = "ticket.failed"    // executing → draft
	EventTicketCancelled = "ticket.cancelled" // → closed (via cancel)
	EventTicketReopened  = "ticket.reopened"  // closed → draft
	EventLeaseExpired    = "lease.expired"    // planning/executing → draft

	// --- CI/merge events ---
	EventTestsFailed = "ticket.tests_failed" // CI checks failed on PR before merge
	EventTestsPassed = "ticket.tests_passed" // CI checks passed on PR

	// --- Deprecated aliases (backward compat for existing subscribers) ---
	EventTicketDone      = EventTicketClosed // alias: done → closed
	EventTicketUnblocked = "ticket.unblocked"

	// --- Cost & budget events ---
	EventBudgetAlert       = "cost.budget_alert"       // budget threshold crossed
	EventBudgetExceeded    = "cost.budget_exceeded"    // spend exceeds limit
	EventRateLimitHit      = "cost.rate_limit_hit"     // provider rate-limited
	EventRateLimitResumed  = "cost.rate_limit_resumed" // rate limit cleared, resuming
	EventCostCallRecorded  = "cost.call_recorded"      // LLM call cost attributed

	// --- Work stream events ---
	EventWorkStreamCompleted = "work_stream.completed" // all tickets in stream are closed

	// --- Plan lifecycle events ---
	EventPlanCreated    = "plan.created"
	EventPlanSubmitted  = "plan.submitted"
	EventPlanClassified = "plan.classified"
	EventPlanApproved   = "plan.approved"
	EventPlanApplied    = "plan.applied"
	EventPlanRejected   = "plan.rejected"
	EventPlanSuperseded = "plan.superseded"
)

// Event carries type and typed payload for the bus.
type Event struct {
	Type    string
	Payload map[string]any
}
