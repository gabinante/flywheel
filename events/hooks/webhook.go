package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookTransformer converts a raw webhook payload from an external system
// into one or more Change events. Implement this for each integration
// (GitHub, PagerDuty, ArgoCD, etc.).
type WebhookTransformer interface {
	// Name returns the transformer identifier (e.g. "github", "argocd").
	Name() string

	// Transform converts a raw payload into Change events.
	// The source string matches the URL path parameter.
	Transform(ctx context.Context, source string, payload []byte) ([]Change, error)
}

// GenericTransformer handles JSON payloads that already match the Change shape
// or a close approximation. This is the default when no specific transformer
// is registered.
type GenericTransformer struct{}

// Name returns "generic".
func (GenericTransformer) Name() string { return "generic" }

// Transform attempts to decode the payload as a Change or array of Changes.
// Falls back to treating the whole payload as a single custom change.
func (GenericTransformer) Transform(_ context.Context, source string, payload []byte) ([]Change, error) {
	// Try array of changes first.
	var changes []Change
	if err := json.Unmarshal(payload, &changes); err == nil && len(changes) > 0 {
		return changes, nil
	}

	// Try single change.
	var single Change
	if err := json.Unmarshal(payload, &single); err == nil && single.ChangeType != "" {
		return []Change{single}, nil
	}

	// Fallback: wrap the raw payload as a custom event.
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("webhook: payload is not valid JSON")
	}
	return []Change{{
		ChangeType: TypeCustom,
		EntityID:   source,
		Metadata:   raw,
	}}, nil
}

// WebhookConfig configures the webhook handler.
type WebhookConfig struct {
	// MaxBodyBytes limits the request body size. Default: 1 MB.
	MaxBodyBytes int64

	// Transformers maps source names to specific transformers.
	// The "generic" transformer is always available as a fallback.
	Transformers map[string]WebhookTransformer
}

// WebhookHandler returns an http.Handler that receives change events from
// external systems via HTTP POST. It is designed to be mounted at a path like:
//
//	mux.Handle("POST /api/v1/hooks/webhook/{source}", hooks.WebhookHandler(client))
//
// The {source} path parameter identifies the external system (e.g. "github",
// "argocd", "datadog"). Query parameters:
//   - project_id: required, scopes the change to a project
//
// Request body is a JSON payload; the source determines how it's transformed.
func WebhookHandler(client *Client, cfg ...WebhookConfig) http.Handler {
	config := WebhookConfig{
		MaxBodyBytes: 1 << 20, // 1 MB
		Transformers: make(map[string]WebhookTransformer),
	}
	if len(cfg) > 0 {
		if cfg[0].MaxBodyBytes > 0 {
			config.MaxBodyBytes = cfg[0].MaxBodyBytes
		}
		for k, v := range cfg[0].Transformers {
			config.Transformers[k] = v
		}
	}

	generic := GenericTransformer{}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		source := r.PathValue("source")
		projectID := r.URL.Query().Get("project_id")

		if source == "" {
			writeWebhookError(w, http.StatusBadRequest, "source path parameter required")
			return
		}
		if projectID == "" {
			writeWebhookError(w, http.StatusBadRequest, "project_id query parameter required")
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, config.MaxBodyBytes))
		if err != nil {
			writeWebhookError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		defer r.Body.Close()

		if len(body) == 0 {
			writeWebhookError(w, http.StatusBadRequest, "empty request body")
			return
		}

		// Pick transformer.
		transformer, ok := config.Transformers[source]
		if !ok {
			transformer = generic
		}

		changes, err := transformer.Transform(r.Context(), source, body)
		if err != nil {
			writeWebhookError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Publish each change, applying project ID and source metadata.
		published := 0
		var firstErr error
		for _, change := range changes {
			change.ProjectID = projectID
			change = change.WithMeta("webhook_source", source)
			if change.Timestamp.IsZero() {
				change.Timestamp = time.Now().UTC()
			}
			if err := client.Publish(r.Context(), change); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			published++
		}

		w.Header().Set("Content-Type", "application/json")
		if firstErr != nil && published == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{
				"error":   firstErr.Error(),
				"changes": 0,
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"changes_published": published,
			"changes_received":  len(changes),
			"source":            source,
		})
	})
}

func writeWebhookError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
