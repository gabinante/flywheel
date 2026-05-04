package rest

import (
	"encoding/json"
	"net/http"

	"github.com/gabinante/flywheel/internal/dispatch"
)

// DispatchHandler serves the dispatch status endpoint.
type DispatchHandler struct {
	Dispatcher *dispatch.Dispatcher
}

func (h *DispatchHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	if h.Dispatcher == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":     "dispatcher not enabled",
			"code":      "service_unavailable",
			"retriable": false,
		})
		return
	}

	projectID := r.URL.Query().Get("project_id")
	status := h.Dispatcher.GetStatus(r.Context(), projectID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(status)
}
