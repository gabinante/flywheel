package rest

import (
	"encoding/json"
	"net/http"

	"github.com/gabinante/flywheel/internal/agent"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
)

// TicketsHandler handles ticket REST endpoints.
type TicketsHandler struct {
	TicketSvc   *ticket.Service
	ProjectSvc  *project.Service
	OrgSvc      *org.Service
	AgentStore  *agent.Store
}

func (h *TicketsHandler) createTicket(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	proj, err := h.ProjectSvc.GetProject(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if proj.Status == "closed" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeProjectClosed, "cannot create ticket: project is closed", false))
		return
	}
	var body struct {
		Title           string               `json:"title"`
		WorkStreamID    string               `json:"work_stream_id"`
		Type            ticket.TicketType    `json:"type"`
		Priority        *int                 `json:"priority"`
		CreatedBy       string               `json:"created_by"`
		DependsOn       []string             `json:"depends_on"`
		Objective       ticket.Objective     `json:"objective"`
		Context         ticket.TicketContext `json:"ticket_context"`
		IdempotencyKey  string               `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if body.Title == "" || body.CreatedBy == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "title and created_by required", false))
		return
	}
	typ := body.Type
	if typ == "" {
		typ = ticket.TypeTask
	}
	prio := ticket.P2
	if body.Priority != nil && *body.Priority >= 0 && *body.Priority <= 3 {
		prio = ticket.Priority(*body.Priority)
	}
	if body.DependsOn == nil {
		body.DependsOn = []string{}
	}
	t, err := h.TicketSvc.CreateTicket(r.Context(), projectID, body.Title, typ, prio, body.CreatedBy, body.DependsOn, body.WorkStreamID, body.Objective, body.Context, body.IdempotencyKey)
	if err != nil {
		WriteStructuredError(w, TicketError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(t)
}

func (h *TicketsHandler) listTickets(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	workStreamID := r.URL.Query().Get("work_stream_id")
	state := ticket.MapLegacyState(ticket.State(r.URL.Query().Get("state")))
	list, err := h.TicketSvc.ListTickets(r.Context(), projectID, workStreamID, state)
	if err != nil {
		WriteStructuredError(w, TicketError(err))
		return
	}
	if list == nil {
		list = []*ticket.Ticket{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (h *TicketsHandler) getTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := PathParam(r, "ticketID")
	t, err := h.TicketSvc.GetTicket(r.Context(), ticketID)
	if err != nil {
		WriteStructuredError(w, TicketError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, t.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(t)
}

func (h *TicketsHandler) updateTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := PathParam(r, "ticketID")
	t, err := h.TicketSvc.GetTicket(r.Context(), ticketID)
	if err != nil {
		WriteStructuredError(w, TicketError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, t.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	var body struct {
		DependsOn []string `json:"depends_on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if err := h.TicketSvc.UpdateDependsOn(r.Context(), ticketID, body.DependsOn); err != nil {
		WriteStructuredError(w, TicketError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TicketsHandler) transition(w http.ResponseWriter, r *http.Request) {
	ticketID := PathParam(r, "ticketID")
	t, err := h.TicketSvc.GetTicket(r.Context(), ticketID)
	if err != nil {
		WriteStructuredError(w, TicketError(err))
		return
	}
	if !EnsureProjectAccess(r.Context(), w, t.ProjectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}
	var body struct {
		Trigger string         `json:"trigger"`
		Payload map[string]any `json:"payload"`
		ActorID string         `json:"actor_id"`
		Actor   string         `json:"actor"` // "human" | "agent" | "system"
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	if body.Trigger == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "trigger required", false))
		return
	}
	actorType := ticket.ActorAgent
	switch body.Actor {
	case "human":
		actorType = ticket.ActorHuman
	case "system":
		actorType = ticket.ActorSystem
	}
	if body.ActorID == "" {
		body.ActorID = "api"
	}
	actor := ticket.Actor{ID: body.ActorID, Type: actorType}
	if err := h.TicketSvc.TransitionTicket(r.Context(), ticketID, body.Trigger, actor, body.Payload); err != nil {
		WriteStructuredError(w, TicketError(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}
