package rest

import (
	"context"
	"encoding/json"
	"net/http"

	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/ticket"
)

// ReviewServiceForHandler is the review operations needed by ReviewsHandler. *review.Service implements it.
type ReviewServiceForHandler interface {
	ListPendingReviews(ctx context.Context, projectID string) ([]string, error)
	ApproveTicket(ctx context.Context, ticketID, reviewerID, notes string) error
	RejectTicket(ctx context.Context, ticketID, reviewerID, notes string) error
	ReopenTicketForReview(ctx context.Context, ticketID, reviewerID, notes string) error
	ListEscalations(ctx context.Context, projectID string) ([]review.Escalation, error)
	ResolveEscalation(ctx context.Context, ticketID, escalationID, reviewerID, answer string) error
}

// ReviewsHandler handles review and escalation REST endpoints.
type ReviewsHandler struct {
	ReviewSvc  ReviewServiceForHandler
	TicketSvc  TicketGetter
	ProjectSvc ProjectGetterForAccess
	OrgSvc     OrgMemberLister
	AgentStore AgentGetter
}

func (h *ReviewsHandler) listPendingReviews(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	ids, err := h.ReviewSvc.ListPendingReviews(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	tickets := make([]*ticket.Ticket, 0, len(ids))
	for _, id := range ids {
		t, _ := h.TicketSvc.GetTicket(r.Context(), id)
		if t != nil {
			tickets = append(tickets, t)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"tickets": tickets})
}

func (h *ReviewsHandler) createReview(w http.ResponseWriter, r *http.Request) {
	ticketID := PathParam(r, "ticketID")
	t, err := h.TicketSvc.GetTicket(r.Context(), ticketID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, t.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	var body struct {
		Decision   review.Decision `json:"decision"`
		Notes      string          `json:"notes"`
		ReviewerID string          `json:"reviewer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if body.ReviewerID == "" {
		body.ReviewerID = "api"
	}
	switch body.Decision {
	case review.DecisionApproved:
		if err := h.ReviewSvc.ApproveTicket(r.Context(), ticketID, body.ReviewerID, body.Notes); err != nil {
			WriteStructuredError(w, TicketError(err))
			return
		}
	case review.DecisionRejected:
		if err := h.ReviewSvc.RejectTicket(r.Context(), ticketID, body.ReviewerID, body.Notes); err != nil {
			WriteStructuredError(w, TicketError(err))
			return
		}
	case review.DecisionReopened:
		if err := h.ReviewSvc.ReopenTicketForReview(r.Context(), ticketID, body.ReviewerID, body.Notes); err != nil {
			WriteStructuredError(w, TicketError(err))
			return
		}
	default:
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "decision must be approved, rejected, or reopened", false))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *ReviewsHandler) listEscalations(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	list, err := h.ReviewSvc.ListEscalations(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (h *ReviewsHandler) resolveEscalation(w http.ResponseWriter, r *http.Request) {
	ticketID := PathParam(r, "ticketID")
	t, err := h.TicketSvc.GetTicket(r.Context(), ticketID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, t.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	escalationID := PathParam(r, "escalationID")
	var body struct {
		Answer     string `json:"answer"`
		ReviewerID string `json:"reviewer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if body.ReviewerID == "" {
		body.ReviewerID = "api"
	}
	if err := h.ReviewSvc.ResolveEscalation(r.Context(), ticketID, escalationID, body.ReviewerID, body.Answer); err != nil {
		WriteStructuredError(w, TicketError(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}
