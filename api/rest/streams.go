package rest

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gabinante/flywheel/internal/stream"
)

// StreamsHandler provides REST endpoints for the three foundational streams
// (entity, state, change) per spec v0.2 section 2.2.
type StreamsHandler struct {
	Svc *stream.Service
}

// RegisterRoutes registers stream endpoints on the mux.
func (h *StreamsHandler) RegisterRoutes(mux *http.ServeMux) {
	// Entity stream
	mux.HandleFunc("POST /api/v1/streams/entity", h.appendEntity)
	mux.HandleFunc("GET /api/v1/streams/entity", h.listEntity)
	mux.HandleFunc("GET /api/v1/streams/entity/{eventID}", h.getEntity)

	// State stream
	mux.HandleFunc("POST /api/v1/streams/state", h.appendState)
	mux.HandleFunc("GET /api/v1/streams/state", h.listState)
	mux.HandleFunc("GET /api/v1/streams/state/{eventID}", h.getState)

	// Change stream
	mux.HandleFunc("POST /api/v1/streams/change", h.appendChange)
	mux.HandleFunc("GET /api/v1/streams/change", h.listChange)
	mux.HandleFunc("GET /api/v1/streams/change/{eventID}", h.getChange)
}

// --- Entity stream handlers ---

func (h *StreamsHandler) appendEntity(w http.ResponseWriter, r *http.Request) {
	var event stream.EntityEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if event.ProjectID == "" || event.EntityID == "" || event.ChangeType == "" || event.EntityType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id, entity_id, change_type, and entity_type are required"})
		return
	}
	if err := h.Svc.AppendEntityEvent(r.Context(), &event); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (h *StreamsHandler) getEntity(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	if eventID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event ID required"})
		return
	}
	event, err := h.Svc.GetEntityEvent(r.Context(), eventID)
	if err != nil {
		if err == stream.ErrEventNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, event)
}

func (h *StreamsHandler) listEntity(w http.ResponseWriter, r *http.Request) {
	q := parseStreamQuery(r)
	if q.ProjectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}
	page, err := h.Svc.ListEntityEvents(r.Context(), q)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// --- State stream handlers ---

func (h *StreamsHandler) appendState(w http.ResponseWriter, r *http.Request) {
	var event stream.StateEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if event.ProjectID == "" || event.EntityID == "" || event.ChangeType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id, entity_id, and change_type are required"})
		return
	}
	if err := h.Svc.AppendStateEvent(r.Context(), &event); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (h *StreamsHandler) getState(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	if eventID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event ID required"})
		return
	}
	event, err := h.Svc.GetStateEvent(r.Context(), eventID)
	if err != nil {
		if err == stream.ErrEventNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, event)
}

func (h *StreamsHandler) listState(w http.ResponseWriter, r *http.Request) {
	q := parseStreamQuery(r)
	if q.ProjectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}
	page, err := h.Svc.ListStateEvents(r.Context(), q)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// --- Change stream handlers ---

func (h *StreamsHandler) appendChange(w http.ResponseWriter, r *http.Request) {
	var event stream.ChangeEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if event.ProjectID == "" || event.ChangeType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id and change_type are required"})
		return
	}
	if err := h.Svc.AppendChangeEvent(r.Context(), &event); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (h *StreamsHandler) getChange(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	if eventID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event ID required"})
		return
	}
	event, err := h.Svc.GetChangeEvent(r.Context(), eventID)
	if err != nil {
		if err == stream.ErrEventNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, event)
}

func (h *StreamsHandler) listChange(w http.ResponseWriter, r *http.Request) {
	q := parseStreamQuery(r)
	if q.ProjectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id query param required"})
		return
	}
	page, err := h.Svc.ListChangeEvents(r.Context(), q)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// --- Helpers ---

// parseStreamQuery extracts common query parameters from the request.
func parseStreamQuery(r *http.Request) stream.StreamQuery {
	q := stream.StreamQuery{
		ProjectID:   r.URL.Query().Get("project_id"),
		EntityID:    r.URL.Query().Get("entity_id"),
		ChangeType:  r.URL.Query().Get("change_type"),
		Environment: r.URL.Query().Get("environment"),
		InitiatorID: r.URL.Query().Get("initiator_id"),
		Source:      r.URL.Query().Get("source"),
	}
	if s := r.URL.Query().Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			q.Since = t
		}
	}
	if s := r.URL.Query().Get("until"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			q.Until = t
		}
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			q.Limit = n
		}
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			q.Offset = n
		}
	}
	return q
}
