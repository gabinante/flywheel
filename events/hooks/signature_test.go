package hooks_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gabinante/flywheel/events/hooks"
)

// captureAuditLogger captures audit entries for test assertions.
type captureAuditLogger struct {
	mu      sync.Mutex
	entries []hooks.AuditEntry
}

func (c *captureAuditLogger) Log(entry hooks.AuditEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, entry)
}

func (c *captureAuditLogger) Entries() []hooks.AuditEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]hooks.AuditEntry, len(c.entries))
	copy(cp, c.entries)
	return cp
}

func TestWebhookHandler_ValidSignature_Processes(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	secret := "test-secret-key"
	body := `{"change_type":"deploy","entity_id":"api","after":"v2.0"}`
	sig := hooks.ComputeHMACSHA256(secret, []byte(body))

	handler := hooks.WebhookHandler(client, hooks.WebhookConfig{
		Signatures: map[string]hooks.SignatureConfig{
			"nmi": {Secret: secret, Header: "X-NMI-Signature"},
		},
	})

	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/nmi?project_id=proj-1", strings.NewReader(body))
	req.SetPathValue("source", "nmi")
	req.Header.Set("X-NMI-Signature", sig)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
}

func TestWebhookHandler_InvalidSignature_Returns401(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	audit := &captureAuditLogger{}

	handler := hooks.WebhookHandler(client, hooks.WebhookConfig{
		Signatures: map[string]hooks.SignatureConfig{
			"nmi": {Secret: "real-secret", Header: "X-NMI-Signature"},
		},
		Audit: audit,
	})

	body := `{"change_type":"deploy","entity_id":"api","after":"v2.0"}`
	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/nmi?project_id=proj-1", strings.NewReader(body))
	req.SetPathValue("source", "nmi")
	req.Header.Set("X-NMI-Signature", "bad-signature")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if len(bus.events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(bus.events))
	}

	// Verify audit log entry.
	entries := audit.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0].Action != "webhook_rejected" {
		t.Errorf("expected action=webhook_rejected, got %q", entries[0].Action)
	}
	if entries[0].Resource != "nmi_webhook" {
		t.Errorf("expected resource=nmi_webhook, got %q", entries[0].Resource)
	}
	if entries[0].Details["reason"] != "invalid signature" {
		t.Errorf("expected reason='invalid signature', got %v", entries[0].Details["reason"])
	}
	if entries[0].Details["source_ip"] == nil || entries[0].Details["source_ip"] == "" {
		t.Error("expected source_ip in audit details")
	}
}

func TestWebhookHandler_MissingSignatureHeader_Returns401(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	audit := &captureAuditLogger{}

	handler := hooks.WebhookHandler(client, hooks.WebhookConfig{
		Signatures: map[string]hooks.SignatureConfig{
			"nmi": {Secret: "real-secret", Header: "X-NMI-Signature"},
		},
		Audit: audit,
	})

	body := `{"change_type":"deploy","entity_id":"api","after":"v2.0"}`
	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/nmi?project_id=proj-1", strings.NewReader(body))
	req.SetPathValue("source", "nmi")
	// No signature header set.
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}

	entries := audit.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0].Details["reason"] == nil {
		t.Error("expected reason in audit details")
	}
}

func TestWebhookHandler_NoSecret_DevMode_WarnsButProcesses(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)

	handler := hooks.WebhookHandler(client, hooks.WebhookConfig{
		Signatures: map[string]hooks.SignatureConfig{
			"nmi": {Secret: "", Header: "X-NMI-Signature"}, // dev mode: no secret
		},
	})

	body := `{"change_type":"deploy","entity_id":"api","after":"v2.0"}`
	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/nmi?project_id=proj-1", strings.NewReader(body))
	req.SetPathValue("source", "nmi")
	// No signature header — should still work in dev mode.
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 in dev mode, got %d: %s", w.Code, w.Body.String())
	}
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event in dev mode, got %d", len(bus.events))
	}
}

func TestWebhookHandler_NoSignatureConfig_SkipsVerification(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)

	// No Signatures map at all — unknown sources skip verification entirely.
	handler := hooks.WebhookHandler(client)

	body := `{"change_type":"deploy","entity_id":"api","after":"v2.0"}`
	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/other?project_id=proj-1", strings.NewReader(body))
	req.SetPathValue("source", "other")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWebhookHandler_DefaultSignatureHeader(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	secret := "my-secret"
	body := `{"change_type":"deploy","entity_id":"svc","after":"v1"}`
	sig := hooks.ComputeHMACSHA256(secret, []byte(body))

	// Use default header (no Header set in config).
	handler := hooks.WebhookHandler(client, hooks.WebhookConfig{
		Signatures: map[string]hooks.SignatureConfig{
			"custom": {Secret: secret}, // Header defaults to "X-Webhook-Signature"
		},
	})

	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/custom?project_id=proj-1", strings.NewReader(body))
	req.SetPathValue("source", "custom")
	req.Header.Set("X-Webhook-Signature", sig)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWebhookHandler_SeamlesschexSignature(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	secret := "seamlesschex-secret"
	body := `{"change_type":"deploy","entity_id":"check","after":"completed"}`
	sig := hooks.ComputeHMACSHA256(secret, []byte(body))

	handler := hooks.WebhookHandler(client, hooks.WebhookConfig{
		Signatures: map[string]hooks.SignatureConfig{
			"seamlesschex": {Secret: secret, Header: "X-Seamlesschex-Signature"},
		},
	})

	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/seamlesschex?project_id=proj-1", strings.NewReader(body))
	req.SetPathValue("source", "seamlesschex")
	req.Header.Set("X-Seamlesschex-Signature", sig)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestComputeHMACSHA256(t *testing.T) {
	// Verify HMAC computation produces a deterministic hex digest.
	secret := "test-secret"
	body := []byte("hello world")

	sig1 := hooks.ComputeHMACSHA256(secret, body)
	sig2 := hooks.ComputeHMACSHA256(secret, body)

	if sig1 != sig2 {
		t.Errorf("HMAC should be deterministic: %q != %q", sig1, sig2)
	}

	// Different secret → different signature.
	sig3 := hooks.ComputeHMACSHA256("other-secret", body)
	if sig1 == sig3 {
		t.Error("different secrets should produce different signatures")
	}

	// Different body → different signature.
	sig4 := hooks.ComputeHMACSHA256(secret, []byte("different body"))
	if sig1 == sig4 {
		t.Error("different bodies should produce different signatures")
	}
}

func TestVerifySignature_Valid(t *testing.T) {
	cfg := hooks.SignatureConfig{Secret: "my-secret", Header: "X-Sig"}
	body := []byte(`{"key":"value"}`)
	sig := hooks.ComputeHMACSHA256("my-secret", body)

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Sig", sig)

	if err := hooks.VerifySignature(cfg, body, req); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestVerifySignature_Invalid(t *testing.T) {
	cfg := hooks.SignatureConfig{Secret: "my-secret", Header: "X-Sig"}
	body := []byte(`{"key":"value"}`)

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Sig", "wrong-signature")

	if err := hooks.VerifySignature(cfg, body, req); err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestVerifySignature_MissingHeader(t *testing.T) {
	cfg := hooks.SignatureConfig{Secret: "my-secret", Header: "X-Sig"}
	body := []byte(`{"key":"value"}`)

	req := httptest.NewRequest("POST", "/test", nil)

	err := hooks.VerifySignature(cfg, body, req)
	if err == nil {
		t.Fatal("expected error for missing header")
	}
	if !strings.Contains(err.Error(), "missing signature header") {
		t.Errorf("expected 'missing signature header' error, got: %v", err)
	}
}

func TestVerifySignature_EmptySecret_DevMode(t *testing.T) {
	cfg := hooks.SignatureConfig{Secret: "", Header: "X-Sig"}
	body := []byte(`{"key":"value"}`)

	req := httptest.NewRequest("POST", "/test", nil)

	if err := hooks.VerifySignature(cfg, body, req); err != nil {
		t.Fatalf("dev mode (empty secret) should pass, got: %v", err)
	}
}

func TestWebhookHandler_AuditEntryIncludesSourceIP(t *testing.T) {
	bus := &testBus{}
	client := hooks.NewClient(bus)
	audit := &captureAuditLogger{}

	handler := hooks.WebhookHandler(client, hooks.WebhookConfig{
		Signatures: map[string]hooks.SignatureConfig{
			"nmi": {Secret: "secret", Header: "X-NMI-Signature"},
		},
		Audit: audit,
	})

	body := `{"change_type":"deploy","entity_id":"api"}`
	req := httptest.NewRequest("POST", "/api/v1/hooks/webhook/nmi?project_id=proj-1", strings.NewReader(body))
	req.SetPathValue("source", "nmi")
	req.Header.Set("X-Real-Ip", "1.2.3.4")
	req.Header.Set("X-NMI-Signature", "invalid")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	entries := audit.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0].Details["source_ip"] != "1.2.3.4" {
		t.Errorf("expected source_ip=1.2.3.4, got %v", entries[0].Details["source_ip"])
	}
}
