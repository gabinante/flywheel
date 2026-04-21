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

	// --- Catalog (Layer 14) events ---
	EventCatalogEntityCreated = "catalog.entity_created"
	EventCatalogEntityUpdated = "catalog.entity_updated"
	EventCatalogEntityDeleted = "catalog.entity_deleted"
	EventCatalogEdgeCreated   = "catalog.edge_created"
	EventCatalogEdgeDeleted   = "catalog.edge_deleted"
	EventCatalogScanCompleted = "catalog.scan_completed"

	// --- Plan lifecycle events ---
	EventPlanCreated    = "plan.created"
	EventPlanSubmitted  = "plan.submitted"
	EventPlanClassified = "plan.classified"
	EventPlanApproved   = "plan.approved"
	EventPlanApplied    = "plan.applied"
	EventPlanRejected   = "plan.rejected"
	EventPlanSuperseded = "plan.superseded"

	// --- Claims registry events (spec v0.2 §4.3) ---
	EventClaimRegistered  = "claim.registered"
	EventClaimReleased    = "claim.released"
	EventConflictDetected = "claim.conflict_detected"
	EventConflictResolved = "claim.conflict_resolved"

	// --- Change stream events (spec v0.2 §2.4) ---
	EventChangePublished    = "change.published"    // change event published via hooks
	EventChangeUnattributed = "change.unattributed" // auto-generated for unexplained state changes
	EventStateGapDetected   = "change.gap_detected" // state change without matching change event

	// --- Plan freshness events (warrant-45) ---
	EventPlanFreshnessStale  = "plan.freshness_stale"  // freshness check detected stale stamp
	EventPlanRePlanTriggered = "plan.replan_triggered"  // re-plan was triggered before apply
	EventPlanRePlanIdentical = "plan.replan_identical"  // re-plan produced identical content (auto-proceed)
	EventPlanRePlanDiverged  = "plan.replan_diverged"   // re-plan produced different content (route to review)

	// --- Notification events ---
	EventNotificationCreated    = "notification.created"     // notification queued or pushed
	EventNotificationSent       = "notification.sent"        // notification delivered successfully
	EventNotificationFailed     = "notification.failed"      // notification delivery failed
	EventNotificationDismissed  = "notification.dismissed"   // operator dismissed notification
	EventNotificationDigestSent = "notification.digest_sent" // digest batch delivered
)

// Event carries type and typed payload for the bus.
type Event struct {
	Type    string
	Payload map[string]any
}
