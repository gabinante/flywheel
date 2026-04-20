package rest

import (
	"context"
	"encoding/json"
	"net/http"

	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/ticket"
)

// TraceService is the execution trace operations needed by TraceHandler. *execution.Service implements it.
type TraceService interface {
	LogStep(ctx context.Context, ticketID, leaseToken string, step execution.Step) error
	GetTrace(ctx context.Context, ticketID string) (*execution.ExecutionTrace, error)
}

// TicketGetter returns a ticket by ID. *ticket.Service implements it.
type TicketGetter interface {
	GetTicket(ctx context.Context, id string) (*ticket.Ticket, error)
}

// TraceHandler handles execution trace endpoints.
type TraceHandler struct {
	TraceSvc   TraceService
	TicketSvc  TicketGetter
	ProjectSvc ProjectGetterForAccess
	OrgSvc     OrgMemberLister
	AgentStore AgentGetter
}

func (h *TraceHandler) logStep(w http.ResponseWriter, r *http.Request) {
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
		LeaseToken string            `json:"lease_token"`
		Step       execution.Step   `json:"step"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if body.LeaseToken == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "lease_token required", false))
		return
	}
	if body.Step.Type == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "step.type required", false))
		return
	}
	if body.Step.Payload == nil {
		body.Step.Payload = make(map[string]any)
	}
	if err := h.TraceSvc.LogStep(r.Context(), ticketID, body.LeaseToken, body.Step); err != nil {
		WriteStructuredError(w, TicketError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TraceHandler) getTrace(w http.ResponseWriter, r *http.Request) {
	ticketID := PathParam(r, "ticketID")
	t, err := h.TicketSvc.GetTicket(r.Context(), ticketID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, t.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	trace, err := h.TraceSvc.GetTrace(r.Context(), ticketID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(trace)
}
