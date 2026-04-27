package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/ticket"
)

// CoordinatorFeedbackSubscriber listens to ticket lifecycle events and
// automatically creates coordinator-feedback findings when tickets are
// rejected, fail, get replanned, or invalidated. This provides the
// coordinator with cross-session learning data.
//
// The subscriber only generates findings when a FindingsProvider is
// configured; otherwise it logs but does nothing.
type CoordinatorFeedbackSubscriber struct {
	findings FindingsProvider
	tickets  *ticket.Service
}

// NewCoordinatorFeedbackSubscriber creates a subscriber and registers it
// with the event bus. It subscribes to rejection, failure, replan, and
// invalidation events.
func NewCoordinatorFeedbackSubscriber(bus events.Bus, findings FindingsProvider, tickets *ticket.Service) *CoordinatorFeedbackSubscriber {
	s := &CoordinatorFeedbackSubscriber{
		findings: findings,
		tickets:  tickets,
	}

	if bus == nil || findings == nil {
		return s // graceful no-op when dependencies aren't configured
	}

	// Subscribe to failure-arc events that indicate coordinator ticket quality issues.
	bus.Subscribe(events.EventTicketRejected, s.onTicketRejected)
	bus.Subscribe(events.EventTicketFailed, s.onTicketFailed)
	bus.Subscribe(events.EventTicketReplanned, s.onTicketReplanned)
	bus.Subscribe(events.EventTicketInvalidated, s.onTicketInvalidated)

	return s
}

// onTicketRejected fires when a ticket is rejected during review.
// This often indicates acceptance criteria ambiguity or scope issues.
func (s *CoordinatorFeedbackSubscriber) onTicketRejected(ctx context.Context, ev events.Event) {
	ticketID := eventStr(ev, "ticket_id")
	projectID := eventStr(ev, "project_id")
	if ticketID == "" || projectID == "" {
		return
	}

	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		slog.Error("coordinator-feedback: failed to get ticket", "ticket", ticketID, "error", err)
		return
	}

	// Extract rejection notes from prior attempts.
	rejectionReason := "Ticket was rejected during review"
	if len(t.Context.PriorAttempts) > 0 {
		latest := t.Context.PriorAttempts[len(t.Context.PriorAttempts)-1]
		if latest.Summary != "" {
			rejectionReason = latest.Summary
		}
	}

	finding := Finding{
		ProjectID:      projectID,
		Claim:          fmt.Sprintf("Ticket %s (%s) was rejected: %s", ticketID, t.Title, rejectionReason),
		FindingType:    "investigation",
		Confidence:     0.85,
		SourceType:     "review_comment",
		SourceTicketID: ticketID,
		TicketRefs:     []string{ticketID},
		Tags:           []string{"coordinator-feedback", "rejection", "author_role:coordinator", "automated"},
		Summary:        fmt.Sprintf("[rejection] %s — %s", t.Title, rejectionReason),
	}

	if _, err := s.findings.SaveFinding(ctx, finding); err != nil {
		slog.Error("coordinator-feedback: failed to save rejection finding", "ticket", ticketID, "error", err)
	}
}

// onTicketFailed fires when a ticket fails during execution (unrecoverable).
func (s *CoordinatorFeedbackSubscriber) onTicketFailed(ctx context.Context, ev events.Event) {
	ticketID := eventStr(ev, "ticket_id")
	projectID := eventStr(ev, "project_id")
	if ticketID == "" || projectID == "" {
		return
	}

	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		slog.Error("coordinator-feedback: failed to get ticket", "ticket", ticketID, "error", err)
		return
	}

	failureReason := "Ticket execution failed"
	if len(t.Context.PriorAttempts) > 0 {
		latest := t.Context.PriorAttempts[len(t.Context.PriorAttempts)-1]
		if latest.Summary != "" {
			failureReason = latest.Summary
		}
	}

	finding := Finding{
		ProjectID:      projectID,
		Claim:          fmt.Sprintf("Ticket %s (%s) failed during execution: %s", ticketID, t.Title, failureReason),
		FindingType:    "investigation",
		Confidence:     0.9,
		SourceType:     "agent_analysis",
		SourceTicketID: ticketID,
		TicketRefs:     []string{ticketID},
		Tags:           []string{"coordinator-feedback", "failure", "author_role:coordinator", "automated"},
		Summary:        fmt.Sprintf("[failure] %s — %s", t.Title, failureReason),
	}

	if _, err := s.findings.SaveFinding(ctx, finding); err != nil {
		slog.Error("coordinator-feedback: failed to save failure finding", "ticket", ticketID, "error", err)
	}
}

// onTicketReplanned fires when a ticket goes back from executing to planning.
// This indicates the original spec was insufficient or the approach was wrong.
func (s *CoordinatorFeedbackSubscriber) onTicketReplanned(ctx context.Context, ev events.Event) {
	ticketID := eventStr(ev, "ticket_id")
	projectID := eventStr(ev, "project_id")
	if ticketID == "" || projectID == "" {
		return
	}

	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		slog.Error("coordinator-feedback: failed to get ticket", "ticket", ticketID, "error", err)
		return
	}

	finding := Finding{
		ProjectID:      projectID,
		Claim:          fmt.Sprintf("Ticket %s (%s) required replanning during execution — original decomposition may have been insufficient", ticketID, t.Title),
		FindingType:    "investigation",
		Confidence:     0.8,
		SourceType:     "agent_analysis",
		SourceTicketID: ticketID,
		TicketRefs:     []string{ticketID},
		Tags:           []string{"coordinator-feedback", "replan", "wrong_decomposition", "author_role:coordinator", "automated"},
		Summary:        fmt.Sprintf("[replan] %s — execution revealed decomposition issues", t.Title),
	}

	if _, err := s.findings.SaveFinding(ctx, finding); err != nil {
		slog.Error("coordinator-feedback: failed to save replan finding", "ticket", ticketID, "error", err)
	}
}

// onTicketInvalidated fires when a validated ticket is sent back to planning.
// This means the validation was wrong or external conditions changed.
func (s *CoordinatorFeedbackSubscriber) onTicketInvalidated(ctx context.Context, ev events.Event) {
	ticketID := eventStr(ev, "ticket_id")
	projectID := eventStr(ev, "project_id")
	if ticketID == "" || projectID == "" {
		return
	}

	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		slog.Error("coordinator-feedback: failed to get ticket", "ticket", ticketID, "error", err)
		return
	}

	finding := Finding{
		ProjectID:      projectID,
		Claim:          fmt.Sprintf("Ticket %s (%s) was invalidated after validation — acceptance criteria may have been misleading or conditions changed", ticketID, t.Title),
		FindingType:    "investigation",
		Confidence:     0.75,
		SourceType:     "review_comment",
		SourceTicketID: ticketID,
		TicketRefs:     []string{ticketID},
		Tags:           []string{"coordinator-feedback", "invalidation", "acceptance_ambiguity", "author_role:coordinator", "automated"},
		Summary:        fmt.Sprintf("[invalidation] %s — post-validation revert", t.Title),
	}

	if _, err := s.findings.SaveFinding(ctx, finding); err != nil {
		slog.Error("coordinator-feedback: failed to save invalidation finding", "ticket", ticketID, "error", err)
	}
}

// eventStr safely extracts a string from an event payload.
func eventStr(ev events.Event, key string) string {
	if v, ok := ev.Payload[key].(string); ok {
		return v
	}
	return ""
}
