package rest

import (
	"encoding/json"
	"net/http"

	"github.com/gabinante/flywheel/internal/agent"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
)

// WebhookConfigHandler handles REST endpoints for webhook secret management.
// Endpoints:
//   - GET  /api/v1/projects/{projectID}/webhooks/config      → masked secret
//   - POST /api/v1/projects/{projectID}/webhooks/secret/rotate → rotate + return new secret
type WebhookConfigHandler struct {
	ProjectSvc *project.Service
	OrgSvc     *org.Service
	AgentStore agent.AgentStore
}

// RegisterRoutes registers webhook config endpoints on the mux.
func (h *WebhookConfigHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/webhooks/config", h.getConfig)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/webhooks/secret/rotate", h.rotateSecret)
}

// getConfig returns the webhook configuration for a project, including the masked secret.
func (h *WebhookConfigHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}

	masked, err := h.ProjectSvc.GetWebhookConfig(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"project_id":     projectID,
		"webhook_secret": masked,
		"headers": map[string]string{
			"X-GHP-Signature":  "sha256={hmac_hex}",
			"X-GHP-Timestamp":  "{unix_epoch_seconds}",
			"X-GHP-Event-Type": "{notification_category}",
		},
		"verification": "Compute HMAC-SHA256(webhook_secret_bytes, \"{timestamp}.{body}\") and compare to signature. Reject if |now - timestamp| > 300.",
	})
}

// rotateSecret generates a new webhook secret and returns the full value once.
func (h *WebhookConfigHandler) rotateSecret(w http.ResponseWriter, r *http.Request) {
	projectID := PathParam(r, "projectID")
	if !EnsureProjectAccess(r.Context(), w, projectID, h.AgentStore, h.OrgSvc, h.ProjectSvc) {
		return
	}

	newSecret, err := h.ProjectSvc.RotateWebhookSecret(r.Context(), projectID)
	if err != nil {
		WriteStructuredError(w, apierrors.MapError(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"project_id":     projectID,
		"webhook_secret": newSecret,
		"note":           "This is the only time the full secret will be shown. Store it securely.",
	})
}
