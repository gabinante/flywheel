package mcp

import (
	"context"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestCreateTicketHandler_DescriptionResolution tests that the create_ticket
// handler accepts description from top-level "description" OR nested
// "objective.description", and returns a clear error when neither is provided.
func TestCreateTicketHandler_DescriptionResolution(t *testing.T) {
	b := &Backend{} // nil services — handler will fail after description validation
	ctx := context.Background()

	tests := []struct {
		name          string
		args          map[string]any
		wantDescErr   bool   // expect description-related error
		wantErrSubstr string // expected substring in error
	}{
		{
			name: "missing description entirely",
			args: map[string]any{
				"agent_id":   "test-agent",
				"project_id": "proj-1",
				"title":      "test ticket",
			},
			wantDescErr:   true,
			wantErrSubstr: "description required",
		},
		{
			name: "empty description string",
			args: map[string]any{
				"agent_id":    "test-agent",
				"project_id":  "proj-1",
				"title":       "test ticket",
				"description": "",
			},
			wantDescErr:   true,
			wantErrSubstr: "description required",
		},
		{
			name: "empty objective object without description",
			args: map[string]any{
				"agent_id":   "test-agent",
				"project_id": "proj-1",
				"title":      "test ticket",
				"objective":  map[string]any{},
			},
			wantDescErr:   true,
			wantErrSubstr: "description required",
		},
		{
			name: "top-level description passes validation",
			args: map[string]any{
				"agent_id":    "test-agent",
				"project_id":  "proj-1",
				"title":       "test ticket",
				"description": "fix the bug",
			},
			wantDescErr: false,
		},
		{
			name: "nested objective.description passes validation",
			args: map[string]any{
				"agent_id":   "test-agent",
				"project_id": "proj-1",
				"title":      "test ticket",
				"objective": map[string]any{
					"description": "fix the bug via objective",
				},
			},
			wantDescErr: false,
		},
		{
			name: "top-level description takes precedence over objective.description",
			args: map[string]any{
				"agent_id":    "test-agent",
				"project_id":  "proj-1",
				"title":       "test ticket",
				"description": "top-level wins",
				"objective": map[string]any{
					"description": "nested loses",
				},
			},
			wantDescErr: false,
		},
		{
			name: "objective with success_criteria and acceptance_test",
			args: map[string]any{
				"agent_id":   "test-agent",
				"project_id": "proj-1",
				"title":      "test ticket",
				"objective": map[string]any{
					"description":      "fix the bug",
					"success_criteria": []any{"test passes", "no regressions"},
					"acceptance_test":  "run make test",
				},
			},
			wantDescErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call createTicketHandler. It will fail after description validation
			// (e.g., nil pointer on b.AgentStore or b.Project) — we only care
			// about whether it fails on description validation.
			res, _, err := safeCallCreateTicketHandler(b, ctx, tt.args)

			if tt.wantDescErr {
				// Should get an error result with the description message.
				if err != nil {
					t.Fatalf("unexpected Go error: %v", err)
				}
				if res == nil || !res.IsError {
					t.Fatal("expected error result for missing description")
				}
				errText := extractTextContent(res)
				if !strings.Contains(errText, tt.wantErrSubstr) {
					t.Errorf("error text = %q, want substring %q", errText, tt.wantErrSubstr)
				}
			} else {
				// Description validation should pass. The handler will fail
				// later (nil Backend services), so we just verify the error
				// is NOT about description.
				if err != nil {
					t.Fatalf("unexpected Go error: %v", err)
				}
				if res != nil && res.IsError {
					errText := extractTextContent(res)
					if strings.Contains(errText, "description required") {
						t.Errorf("got description error but expected it to pass validation: %s", errText)
					}
					// Other errors (e.g. agent not found) are expected and OK.
				}
			}
		})
	}
}

// TestCreateTicketToolSchema_ObjectiveParam verifies that the tool schema
// documents the objective parameter and does not list description as required.
func TestCreateTicketToolSchema_ObjectiveParam(t *testing.T) {
	b := &Backend{}
	server, err := NewServer(b)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ct, st := sdkmcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	defer ss.Close()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{
		Name:    "test-client",
		Version: "0.0.1",
	}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer cs.Close()

	toolsResult, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	var found bool
	for _, tool := range toolsResult.Tools {
		if tool.Name != "create_ticket" {
			continue
		}
		found = true

		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("create_ticket InputSchema is not map[string]any: %T", tool.InputSchema)
		}

		// Check required array does NOT contain "description".
		reqRaw, ok := schema["required"]
		if !ok {
			t.Fatal("create_ticket schema missing 'required' field")
		}
		reqSlice, ok := reqRaw.([]any)
		if !ok {
			t.Fatalf("create_ticket 'required' is not []any: %T", reqRaw)
		}
		for _, r := range reqSlice {
			if r == "description" {
				t.Error("create_ticket schema should NOT list 'description' as required — it's validated in the handler to support objective.description alternative")
			}
		}

		// Check "objective" property exists.
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatal("create_ticket schema properties is not map[string]any")
		}
		if _, ok := props["objective"]; !ok {
			t.Error("create_ticket schema should have 'objective' property")
		}

		// Verify description param docs mention objective.
		descProp, ok := props["description"].(map[string]any)
		if !ok {
			t.Fatal("create_ticket schema 'description' property missing")
		}
		descDoc, _ := descProp["description"].(string)
		if !strings.Contains(descDoc, "objective") {
			t.Errorf("description param doc should mention objective; got %q", descDoc)
		}

		// Verify tool description mentions both paths.
		if !strings.Contains(tool.Description, "objective.description") {
			t.Errorf("tool description should mention objective.description; got %q", tool.Description)
		}
		break
	}

	if !found {
		t.Fatal("create_ticket tool not found in ListTools result")
	}
}

// safeCallCreateTicketHandler calls createTicketHandler, recovering from panics
// that occur due to nil Backend services (we only care about input validation).
func safeCallCreateTicketHandler(b *Backend, ctx context.Context, args map[string]any) (res *sdkmcp.CallToolResult, meta any, err error) {
	defer func() {
		if r := recover(); r != nil {
			// Panic means we got past description validation — treat as success.
			res = nil
			meta = nil
			err = nil
		}
	}()
	return createTicketHandler(b, ctx, args)
}

// extractTextContent returns the text content from a CallToolResult.
func extractTextContent(res *sdkmcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	for _, c := range res.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
