package progress

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/ticket"
)

// OrchestratorInjector injects system events into the command center thread.
type OrchestratorInjector interface {
	InjectSystemEvent(ctx context.Context, projectID, category, summary string) error
}

// TicketGetter retrieves tickets for threshold checks.
type TicketGetter interface {
	GetTicket(ctx context.Context, id string) (*ticket.Ticket, error)
}

// Monitor subscribes to ticket lifecycle events and bridges them to the
// command center via orchestrator system messages. It applies threshold
// logic so the command center only sees actionable events.
type Monitor struct {
	bus          events.Bus
	orchestrator OrchestratorInjector
	tickets      TicketGetter
}

// NewMonitor creates and starts a progress monitor.
func NewMonitor(bus events.Bus, orchestrator OrchestratorInjector, tickets TicketGetter) *Monitor {
	m := &Monitor{
		bus:          bus,
		orchestrator: orchestrator,
		tickets:      tickets,
	}
	m.subscribe()
	return m
}

func (m *Monitor) subscribe() {
	m.bus.Subscribe(events.EventTicketEscalated, m.handleEscalated)
	m.bus.Subscribe(events.EventTicketFailed, m.handleFailed)
	m.bus.Subscribe(events.EventLeaseExpired, m.handleLeaseExpired)
	m.bus.Subscribe(events.EventTicketInvalidated, m.handleInvalidated)
	m.bus.Subscribe(events.EventTicketReplanned, m.handleReplanned)
	m.bus.Subscribe(events.EventWorkflowGateReached, m.handleGateReached)
	m.bus.Subscribe(events.EventWorkStreamCompleted, m.handleWorkStreamCompleted)
	m.bus.Subscribe(events.EventTicketMerged, m.handleTicketMerged)
}

func (m *Monitor) inject(ctx context.Context, projectID, category, summary string) {
	if err := m.orchestrator.InjectSystemEvent(ctx, projectID, category, summary); err != nil {
		slog.Error("progress: failed to inject system event", "category", category, "error", err)
	}
}

func (m *Monitor) priorAttemptCount(ctx context.Context, ticketID string) int {
	if m.tickets == nil || ticketID == "" {
		return 0
	}
	t, err := m.tickets.GetTicket(ctx, ticketID)
	if err != nil || t == nil {
		return 0
	}
	return len(t.Context.PriorAttempts)
}

func (m *Monitor) handleEscalated(ctx context.Context, event events.Event) {
	ticketID, _ := event.Payload["ticket_id"].(string)
	projectID, _ := event.Payload["project_id"].(string)
	reason, _ := event.Payload["reason"].(string)
	if projectID == "" || ticketID == "" {
		return
	}
	summary := fmt.Sprintf("%s: agent needs human decision", ticketID)
	if reason != "" {
		summary += " — " + reason
	}
	m.inject(ctx, projectID, "ESCALATION", summary)
}

func (m *Monitor) handleFailed(ctx context.Context, event events.Event) {
	ticketID, _ := event.Payload["ticket_id"].(string)
	projectID, _ := event.Payload["project_id"].(string)
	if projectID == "" || ticketID == "" {
		return
	}
	attempts := m.priorAttemptCount(ctx, ticketID)
	if attempts < 2 {
		return
	}
	m.inject(ctx, projectID, "FAILURE",
		fmt.Sprintf("%s: failed %d times, may need respec or human intervention", ticketID, attempts))
}

func (m *Monitor) handleLeaseExpired(ctx context.Context, event events.Event) {
	ticketID, _ := event.Payload["ticket_id"].(string)
	projectID, _ := event.Payload["project_id"].(string)
	if projectID == "" || ticketID == "" {
		return
	}
	attempts := m.priorAttemptCount(ctx, ticketID)
	if attempts < 2 {
		return
	}
	m.inject(ctx, projectID, "STALE",
		fmt.Sprintf("%s: worker crashed %d times", ticketID, attempts))
}

func (m *Monitor) handleInvalidated(ctx context.Context, event events.Event) {
	ticketID, _ := event.Payload["ticket_id"].(string)
	projectID, _ := event.Payload["project_id"].(string)
	if projectID == "" || ticketID == "" {
		return
	}
	m.inject(ctx, projectID, "INVALIDATED",
		fmt.Sprintf("%s: sent back from validated to planning", ticketID))
}

func (m *Monitor) handleReplanned(ctx context.Context, event events.Event) {
	ticketID, _ := event.Payload["ticket_id"].(string)
	projectID, _ := event.Payload["project_id"].(string)
	if projectID == "" || ticketID == "" {
		return
	}
	attempts := m.priorAttemptCount(ctx, ticketID)
	if attempts < 2 {
		return
	}
	m.inject(ctx, projectID, "REPLAN",
		fmt.Sprintf("%s: replanned %d times", ticketID, attempts))
}

func (m *Monitor) handleGateReached(ctx context.Context, event events.Event) {
	ticketID, _ := event.Payload["ticket_id"].(string)
	projectID, _ := event.Payload["project_id"].(string)
	phaseName, _ := event.Payload["phase_name"].(string)
	if projectID == "" || ticketID == "" {
		return
	}
	m.inject(ctx, projectID, "GATE",
		fmt.Sprintf("%s: workflow phase '%s' requires approval", ticketID, phaseName))
}

func (m *Monitor) handleTicketMerged(ctx context.Context, event events.Event) {
	ticketID, _ := event.Payload["ticket_id"].(string)
	projectID, _ := event.Payload["project_id"].(string)
	prURL, _ := event.Payload["pr_url"].(string)
	if projectID == "" || ticketID == "" {
		return
	}
	summary := fmt.Sprintf("%s: PR merged", ticketID)
	if prURL != "" {
		summary += " — " + prURL
	}
	m.inject(ctx, projectID, "MERGED", summary)
}

func (m *Monitor) handleWorkStreamCompleted(ctx context.Context, event events.Event) {
	projectID, _ := event.Payload["project_id"].(string)
	ticketCount, _ := event.Payload["ticket_count"].(float64) // JSON numbers are float64
	streamID, _ := event.Payload["work_stream_id"].(string)
	if projectID == "" {
		return
	}
	summary := "Work stream done"
	if streamID != "" {
		summary = fmt.Sprintf("Work stream %s done", streamID)
	}
	if ticketCount > 0 {
		summary += fmt.Sprintf(": %d tickets closed", int(ticketCount))
	}
	m.inject(ctx, projectID, "COMPLETE", summary)
}
