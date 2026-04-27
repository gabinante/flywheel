package claims

import (
	"context"
	"log/slog"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/plan"
	"github.com/gabinante/flywheel/internal/ticket"
)

// PlanLister looks up plans for a given ticket (implemented by plan.Service).
type PlanLister interface {
	ListPlansByTicket(ctx context.Context, ticketID string) ([]*plan.Plan, error)
}

// TicketGetter retrieves a ticket by ID (implemented by ticket.Service).
type TicketGetter interface {
	GetTicket(ctx context.Context, id string) (*ticket.Ticket, error)
}

// LifecycleHandler subscribes to ticket lifecycle events and manages claim
// registration and release automatically. It bridges the ticket state machine
// with the claims registry:
//   - On ticket.started → register claims from the ticket's plan(s)
//   - On ticket.closed, ticket.cancelled, lease.expired, ticket.failed → release claims
type LifecycleHandler struct {
	claimsSvc *Service
	plans     PlanLister
	tickets   TicketGetter
}

// NewLifecycleHandler creates a lifecycle handler and subscribes to the event bus.
func NewLifecycleHandler(bus events.Bus, claimsSvc *Service, plans PlanLister, tickets TicketGetter) *LifecycleHandler {
	h := &LifecycleHandler{
		claimsSvc: claimsSvc,
		plans:     plans,
		tickets:   tickets,
	}

	// Register claims when a ticket enters execution.
	bus.Subscribe(events.EventTicketStarted, h.onTicketStarted)

	// Release claims when a ticket leaves execution (any terminal path).
	bus.Subscribe(events.EventTicketClosed, h.onTicketCompleted)
	bus.Subscribe(events.EventTicketCancelled, h.onTicketCompleted)
	bus.Subscribe(events.EventLeaseExpired, h.onTicketCompleted)
	bus.Subscribe(events.EventTicketFailed, h.onTicketCompleted)

	return h
}

// onTicketStarted handles the ticket.started event by extracting touches from
// the ticket's plans and registering them as claims.
func (h *LifecycleHandler) onTicketStarted(ctx context.Context, event events.Event) {
	ticketID, ok := event.Payload["ticket_id"].(string)
	if !ok || ticketID == "" {
		return
	}

	// Get the ticket to determine environment scope.
	t, err := h.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		slog.Error("claims/lifecycle: failed to get ticket", "ticket", ticketID, "error", err)
		return
	}

	// Environment comes from the ticket's environment field.
	environment := string(t.Environment)

	// Look up all plans for this ticket and extract touches.
	plans, err := h.plans.ListPlansByTicket(ctx, ticketID)
	if err != nil {
		slog.Error("claims/lifecycle: failed to list plans", "ticket", ticketID, "error", err)
		return
	}

	var allTouches []Touch
	for _, p := range plans {
		// Only consider plans in active states (approved or applied, not rejected/superseded).
		if p.State == plan.StateRejected || p.State == plan.StateSuperseded {
			continue
		}
		touches := ExtractTouches(p, environment)
		allTouches = append(allTouches, touches...)
	}

	if len(allTouches) == 0 {
		// No touches declared — nothing to claim. This is normal for tickets
		// without plans or with plans that don't declare resource touches.
		return
	}

	// Run conflict detection and record conflicts before registering.
	result, err := h.claimsSvc.DetectAndRecordConflicts(ctx, ticketID, allTouches)
	if err != nil {
		slog.Error("claims/lifecycle: conflict detection failed", "ticket", ticketID, "error", err)
		// Continue with registration even if detection fails — the claim itself is still valid.
	} else if result.HardCount > 0 {
		slog.Warn("claims/lifecycle: hard conflicts detected", "ticket", ticketID, "hard_count", result.HardCount)
	}

	// Register the claims.
	registered, err := h.claimsSvc.RegisterClaims(ctx, ticketID, allTouches)
	if err != nil {
		slog.Error("claims/lifecycle: failed to register claims", "ticket", ticketID, "error", err)
		return
	}

	slog.Info("claims/lifecycle: registered claims", "count", len(registered), "ticket", ticketID)
}

// onTicketCompleted handles events that indicate a ticket has left execution
// (closed, cancelled, lease expired, failed). Releases all active claims.
func (h *LifecycleHandler) onTicketCompleted(ctx context.Context, event events.Event) {
	ticketID, ok := event.Payload["ticket_id"].(string)
	if !ok || ticketID == "" {
		return
	}

	count, err := h.claimsSvc.ReleaseClaims(ctx, ticketID)
	if err != nil {
		slog.Error("claims/lifecycle: failed to release claims", "ticket", ticketID, "error", err)
		return
	}

	if count > 0 {
		slog.Info("claims/lifecycle: released claims", "count", count, "ticket", ticketID)
	}
}
