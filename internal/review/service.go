package review

import (
	"context"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/ticket"
)

// TicketService is the subset of ticket.Service needed for review.
type TicketService interface {
	TransitionTicket(ctx context.Context, id string, trigger string, actor ticket.Actor, payload map[string]any) error
	AppendRejectionNotes(ctx context.Context, ticketID, reviewerID, notes string) error
	InjectEscalationAnswer(ctx context.Context, ticketID, answer string) error
	GetTicket(ctx context.Context, id string) (*ticket.Ticket, error)
}

// Service provides review and escalation operations.
type Service struct {
	store     *Store
	ticketSvc TicketService
	bus       events.Bus
}

// NewService returns a new Service and subscribes to ticket.escalated to record escalations.
func NewService(store *Store, ticketSvc TicketService, bus events.Bus) *Service {
	svc := &Service{store: store, ticketSvc: ticketSvc, bus: bus}
	bus.Subscribe(events.EventTicketEscalated, func(ctx context.Context, ev events.Event) {
		ticketID, _ := ev.Payload["ticket_id"].(string)
		agentID, _ := ev.Payload["agent_id"].(string)
		reason, _ := ev.Payload["reason"].(string)
		question, _ := ev.Payload["question"].(string)
		if ticketID == "" || agentID == "" {
			return
		}
		_ = store.CreateEscalation(ctx, &Escalation{
			TicketID:  ticketID,
			AgentID:   agentID,
			Reason:    reason,
			Question:  question,
			CreatedAt: time.Now().UTC(),
		})
	})
	return svc
}

// ApproveTicket transitions the ticket to done and records the review.
func (s *Service) ApproveTicket(ctx context.Context, ticketID, reviewerID, notes string) error {
	actor := ticket.Actor{ID: reviewerID, Type: ticket.ActorHuman}
	if err := s.ticketSvc.TransitionTicket(ctx, ticketID, ticket.TriggerApprove, actor, nil); err != nil {
		return err
	}
	return s.store.CreateReview(ctx, &Review{
		TicketID:   ticketID,
		ReviewerID: reviewerID,
		Decision:   DecisionApproved,
		Notes:      notes,
		CreatedAt:  time.Now().UTC(),
	})
}

// ReopenTicketForReview transitions done → awaiting_review and records the review.
// Use when a ticket was approved by mistake or needs another review pass; outputs are preserved.
func (s *Service) ReopenTicketForReview(ctx context.Context, ticketID, reviewerID, notes string) error {
	actor := ticket.Actor{ID: reviewerID, Type: ticket.ActorHuman}
	if err := s.ticketSvc.TransitionTicket(ctx, ticketID, ticket.TriggerReopenReview, actor, nil); err != nil {
		return err
	}
	return s.store.CreateReview(ctx, &Review{
		TicketID:   ticketID,
		ReviewerID: reviewerID,
		Decision:   DecisionReopened,
		Notes:      notes,
		CreatedAt:  time.Now().UTC(),
	})
}

// RejectTicket transitions back to executing, appends notes to context, and records the review.
func (s *Service) RejectTicket(ctx context.Context, ticketID, reviewerID, notes string) error {
	actor := ticket.Actor{ID: reviewerID, Type: ticket.ActorHuman}
	if err := s.ticketSvc.TransitionTicket(ctx, ticketID, ticket.TriggerReject, actor, nil); err != nil {
		return err
	}
	if err := s.ticketSvc.AppendRejectionNotes(ctx, ticketID, reviewerID, notes); err != nil {
		return err
	}
	return s.store.CreateReview(ctx, &Review{
		TicketID:   ticketID,
		ReviewerID: reviewerID,
		Decision:   DecisionRejected,
		Notes:      notes,
		CreatedAt:  time.Now().UTC(),
	})
}

// ResolveEscalation transitions needs_human -> executing, injects answer, and marks escalation resolved.
func (s *Service) ResolveEscalation(ctx context.Context, ticketID, escalationID, reviewerID, answer string) error {
	actor := ticket.Actor{ID: reviewerID, Type: ticket.ActorHuman}
	if err := s.ticketSvc.TransitionTicket(ctx, ticketID, ticket.TriggerApprove, actor, nil); err != nil {
		return err
	}
	if err := s.ticketSvc.InjectEscalationAnswer(ctx, ticketID, answer); err != nil {
		return err
	}
	return s.store.UpdateEscalationResolved(ctx, escalationID, answer, reviewerID)
}

// ListPendingReviews returns ticket IDs in awaiting_review for the project. Caller can fetch full tickets.
func (s *Service) ListPendingReviews(ctx context.Context, projectID string) ([]string, error) {
	return s.store.ListPendingReviewTicketIDs(ctx, projectID)
}

// ListEscalations returns unresolved escalations for the project.
func (s *Service) ListEscalations(ctx context.Context, projectID string) ([]Escalation, error) {
	return s.store.ListEscalationsByProject(ctx, projectID)
}

// GetReviewCountsByReviewer returns how many reviews (approved and rejected) the reviewer has made (lifetime).
func (s *Service) GetReviewCountsByReviewer(ctx context.Context, reviewerID string) (approved, rejected int, err error) {
	return s.store.CountByReviewer(ctx, reviewerID)
}

// GetReviewCountsByReviewerPerDay returns daily approved and rejected counts for the last days (oldest first).
func (s *Service) GetReviewCountsByReviewerPerDay(ctx context.Context, reviewerID string, days int) (approved, rejected []int, err error) {
	return s.store.CountByReviewerPerDay(ctx, reviewerID, days)
}
