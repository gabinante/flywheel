package dispatch

import (
	"testing"
)

func TestBuildMCPConfigWithAPIKey(t *testing.T) {
	cfg := buildMCPConfig("http://localhost:8080", "my-api-key")

	if len(cfg.MCPServers) != 1 {
		t.Fatalf("expected 1 MCP server, got %d", len(cfg.MCPServers))
	}

	srv, ok := cfg.MCPServers["flywheel"]
	if !ok {
		t.Fatal("missing 'flywheel' MCP server")
	}

	if srv.Type != "sse" {
		t.Errorf("expected type 'sse', got %q", srv.Type)
	}
	if srv.URL != "http://localhost:8080/sse" {
		t.Errorf("expected URL 'http://localhost:8080/sse', got %q", srv.URL)
	}
	if srv.Headers == nil {
		t.Fatal("expected headers to be set")
	}
	if srv.Headers["X-API-Key"] != "my-api-key" {
		t.Errorf("expected X-API-Key 'my-api-key', got %q", srv.Headers["X-API-Key"])
	}
}

func TestBuildMCPConfigWithoutAPIKey(t *testing.T) {
	cfg := buildMCPConfig("http://localhost:9090", "")

	srv := cfg.MCPServers["flywheel"]
	if srv.URL != "http://localhost:9090/sse" {
		t.Errorf("expected URL 'http://localhost:9090/sse', got %q", srv.URL)
	}
	if srv.Headers != nil {
		t.Error("expected no headers when API key is empty")
	}
}

func TestBuildMCPConfigURLSuffix(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		wantURL   string
	}{
		{"plain http", "http://example.com", "http://example.com/sse"},
		{"with port", "http://localhost:8083", "http://localhost:8083/sse"},
		{"https", "https://api.flywheel.dev", "https://api.flywheel.dev/sse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := buildMCPConfig(tt.serverURL, "")
			if cfg.MCPServers["flywheel"].URL != tt.wantURL {
				t.Errorf("got %q, want %q", cfg.MCPServers["flywheel"].URL, tt.wantURL)
			}
		})
	}
}

func TestBuildTaskPrompt(t *testing.T) {
	prompt := buildTaskPrompt("ticket-42", "project-1")

	if prompt == "" {
		t.Fatal("expected non-empty task prompt")
	}

	// Should contain the ticket ID.
	if !containsStr(prompt, "ticket-42") {
		t.Error("task prompt should contain the ticket ID")
	}

	// Should reference core workflow steps.
	if !containsStr(prompt, "claim") {
		t.Error("task prompt should reference claiming")
	}
	if !containsStr(prompt, "submit") {
		t.Error("task prompt should reference submitting")
	}
	if !containsStr(prompt, "log_step") {
		t.Error("task prompt should reference log_step")
	}
	if !containsStr(prompt, "commit") {
		t.Error("task prompt should reference committing changes")
	}
}

func TestSanitizeContainerName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "ticket-42", "ticket-42"},
		{"uppercase", "TICKET-42", "ticket-42"},
		{"mixed case", "Ticket-ABC", "ticket-abc"},
		{"special chars", "org/proj#42", "org-proj-42"},
		{"spaces", "my ticket", "my-ticket"},
		{"dots", "v1.2.3", "v1-2-3"},
		{"underscores", "my_ticket", "my-ticket"},
		{"already clean", "abc-123", "abc-123"},
		{"all special", "!@#$%", "-----"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeContainerName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeContainerName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWorkerResultFields(t *testing.T) {
	// Test that WorkerResult struct is properly initialized.
	r := &WorkerResult{
		Success: true,
		Output:  "all tests passed",
		Error:   "",
	}
	if !r.Success {
		t.Error("expected Success to be true")
	}
	if r.Output != "all tests passed" {
		t.Errorf("expected Output 'all tests passed', got %q", r.Output)
	}
	if r.Error != "" {
		t.Errorf("expected empty Error, got %q", r.Error)
	}

	// Failed result.
	r2 := &WorkerResult{
		Success: false,
		Output:  "FAIL",
		Error:   "exit status 1",
	}
	if r2.Success {
		t.Error("expected Success to be false")
	}
	if r2.Error != "exit status 1" {
		t.Errorf("expected Error 'exit status 1', got %q", r2.Error)
	}
}

// containsStr is a small helper to check substring existence.
func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
