package mcp

import "testing"

func TestGetToolScope_ReadTools(t *testing.T) {
	readTools := []string{
		"list_orgs", "list_projects", "get_project_context",
		"list_tickets", "get_ticket", "list_work_streams",
		"get_work_stream", "list_pending_reviews", "get_trace",
		"flywheel_show_git_notes", "flywheel_log_git_notes", "flywheel_diff_git_notes",
	}

	for _, name := range readTools {
		t.Run(name, func(t *testing.T) {
			scope := GetToolScope(name)
			if scope != ToolScopeRead {
				t.Errorf("GetToolScope(%q) = %q, want %q", name, scope, ToolScopeRead)
			}
		})
	}
}

func TestGetToolScope_WriteInternalTools(t *testing.T) {
	writeTools := []string{
		"create_project", "update_project_context", "update_project_status",
		"create_ticket", "update_ticket",
		"create_work_stream", "update_work_stream", "update_work_stream_plan",
		"claim_ticket", "start_ticket", "log_step", "submit_ticket",
		"escalate_ticket", "renew_lease", "force_release_lease",
	}

	for _, name := range writeTools {
		t.Run(name, func(t *testing.T) {
			scope := GetToolScope(name)
			if scope != ToolScopeWriteInternal {
				t.Errorf("GetToolScope(%q) = %q, want %q", name, scope, ToolScopeWriteInternal)
			}
		})
	}
}

func TestGetToolScope_WriteExternalTools(t *testing.T) {
	externalTools := []string{
		"flywheel_add_git_note", "flywheel_sync_git_notes",
	}

	for _, name := range externalTools {
		t.Run(name, func(t *testing.T) {
			scope := GetToolScope(name)
			if scope != ToolScopeWriteExternal {
				t.Errorf("GetToolScope(%q) = %q, want %q", name, scope, ToolScopeWriteExternal)
			}
		})
	}
}

func TestGetToolScope_ForbiddenCoordinatorTools(t *testing.T) {
	forbiddenTools := []string{
		"approve_ticket", "reject_ticket", "reopen_ticket",
	}

	for _, name := range forbiddenTools {
		t.Run(name, func(t *testing.T) {
			scope := GetToolScope(name)
			if scope != ToolScopeForbiddenCoordinator {
				t.Errorf("GetToolScope(%q) = %q, want %q", name, scope, ToolScopeForbiddenCoordinator)
			}
		})
	}
}

func TestGetToolScope_UnknownDefaultsToWriteExternal(t *testing.T) {
	scope := GetToolScope("unknown_tool_name")
	if scope != ToolScopeWriteExternal {
		t.Errorf("GetToolScope(unknown) = %q, want %q (defense-in-depth default)", scope, ToolScopeWriteExternal)
	}
}

func TestIsWriteToolRequiringConfirmation(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// Read tools don't need confirmation
		{"list_tickets", false},
		{"get_ticket", false},
		{"get_project_context", false},
		// Internal write tools don't need confirmation
		{"create_ticket", false},
		{"update_ticket", false},
		{"log_step", false},
		// External write tools need confirmation
		{"flywheel_add_git_note", true},
		{"flywheel_sync_git_notes", true},
		// Forbidden coordinator tools need confirmation (blocked entirely)
		{"approve_ticket", true},
		{"reject_ticket", true},
		// Unknown tools default to needing confirmation
		{"some_new_dangerous_tool", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsWriteToolRequiringConfirmation(tt.name)
			if got != tt.want {
				t.Errorf("IsWriteToolRequiringConfirmation(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestToolScopeRegistryCompleteness(t *testing.T) {
	// Verify that all registered tools in the MCP server are covered by the scope registry.
	// This test documents the expected tool set; if new tools are added without scope
	// classification, they'll default to write_external (safe but restrictive).
	expectedTools := []string{
		"list_orgs", "create_project", "create_work_stream", "list_work_streams",
		"get_work_stream", "update_work_stream", "update_work_stream_plan",
		"create_ticket", "list_projects", "get_project_context",
		"update_project_context", "update_project_status", "list_tickets",
		"get_ticket", "update_ticket", "claim_ticket", "start_ticket",
		"log_step", "submit_ticket", "escalate_ticket", "renew_lease",
		"force_release_lease", "list_pending_reviews", "get_trace",
		"approve_ticket", "reject_ticket", "reopen_ticket",
		"flywheel_add_git_note", "flywheel_show_git_notes",
		"flywheel_log_git_notes", "flywheel_diff_git_notes", "flywheel_sync_git_notes",
	}

	for _, name := range expectedTools {
		if _, ok := ToolScopeRegistry[name]; !ok {
			t.Errorf("tool %q is not in ToolScopeRegistry (will default to write_external)", name)
		}
	}
}
