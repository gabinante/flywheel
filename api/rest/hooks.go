package rest

import (
	"net/http"

	"github.com/gabinante/flywheel/events/hooks"
)

// HooksHandler provides REST endpoints for the change event hook library.
type HooksHandler struct {
	Client *hooks.Client
}

// RegisterRoutes registers hook endpoints on the mux.
// These are additive endpoints not in the generated OpenAPI spec.
func (h *HooksHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/hooks/webhook/{source}", hooks.WebhookHandler(h.Client))
}
