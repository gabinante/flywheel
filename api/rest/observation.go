package rest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gabinante/flywheel/internal/observation"
)

// ObservationHandler provides REST endpoints for production observation,
// signal ingestion, attribution, and ambiguity metrics.
type ObservationHandler struct {
	Svc *observation.Service
}

// RegisterRoutes registers observation endpoints on the mux.
// These are additive endpoints not in the generated OpenAPI spec.
func (h *ObservationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/observation/windows", h.openWindow)
	mux.HandleFunc("POST /api/v1/observation/windows/{windowID}/close", h.closeWindow)
	mux.HandleFunc("GET /api/v1/observation/windows", h.listOpenWindows)
	mux.HandleFunc("POST /api/v1/observation/signals", h.ingestSignal)
	mux.HandleFunc("POST /api/v1/observation/signals/webhook/{source}", h.webhookSignal)
	mux.HandleFunc("GET /api/v1/observation/signals", h.listSignals)
	mux.HandleFunc("GET /api/v1/observation/attributions", h.listUnresolvedAttributions)
	mux.HandleFunc("POST /api/v1/observation/attributions/{attrID}/resolve", h.resolveAttribution)
	mux.HandleFunc("GET /api/v1/observation/metrics/ambiguity", h.getAmbiguityMetrics)
	mux.HandleFunc("POST /api/v1/observation/poll", h.pollSources)
}

// openWindow opens an observation window for a ticket.
func (h *ObservationHandler) openWindow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TicketID  string           `json:"ticket_id"`
		ProjectID string           `json:"project_id"`
		Scope     observation.Scope `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.TicketID == "" || req.ProjectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ticket_id and project_id required"})
		return
	}

	window, err := h.Svc.OpenWindow(r.Context(), req.TicketID, req.ProjectID, req.Scope)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, window)
}

// closeWindow closes an observation window.
func (h *ObservationHandler) closeWindow(w http.ResponseWriter, r *http.Request) {
	windowID := r.PathValue("windowID")
	if windowID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "window_id required"})
		return
	}
	if err := h.Svc.CloseWindow(r.Context(), windowID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listOpenWindows returns all open observation windows for a project.
func (h *ObservationHandler) listOpenWindows(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}
	windows, err := h.Svc.GetOpenWindows(r.Context(), projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if windows == nil {
		windows = []*observation.ObservationWindow{}
	}
	writeJSON(w, http.StatusOK, windows)
}

// ingestSignal processes a production signal through the attribution pipeline.
func (h *ObservationHandler) ingestSignal(w http.ResponseWriter, r *http.Request) {
	var signal observation.Signal
	if err := json.NewDecoder(r.Body).Decode(&signal); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if signal.ProjectID == "" || signal.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id and title required"})
		return
	}
	if signal.OccurredAt.IsZero() {
		signal.OccurredAt = time.Now().UTC()
	}

	attr, err := h.Svc.IngestSignal(r.Context(), signal)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	resp := struct {
		SignalID    string                  `json:"signal_id"`
		Attribution *observation.Attribution `json:"attribution,omitempty"`
	}{
		SignalID:    signal.ID,
		Attribution: attr,
	}
	writeJSON(w, http.StatusOK, resp)
}

// webhookSignal handles incoming webhooks from external signal sources.
func (h *ObservationHandler) webhookSignal(w http.ResponseWriter, r *http.Request) {
	source := r.PathValue("source")
	projectID := r.URL.Query().Get("project_id")
	if source == "" || projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source path param and project_id query param required"})
		return
	}

	// Get the source from the registry.
	src := h.Svc.GetSource(source)
	if src == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown signal source: " + source})
		return
	}

	// Check if it supports webhooks.
	webhookSrc, ok := src.(observation.WebhookSource)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source does not support webhooks"})
		return
	}

	// Read and parse the payload.
	var payload []byte
	if r.Body != nil {
		defer r.Body.Close()
		buf := make([]byte, 1024*1024) // 1MB max
		n, _ := r.Body.Read(buf)
		payload = buf[:n]
	}

	signals, err := webhookSrc.ParseWebhook(r.Context(), projectID, payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Ingest each parsed signal.
	var attributions []*observation.Attribution
	for _, sig := range signals {
		sig.ProjectID = projectID
		attr, err := h.Svc.IngestSignal(r.Context(), sig)
		if err != nil {
			continue
		}
		if attr != nil {
			attributions = append(attributions, attr)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"signals_processed": len(signals),
		"attributions":      len(attributions),
	})
}

// listSignals returns recent signals for a project.
func (h *ObservationHandler) listSignals(w http.ResponseWriter, r *http.Request) {
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
	signals, err := h.Svc.ListSignals(r.Context(), projectID, since, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if signals == nil {
		signals = []*observation.Signal{}
	}
	writeJSON(w, http.StatusOK, signals)
}

// listUnresolvedAttributions returns attributions needing human review.
func (h *ObservationHandler) listUnresolvedAttributions(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}
	attrs, err := h.Svc.ListUnresolvedAttributions(r.Context(), projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if attrs == nil {
		attrs = []*observation.Attribution{}
	}
	writeJSON(w, http.StatusOK, attrs)
}

// resolveAttribution marks an attribution as resolved by a human.
func (h *ObservationHandler) resolveAttribution(w http.ResponseWriter, r *http.Request) {
	attrID := r.PathValue("attrID")
	if attrID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "attribution ID required"})
		return
	}
	var req struct {
		ResolvedBy string `json:"resolved_by"`
		Resolution string `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ResolvedBy == "" || req.Resolution == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "resolved_by and resolution required"})
		return
	}

	if err := h.Svc.ResolveAttribution(r.Context(), attrID, req.ResolvedBy, req.Resolution); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getAmbiguityMetrics returns attribution ambiguity metrics for a time range.
func (h *ObservationHandler) getAmbiguityMetrics(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}

	// Default to last 7 days.
	end := time.Now().UTC()
	start := end.Add(-7 * 24 * time.Hour)
	if s := r.URL.Query().Get("start"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			start = t
		}
	}
	if e := r.URL.Query().Get("end"); e != "" {
		if t, err := time.Parse(time.RFC3339, e); err == nil {
			end = t
		}
	}

	metrics, err := h.Svc.GetAmbiguityMetrics(r.Context(), projectID, start, end)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

// pollSources triggers polling of all registered signal sources.
func (h *ObservationHandler) pollSources(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ProjectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id required"})
		return
	}

	attrs, err := h.Svc.PollSources(r.Context(), req.ProjectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"attributions_created": len(attrs),
		"attributions":         attrs,
	})
}

