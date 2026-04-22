package rest

import (
	"encoding/json"
	"net/http"
	"time"

	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/pillar"
)

// PillarsHandler provides REST endpoints for the pillar and strategy layer.
type PillarsHandler struct {
	PillarSvc *pillar.Service
}

// RegisterRoutes registers all pillar routes on the mux.
func (h *PillarsHandler) RegisterRoutes(mux *http.ServeMux) {
	// Entries
	mux.HandleFunc("POST /pillars", h.createEntry)
	mux.HandleFunc("GET /pillars/{entryID}", h.getEntry)
	mux.HandleFunc("GET /projects/{projectID}/pillars", h.listEntries)
	mux.HandleFunc("PUT /pillars/{entryID}", h.updateEntry)
	mux.HandleFunc("DELETE /pillars/{entryID}", h.deleteEntry)
	mux.HandleFunc("POST /pillars/{entryID}/review", h.markReviewed)
	mux.HandleFunc("GET /projects/{projectID}/pillars/due", h.listDueForReview)

	// Claims
	mux.HandleFunc("POST /pillars/{entryID}/claims", h.createClaim)
	mux.HandleFunc("GET /pillar-claims/{claimID}", h.getClaim)
	mux.HandleFunc("GET /pillars/{entryID}/claims", h.listClaims)
	mux.HandleFunc("PUT /pillar-claims/{claimID}", h.updateClaimStatus)
	mux.HandleFunc("DELETE /pillar-claims/{claimID}", h.deleteClaim)

	// Evaluations
	mux.HandleFunc("POST /pillars/{entryID}/evaluate", h.evaluateEntry)
	mux.HandleFunc("GET /pillars/{entryID}/evaluations", h.listEvaluations)

	// Templates
	mux.HandleFunc("GET /pillar-templates", h.listTemplates)
	mux.HandleFunc("GET /pillar-templates/{pillarType}", h.getTemplate)
}

// ────────────────────────────────────────────────────────────────────────────
// Entry endpoints
// ────────────────────────────────────────────────────────────────────────────

func (h *PillarsHandler) createEntry(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectID  string       `json:"project_id"`
		EntityID   string       `json:"entity_id"`
		EntityType string       `json:"entity_type"`
		PillarType string       `json:"pillar_type"`
		Strategy   string       `json:"strategy"`
		Gaps       []pillar.Gap `json:"gaps"`
		Cadence    string       `json:"review_cadence"`
		CreatedBy  string       `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body: "+err.Error(), false))
		return
	}
	if body.ProjectID == "" || body.EntityID == "" || body.PillarType == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "project_id, entity_id, and pillar_type required", false))
		return
	}
	if body.CreatedBy == "" {
		if agentID := GetAgentID(r.Context()); agentID != "" {
			body.CreatedBy = agentID
		} else {
			body.CreatedBy = "api"
		}
	}

	entry, err := h.PillarSvc.CreateEntry(r.Context(),
		body.ProjectID, body.EntityID, body.EntityType, body.PillarType,
		body.Strategy, body.CreatedBy, body.Gaps, body.Cadence)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(entry)
}

func (h *PillarsHandler) getEntry(w http.ResponseWriter, r *http.Request) {
	entryID := PathParam(r, "entryID")
	entry, err := h.PillarSvc.GetEntry(r.Context(), entryID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entry)
}

func (h *PillarsHandler) listEntries(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	entityID := r.URL.Query().Get("entity_id")
	pillarType := r.URL.Query().Get("pillar_type")
	entries, err := h.PillarSvc.ListEntries(r.Context(), projectID, entityID, pillarType, 50)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entries)
}

func (h *PillarsHandler) updateEntry(w http.ResponseWriter, r *http.Request) {
	entryID := PathParam(r, "entryID")
	var body struct {
		Strategy string       `json:"strategy"`
		Gaps     []pillar.Gap `json:"gaps"`
		Cadence  string       `json:"review_cadence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body: "+err.Error(), false))
		return
	}
	entry, err := h.PillarSvc.UpdateEntry(r.Context(), entryID, body.Strategy, body.Gaps, body.Cadence)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entry)
}

func (h *PillarsHandler) deleteEntry(w http.ResponseWriter, r *http.Request) {
	entryID := PathParam(r, "entryID")
	if err := h.PillarSvc.DeleteEntry(r.Context(), entryID); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PillarsHandler) markReviewed(w http.ResponseWriter, r *http.Request) {
	entryID := PathParam(r, "entryID")
	entry, err := h.PillarSvc.MarkReviewed(r.Context(), entryID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entry)
}

func (h *PillarsHandler) listDueForReview(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	entries, err := h.PillarSvc.ListDueForReview(r.Context(), projectID, time.Now().UTC(), 50)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entries)
}

// ────────────────────────────────────────────────────────────────────────────
// Claim endpoints
// ────────────────────────────────────────────────────────────────────────────

func (h *PillarsHandler) createClaim(w http.ResponseWriter, r *http.Request) {
	entryID := PathParam(r, "entryID")
	var body struct {
		Statement     string `json:"statement"`
		EntityRefID   string `json:"entity_ref_id"`
		EntityRefType string `json:"entity_ref_type"`
		Evidence      string `json:"evidence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body: "+err.Error(), false))
		return
	}
	if body.Statement == "" || body.EntityRefID == "" {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "statement and entity_ref_id required", false))
		return
	}
	claim, err := h.PillarSvc.CreateClaim(r.Context(), entryID,
		body.Statement, body.EntityRefID, body.EntityRefType, body.Evidence)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(claim)
}

func (h *PillarsHandler) getClaim(w http.ResponseWriter, r *http.Request) {
	claimID := PathParam(r, "claimID")
	claim, err := h.PillarSvc.GetClaim(r.Context(), claimID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(claim)
}

func (h *PillarsHandler) listClaims(w http.ResponseWriter, r *http.Request) {
	entryID := PathParam(r, "entryID")
	claims, err := h.PillarSvc.ListClaims(r.Context(), entryID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(claims)
}

func (h *PillarsHandler) updateClaimStatus(w http.ResponseWriter, r *http.Request) {
	claimID := PathParam(r, "claimID")
	var body struct {
		Status   string `json:"status"`
		Evidence string `json:"evidence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteStructuredError(w, apierrors.New(apierrors.CodeInvalidInput, "invalid body: "+err.Error(), false))
		return
	}
	claim, err := h.PillarSvc.UpdateClaimStatus(r.Context(), claimID, body.Status, body.Evidence)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(claim)
}

func (h *PillarsHandler) deleteClaim(w http.ResponseWriter, r *http.Request) {
	claimID := PathParam(r, "claimID")
	if err := h.PillarSvc.DeleteClaim(r.Context(), claimID); err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ────────────────────────────────────────────────────────────────────────────
// Evaluation endpoints
// ────────────────────────────────────────────────────────────────────────────

func (h *PillarsHandler) evaluateEntry(w http.ResponseWriter, r *http.Request) {
	entryID := PathParam(r, "entryID")
	evals, err := h.PillarSvc.EvaluateEntry(r.Context(), entryID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(evals)
}

func (h *PillarsHandler) listEvaluations(w http.ResponseWriter, r *http.Request) {
	entryID := PathParam(r, "entryID")
	evals, err := h.PillarSvc.ListEvaluations(r.Context(), entryID, 20)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(evals)
}

// ────────────────────────────────────────────────────────────────────────────
// Template endpoints
// ────────────────────────────────────────────────────────────────────────────

func (h *PillarsHandler) listTemplates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.PillarSvc.GetTemplates())
}

func (h *PillarsHandler) getTemplate(w http.ResponseWriter, r *http.Request) {
	pillarType := PathParam(r, "pillarType")
	tmpl, err := h.PillarSvc.GetTemplate(pillarType)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tmpl)
}
