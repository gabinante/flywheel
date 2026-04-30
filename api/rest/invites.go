package rest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gabinante/flywheel/internal/agent"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/org"
)

// InvitesHandler handles org invite REST endpoints.
type InvitesHandler struct {
	OrgSvc     *org.Service
	AgentStore agent.AgentStore
}

// RegisterRoutes registers invite routes on the mux.
func (h *InvitesHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /orgs/{orgID}/invites", h.createInvite)
	mux.HandleFunc("GET /orgs/{orgID}/invites", h.listInvites)
	mux.HandleFunc("DELETE /orgs/{orgID}/invites/{inviteID}", h.revokeInvite)
	mux.HandleFunc("GET /orgs/{orgID}/members", h.listMembers)
	mux.HandleFunc("POST /invites/{code}/accept", h.acceptInvite)
}

func (h *InvitesHandler) resolveUser(r *http.Request) (agentID, userID string, err *apierrors.StructuredError) {
	agentID = GetAgentID(r.Context())
	if agentID == "" {
		return "", "", apierrors.New(apierrors.CodeUnauthorized, "authentication required", false)
	}
	a, e := h.AgentStore.GetByID(r.Context(), agentID)
	if e != nil || a == nil {
		return "", "", apierrors.New(apierrors.CodeUnauthorized, "agent not found", false)
	}
	if a.UserID == "" {
		return "", "", apierrors.New(apierrors.CodeUnauthorized, "OAuth required (agent must be linked to a user)", false)
	}
	return agentID, a.UserID, nil
}

func (h *InvitesHandler) ensureOrgAdmin(r *http.Request, orgID, userID string) *apierrors.StructuredError {
	role, err := h.OrgSvc.GetMemberRole(r.Context(), orgID, userID)
	if err != nil {
		return apierrors.MapError(err)
	}
	if role != org.RoleOwner && role != org.RoleAdmin {
		return apierrors.New(apierrors.CodeForbidden, "owner or admin role required", false)
	}
	return nil
}

func (h *InvitesHandler) createInvite(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")
	_, userID, authErr := h.resolveUser(r)
	if authErr != nil {
		WriteStructuredError(w, authErr)
		return
	}
	if err := h.ensureOrgAdmin(r, orgID, userID); err != nil {
		WriteStructuredError(w, err)
		return
	}
	var body struct {
		Role     string `json:"role"`
		MaxUses  int    `json:"max_uses"`
		ExpireIn string `json:"expire_in"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body", false))
		return
	}
	role := org.RoleMember
	if body.Role == "admin" {
		role = org.RoleAdmin
	}
	maxUses := body.MaxUses
	if maxUses < 1 {
		maxUses = 1
	}
	expiresIn := 7 * 24 * time.Hour // default 7 days
	if body.ExpireIn != "" {
		d, err := time.ParseDuration(body.ExpireIn)
		if err == nil && d > 0 {
			expiresIn = d
		}
	}
	inv, err := h.OrgSvc.CreateInvite(r.Context(), orgID, userID, role, expiresIn, maxUses)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(inv)
}

func (h *InvitesHandler) listInvites(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")
	_, userID, authErr := h.resolveUser(r)
	if authErr != nil {
		WriteStructuredError(w, authErr)
		return
	}
	if err := h.ensureOrgAdmin(r, orgID, userID); err != nil {
		WriteStructuredError(w, err)
		return
	}
	invites, err := h.OrgSvc.ListInvites(r.Context(), orgID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if invites == nil {
		invites = []org.Invite{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(invites)
}

func (h *InvitesHandler) revokeInvite(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")
	inviteID := PathParam(r, "inviteID")
	_, userID, authErr := h.resolveUser(r)
	if authErr != nil {
		WriteStructuredError(w, authErr)
		return
	}
	if err := h.ensureOrgAdmin(r, orgID, userID); err != nil {
		WriteStructuredError(w, err)
		return
	}
	if err := h.OrgSvc.RevokeInvite(r.Context(), orgID, inviteID); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *InvitesHandler) listMembers(w http.ResponseWriter, r *http.Request) {
	orgID := PathParam(r, "orgID")
	if !EnsureOrgAccess(r.Context(), w, orgID, h.AgentStore, h.OrgSvc) {
		return
	}
	members, err := h.OrgSvc.ListMembers(r.Context(), orgID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	if members == nil {
		members = []org.Member{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(members)
}

func (h *InvitesHandler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	code := PathParam(r, "code")
	_, userID, authErr := h.resolveUser(r)
	if authErr != nil {
		WriteStructuredError(w, authErr)
		return
	}
	inv, err := h.OrgSvc.AcceptInvite(r.Context(), code, userID)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		return
	}
	o, _ := h.OrgSvc.GetOrg(r.Context(), inv.OrgID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"invite": inv, "org": o})
}
