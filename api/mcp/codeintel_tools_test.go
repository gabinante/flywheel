package mcp

import (
	"context"
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// testCodeIntelBackend creates a backend with an in-memory code intel provider
// seeded with test data.
func testCodeIntelBackend(t *testing.T) *Backend {
	t.Helper()
	ci := NewTreeSitterCodeIntel()

	// Seed test data directly.
	ci.mu.Lock()
	ci.indexes["test-project"] = &projectIndex{
		projectID: "test-project",
		rootPath:  "/tmp/test",
		symbols: []SymbolInfo{
			{ID: "main.go:main", Name: "main", Kind: "function", File: "main.go", Line: 10, Language: "go", Visibility: "private"},
			{ID: "main.go:Run", Name: "Run", Kind: "function", File: "main.go", Line: 20, Language: "go", Package: "cmd", Visibility: "public", Signature: "func Run(ctx context.Context) error"},
			{ID: "service.go:Service", Name: "Service", Kind: "type", File: "service.go", Line: 5, Language: "go", Package: "svc", Visibility: "public"},
			{ID: "service.go:Handle", Name: "Handle", Kind: "method", File: "service.go", Line: 15, Language: "go", Package: "svc", Visibility: "public"},
			{ID: "util.py:process", Name: "process", Kind: "function", File: "util.py", Line: 1, Language: "python", Visibility: "public"},
			{ID: "app.ts:handler", Name: "handler", Kind: "function", File: "app.ts", Line: 3, Language: "typescript", Visibility: "public"},
		},
		edges: []symbolEdge{
			{callerID: "main.go:main", calleeID: "main.go:Run", file: "main.go", line: 12},
			{callerID: "main.go:Run", calleeID: "service.go:Handle", file: "main.go", line: 25},
			{callerID: "service.go:Handle", calleeID: "util.py:process", file: "service.go", line: 18},
		},
		imports: []importEdge{
			{file: "main.go", pkg: "fmt"},
			{file: "main.go", pkg: "github.com/example/svc"},
			{file: "service.go", pkg: "github.com/example/util"},
			{file: "util.py", pkg: "os"},
		},
	}
	ci.mu.Unlock()

	return &Backend{CodeIntel: ci}
}

// extractText gets the text from the first content block of a CallToolResult.
func extractText(t *testing.T, result *sdkmcp.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("empty result content")
	}
	tc, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("content is not TextContent: %T", result.Content[0])
	}
	return tc.Text
}

// parseJSON unmarshals the result text into a map.
func parseJSON(t *testing.T, result *sdkmcp.CallToolResult) map[string]any {
	t.Helper()
	text := extractText(t, result)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal: %v\ntext: %s", err, text)
	}
	return parsed
}

func TestCodeSymbolLookupHandler(t *testing.T) {
	b := testCodeIntelBackend(t)
	ctx := context.Background()

	tests := []struct {
		name      string
		args      map[string]any
		wantCount int
		wantErr   bool
	}{
		{
			name:      "search by name",
			args:      map[string]any{"project_id": "test-project", "query": "Run"},
			wantCount: 1,
		},
		{
			name:      "search all",
			args:      map[string]any{"project_id": "test-project", "query": ""},
			wantCount: 6,
		},
		{
			name:      "filter by language",
			args:      map[string]any{"project_id": "test-project", "query": "", "language": "go"},
			wantCount: 4,
		},
		{
			name:      "filter by kind",
			args:      map[string]any{"project_id": "test-project", "query": "", "kind": "function"},
			wantCount: 4,
		},
		{
			name:      "limit results",
			args:      map[string]any{"project_id": "test-project", "query": "", "limit": float64(2)},
			wantCount: 2,
		},
		{
			name:    "missing project_id",
			args:    map[string]any{"query": "Run"},
			wantErr: true,
		},
		{
			name:    "project not indexed",
			args:    map[string]any{"project_id": "unknown", "query": "Run"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _, _ := codeSymbolLookupHandler(b, ctx, tt.args)
			if result == nil {
				t.Fatal("nil result")
			}
			if tt.wantErr {
				if !result.IsError {
					t.Error("expected error result")
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", extractText(t, result))
			}

			parsed := parseJSON(t, result)
			count := int(parsed["count"].(float64))
			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

func TestCodeCallersHandler(t *testing.T) {
	b := testCodeIntelBackend(t)
	ctx := context.Background()

	result, _, _ := codeCallersHandler(b, ctx, map[string]any{
		"project_id": "test-project",
		"symbol":     "Run",
	})
	if result == nil || result.IsError {
		t.Fatal("expected non-error result")
	}

	parsed := parseJSON(t, result)
	count := int(parsed["count"].(float64))
	if count != 1 {
		t.Errorf("callers count = %d, want 1 (main calls Run)", count)
	}
}

func TestCodeCalleesHandler(t *testing.T) {
	b := testCodeIntelBackend(t)
	ctx := context.Background()

	result, _, _ := codeCalleesHandler(b, ctx, map[string]any{
		"project_id": "test-project",
		"symbol":     "Run",
	})
	if result == nil || result.IsError {
		t.Fatal("expected non-error result")
	}

	parsed := parseJSON(t, result)
	count := int(parsed["count"].(float64))
	if count != 1 {
		t.Errorf("callees count = %d, want 1 (Run calls Handle)", count)
	}
}

func TestCodeBlastRadiusHandler(t *testing.T) {
	b := testCodeIntelBackend(t)
	ctx := context.Background()

	result, _, _ := codeBlastRadiusHandler(b, ctx, map[string]any{
		"project_id": "test-project",
		"symbol":     "Handle",
		"depth":      float64(3),
	})
	if result == nil || result.IsError {
		t.Fatal("expected non-error result")
	}

	parsed := parseJSON(t, result)
	scope := int(parsed["transitive_scope"].(float64))
	if scope < 1 {
		t.Errorf("transitive_scope = %d, want >= 1", scope)
	}
	riskLevel, ok := parsed["risk_level"].(string)
	if !ok || riskLevel == "" {
		t.Error("expected non-empty risk_level")
	}
}

func TestCodeImportersHandler(t *testing.T) {
	b := testCodeIntelBackend(t)
	ctx := context.Background()

	result, _, _ := codeImportersHandler(b, ctx, map[string]any{
		"project_id": "test-project",
		"package":    "svc",
	})
	if result == nil || result.IsError {
		t.Fatal("expected non-error result")
	}

	parsed := parseJSON(t, result)
	count := int(parsed["count"].(float64))
	if count != 1 {
		t.Errorf("importers count = %d, want 1 (main.go imports svc)", count)
	}
}

func TestCodeReindexHandler(t *testing.T) {
	b := testCodeIntelBackend(t)
	ctx := context.Background()

	result, _, _ := codeReindexHandler(b, ctx, map[string]any{
		"project_id": "test-project",
		"commit_sha": "abc123",
	})
	if result == nil || result.IsError {
		t.Fatal("expected non-error result")
	}

	parsed := parseJSON(t, result)
	if parsed["reindexed"] != true {
		t.Error("expected reindexed=true")
	}
}

func TestCodeSymbolMetadataHandler(t *testing.T) {
	b := testCodeIntelBackend(t)
	ctx := context.Background()

	result, _, _ := codeSymbolMetadataHandler(b, ctx, map[string]any{
		"project_id": "test-project",
		"symbol":     "Run",
	})
	if result == nil || result.IsError {
		t.Fatal("expected non-error result")
	}

	parsed := parseJSON(t, result)
	if parsed["name"] != "Run" {
		t.Errorf("name = %v, want Run", parsed["name"])
	}
}

func TestCodeSymbolLookupHandler_MissingQuery(t *testing.T) {
	b := testCodeIntelBackend(t)
	ctx := context.Background()

	result, _, _ := codeSymbolLookupHandler(b, ctx, map[string]any{
		"project_id": "test-project",
		// missing query
	})
	if result == nil {
		t.Fatal("nil result")
	}
	if !result.IsError {
		t.Error("expected error for missing query")
	}
}

func TestRegisterCodeIntelTools_NilProvider(t *testing.T) {
	// Should not panic when CodeIntel is nil.
	b := &Backend{CodeIntel: nil}
	s := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterCodeIntelTools(s, b) // should be a no-op
}
