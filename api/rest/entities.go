package rest

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gabinante/flywheel/internal/entity"
)

// EntitiesHandler handles REST API requests for entity management.
type EntitiesHandler struct {
	EntitySvc *entity.Service
}

// Register mounts entity routes on the mux.
func (h *EntitiesHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/entities", h.createEntity)
	mux.HandleFunc("GET /api/v1/entities/{entityID}", h.getEntity)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/entities", h.listEntities)
	mux.HandleFunc("PATCH /api/v1/entities/{entityID}", h.updateEntity)
	mux.HandleFunc("DELETE /api/v1/entities/{entityID}", h.retireEntity)
	mux.HandleFunc("POST /api/v1/entities/{entityID}/instances", h.createInstance)
	mux.HandleFunc("GET /api/v1/entities/{entityID}/instances", h.listInstances)
	mux.HandleFunc("DELETE /api/v1/entity-instances/{instanceID}", h.retireInstance)
	mux.HandleFunc("GET /api/v1/entities/{entityID}/stream", h.getStream)
}

// --- Request/Response types ---

type createEntityRequest struct {
	ProjectID   string         `json:"project_id"`
	Type        string         `json:"type"`
	LogicalName string         `json:"logical_name"`
	Description string         `json:"description,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

type updateEntityRequest struct {
	LogicalName *string        `json:"logical_name,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

type createInstanceRequest struct {
	Environment string         `json:"environment"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

// --- Handlers ---

func (h *EntitiesHandler) createEntity(w http.ResponseWriter, r *http.Request) {
	var req createEntityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	actor := GetAgentID(r.Context())
	if actor == "" {
		actor = "anonymous"
	}
	e, err := h.EntitySvc.CreateEntity(r.Context(), req.ProjectID, entity.Type(req.Type), req.LogicalName, req.Description, req.Attributes, actor)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *EntitiesHandler) getEntity(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entityID")
	e, err := h.EntitySvc.GetEntity(r.Context(), entityID)
	if err != nil {
		if errors.Is(err, entity.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "entity not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *EntitiesHandler) listEntities(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	entityType := entity.Type(r.URL.Query().Get("type"))
	includeRetired := r.URL.Query().Get("include_retired") == "true"

	entities, err := h.EntitySvc.ListEntities(r.Context(), projectID, entityType, includeRetired)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entities == nil {
		entities = []*entity.Entity{}
	}
	writeJSON(w, http.StatusOK, entities)
}

func (h *EntitiesHandler) updateEntity(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entityID")
	var req updateEntityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	actor := GetAgentID(r.Context())
	if actor == "" {
		actor = "anonymous"
	}

	if req.LogicalName != nil {
		if err := h.EntitySvc.RenameEntity(r.Context(), entityID, *req.LogicalName, actor); err != nil {
			if errors.Is(err, entity.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "entity not found")
				return
			}
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.Attributes != nil && len(req.Attributes) > 0 {
		if err := h.EntitySvc.UpdateAttributes(r.Context(), entityID, req.Attributes, actor); err != nil {
			if errors.Is(err, entity.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "entity not found")
				return
			}
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	e, err := h.EntitySvc.GetEntity(r.Context(), entityID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "entity not found")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *EntitiesHandler) retireEntity(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entityID")
	actor := GetAgentID(r.Context())
	if actor == "" {
		actor = "anonymous"
	}
	if err := h.EntitySvc.RetireEntity(r.Context(), entityID, actor); err != nil {
		if errors.Is(err, entity.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "entity not found or already retired")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entity_id": entityID})
}

func (h *EntitiesHandler) createInstance(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entityID")
	var req createInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	actor := GetAgentID(r.Context())
	if actor == "" {
		actor = "anonymous"
	}
	inst, err := h.EntitySvc.CreateInstance(r.Context(), entityID, req.Environment, req.Attributes, actor)
	if err != nil {
		if errors.Is(err, entity.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "entity not found")
			return
		}
		if errors.Is(err, entity.ErrDuplicateEnv) {
			writeJSONError(w, http.StatusConflict, "instance for this environment already exists")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, inst)
}

func (h *EntitiesHandler) listInstances(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entityID")
	includeRetired := r.URL.Query().Get("include_retired") == "true"
	instances, err := h.EntitySvc.ListInstances(r.Context(), entityID, includeRetired)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if instances == nil {
		instances = []*entity.Instance{}
	}
	writeJSON(w, http.StatusOK, instances)
}

func (h *EntitiesHandler) retireInstance(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("instanceID")
	actor := GetAgentID(r.Context())
	if actor == "" {
		actor = "anonymous"
	}
	if err := h.EntitySvc.RetireInstance(r.Context(), instanceID, actor); err != nil {
		if errors.Is(err, entity.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "instance not found or already retired")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "instance_id": instanceID})
}

func (h *EntitiesHandler) getStream(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entityID")
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	events, err := h.EntitySvc.GetStream(r.Context(), entityID, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if events == nil {
		events = []*entity.StreamEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}
