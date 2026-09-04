package rest

import (
	"encoding/json"
	"net/http"

	"github.com/gabinante/flywheel/internal/agent"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/queue"
	"github.com/gabinante/flywheel/internal/ticket"
)

// QueueHandler handles queue and lease REST endpoints.
type QueueHandler struct {
	QueueSvc   *queue.Service
	TicketSvc  *ticket.Service
	ProjectSvc *project.Service
	OrgSvc     *org.Service
	AgentStore agent.AgentStore
}

func (h *QueueHandler) claim(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	callerID := GetAgentID(r.Context())
	if callerID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeUnauthorized, "authentication required to claim a ticket", false))
		return
	}
	var body struct {
		AgentID        string `json:"agent_id"`
		Priority       *int   `json:"priority"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	// Caller may only claim for themselves; body.agent_id if present must match.
	if body.AgentID != "" && body.AgentID != callerID {
		WriteStructuredError(w, apierrors.New(apierrors.CodeForbidden, "you may only claim a ticket for your own agent", false))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	proj, err := h.ProjectSvc.GetProject(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if proj.Status == "closed" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeProjectClosed, "cannot claim from queue: project is closed", false))
		return
	}
	t, lease, err := h.QueueSvc.ClaimTicket(r.Context(), callerID, projectID, body.Priority, body.IdempotencyKey)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"ticket": t, "lease": lease})
}

func (h *QueueHandler) renewLease(w http.ResponseWriter, r *http.Request) {
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
		LeaseToken string `json:"lease_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if body.LeaseToken == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "lease_token required", false))
		return
	}
	newExpiresAt, err := h.QueueSvc.RenewLease(r.Context(), ticketID, body.LeaseToken)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"expires_at": newExpiresAt})
}

func (h *QueueHandler) releaseLease(w http.ResponseWriter, r *http.Request) {
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
		LeaseToken string `json:"lease_token"`
	}
	// Allow token in body for DELETE
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.LeaseToken == "" {
		// Optional: support ?lease_token= for DELETE
		body.LeaseToken = r.URL.Query().Get("lease_token")
	}
	if body.LeaseToken == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "lease_token required", false))
		return
	}
	if err := h.QueueSvc.ReleaseLease(r.Context(), ticketID, body.LeaseToken); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
