package rest

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gabinante/flywheel/internal/catalog"
)

// CatalogHandler provides REST endpoints for the Layer 14 project map (catalog).
type CatalogHandler struct {
	Svc     *catalog.Service
	Scanner *catalog.Scanner
}

// RegisterRoutes registers catalog endpoints on the mux.
func (h *CatalogHandler) RegisterRoutes(mux *http.ServeMux) {
	// Entities
	mux.HandleFunc("GET /api/v1/catalog/entities", h.listEntities)
	mux.HandleFunc("POST /api/v1/catalog/entities", h.createEntity)
	mux.HandleFunc("GET /api/v1/catalog/entities/{entityID}", h.getEntity)
	mux.HandleFunc("PATCH /api/v1/catalog/entities/{entityID}", h.updateEntity)
	mux.HandleFunc("DELETE /api/v1/catalog/entities/{entityID}", h.deleteEntity)

	// Edges
	mux.HandleFunc("GET /api/v1/catalog/edges", h.listEdges)
	mux.HandleFunc("POST /api/v1/catalog/edges", h.createEdge)
	mux.HandleFunc("DELETE /api/v1/catalog/edges/{edgeID}", h.deleteEdge)

	// Deployment matrix
	mux.HandleFunc("GET /api/v1/catalog/deployments", h.deploymentMatrix)

	// Bootstrap scan
	mux.HandleFunc("POST /api/v1/catalog/scan", h.bootstrapScan)
}

func (h *CatalogHandler) listEntities(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id required"})
		return
	}
	entityType := r.URL.Query().Get("type")
	label := r.URL.Query().Get("label")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	entities, err := h.Svc.ListEntities(r.Context(), projectID, entityType, label, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if entities == nil {
		entities = []*catalog.Entity{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entities": entities})
}

func (h *CatalogHandler) createEntity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID   string            `json:"project_id"`
		EntityType  string            `json:"entity_type"`
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Labels      map[string]string `json:"labels"`
		Metadata    map[string]string `json:"metadata"`
		Source      string            `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ProjectID == "" || req.EntityType == "" || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id, entity_type, and name required"})
		return
	}

	entity, err := h.Svc.CreateEntity(r.Context(), req.ProjectID, req.EntityType, req.Name, req.Description, req.Labels, req.Metadata, req.Source)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, entity)
}

func (h *CatalogHandler) getEntity(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entityID")
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" || entityID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id and entity_id required"})
		return
	}

	entity, err := h.Svc.GetEntity(r.Context(), projectID, entityID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, entity)
}

func (h *CatalogHandler) updateEntity(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entityID")
	var req struct {
		ProjectID   string            `json:"project_id"`
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Labels      map[string]string `json:"labels"`
		Metadata    map[string]string `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ProjectID == "" || entityID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id and entity_id required"})
		return
	}

	entity, err := h.Svc.UpdateEntity(r.Context(), req.ProjectID, entityID, req.Name, req.Description, req.Labels, req.Metadata)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, entity)
}

func (h *CatalogHandler) deleteEntity(w http.ResponseWriter, r *http.Request) {
	entityID := r.PathValue("entityID")
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" || entityID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id and entity_id required"})
		return
	}

	if err := h.Svc.DeleteEntity(r.Context(), projectID, entityID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CatalogHandler) listEdges(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	entityID := r.URL.Query().Get("entity_id")
	if projectID == "" || entityID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id and entity_id required"})
		return
	}
	edgeType := r.URL.Query().Get("edge_type")
	direction := r.URL.Query().Get("direction")

	edges, err := h.Svc.ListEdges(r.Context(), projectID, entityID, edgeType, direction)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if edges == nil {
		edges = []*catalog.Edge{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"edges": edges})
}

func (h *CatalogHandler) createEdge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID string            `json:"project_id"`
		FromID    string            `json:"from_id"`
		ToID      string            `json:"to_id"`
		EdgeType  string            `json:"edge_type"`
		Metadata  map[string]string `json:"metadata"`
		Source    string            `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ProjectID == "" || req.FromID == "" || req.ToID == "" || req.EdgeType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id, from_id, to_id, and edge_type required"})
		return
	}

	edge, err := h.Svc.CreateEdge(r.Context(), req.ProjectID, req.FromID, req.ToID, req.EdgeType, req.Metadata, req.Source)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, edge)
}

func (h *CatalogHandler) deleteEdge(w http.ResponseWriter, r *http.Request) {
	edgeID := r.PathValue("edgeID")
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" || edgeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id and edge_id required"})
		return
	}

	if err := h.Svc.DeleteEdge(r.Context(), projectID, edgeID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CatalogHandler) deploymentMatrix(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id required"})
		return
	}
	serviceID := r.URL.Query().Get("service_id")

	deployments, err := h.Svc.DeploymentMatrix(r.Context(), projectID, serviceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if deployments == nil {
		deployments = []*catalog.DeploymentEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": deployments})
}

func (h *CatalogHandler) bootstrapScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID string `json:"project_id"`
		RepoPath  string `json:"repo_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ProjectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id required"})
		return
	}
	if req.RepoPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "repo_path required"})
		return
	}

	scanResult, err := h.Scanner.ScanRepo(req.RepoPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	imported, err := h.Svc.ImportScanResult(r.Context(), req.ProjectID, scanResult)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entities_created": len(imported.Entities),
		"edges_created":    len(imported.Edges),
		"errors":           imported.Errors,
	})
}
