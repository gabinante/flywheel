package hooks_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/events/hooks"
)

func TestWebhookHandler_GenericPayload(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	handler := hooks.WebhookHandler(client)

	body := `{"change_type":"deploy","entity_id":"api","after":"v2.0"}`
	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/ci?project_id=proj-1", strings.NewReader(body))
	req.SetPathValue("source", "ci")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	ev := bus.events[0]
	if ev.Type != events.EventChangePublished {
		t.Errorf("expected type %q, got %q", events.EventChangePublished, ev.Type)
	}
	if ev.Payload["project_id"] != "proj-1" {
		t.Errorf("expected project_id=proj-1, got %v", ev.Payload["project_id"])
	}
}

func TestWebhookHandler_ArrayPayload(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	handler := hooks.WebhookHandler(client)

	body := `[
		{"change_type":"deploy","entity_id":"api","after":"v2.0"},
		{"change_type":"config_update","entity_id":"flags"}
	]`
	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/ci?project_id=proj-1", strings.NewReader(body))
	req.SetPathValue("source", "ci")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(bus.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(bus.events))
	}
}

func TestWebhookHandler_RawPayloadFallback(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	handler := hooks.WebhookHandler(client)

	// Payload doesn't match Change shape — should be wrapped as custom.
	body := `{"action":"completed","repository":"my-repo"}`
	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/github?project_id=proj-1", strings.NewReader(body))
	req.SetPathValue("source", "github")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	if bus.events[0].Payload["change_type"] != "custom" {
		t.Errorf("expected custom change_type, got %v", bus.events[0].Payload["change_type"])
	}
}

func TestWebhookHandler_MissingProjectID(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	handler := hooks.WebhookHandler(client)

	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/ci", strings.NewReader(`{}`))
	req.SetPathValue("source", "ci")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWebhookHandler_MissingSource(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	handler := hooks.WebhookHandler(client)

	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/?project_id=proj-1", strings.NewReader(`{}`))
	// Don't set path value — source is empty.
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWebhookHandler_EmptyBody(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	handler := hooks.WebhookHandler(client)

	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/ci?project_id=proj-1", strings.NewReader(""))
	req.SetPathValue("source", "ci")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWebhookHandler_CustomTransformer(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)

	// Register a custom transformer.
	argoTransformer := &mockTransformer{
		name: "argocd",
		result: []hooks.Change{
			hooks.Deploy("my-app", "v3.0").By("argocd"),
		},
	}

	handler := hooks.WebhookHandler(client, hooks.WebhookConfig{
		Transformers: map[string]hooks.WebhookTransformer{
			"argocd": argoTransformer,
		},
	})

	body := `{"kind":"Application","status":"Synced"}`
	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/argocd?project_id=proj-1", strings.NewReader(body))
	req.SetPathValue("source", "argocd")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
	if bus.events[0].Payload["initiator"] != "argocd" {
		t.Errorf("expected initiator=argocd, got %v", bus.events[0].Payload["initiator"])
	}
}

func TestWebhookHandler_InvalidJSON(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	handler := hooks.WebhookHandler(client)

	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/ci?project_id=proj-1", strings.NewReader("not json"))
	req.SetPathValue("source", "ci")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// mockTransformer is a test double for WebhookTransformer.
type mockTransformer struct {
	name   string
	result []hooks.Change
	err    error
}

func (m *mockTransformer) Name() string { return m.name }
func (m *mockTransformer) Transform(_ context.Context, _ string, _ []byte) ([]hooks.Change, error) {
	return m.result, m.err
}
