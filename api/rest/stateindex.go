package rest

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gabinante/flywheel/internal/stateindex"
)

// StateIndexHandler provides REST endpoints for the observed state index
// (spec v0.2 Layer 10): resource queries, staleness checks, drift detection,
// attribution management, and provider-based ingestion.
type StateIndexHandler struct {
	Svc *stateindex.Service
}

// RegisterRoutes registers state index endpoints on the mux.
// These are additive endpoints not in the generated OpenAPI spec.
func (h *StateIndexHandler) RegisterRoutes(mux *http.ServeMux) {
	// Resource CRUD and queries
	mux.HandleFunc("GET /api/v1/state/resources", h.queryResources)
	mux.HandleFunc("GET /api/v1/state/resources/{resourceID}", h.getResource)
	mux.HandleFunc("POST /api/v1/state/resources", h.observeResource)

	// Staleness and drift
	mux.HandleFunc("GET /api/v1/state/staleness", h.checkStaleness)
	mux.HandleFunc("GET /api/v1/state/drift", h.detectDrift)

	// Attribution
	mux.HandleFunc("POST /api/v1/state/attributions", h.attributeChange)
	mux.HandleFunc("GET /api/v1/state/unattributed", h.listUnattributed)

	// Changes
	mux.HandleFunc("GET /api/v1/state/changes", h.listChanges)

	// Summary and snapshots
	mux.HandleFunc("GET /api/v1/state/summary", h.getSummary)
	mux.HandleFunc("GET /api/v1/state/snapshot", h.getSnapshot)

	// Provider-based ingestion
	mux.HandleFunc("POST /api/v1/state/ingest", h.ingestFromProvider)
}

// queryResources queries the observed state index with filters.
func (h *StateIndexHandler) queryResources(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	q := stateindex.ResourceQuery{
		ProjectID:    projectID,
		ResourceType: stateindex.ResourceType(r.URL.Query().Get("resource_type")),
		Environment:  r.URL.Query().Get("environment"),
		Limit:        limit,
	}

	resources, err := h.Svc.QueryResources(r.Context(), q)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if resources == nil {
		resources = []*stateindex.ObservedResource{}
	}
	writeJSON(w, http.StatusOK, resources)
}

// getResource returns a single resource by ID.
func (h *StateIndexHandler) getResource(w http.ResponseWriter, r *http.Request) {
	resourceID := r.PathValue("resourceID")
	if resourceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "resource_id required"})
		return
	}

	resource, err := h.Svc.GetResource(r.Context(), resourceID)
	if err != nil {
		if err == stateindex.ErrResourceNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "resource not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resource)
}

// observeResource records a new observation of a resource's state.
func (h *StateIndexHandler) observeResource(w http.ResponseWriter, r *http.Request) {
	var resource stateindex.ObservedResource
	if err := json.NewDecoder(r.Body).Decode(&resource); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if resource.ProjectID == "" || resource.Name == "" || resource.ResourceType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id, name, and resource_type required"})
		return
	}

	change, err := h.Svc.ObserveResource(r.Context(), &resource)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	resp := map[string]any{
		"resource_id": resource.ID,
		"change":      change,
	}
	writeJSON(w, http.StatusCreated, resp)
}

// checkStaleness returns resources whose observations are older than the threshold.
func (h *StateIndexHandler) checkStaleness(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}

	thresholdSeconds := 300 // default 5 minutes
	if t := r.URL.Query().Get("threshold_seconds"); t != "" {
		if parsed, err := strconv.Atoi(t); err == nil && parsed > 0 {
			thresholdSeconds = parsed
		}
	}

	resourceType := stateindex.ResourceType(r.URL.Query().Get("resource_type"))
	environment := r.URL.Query().Get("environment")

	report, err := h.Svc.CheckStaleness(r.Context(), projectID, resourceType, environment, thresholdSeconds)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// detectDrift finds resources where observed state differs from declared state.
func (h *StateIndexHandler) detectDrift(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}

	resourceID := r.URL.Query().Get("resource_id")
	environment := r.URL.Query().Get("environment")

	report, err := h.Svc.DetectDrift(r.Context(), projectID, resourceID, environment)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// attributeChange assigns a state change to a ticket or external actor.
func (h *StateIndexHandler) attributeChange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID  string `json:"project_id"`
		ResourceID string `json:"resource_id"`
		TicketID   string `json:"ticket_id"`
		Actor      string `json:"actor"`
		ChangeID   string `json:"change_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ProjectID == "" || req.ResourceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id and resource_id required"})
		return
	}

	attr, err := h.Svc.AttributeChange(r.Context(), req.ProjectID, req.ResourceID, req.TicketID, req.Actor, req.ChangeID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, attr)
}

// listUnattributed returns state changes without attribution.
func (h *StateIndexHandler) listUnattributed(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	attrs, err := h.Svc.ListUnattributed(r.Context(), projectID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if attrs == nil {
		attrs = []*stateindex.StateAttribution{}
	}
	writeJSON(w, http.StatusOK, attrs)
}

// listChanges returns recent state changes for a project.
func (h *StateIndexHandler) listChanges(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}

	since := time.Now().UTC().Add(-24 * time.Hour)
	if s := r.URL.Query().Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	changes, err := h.Svc.ListChanges(r.Context(), projectID, since, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if changes == nil {
		changes = []*stateindex.StateChange{}
	}
	writeJSON(w, http.StatusOK, changes)
}

// getSummary returns a high-level overview of the state index.
func (h *StateIndexHandler) getSummary(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}

	summary, err := h.Svc.GetSummary(r.Context(), projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// getSnapshot returns a point-in-time snapshot of resource states for freshness stamping.
func (h *StateIndexHandler) getSnapshot(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}

	resourceIDs := r.URL.Query()["resource_ids"]
	if len(resourceIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "resource_ids query param required"})
		return
	}

	environment := r.URL.Query().Get("environment")

	resources, err := h.Svc.GetSnapshot(r.Context(), projectID, resourceIDs, environment)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Build snapshot with timestamp for freshness stamping.
	snapshot := map[string]any{
		"project_id":    projectID,
		"snapshot_at":   time.Now().UTC().Format(time.RFC3339),
		"resource_count": len(resources),
		"resources":     resources,
		"hash":          stateindex.HashResult(resources),
	}
	writeJSON(w, http.StatusOK, snapshot)
}

// ingestFromProvider triggers resource ingestion from a registered state provider.
func (h *StateIndexHandler) ingestFromProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID    string `json:"project_id"`
		Provider     string `json:"provider"`
		ResourceType string `json:"resource_type"`
		Environment  string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ProjectID == "" || req.Provider == "" || req.ResourceType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id, provider, and resource_type required"})
		return
	}

	count, err := h.Svc.IngestFromProvider(r.Context(), req.ProjectID, req.Provider, stateindex.ResourceType(req.ResourceType), req.Environment)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"resources_observed": count,
		"provider":           req.Provider,
		"resource_type":      req.ResourceType,
	})
}
