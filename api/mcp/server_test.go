package mcp

import (
	"context"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// testClientSession creates a connected client session to the Flywheel MCP server
// over in-memory transport. Returns the session; caller should defer cs.Close().
func testClientSession(t *testing.T, b *Backend) *sdkmcp.ClientSession {
	t.Helper()

	server, err := NewServer(b)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ct, st := sdkmcp.NewInMemoryTransports()

	// Connect server first (requirement of InMemoryTransport).
	ctx := context.Background()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := sdkmcp.NewClient(&sdkmcp.Implementation{
		Name:    "test-client",
		Version: "0.0.1",
	}, nil)

	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	return cs
}

// TestNewServer_NilBackend verifies that NewServer returns an error for nil backends.
func TestNewServer_NilBackend(t *testing.T) {
	_, err := NewServer(nil)
	if err == nil {
		t.Fatal("NewServer(nil) should return an error")
	}
}

// TestNewServer_InstructionsSet verifies the server sends instructions during initialization.
func TestNewServer_InstructionsSet(t *testing.T) {
	if ServerInstructions == "" {
		t.Fatal("ServerInstructions should not be empty")
	}
	if !strings.Contains(ServerInstructions, "Quick start") {
		t.Error("ServerInstructions should contain Quick start section")
	}
	if !strings.Contains(ServerInstructions, "claim_ticket") {
		t.Error("ServerInstructions should mention claim_ticket")
	}
	if !strings.Contains(ServerInstructions, "flywheel://docs/agent-guide") {
		t.Error("ServerInstructions should reference the agent guide resource")
	}
}

// TestNewServer_RegistersCoreTools verifies all core tools are registered on a minimal backend.
func TestNewServer_RegistersCoreTools(t *testing.T) {
	cs := testClientSession(t, &Backend{})
	ctx := context.Background()

	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	// Core tools that should always be registered (from tools.go).
	coreTools := []string{
		// Organization & project management
		"list_orgs",
		"create_project",
		"list_projects",
		"get_project_context",
		"update_project_context",
		"update_project_status",

		// Work stream management
		"create_work_stream",
		"list_work_streams",
		"get_work_stream",
		"update_work_stream",
		"update_work_stream_plan",

		// Ticket CRUD
		"create_ticket",
		"list_tickets",
		"get_ticket",
		"update_ticket",

		// Ticket lifecycle
		"claim_ticket",
		"start_ticket",
		"log_step",
		"submit_ticket",
		"escalate_ticket",
		"renew_lease",
		"force_release_lease",

		// Review
		"list_pending_reviews",
		"get_trace",
		"approve_ticket",
		"reject_ticket",
		"reopen_ticket",

		// Git notes
		"flywheel_add_git_note",
		"flywheel_show_git_notes",
		"flywheel_log_git_notes",
		"flywheel_diff_git_notes",
		"flywheel_sync_git_notes",

		// Notifications
		"get_notification_preferences",
		"set_notification_preferences",
		"get_dismissal_rates",
	}

	for _, name := range coreTools {
		if !toolNames[name] {
			t.Errorf("Core tool %q not registered", name)
		}
	}

	// Verify a reasonable total count (core tools minus optional providers).
	if len(result.Tools) < len(coreTools) {
		t.Errorf("Total tool count %d is less than expected core tools %d", len(result.Tools), len(coreTools))
	}
}

// TestNewServer_ToolDescriptionsNonEmpty verifies every registered tool has a meaningful description.
func TestNewServer_ToolDescriptionsNonEmpty(t *testing.T) {
	cs := testClientSession(t, &Backend{})
	ctx := context.Background()

	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	for _, tool := range result.Tools {
		if tool.Description == "" {
			t.Errorf("Tool %q has empty description", tool.Name)
		}
		// All descriptions should be at least 20 chars to be useful for agents.
		if len(tool.Description) < 20 {
			t.Errorf("Tool %q description too short (%d chars): %q", tool.Name, len(tool.Description), tool.Description)
		}
	}
}

// TestNewServer_ToolInputSchemasValid verifies every tool has a valid JSON Schema.
func TestNewServer_ToolInputSchemasValid(t *testing.T) {
	cs := testClientSession(t, &Backend{})
	ctx := context.Background()

	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	for _, tool := range result.Tools {
		// InputSchema is any (map[string]any on client side).
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Errorf("Tool %q InputSchema is not map[string]any: %T", tool.Name, tool.InputSchema)
			continue
		}
		if typ, ok := schema["type"]; !ok || typ != "object" {
			t.Errorf("Tool %q InputSchema type = %v, want \"object\"", tool.Name, typ)
		}
		if _, ok := schema["properties"]; !ok {
			t.Errorf("Tool %q InputSchema has no properties", tool.Name)
		}
		if ap, ok := schema["additionalProperties"]; !ok || ap != false {
			t.Errorf("Tool %q InputSchema additionalProperties = %v, want false", tool.Name, ap)
		}
	}
}

// TestNewServer_OptionalProvidersConditional verifies that optional tools are only registered
// when their provider is set on the Backend.
func TestNewServer_OptionalProvidersConditional(t *testing.T) {
	// Without providers — code intel and findings tools should NOT be registered.
	cs := testClientSession(t, &Backend{})
	ctx := context.Background()

	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	// Code intel tools should not be registered without CodeIntel provider.
	codeIntelTools := []string{
		"code_symbol_lookup",
		"code_callers",
		"code_callees",
		"code_blast_radius",
		"code_importers",
	}
	for _, name := range codeIntelTools {
		if toolNames[name] {
			t.Errorf("Tool %q should not be registered without CodeIntel provider", name)
		}
	}

	// Findings tools should not be registered without Findings provider.
	findingsTools := []string{
		"findings_save",
		"findings_query",
		"findings_by_symbol",
		"findings_by_ticket",
		"findings_invalidate",
		"findings_get",
	}
	for _, name := range findingsTools {
		if toolNames[name] {
			t.Errorf("Tool %q should not be registered without Findings provider", name)
		}
	}
}

// TestNewServer_WithCodeIntel verifies code intelligence tools appear when provider is set.
func TestNewServer_WithCodeIntel(t *testing.T) {
	b := &Backend{
		CodeIntel: NewTreeSitterCodeIntel(),
	}
	cs := testClientSession(t, b)
	ctx := context.Background()

	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	codeIntelTools := []string{
		"code_symbol_lookup",
		"code_callers",
		"code_callees",
		"code_blast_radius",
		"code_importers",
	}
	for _, name := range codeIntelTools {
		if !toolNames[name] {
			t.Errorf("Tool %q should be registered when CodeIntel provider is set", name)
		}
	}
}

// TestNewServer_AgentGuideResource verifies the agent guide resource is registered and readable.
func TestNewServer_AgentGuideResource(t *testing.T) {
	cs := testClientSession(t, &Backend{})
	ctx := context.Background()

	result, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources() error = %v", err)
	}

	found := false
	for _, r := range result.Resources {
		if r.URI == AgentGuideURI {
			found = true
			if r.MIMEType != "text/markdown" {
				t.Errorf("Agent guide MIME type = %q, want %q", r.MIMEType, "text/markdown")
			}
			break
		}
	}
	if !found {
		t.Errorf("Agent guide resource not registered at %s", AgentGuideURI)
	}

	// Verify the content is readable.
	readResult, err := cs.ReadResource(ctx, &sdkmcp.ReadResourceParams{URI: AgentGuideURI})
	if err != nil {
		t.Fatalf("ReadResource() error = %v", err)
	}
	if len(readResult.Contents) == 0 {
		t.Fatal("ReadResource returned empty contents")
	}
	content := readResult.Contents[0].Text
	if !strings.Contains(content, "## Setup") {
		t.Error("Agent guide should contain Setup section")
	}
	if !strings.Contains(content, "Claude Code") {
		t.Error("Agent guide should mention Claude Code")
	}
	if !strings.Contains(content, ".claude/settings.json") {
		t.Error("Agent guide should reference .claude/settings.json")
	}
}

// TestAgentGuideContent_WorkflowCompleteness verifies the agent guide covers all key workflows.
func TestAgentGuideContent_WorkflowCompleteness(t *testing.T) {
	requiredSections := []string{
		"## Setup",
		"## Typical flow",
		"## Tool summary",
		"## Ticket states",
		"## Work streams and Git branches",
		"## Coordinator mode",
		"## Worker mode",
		"## Error handling",
	}

	for _, section := range requiredSections {
		if !strings.Contains(AgentGuideContent, section) {
			t.Errorf("Agent guide missing section: %s", section)
		}
	}
}

// TestAgentGuideContent_ToolCoverage verifies the agent guide documents all core tools.
func TestAgentGuideContent_ToolCoverage(t *testing.T) {
	coreTools := []string{
		"list_projects",
		"get_project_context",
		"create_ticket",
		"claim_ticket",
		"start_ticket",
		"log_step",
		"submit_ticket",
		"escalate_ticket",
		"create_work_stream",
		"update_work_stream",
		"approve_ticket",
		"reject_ticket",
	}

	for _, tool := range coreTools {
		if !strings.Contains(AgentGuideContent, tool) {
			t.Errorf("Agent guide does not mention core tool: %s", tool)
		}
	}
}
