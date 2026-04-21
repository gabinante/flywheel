package rest

import (
	"encoding/json"
	"net/http"

	"github.com/gabinante/flywheel/internal/claims"
	apierrors "github.com/gabinante/flywheel/internal/errors"
)

// ClaimsHandler handles claims registry REST operations.
type ClaimsHandler struct {
	ClaimsSvc *claims.Service
}

// RegisterRoutes registers claims routes on the mux.
func (h *ClaimsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /claims/active", h.listActive)
	mux.HandleFunc("GET /claims/ticket/{ticketID}", h.listByTicket)
	mux.HandleFunc("GET /claims/entity/{entityID}", h.listByEntity)
	mux.HandleFunc("POST /claims/detect", h.detectConflicts)
	mux.HandleFunc("GET /claims/conflicts/{ticketID}", h.getConflicts)
}

// listActive returns all active claims, optionally filtered by environment.
func (h *ClaimsHandler) listActive(w http.ResponseWriter, r *http.Request) {
	env := r.URL.Query().Get("environment")
	if env == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "environment query parameter required", false))
		return
	}

	active, err := h.ClaimsSvc.GetActiveClaimsByEnvironment(r.Context(), env)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "failed to list claims: "+err.Error(), true))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"claims": active,
		"count":  len(active),
	})
}

// listByTicket returns active claims for a specific ticket.
func (h *ClaimsHandler) listByTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := r.PathValue("ticketID")
	if ticketID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "ticketID required", false))
		return
	}

	active, err := h.ClaimsSvc.GetActiveClaims(r.Context(), ticketID)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "failed to list claims: "+err.Error(), true))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"claims":    active,
		"count":     len(active),
		"ticket_id": ticketID,
	})
}

// listByEntity returns active claims for a specific entity+environment.
func (h *ClaimsHandler) listByEntity(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entityID")
	if entityID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "entityID required", false))
		return
	}

	env := r.URL.Query().Get("environment")
	if env == "" {
		env = "" // Empty environment means global scope
	}

	active, err := h.ClaimsSvc.GetActiveClaimsByEntity(r.Context(), entityID, env)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "failed to list claims: "+err.Error(), true))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"claims":      active,
		"count":       len(active),
		"entity_id":   entityID,
		"environment": env,
	})
}

// detectConflictsRequest is the request body for conflict detection.
type detectConflictsRequest struct {
	TicketID string         `json:"ticket_id"`
	Touches  []claims.Touch `json:"touches"`
}

// detectConflicts runs conflict detection for a set of touches without registering them.
func (h *ClaimsHandler) detectConflicts(w http.ResponseWriter, r *http.Request) {
	var body detectConflictsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body: "+err.Error(), false))
		return
	}

	if body.TicketID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "ticket_id required", false))
		return
	}
	if len(body.Touches) == 0 {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "touches required", false))
		return
	}

	result, err := h.ClaimsSvc.DetectConflicts(r.Context(), body.TicketID, body.Touches)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "conflict detection failed: "+err.Error(), true))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// getConflicts returns unresolved conflicts for a ticket.
func (h *ClaimsHandler) getConflicts(w http.ResponseWriter, r *http.Request) {
	ticketID := r.PathValue("ticketID")
	if ticketID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "ticketID required", false))
		return
	}

	conflicts, err := h.ClaimsSvc.GetUnresolvedConflicts(r.Context(), ticketID)
	if err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInternal, "failed to get conflicts: "+err.Error(), true))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"conflicts": conflicts,
		"count":     len(conflicts),
		"ticket_id": ticketID,
	})
}
