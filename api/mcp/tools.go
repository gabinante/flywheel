package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabinante/flywheel/api/rest"
	"github.com/gabinante/flywheel/internal/auth"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/queue"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/workstream"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type sessionContextKey struct{}

// ToolScope categorizes MCP tools by their access level for defense-in-depth.
// Read tools need no confirmation; write tools that affect only Flywheel state
// are allowed for coordinators; write tools with external side effects require
// human confirmation; forbidden tools are structurally blocked for coordinators.
type ToolScope string

const (
	// ToolScopeRead indicates a read-only tool with no side effects.
	ToolScopeRead ToolScope = "read"
	// ToolScopeWriteInternal indicates a tool that mutates Flywheel state only (tickets, streams).
	ToolScopeWriteInternal ToolScope = "write_internal"
	// ToolScopeWriteExternal indicates a tool with external side effects requiring confirmation.
	ToolScopeWriteExternal ToolScope = "write_external"
	// ToolScopeForbiddenCoordinator indicates a tool structurally forbidden for coordinator role.
	ToolScopeForbiddenCoordinator ToolScope = "forbidden_coordinator"
)

// ToolScopeRegistry maps tool names to their scope classification.
// This registry enforces defense-in-depth: write tools with external effects
// require human confirmation flows, and forbidden tools are structurally blocked
// for the coordinator role regardless of what external content may instruct.
var ToolScopeRegistry = map[string]ToolScope{
	// Read-only tools (no confirmation needed)
	"list_orgs":               ToolScopeRead,
	"list_projects":           ToolScopeRead,
	"get_project_context":     ToolScopeRead,
	"list_tickets":            ToolScopeRead,
	"get_ticket":              ToolScopeRead,
	"list_work_streams":       ToolScopeRead,
	"get_work_stream":         ToolScopeRead,
	"list_pending_reviews":    ToolScopeRead,
	"get_trace":               ToolScopeRead,
	"flywheel_show_git_notes": ToolScopeRead,
	"flywheel_log_git_notes":  ToolScopeRead,
	"flywheel_diff_git_notes": ToolScopeRead,

	// Write tools — internal Flywheel state only (allowed for coordinator)
	"create_project":          ToolScopeWriteInternal,
	"update_project_context":  ToolScopeWriteInternal,
	"update_project_status":   ToolScopeWriteInternal,
	"create_ticket":           ToolScopeWriteInternal,
	"update_ticket":           ToolScopeWriteInternal,
	"create_work_stream":      ToolScopeWriteInternal,
	"update_work_stream":      ToolScopeWriteInternal,
	"update_work_stream_plan": ToolScopeWriteInternal,

	// Write tools — worker lifecycle (internal but role-scoped)
	"claim_ticket":        ToolScopeWriteInternal,
	"start_ticket":        ToolScopeWriteInternal,
	"log_step":            ToolScopeWriteInternal,
	"submit_ticket":       ToolScopeWriteInternal,
	"escalate_ticket":     ToolScopeWriteInternal,
	"renew_lease":         ToolScopeWriteInternal,
	"force_release_lease": ToolScopeWriteInternal,

	// Write tools — external side effects (require human confirmation)
	"flywheel_add_git_note":   ToolScopeWriteExternal,
	"flywheel_sync_git_notes": ToolScopeWriteExternal,

	// Cancel — coordinators can cancel tickets
	"cancel_ticket": ToolScopeWriteInternal,

	// Review tools — human-gated by design
	"approve_ticket":       ToolScopeForbiddenCoordinator,
	"reject_ticket":        ToolScopeForbiddenCoordinator,
	"reopen_ticket":        ToolScopeForbiddenCoordinator,
	"resolve_ticket_input": ToolScopeForbiddenCoordinator,

	// Workflow tools (read-only)
	"get_workflow_position":   ToolScopeRead,
	"list_workflow_templates": ToolScopeRead,

	// Project template tools (read-only)
	"list_project_templates":    ToolScopeRead,
	"list_workstream_templates": ToolScopeRead,
}

// GetToolScope returns the scope classification for a tool name.
// Unknown tools default to ToolScopeWriteExternal (require confirmation).
func GetToolScope(toolName string) ToolScope {
	if scope, ok := ToolScopeRegistry[toolName]; ok {
		return scope
	}
	// Default: unknown tools require confirmation (defense-in-depth)
	return ToolScopeWriteExternal
}

// IsWriteToolRequiringConfirmation returns true if the tool has external side effects
// and should require human confirmation before execution.
func IsWriteToolRequiringConfirmation(toolName string) bool {
	scope := GetToolScope(toolName)
	return scope == ToolScopeWriteExternal || scope == ToolScopeForbiddenCoordinator
}

// wrapFn is the signature for the wrap closure used when registering tools.
type wrapFn = func(func(*Backend, context.Context, map[string]any) (*mcp.CallToolResult, any, error)) func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error)

// RegisterTools adds all Flywheel MCP tools to the MCP server (official go-sdk).
func RegisterTools(s *mcp.Server, b *Backend) {
	if b == nil {
		return
	}
	wrap := func(f func(*Backend, context.Context, map[string]any) (*mcp.CallToolResult, any, error)) func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
		return func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			if req != nil && req.Session != nil {
				ctx = context.WithValue(ctx, sessionContextKey{}, req.Session)
			}
			return f(b, ctx, args)
		}
	}

	mcp.AddTool(s, &mcp.Tool{Name: "list_orgs", Description: "List organizations you belong to. Returns id, name, slug for each. Requires OAuth. Use this to see your workspaces (e.g. personal org, or teams you were added to).", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"additionalProperties": false,
	}}, wrap(listOrgsHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "create_project", Description: "Create a project in your default (first) organization. Use for initiatives, epics, or any work container. You do not pass org_id; the project is created in an org you belong to. Optionally seed from a project template: pass template_id to include all its workstreams, or template_id + work_stream_template_ids to select a subset.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":                     map[string]any{"type": "string", "description": "Project name"},
			"slug":                     map[string]any{"type": "string", "description": "URL-friendly slug (optional, auto-generated if omitted)"},
			"agent_id":                 map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
			"template_id":              map[string]any{"type": "string", "description": "Project template ID to seed workstreams and tickets from (optional). Use list_project_templates to see available templates."},
			"work_stream_template_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Specific workstream template IDs to include (optional). If omitted with template_id, all template workstreams are included."},
		},
		"required":             []string{"name"},
		"additionalProperties": false,
	}}, wrap(createProjectHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "create_work_stream", Description: "Create a work stream in a project. Work streams group tickets toward a goal (e.g. 'Productionize feature A'). When the project has repo_url: Flywheel does NOT create a Git branch. The response includes git_instruction—follow it immediately: create or checkout the branch in the repo, then call update_work_stream with branch (required in that session; do not stop after only setting plan text). Until branch is set, claim_ticket/get_ticket repeat create_or_set_branch. Params: project_id, name (required), slug (optional), plan (optional Markdown). Returns work_stream and optional git_instruction.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project ID"},
			"name":       map[string]any{"type": "string", "description": "Work stream name"},
			"slug":       map[string]any{"type": "string", "description": "URL-friendly slug (optional, auto-generated if omitted)"},
			"plan":       map[string]any{"type": "string", "description": "Markdown plan for the work stream (optional)"},
			"agent_id":   map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"project_id", "name"},
		"additionalProperties": false,
	}}, wrap(createWorkStreamHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "list_work_streams", Description: "List work streams for a project. Params: project_id, optional status (active, closed, all).", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project ID"},
			"status":     map[string]any{"type": "string", "description": "Filter by status: active, closed, or all (default: active)", "enum": []string{"active", "closed", "all"}},
			"agent_id":   map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"project_id"},
		"additionalProperties": false,
	}}, wrap(listWorkStreamsHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "get_work_stream", Description: "Get a work stream by ID. Params: project_id, work_stream_id. When project has repo_url, response includes git_instruction: if branch is empty, you must create/checkout the branch and call update_work_stream with branch (plan updates alone are insufficient).", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project ID"},
			"work_stream_id": map[string]any{"type": "string", "description": "Work stream ID"},
			"agent_id":       map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"project_id", "work_stream_id"},
		"additionalProperties": false,
	}}, wrap(getWorkStreamHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "update_work_stream", Description: "Update a work stream (name, plan, branch, status). When project has repo_url: you MUST set branch (pass the real Git branch name) as soon as you create or checkout that branch—this is easy to forget if you only update the Markdown plan. update_work_stream_plan does NOT set branch. Omit branch only to leave the stored branch unchanged. When closing (status=closed) and project has repo_url, returns git_instruction to checkout default branch. Params: project_id, work_stream_id, optional name, plan, branch, status (active|closed).", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project ID"},
			"work_stream_id": map[string]any{"type": "string", "description": "Work stream ID"},
			"name":           map[string]any{"type": "string", "description": "New name (optional, keeps current if omitted)"},
			"plan":           map[string]any{"type": "string", "description": "Markdown plan (optional, keeps current if omitted)"},
			"branch":         map[string]any{"type": "string", "description": "Git branch name (optional, keeps current if omitted)"},
			"status":         map[string]any{"type": "string", "description": "Status: active or closed (optional, keeps current if omitted)", "enum": []string{"active", "closed"}},
			"agent_id":       map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"project_id", "work_stream_id"},
		"additionalProperties": false,
	}}, wrap(updateWorkStreamHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "update_work_stream_plan", Description: "Replace only the work stream's Markdown plan. Does NOT set or change the Git branch—if repo_url is set and branch is still empty, you must still call update_work_stream with branch after creating/checking out the branch. Params: project_id, work_stream_id, plan (required). Returns work_stream and optional git_instruction when repo_url is set and stream is active.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project ID"},
			"work_stream_id": map[string]any{"type": "string", "description": "Work stream ID"},
			"plan":           map[string]any{"type": "string", "description": "New Markdown plan content"},
			"agent_id":       map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"project_id", "work_stream_id", "plan"},
		"additionalProperties": false,
	}}, wrap(updateWorkStreamPlanHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "create_ticket", Description: "Create a ticket in a project. Response JSON has **ticket** (the new ticket) and **workflow** (next_steps + note) to remind you to claim → start → log_step → submit. The ticket is created as pending; agents claim via claim_ticket. created_by is set to your agent identity. **description:** pass the ticket description either as a top-level \"description\" param or nested inside an \"objective\" object as objective.description — both work. The value is stored as objective.description on the ticket. **work_stream_id:** pass when the work belongs to a stream—otherwise the ticket will not appear in the web UI when that stream is filtered (use **update_ticket** later to attach). If set and project has repo_url, the work stream must already have **branch** set via **update_work_stream** after you create/checkout that branch (plan-only updates do not count). **target_repo:** for multi-repo projects, pass the repo alias (from list_project_repositories) to target a specific repo; omit for the primary repo. Optional idempotency_key.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string", "description": "Project ID"},
			"title":       map[string]any{"type": "string", "description": "Ticket title"},
			"description": map[string]any{"type": "string", "description": "Ticket description — stored as objective.description on the created ticket. You can pass this at the top level OR nest it inside the \"objective\" parameter."},
			"objective": map[string]any{"type": "object", "description": "Alternative: pass the objective as a nested object. objective.description is equivalent to the top-level description param. If both are provided, top-level description wins.", "properties": map[string]any{
				"description":      map[string]any{"type": "string", "description": "Ticket description (same as top-level description)"},
				"success_criteria": map[string]any{"type": "array", "description": "Array of success criteria strings (optional)", "items": map[string]any{"type": "string"}},
				"acceptance_test":  map[string]any{"type": "string", "description": "Acceptance test description (optional)"},
			}},
			"ticket_type":      map[string]any{"type": "string", "description": "Ticket type: task, bug, spike, or review (default: task)", "enum": []string{"task", "bug", "spike", "review"}},
			"priority":         map[string]any{"type": "integer", "description": "Priority 0-3 (0=P0 highest, default: 2)", "minimum": 0, "maximum": 3},
			"success_criteria": map[string]any{"type": "string", "description": "JSON array of success criteria strings (optional). Also accepted inside the objective parameter."},
			"acceptance_test":  map[string]any{"type": "string", "description": "Acceptance test description (optional). Also accepted inside the objective parameter."},
			"idempotency_key":  map[string]any{"type": "string", "description": "Idempotency key to prevent duplicate creation (optional)"},
			"work_stream_id":   map[string]any{"type": "string", "description": "Work stream ID to attach this ticket to (optional)"},
			"depends_on":       map[string]any{"type": "array", "description": "Array of ticket IDs this ticket depends on (optional)", "items": map[string]any{"type": "string"}},
			"target_repo":      map[string]any{"type": "string", "description": "Target repository alias for multi-repo projects (optional, from list_project_repositories; omit for primary repo)"},
			"workflow_id":      map[string]any{"type": "string", "description": "Workflow template to assign (optional). Subtickets should use a simpler workflow like 'Subticket SDLC' or 'Fast Track'. If omitted, uses project default."},
			"inputs":           map[string]any{"type": "object", "description": "Key-value pairs for ticket inputs (optional). Use for metadata like decomposed_from.", "additionalProperties": true},
			"agent_id":         map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"project_id", "title"},
		"additionalProperties": false,
	}}, wrap(createTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "list_projects", Description: "List projects for the authenticated user's organization(s). Requires OAuth (agent linked to a user). Returns only active projects by default. Pass include_closed: true to include closed projects. Optionally pass org_id to limit to one org (must be an org you belong to).", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"org_id":         map[string]any{"type": "string", "description": "Organization ID to filter by (optional, must be an org you belong to)"},
			"include_closed": map[string]any{"type": "boolean", "description": "Include closed projects (default: false)"},
			"agent_id":       map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"additionalProperties": false,
	}}, wrap(listProjectsHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "get_project_context", Description: "Return the full context pack for a project: conventions, key files, system prompt, and extra hints. Call this after list_projects to load the project's context before claiming or inspecting tickets.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project ID"},
		},
		"required":             []string{"project_id"},
		"additionalProperties": false,
	}}, wrap(getProjectContextHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "update_project_context", Description: "Update a project's context pack. Pass project_id and any of: conventions (string), system_prompt (string), key_files (JSON array of {path, snippet}), extra (JSON object of string key-value pairs). Merges with existing context pack. Returns the updated context pack.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project ID"},
			"conventions":   map[string]any{"type": "string", "description": "Conventions text (optional, merges with existing)"},
			"system_prompt": map[string]any{"type": "string", "description": "System prompt (optional, replaces existing)"},
			"key_files":     map[string]any{"type": "array", "description": "JSON array of {path, snippet} objects (optional, replaces existing)", "items": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "snippet": map[string]any{"type": "string"}}, "required": []string{"path", "snippet"}}},
			"extra":         map[string]any{"type": "object", "description": "JSON object of string key-value pairs (optional, replaces existing)", "additionalProperties": map[string]any{"type": "string"}},
			"agent_id":      map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"project_id"},
		"additionalProperties": false,
	}}, wrap(updateProjectContextHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "update_project_status", Description: "Set a project's status to active or closed. Use to close a project when work is done, or reopen it (set to active) for follow-up. Requires OAuth and org access. Pass project_id and status (\"active\" or \"closed\"). Returns the updated project.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project ID"},
			"status":     map[string]any{"type": "string", "description": "New status: active or closed", "enum": []string{"active", "closed"}},
			"agent_id":   map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"project_id", "status"},
		"additionalProperties": false,
	}}, wrap(updateProjectStatusHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "list_tickets", Description: "List tickets for a project. Response JSON has **tickets** (array). When you omit **state** or set state=pending, **workflow** (next_steps + note) is included to nudge claim → start → submit. Optionally filter by state, priority (0–3), or work_stream_id. Use after get_project_context to see what work is available.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project ID"},
			"work_stream_id": map[string]any{"type": "string", "description": "Filter by work stream ID (optional)"},
			"state":          map[string]any{"type": "string", "description": "Filter by state (optional)", "enum": []string{"draft", "planning", "executing", "awaiting_input", "awaiting_validation", "validated", "closed"}},
			"priority":       map[string]any{"type": "integer", "description": "Filter by priority 0-3 (optional)", "minimum": 0, "maximum": 3},
		},
		"required":             []string{"project_id"},
		"additionalProperties": false,
	}}, wrap(listTicketsHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "get_ticket", Description: "Get the full ticket payload (ticket_id also accepts a Linear identifier such as RLETD-465 when the project mirrors Linear): objective, success criteria, acceptance test, context pack, dependency outputs (from tickets this one depends on), prior attempts, and human answers. This is the main input for doing the work. Call after claim_ticket and before start_ticket to load everything you need. If the ticket has a work_stream and the project has repo_url, the response may include git_instruction (checkout branch, or create branch + update_work_stream if branch is not set yet).", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
		},
		"required":             []string{"ticket_id"},
		"additionalProperties": false,
	}}, wrap(getTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "update_ticket", Description: "Update ticket metadata. Pass project_id and ticket_id. Optional: depends_on (JSON array string), work_stream_id. For title/objective text, use REST PATCH /tickets/{id} with JSON body (title, objective partial merge) or recreate the ticket.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project ID"},
			"ticket_id":      map[string]any{"type": "string", "description": "Ticket ID"},
			"depends_on":     map[string]any{"type": "array", "description": "Array of ticket ID strings this ticket depends on (optional)", "items": map[string]any{"type": "string"}},
			"work_stream_id": map[string]any{"type": "string", "description": "Work stream ID to attach this ticket to (optional)"},
			"agent_id":       map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"project_id", "ticket_id"},
		"additionalProperties": false,
	}}, wrap(updateTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "claim_ticket", Description: "Claim the next available ticket in the queue for a project. Returns the ticket, lease (lease_token, expires_at), and **workflow** (next_steps + note). If the ticket has a work_stream and the project has repo_url, the response may include git_instruction (checkout branch, or create branch + update_work_stream if branch is not set yet). Optional idempotency_key: retries with the same key return the same ticket/lease (renewed if still valid) so the same agent does not claim a different ticket. You must start_ticket and then either submit_ticket or escalate_ticket before the lease expires, or renew_lease to extend. agent_id is inferred from OAuth when using URL auth.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":      map[string]any{"type": "string", "description": "Project ID"},
			"priority":        map[string]any{"type": "integer", "description": "Claim only tickets at this priority level 0-3 (optional, claims next available if omitted)", "minimum": 0, "maximum": 3},
			"idempotency_key": map[string]any{"type": "string", "description": "Idempotency key to prevent claiming multiple tickets (optional)"},
			"agent_id":        map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"project_id"},
		"additionalProperties": false,
	}}, wrap(claimTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "start_ticket", Description: "Move the ticket from claimed to executing. Response includes **workflow** (next_steps + note). Call after claim_ticket and get_ticket when you are ready to do the work. Requires the lease_token from claim_ticket. agent_id is inferred from OAuth when using URL auth.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id":   map[string]any{"type": "string", "description": "Ticket ID"},
			"lease_token": map[string]any{"type": "string", "description": "Lease token from claim_ticket"},
			"agent_id":    map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"ticket_id", "lease_token"},
		"additionalProperties": false,
	}}, wrap(startTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "log_step", Description: "Append a step to the execution trace. Call this as you work—after each significant tool use (step_type tool_call, payload as object e.g. {\"name\":\"write\",\"input\":{\"path\":\"...\"}}), for key observations or decisions (observation/thought), and on errors (error). payload can be a JSON object or JSON string. Reviewers see this trace when approving the ticket; call it regularly so they know what was done.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id":   map[string]any{"type": "string", "description": "Ticket ID"},
			"lease_token": map[string]any{"type": "string", "description": "Lease token from claim_ticket"},
			"step_type":   map[string]any{"type": "string", "description": "Step type: tool_call, observation, thought, or error", "enum": []string{"tool_call", "observation", "thought", "error"}},
			"payload":     map[string]any{"type": "object", "description": "Step payload as a JSON object (or JSON string)"},
			"worker_type": map[string]any{"type": "string", "description": "Worker type that produced this step (planner, executor, validator, deployer, investigator). Optional — auto-recorded from dispatch context.", "enum": []string{"planner", "executor", "validator", "deployer", "investigator"}},
		},
		"required":             []string{"ticket_id", "lease_token", "step_type"},
		"additionalProperties": false,
	}}, wrap(logStepHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "submit_ticket", Description: "Submit your outputs and move the ticket to awaiting_validation. outputs must be a JSON object with a top-level pr_url if you opened a PR (e.g. {\"summary\":\"...\", \"pr_url\":\"https://github.com/org/repo/pull/123\"}). A human will approve or reject via the REST API. Call when the work is done.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id":   map[string]any{"type": "string", "description": "Ticket ID"},
			"lease_token": map[string]any{"type": "string", "description": "Lease token from claim_ticket"},
			"outputs":     map[string]any{"type": "string", "description": "JSON object string with outputs. Include pr_url at top level if a PR was opened (e.g. {\"summary\":\"...\", \"pr_url\":\"https://github.com/org/repo/pull/123\"})"},
		},
		"required":             []string{"ticket_id", "lease_token", "outputs"},
		"additionalProperties": false,
	}}, wrap(submitTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "escalate_ticket", Description: "Escalate to a human when you need help. Moves the ticket to awaiting_input. Provide a reason and a specific question; the human's answer is stored and the ticket returns to executing so you can continue. Use when blocked or when the objective is ambiguous.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id":   map[string]any{"type": "string", "description": "Ticket ID"},
			"lease_token": map[string]any{"type": "string", "description": "Lease token from claim_ticket"},
			"reason":      map[string]any{"type": "string", "description": "Reason for escalation"},
			"question":    map[string]any{"type": "string", "description": "Specific question for the human"},
		},
		"required":             []string{"ticket_id", "lease_token", "reason", "question"},
		"additionalProperties": false,
	}}, wrap(escalateTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "renew_lease", Description: "Extend the lease TTL so the ticket is not returned to the queue. Call periodically while working if the job takes longer than the lease duration. Returns the new expires_at.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id":   map[string]any{"type": "string", "description": "Ticket ID"},
			"lease_token": map[string]any{"type": "string", "description": "Lease token from claim_ticket"},
		},
		"required":             []string{"ticket_id", "lease_token"},
		"additionalProperties": false,
	}}, wrap(renewLeaseHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "force_release_lease", Description: "Force-release a ticket's lease (no token needed). Use when the user directs you to release a stuck ticket so you can claim it in this session (e.g. 'release agent-reliability-3 and claim it'). Caller must have access to the ticket's project. Ticket returns to pending; then use claim_ticket to claim it.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID to force-release"},
			"agent_id":  map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"ticket_id"},
		"additionalProperties": false,
	}}, wrap(forceReleaseLeaseHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "list_pending_reviews", Description: "List tickets in awaiting_validation for a project. Use this when the user asks 'what needs my review?' or 'show pending reviews'. Returns full tickets so you can summarize them in chat; use get_trace(ticket_id) to show execution steps for each.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project ID"},
			"agent_id":   map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"project_id"},
		"additionalProperties": false,
	}}, wrap(listPendingReviewsHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "get_trace", Description: "Get the execution trace for a ticket (all log_step entries). Use when summarizing a ticket for review so the user can see what was done before approving or rejecting.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
		},
		"required":             []string{"ticket_id"},
		"additionalProperties": false,
	}}, wrap(getTraceHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "approve_ticket", Description: "Approve a ticket in awaiting_validation. Moves it to validated. Call when the user says to approve, ship it, looks good, etc. reviewer_id is inferred from OAuth.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
			"notes":     map[string]any{"type": "string", "description": "Reviewer notes (optional)"},
			"agent_id":  map[string]any{"type": "string", "description": "Reviewer ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"ticket_id"},
		"additionalProperties": false,
	}}, wrap(approveTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "reject_ticket", Description: "Reject a ticket in awaiting_validation. Returns it to executing with your notes appended so the agent can fix and resubmit. Call when the user says reject, needs changes, etc. reviewer_id is inferred from OAuth.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
			"notes":     map[string]any{"type": "string", "description": "Rejection notes explaining what to fix"},
			"agent_id":  map[string]any{"type": "string", "description": "Reviewer ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"ticket_id", "notes"},
		"additionalProperties": false,
	}}, wrap(rejectTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "reopen_ticket", Description: "Move a ticket from closed back to draft (e.g. mistaken closure). Preserves outputs. Only call when the user explicitly asks to reopen a completed ticket. reviewer_id is inferred from OAuth.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
			"notes":     map[string]any{"type": "string", "description": "Notes explaining why the ticket is being reopened (optional)"},
			"agent_id":  map[string]any{"type": "string", "description": "Reviewer ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"ticket_id"},
		"additionalProperties": false,
	}}, wrap(reopenTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "cancel_ticket", Description: "Cancel a ticket from any early or blocked state (draft, planning, executing, awaiting_input, awaiting_validation). Moves it to closed. Use when a ticket is no longer needed or should be abandoned.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
			"reason":    map[string]any{"type": "string", "description": "Reason for cancellation"},
			"agent_id":  map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"ticket_id", "reason"},
		"additionalProperties": false,
	}}, wrap(cancelTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "resolve_ticket_input", Description: "Resolve a ticket stuck in awaiting_input by providing an answer and moving it back to planning or executing. Auto-resolves the latest unresolved escalation. Use when a human has the answer to an agent's escalation question.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
			"answer":    map[string]any{"type": "string", "description": "Answer to the agent's question"},
			"resume_to": map[string]any{"type": "string", "description": "State to resume to: planning (default) or executing", "enum": []string{"planning", "executing"}},
			"agent_id":  map[string]any{"type": "string", "description": "Reviewer ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required":             []string{"ticket_id", "answer"},
		"additionalProperties": false,
	}}, wrap(resolveTicketInputHandler))

	// Git notes (Flywheel integration): if repo_path provided and server has access, run git notes; else return commands for flywheel-git CLI.

	// --- Claims registry tools (spec v0.2 §4.3) ---

	// --- Notification tools ---

	// Pillar and strategy layer (Layer 15).

	// Workflow tools.
	registerWorkflowTools(s, b, wrap)

	// Project template tools.
}

func requireString(args map[string]any, key string) (string, error) {
	if args == nil {
		return "", fmt.Errorf("%s required", key)
	}
	v, ok := args[key]
	if !ok || v == nil {
		return "", fmt.Errorf("%s required", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return s, nil
}

// getStringArray extracts a []string from args[key]. It handles three formats:
// 1. Native JSON array ([]interface{} from JSON decoding) — e.g. MCP clients sending ["a","b"]
// 2. JSON-encoded string — e.g. "[\"a\",\"b\"]"
// 3. Missing/nil — returns nil
func getStringArray(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	// Case 1: native JSON array ([]interface{} from JSON decoding)
	if arr, ok := v.([]interface{}); ok {
		out := make([]string, 0, len(arr))
		for _, elem := range arr {
			if s, ok := elem.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	// Case 2: already a []string (rare, but possible)
	if arr, ok := v.([]string); ok {
		return arr
	}
	// Case 3: JSON-encoded string containing an array
	if s, ok := v.(string); ok && s != "" {
		var out []string
		if err := json.Unmarshal([]byte(s), &out); err == nil {
			return out
		}
	}
	return nil
}

func getString(args map[string]any, key, def string) string {
	if args == nil {
		return def
	}
	v, ok := args[key]
	if !ok || v == nil {
		return def
	}
	s, ok := v.(string)
	if !ok {
		return def
	}
	return s
}

func getInt(args map[string]any, key string, def int) int {
	if args == nil {
		return def
	}
	v, ok := args[key]
	if !ok || v == nil {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return def
	}
}

func getBool(args map[string]any, key string, def bool) bool {
	if args == nil {
		return def
	}
	v, ok := args[key]
	if !ok || v == nil {
		return def
	}
	b, ok := v.(bool)
	if !ok {
		return def
	}
	return b
}

// getPayloadMap returns the payload for log_step as map[string]any. Accepts either a JSON string or an object so MCP clients can send payload as object and it is stored.
func getPayloadMap(args map[string]any, key string) map[string]any {
	if args == nil {
		return map[string]any{}
	}
	v, ok := args[key]
	if !ok || v == nil {
		return map[string]any{}
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	if s, ok := v.(string); ok && s != "" {
		var out map[string]any
		if json.Unmarshal([]byte(s), &out) == nil {
			return out
		}
	}
	return map[string]any{}
}

func getAgentIDFromArgs(ctx context.Context, args map[string]any) (string, error) {
	// HTTP auth context is authoritative — never let args override the
	// authenticated identity (prevents impersonation via agent_id arg).
	if id := rest.GetAgentID(ctx); id != "" {
		return id, nil
	}
	// Stdio / non-HTTP transports: accept agent_id from args since there is
	// no HTTP auth layer.
	if id := getString(args, "agent_id", ""); id != "" {
		return id, nil
	}
	// Stdio fallback: resolve agent ID from FLYWHEEL_TOKEN env var (JWT).
	if token := os.Getenv("FLYWHEEL_TOKEN"); token != "" {
		if secret := os.Getenv("JWT_SECRET"); secret != "" {
			if id, err := auth.VerifyJWT(secret, token); err == nil && id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("agent_id required (inferred from OAuth when using URL auth, or pass in request)")
}

// stuckTicketIDs returns ticket IDs in state claimed or executing for the project (so the user can direct force-release).
func stuckTicketIDs(ctx context.Context, b *Backend, projectID string) []string {
	claimed, _ := b.Ticket.ListByState(ctx, projectID, ticket.StatePlanning)
	executing, _ := b.Ticket.ListByState(ctx, projectID, ticket.StateExecuting)
	seen := make(map[string]bool)
	var ids []string
	for _, t := range claimed {
		if !seen[t.ID] {
			seen[t.ID] = true
			ids = append(ids, t.ID)
		}
	}
	for _, t := range executing {
		if !seen[t.ID] {
			seen[t.ID] = true
			ids = append(ids, t.ID)
		}
	}
	return ids
}

// sessionFromContext returns the MCP ServerSession from ctx if present (for elicitation).
func sessionFromContext(ctx context.Context) *mcp.ServerSession {
	v := ctx.Value(sessionContextKey{})
	if v == nil {
		return nil
	}
	ss, _ := v.(*mcp.ServerSession)
	return ss
}

func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), false))
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

func toolErrTriple(se *apierrors.StructuredError) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: se.JSON()}}}, nil, nil
}

func listOrgsHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	agent, err := b.AgentStore.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "agent not found", false))
	}
	if agent.UserID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "list_orgs requires OAuth login (agent must be linked to a user). Use GitHub sign-in via MCP URL auth.", false))
	}
	orgs, err := b.Org.ListOrgsForUser(ctx, agent.UserID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	if orgs == nil {
		orgs = []*org.Org{}
	}
	return jsonResult(orgs)
}

func createProjectHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	agent, err := b.AgentStore.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "agent not found", false))
	}
	if agent.UserID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "create_project requires OAuth login.", false))
	}
	orgs, err := b.Org.ListOrgsForUser(ctx, agent.UserID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	if len(orgs) == 0 {
		return toolErrTriple(apierrors.New(apierrors.CodeForbidden, "you have no organization; sign in again to ensure your default org exists", false))
	}
	name, err := requireString(args, "name")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	slug := getString(args, "slug", "")
	// Prefer personal org (slug "u-..."); otherwise use first org
	var targetOrg *org.Org
	for _, o := range orgs {
		if strings.HasPrefix(o.Slug, "u-") {
			targetOrg = o
			break
		}
	}
	if targetOrg == nil {
		targetOrg = orgs[0]
	}
	p, err := b.Project.CreateProject(ctx, targetOrg.ID, name, slug, "", nil)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}

	return jsonResult(map[string]any{"project": p})
}

// workStreamGitInstruction returns checkout/create guidance when the project is git-backed.
// If ws.Branch is set, agents should check out that branch; otherwise suggest feature/<slug> and update_work_stream.
func workStreamGitInstruction(proj *project.Project, ws *workstream.WorkStream) map[string]any {
	if proj == nil || ws == nil || proj.RepoURL == "" {
		return nil
	}
	if ws.Branch != "" {
		return map[string]any{
			"enabled":          true,
			"action":           "checkout_branch",
			"suggested_branch": ws.Branch,
			"message":          "**Required:** Check out branch '" + ws.Branch + "' before start_ticket or committing. If it does not exist: `git checkout -b " + ws.Branch + "`. The work stream is already linked to this branch in Flywheel.",
		}
	}
	if ws.Slug == "" {
		return nil
	}
	suggestedBranch := "feature/" + ws.Slug
	return map[string]any{
		"enabled":          true,
		"action":           "create_or_set_branch",
		"suggested_branch": suggestedBranch,
		"message":          "**Mandatory next step (do not skip):** Create or choose a branch—e.g. `git checkout -b " + suggestedBranch + "`—or use `git branch --show-current` if you are already on the right branch. Then call **update_work_stream** (project_id, work_stream_id, branch) with that exact name. Updating **plan** only does NOT set branch; until you call **update_work_stream** with **branch**, **claim_ticket** / **get_ticket** will keep showing this reminder.",
	}
}

func workStreamJSONWithGit(proj *project.Project, ws *workstream.WorkStream) (map[string]any, error) {
	raw, err := json.Marshal(ws)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	if gi := workStreamGitInstruction(proj, ws); gi != nil {
		out["git_instruction"] = gi
	}
	return out, nil
}

func attachWorkStreamGit(out map[string]any, proj *project.Project, ws *workstream.WorkStream) {
	if gi := workStreamGitInstruction(proj, ws); gi != nil {
		out["git_instruction"] = gi
	}
}

func createWorkStreamHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.WorkStream == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "work stream service not configured", false))
	}
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	name, err := requireString(args, "name")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	if err := checkProjectAccess(ctx, b, agentID, projectID); err != nil {
		return toolErrTriple(err)
	}
	slug := getString(args, "slug", "")
	plan := getString(args, "plan", "")
	w, err := b.WorkStream.CreateWorkStream(ctx, projectID, name, slug, plan)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	out := map[string]any{"work_stream": w}
	if proj, _ := b.Project.GetProject(ctx, projectID); proj != nil {
		attachWorkStreamGit(out, proj, w)
	}
	return jsonResult(out)
}

func listWorkStreamsHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.WorkStream == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "work stream service not configured", false))
	}
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	if err := checkProjectAccess(ctx, b, agentID, projectID); err != nil {
		return toolErrTriple(err)
	}
	statusFilter := getString(args, "status", "active")
	list, err := b.WorkStream.ListWorkStreams(ctx, projectID, statusFilter)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(list)
}

func getWorkStreamHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.WorkStream == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "work stream service not configured", false))
	}
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	workStreamID, err := requireString(args, "work_stream_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	if err := checkProjectAccess(ctx, b, agentID, projectID); err != nil {
		return toolErrTriple(err)
	}
	w, err := b.WorkStream.GetWorkStream(ctx, workStreamID)
	if err != nil {
		if errors.Is(err, workstream.ErrWorkStreamNotFound) {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "work stream not found", false))
		}
		return toolErrTriple(apierrors.MapError(err))
	}
	if w.ProjectID != projectID {
		return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "work stream not found", false))
	}
	proj, _ := b.Project.GetProject(ctx, projectID)
	if m, err := workStreamJSONWithGit(proj, w); err == nil {
		return jsonResult(m)
	}
	return jsonResult(w)
}

func updateWorkStreamHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.WorkStream == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "work stream service not configured", false))
	}
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	workStreamID, err := requireString(args, "work_stream_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	if err := checkProjectAccess(ctx, b, agentID, projectID); err != nil {
		return toolErrTriple(err)
	}
	w, err := b.WorkStream.GetWorkStream(ctx, workStreamID)
	if err != nil {
		if errors.Is(err, workstream.ErrWorkStreamNotFound) {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "work stream not found", false))
		}
		return toolErrTriple(apierrors.MapError(err))
	}
	if w.ProjectID != projectID {
		return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "work stream not found", false))
	}
	name := getString(args, "name", w.Name)
	plan := getString(args, "plan", w.Plan)
	branch := getString(args, "branch", w.Branch)
	status := getString(args, "status", w.Status)
	if status == "" {
		status = w.Status
	}
	if err := b.WorkStream.UpdateWorkStream(ctx, workStreamID, name, plan, branch, status); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	updated, _ := b.WorkStream.GetWorkStream(ctx, workStreamID)
	out := map[string]any{"work_stream": updated}
	proj, _ := b.Project.GetProject(ctx, projectID)
	if status != "closed" && proj != nil && updated != nil {
		attachWorkStreamGit(out, proj, updated)
	}
	if status == "closed" {
		if proj != nil && proj.RepoURL != "" {
			defaultBranch := proj.DefaultBranch
			if defaultBranch == "" {
				defaultBranch = "main"
			}
			out["git_instruction"] = map[string]any{
				"enabled": true,
				"action":  "checkout_default_branch",
				"branch":  defaultBranch,
				"message": "Work stream closed. Checkout default branch with `git checkout " + defaultBranch + "`",
			}
		}
	}
	return jsonResult(out)
}

func updateWorkStreamPlanHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.WorkStream == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "work stream service not configured", false))
	}
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	workStreamID, err := requireString(args, "work_stream_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	plan, err := requireString(args, "plan")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	if err := checkProjectAccess(ctx, b, agentID, projectID); err != nil {
		return toolErrTriple(err)
	}
	w, err := b.WorkStream.GetWorkStream(ctx, workStreamID)
	if err != nil {
		if errors.Is(err, workstream.ErrWorkStreamNotFound) {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "work stream not found", false))
		}
		return toolErrTriple(apierrors.MapError(err))
	}
	if w.ProjectID != projectID {
		return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "work stream not found", false))
	}
	if err := b.WorkStream.UpdateWorkStream(ctx, workStreamID, w.Name, plan, w.Branch, w.Status); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	updated, _ := b.WorkStream.GetWorkStream(ctx, workStreamID)
	out := map[string]any{"work_stream": updated}
	proj, _ := b.Project.GetProject(ctx, projectID)
	if w.Status != "closed" && proj != nil && updated != nil {
		attachWorkStreamGit(out, proj, updated)
	}
	return jsonResult(out)
}

func checkProjectAccess(ctx context.Context, b *Backend, agentID, projectID string) *apierrors.StructuredError {
	agent, err := b.AgentStore.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		return apierrors.New(apierrors.CodeUnauthorized, "agent not found", false)
	}
	if agent.UserID == "" {
		return apierrors.New(apierrors.CodeUnauthorized, "OAuth login required", false)
	}
	proj, err := b.Project.GetProject(ctx, projectID)
	if err != nil {
		return apierrors.MapError(err)
	}
	orgIDs, err := b.Org.ListOrgIDsForUser(ctx, agent.UserID)
	if err != nil {
		return apierrors.MapError(err)
	}
	for _, id := range orgIDs {
		if id == proj.OrgID {
			return nil
		}
	}
	return apierrors.New(apierrors.CodeForbidden, "you do not have access to that project", false)
}

func createTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	title, err := requireString(args, "title")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	// Accept description from top-level "description" or nested "objective.description".
	description := getString(args, "description", "")
	if description == "" {
		if obj, ok := args["objective"].(map[string]any); ok {
			if d, ok := obj["description"].(string); ok {
				description = d
			}
		}
	}
	if description == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "description required (pass as top-level \"description\" or inside \"objective.description\")", false))
	}
	// Verify user has access to the project (project's org is one of user's orgs)
	proj, err := b.Project.GetProject(ctx, projectID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	agent, err := b.AgentStore.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "agent not found", false))
	}
	if agent.UserID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "create_ticket requires OAuth login.", false))
	}
	orgIDs, err := b.Org.ListOrgIDsForUser(ctx, agent.UserID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	allowed := false
	for _, id := range orgIDs {
		if id == proj.OrgID {
			allowed = true
			break
		}
	}
	if !allowed {
		return toolErrTriple(apierrors.New(apierrors.CodeForbidden, "you do not have access to that project", false))
	}
	if proj.Status == "closed" {
		return toolErrTriple(apierrors.New(apierrors.CodeProjectClosed, "project is closed; reopen it with update_project_status or choose another project", false))
	}
	typ := ticket.TypeTask
	if t := getString(args, "ticket_type", ""); t != "" {
		switch t {
		case "task", "bug", "spike", "review":
			typ = ticket.TicketType(t)
		}
	}
	prio := ticket.P2
	if p := getInt(args, "priority", -1); p >= 0 && p <= 3 {
		prio = ticket.Priority(p)
	}
	var successCriteria []string
	if s := getString(args, "success_criteria", ""); s != "" {
		_ = json.Unmarshal([]byte(s), &successCriteria)
	}
	// Also accept success_criteria and acceptance_test from nested objective.
	if len(successCriteria) == 0 {
		if obj, ok := args["objective"].(map[string]any); ok {
			if sc, ok := obj["success_criteria"].([]any); ok {
				for _, v := range sc {
					if s, ok := v.(string); ok {
						successCriteria = append(successCriteria, s)
					}
				}
			}
		}
	}
	acceptanceTest := getString(args, "acceptance_test", "")
	if acceptanceTest == "" {
		if obj, ok := args["objective"].(map[string]any); ok {
			if at, ok := obj["acceptance_test"].(string); ok {
				acceptanceTest = at
			}
		}
	}
	objective := ticket.Objective{
		Description:     description,
		SuccessCriteria: successCriteria,
		AcceptanceTest:  acceptanceTest,
	}
	idempotencyKey := getString(args, "idempotency_key", "")
	workStreamID := getString(args, "work_stream_id", "")
	if workStreamID != "" && b.WorkStream != nil {
		ws, err := b.WorkStream.GetWorkStream(ctx, workStreamID)
		if err != nil || ws == nil || ws.ProjectID != projectID {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "work stream not found or does not belong to project", false))
		}
	}
	dependsOn := getStringArray(args, "depends_on")
	if dependsOn == nil {
		dependsOn = []string{}
	}
	targetRepo := getString(args, "target_repo", "")
	// Validate target_repo alias exists if provided and multi-repo is configured.
	if targetRepo != "" && b.Repos != nil {
		_, repoErr := b.Repos.GetRepository(ctx, projectID, targetRepo)
		if repoErr != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "target_repo alias not found: "+targetRepo+". Use list_project_repositories to see available aliases.", false))
		}
	}
	t, err := b.Ticket.CreateTicket(ctx, projectID, title, typ, prio, agentID, dependsOn, workStreamID, objective, ticket.TicketContext{}, idempotencyKey, targetRepo)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	// Apply optional inputs metadata (e.g. decomposed_from for subticket provenance).
	if inputsRaw, ok := args["inputs"].(map[string]any); ok && len(inputsRaw) > 0 {
		if patchErr := b.Ticket.PatchInputs(ctx, t.ID, inputsRaw); patchErr != nil {
			return toolErrTriple(apierrors.MapError(patchErr))
		}
		for k, v := range inputsRaw {
			t.Inputs[k] = v
		}
	}
	// Override workflow if specified (e.g. subtickets using a simpler workflow).
	if wfID := getString(args, "workflow_id", ""); wfID != "" && b.Workflow != nil {
		if wfDef, wfErr := b.Workflow.GetDefinition(ctx, wfID); wfErr == nil && wfDef != nil && len(wfDef.Phases) > 0 {
			firstPhase := wfDef.Phases[0].ID
			if updErr := b.Ticket.UpdateWorkflow(ctx, t.ID, wfDef.ID, wfDef.Version, firstPhase); updErr != nil {
				return toolErrTriple(apierrors.MapError(updErr))
			}
			t.WorkflowID = wfDef.ID
			t.WorkflowVersion = wfDef.Version
			t.WorkflowPhase = firstPhase
		}
	}
	return jsonResult(map[string]any{
		"ticket":   t,
		"workflow": workflowAfterCreateTicket(projectID),
	})
}

func listProjectsHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	agent, err := b.AgentStore.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "agent not found", false))
	}
	if agent.UserID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "list_projects requires OAuth login (agent must be linked to a user). Use GitHub sign-in via MCP URL auth.", false))
	}
	orgIDs, err := b.Org.ListOrgIDsForUser(ctx, agent.UserID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	if len(orgIDs) == 0 {
		return jsonResult([]any{})
	}
	filterOrgID := getString(args, "org_id", "")
	if filterOrgID != "" {
		allowed := false
		for _, id := range orgIDs {
			if id == filterOrgID {
				allowed = true
				break
			}
		}
		if !allowed {
			return toolErrTriple(apierrors.New(apierrors.CodeForbidden, "you are not a member of that organization", false))
		}
		orgIDs = []string{filterOrgID}
	}
	includeClosed := getBool(args, "include_closed", false)
	statusFilter := "active"
	if includeClosed {
		statusFilter = "all"
	}
	var all []project.Project
	for _, oid := range orgIDs {
		list, err := b.Project.ListByOrgID(ctx, oid, statusFilter)
		if err != nil {
			return toolErrTriple(apierrors.MapError(err))
		}
		all = append(all, list...)
	}
	return jsonResult(all)
}

func getProjectContextHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	pack, err := b.Project.AssembleContextPack(ctx, projectID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(pack)
}

func updateProjectContextHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	proj, err := b.Project.GetProject(ctx, projectID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	agent, err := b.AgentStore.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "agent not found", false))
	}
	if agent.UserID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "update_project_context requires OAuth login", false))
	}
	orgIDs, err := b.Org.ListOrgIDsForUser(ctx, agent.UserID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	allowed := false
	for _, id := range orgIDs {
		if id == proj.OrgID {
			allowed = true
			break
		}
	}
	if !allowed {
		return toolErrTriple(apierrors.New(apierrors.CodeForbidden, "you do not have access to that project", false))
	}
	pack := proj.ContextPack
	if c := getString(args, "conventions", ""); c != "" {
		pack.Conventions = c
	}
	if sp := getString(args, "system_prompt", ""); sp != "" {
		pack.SystemPrompt = sp
	}
	if kf, ok := args["key_files"]; ok && kf != nil {
		var keyFiles []project.FileRef
		raw, err := json.Marshal(kf)
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "key_files must be a JSON array of {path, snippet} objects", false))
		}
		if err := json.Unmarshal(raw, &keyFiles); err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "key_files must be a JSON array of {path, snippet} objects", false))
		}
		pack.KeyFiles = keyFiles
	}
	if ex, ok := args["extra"]; ok && ex != nil {
		var extra map[string]string
		raw, err := json.Marshal(ex)
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "extra must be a JSON object of string key-value pairs", false))
		}
		if err := json.Unmarshal(raw, &extra); err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "extra must be a JSON object of string key-value pairs", false))
		}
		pack.Extra = extra
	}
	if err := b.Project.UpdateContextPack(ctx, projectID, pack); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(pack)
}

func updateProjectStatusHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	status, err := requireString(args, "status")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	if status != "active" && status != "closed" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "status must be active or closed", false))
	}
	proj, err := b.Project.GetProject(ctx, projectID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	agent, err := b.AgentStore.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "agent not found", false))
	}
	if agent.UserID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "update_project_status requires OAuth login", false))
	}
	orgIDs, err := b.Org.ListOrgIDsForUser(ctx, agent.UserID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	allowed := false
	for _, id := range orgIDs {
		if id == proj.OrgID {
			allowed = true
			break
		}
	}
	if !allowed {
		return toolErrTriple(apierrors.New(apierrors.CodeForbidden, "you do not have access to that project", false))
	}
	if err := b.Project.UpdateStatus(ctx, projectID, status); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	updated, err := b.Project.GetProject(ctx, projectID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(updated)
}

func listTicketsHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	workStreamID := getString(args, "work_stream_id", "")
	state := ticket.State(getString(args, "state", ""))
	list, err := b.Ticket.ListTickets(ctx, projectID, workStreamID, state)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	if p := getInt(args, "priority", -1); p >= 0 && p <= 3 {
		filtered := make([]*ticket.Ticket, 0)
		for _, t := range list {
			if int(t.Priority) == p {
				filtered = append(filtered, t)
			}
		}
		list = filtered
	}
	// Return slim ticket summaries to keep response size manageable.
	// Full ticket details (outputs, inputs, context) are available via get_ticket.
	summaries := make([]map[string]any, 0, len(list))
	for _, t := range list {
		s := map[string]any{
			"id":         t.ID,
			"title":      t.Title,
			"type":       t.Type,
			"state":      t.State,
			"priority":   t.Priority,
			"project_id": t.ProjectID,
			"created_at": t.CreatedAt,
			"updated_at": t.UpdatedAt,
		}
		if t.WorkStreamID != "" {
			s["work_stream_id"] = t.WorkStreamID
		}
		if t.AssignedTo != "" {
			s["assigned_to"] = t.AssignedTo
		}
		if len(t.DependsOn) > 0 {
			s["depends_on"] = t.DependsOn
		}
		if t.Objective.Description != "" {
			s["objective"] = t.Objective.Description
		}
		summaries = append(summaries, s)
	}
	stateStr := getString(args, "state", "")
	out := map[string]any{"tickets": summaries, "count": len(summaries)}
	if stateStr == "" || ticket.State(stateStr) == ticket.StateDraft {
		out["workflow"] = workflowAfterListTicketsPending()
	}
	return jsonResult(out)
}

func getTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	t, err := b.Ticket.GetTicket(ctx, ticketID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	pack, _ := b.Project.AssembleContextPack(ctx, t.ProjectID)
	deps, _ := b.Ticket.GetTicketsByIDs(ctx, t.DependsOn)
	depOutputs := ticket.GetDependencyOutputs(t, deps)
	sum, _ := b.Trace.SummarizeTrace(ctx, ticketID)
	out := map[string]any{
		"ticket":             t,
		"context_pack":       pack,
		"dependency_outputs": depOutputs,
		"prior_attempts":     t.Context.PriorAttempts,
		"human_answers":      t.Context.HumanAnswers,
	}
	if sum != nil {
		out["latest_attempt_summary"] = sum
	}
	if t.WorkStreamID != "" && b.WorkStream != nil {
		if ws, err := b.WorkStream.GetWorkStream(ctx, t.WorkStreamID); err == nil && ws != nil {
			out["work_stream"] = ws
			if proj, _ := b.Project.GetProject(ctx, t.ProjectID); proj != nil {
				attachWorkStreamGit(out, proj, ws)
			}
		}
	}
	// Include target repo info for multi-repo tickets.
	if t.TargetRepo != "" && b.Repos != nil {
		if repo, err := b.Repos.GetRepository(ctx, t.ProjectID, t.TargetRepo); err == nil && repo != nil {
			out["target_repository"] = repo
		}
	}
	return jsonResult(out)
}

func updateTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	agent, err := b.AgentStore.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "agent not found", false))
	}
	if agent.UserID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "update_ticket requires OAuth login.", false))
	}
	proj, err := b.Project.GetProject(ctx, projectID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	orgIDs, err := b.Org.ListOrgIDsForUser(ctx, agent.UserID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	allowed := false
	for _, id := range orgIDs {
		if id == proj.OrgID {
			allowed = true
			break
		}
	}
	if !allowed {
		return toolErrTriple(apierrors.New(apierrors.CodeForbidden, "you do not have access to that project", false))
	}
	t, err := b.Ticket.GetTicket(ctx, ticketID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	if t.ProjectID != projectID {
		return toolErrTriple(apierrors.New(apierrors.CodeForbidden, "ticket does not belong to that project", false))
	}
	if dependsOn := getStringArray(args, "depends_on"); dependsOn != nil {
		if err := b.Ticket.UpdateDependsOn(ctx, ticketID, dependsOn); err != nil {
			return toolErrTriple(apierrors.MapError(err))
		}
	}
	workStreamID := getString(args, "work_stream_id", "")
	if workStreamID != "" && b.WorkStream != nil {
		ws, err := b.WorkStream.GetWorkStream(ctx, workStreamID)
		if err != nil || ws == nil || ws.ProjectID != projectID {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "work stream not found or does not belong to project", false))
		}
		if err := b.Ticket.UpdateWorkStreamID(ctx, ticketID, workStreamID); err != nil {
			return toolErrTriple(apierrors.MapError(err))
		}
	}
	updated, err := b.Ticket.GetTicket(ctx, ticketID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(updated)
}

func claimTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	proj, err := b.Project.GetProject(ctx, projectID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	if proj.Status == "closed" {
		return toolErrTriple(apierrors.New(apierrors.CodeProjectClosed, "project is closed; reopen it with update_project_status or choose another project", false))
	}
	var priority *int
	if p := getInt(args, "priority", -1); p >= 0 && p <= 3 {
		priority = &p
	}
	idempotencyKey := getString(args, "idempotency_key", "")
	t, lease, err := b.Queue.ClaimTicket(ctx, agentID, projectID, priority, idempotencyKey)
	if err != nil {
		if errors.Is(err, queue.ErrNoTicketAvailable) {
			if ss := sessionFromContext(ctx); ss != nil {
				// List tickets that are claimed or executing so the user can direct us to release one.
				stuckIDs := stuckTicketIDs(ctx, b, projectID)
				msg := "No ticket available to claim. Confirm below to have the agent retry, or choose a stuck ticket to force-release so the agent can claim it in this session."
				if len(stuckIDs) > 0 {
					msg = "No ticket available to claim. The following tickets may be stuck (claimed or executing): " + strings.Join(stuckIDs, ", ") + ". To direct the agent to claim one, set ticket_id_to_release to that ticket ID and confirm—the server will force-release it and retry claim."
				}
				schemaProps := map[string]any{
					"confirmed": map[string]any{"type": "boolean", "description": "Confirm to retry claim (and optionally release the chosen ticket)"},
				}
				if len(stuckIDs) > 0 {
					schemaProps["ticket_id_to_release"] = map[string]any{"type": "string", "description": "Optional: ticket ID to force-release so the agent can claim it (e.g. " + stuckIDs[0] + "). Leave empty to just retry."}
				}
				var elicitRes *mcp.ElicitResult
				var elicitErr error
				var elicitPanic error
				func() {
					defer func() {
						if r := recover(); r != nil {
							elicitPanic = fmt.Errorf("elicitation panic: %v", r)
						}
					}()
					elicitRes, elicitErr = ss.Elicit(ctx, &mcp.ElicitParams{
						Mode:            "form",
						Message:         msg,
						RequestedSchema: map[string]any{"type": "object", "properties": schemaProps},
					})
				}()
				if elicitPanic != nil {
					return toolErrTriple(apierrors.New(apierrors.CodeInternal, elicitPanic.Error(), true))
				}
				if elicitErr == nil && elicitRes != nil && elicitRes.Action == "accept" {
					content := elicitRes.Content
					if content == nil {
						content = map[string]any{}
					}
					if toRelease, _ := content["ticket_id_to_release"].(string); toRelease != "" {
						allowed := false
						for _, id := range stuckIDs {
							if id == toRelease {
								allowed = true
								break
							}
						}
						if allowed {
							_ = b.Queue.ForceReleaseLease(ctx, toRelease)
						}
					}
					t, lease, retryErr := b.Queue.ClaimTicket(ctx, agentID, projectID, priority, idempotencyKey)
					if retryErr == nil {
						return claimTicketResult(b, ctx, t, lease)
					}
				}
			}
		}
		return toolErrTriple(apierrors.MapError(err))
	}
	return claimTicketResult(b, ctx, t, lease)
}

func claimTicketResult(b *Backend, ctx context.Context, t *ticket.Ticket, lease *queue.Lease) (*mcp.CallToolResult, any, error) {
	wf := workflowAfterClaim()
	out := map[string]any{"ticket": t, "lease": lease, "workflow": wf}
	if t.WorkStreamID != "" && b.WorkStream != nil {
		if ws, err := b.WorkStream.GetWorkStream(ctx, t.WorkStreamID); err == nil && ws != nil {
			out["work_stream"] = ws
			if proj, _ := b.Project.GetProject(ctx, t.ProjectID); proj != nil {
				attachWorkStreamGit(out, proj, ws)
			}
		}
	}
	return jsonResult(out)
}

func startTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	leaseToken, err := requireString(args, "lease_token")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	actor := ticket.Actor{ID: agentID, Type: ticket.ActorAgent}
	if err := b.Ticket.TransitionTicket(ctx, ticketID, ticket.TriggerStart, actor, nil); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	_ = leaseToken
	return jsonResult(map[string]any{
		"ok":       true,
		"workflow": workflowAfterStart(),
	})
}

func logStepHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	leaseToken, err := requireString(args, "lease_token")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	stepType, err := requireString(args, "step_type")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	payload := getPayloadMap(args, "payload")
	step := execution.Step{Type: execution.StepType(stepType), Payload: payload}
	// Record worker type in the step if provided.
	if wt := getString(args, "worker_type", ""); wt != "" {
		step.WorkerType = wt
	}
	if err := b.Trace.LogStep(ctx, ticketID, leaseToken, step); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"ok": true})
}

func submitTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	leaseToken, err := requireString(args, "lease_token")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	outputsStr, err := requireString(args, "outputs")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	var outputs map[string]any
	if err := json.Unmarshal([]byte(outputsStr), &outputs); err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, fmt.Sprintf("invalid outputs JSON: %v", err), false))
	}
	if err := b.Ticket.SubmitTicket(ctx, ticketID, leaseToken, outputs); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	// Clean up lease since ticket has moved past lease-tracked states.
	_ = b.Queue.CleanupLease(ctx, ticketID)
	return jsonResult(map[string]any{"ok": true})
}

func escalateTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	leaseToken, err := requireString(args, "lease_token")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	reason, _ := requireString(args, "reason")
	question, _ := requireString(args, "question")
	if err := b.Ticket.EscalateTicket(ctx, ticketID, leaseToken, reason, question); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	// Clean up lease since ticket has moved past lease-tracked states.
	_ = b.Queue.CleanupLease(ctx, ticketID)
	return jsonResult(map[string]any{"ok": true})
}

func renewLeaseHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	leaseToken, err := requireString(args, "lease_token")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	expiresAt, err := b.Queue.RenewLease(ctx, ticketID, leaseToken)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"expires_at": expiresAt})
}

func forceReleaseLeaseHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	t, err := b.Ticket.GetTicket(ctx, ticketID)
	if err != nil || t == nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	proj, err := b.Project.GetProject(ctx, t.ProjectID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	agent, err := b.AgentStore.GetByID(ctx, agentID)
	if err != nil || agent == nil || agent.UserID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "force_release_lease requires OAuth login", false))
	}
	orgIDs, err := b.Org.ListOrgIDsForUser(ctx, agent.UserID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	allowed := false
	for _, id := range orgIDs {
		if id == proj.OrgID {
			allowed = true
			break
		}
	}
	if !allowed {
		return toolErrTriple(apierrors.New(apierrors.CodeForbidden, "you do not have access to that ticket's project", false))
	}
	if err := b.Queue.ForceReleaseLease(ctx, ticketID); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"ok": true, "ticket_id": ticketID, "message": "Ticket returned to pending; use claim_ticket to claim it."})
}

func listPendingReviewsHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.Review == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "reviews not configured", false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	agent, err := b.AgentStore.GetByID(ctx, agentID)
	if err != nil || agent == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "agent not found", false))
	}
	if agent.UserID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeUnauthorized, "list_pending_reviews requires OAuth login.", false))
	}
	proj, err := b.Project.GetProject(ctx, projectID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	orgIDs, err := b.Org.ListOrgIDsForUser(ctx, agent.UserID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	allowed := false
	for _, id := range orgIDs {
		if id == proj.OrgID {
			allowed = true
			break
		}
	}
	if !allowed {
		return toolErrTriple(apierrors.New(apierrors.CodeForbidden, "you do not have access to that project", false))
	}
	ids, err := b.Review.ListPendingReviews(ctx, projectID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	tickets := make([]*ticket.Ticket, 0, len(ids))
	for _, id := range ids {
		t, _ := b.Ticket.GetTicket(ctx, id)
		if t != nil {
			tickets = append(tickets, t)
		}
	}
	return jsonResult(map[string]any{"tickets": tickets})
}

func getTraceHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	trace, err := b.Trace.GetTrace(ctx, ticketID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(trace)
}

func approveTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.Review == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "reviews not configured", false))
	}
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	reviewerID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	notes := getString(args, "notes", "")
	if err := b.Review.ApproveTicket(ctx, ticketID, reviewerID, notes); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"ok": true, "decision": review.DecisionApproved})
}

func rejectTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.Review == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "reviews not configured", false))
	}
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	reviewerID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	notes, err := requireString(args, "notes")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "notes required when rejecting (so the agent knows what to fix)", false))
	}
	if err := b.Review.RejectTicket(ctx, ticketID, reviewerID, notes); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"ok": true, "decision": review.DecisionRejected})
}

func reopenTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.Review == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "reviews not configured", false))
	}
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	reviewerID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	notes := getString(args, "notes", "")
	if err := b.Review.ReopenTicketForReview(ctx, ticketID, reviewerID, notes); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"ok": true, "decision": review.DecisionReopened})
}

func cancelTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	reason, err := requireString(args, "reason")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	actor := ticket.Actor{ID: agentID, Type: ticket.ActorHuman}
	payload := map[string]any{"reason": reason}
	if err := b.Ticket.TransitionTicket(ctx, ticketID, ticket.TriggerCancel, actor, payload); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	_ = b.Queue.CleanupLease(ctx, ticketID)
	return jsonResult(map[string]any{"ok": true, "ticket_id": ticketID, "state": "closed"})
}

func resolveTicketInputHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.Review == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "reviews not configured", false))
	}
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	answer, err := requireString(args, "answer")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	reviewerID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	resumeTo := getString(args, "resume_to", "planning")
	if err := b.Review.ResolveTicketInput(ctx, ticketID, reviewerID, answer, resumeTo); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"ok": true, "ticket_id": ticketID, "resumed_to": resumeTo})
}

// repoPathAccessible returns true if repoPath is non-empty and the path exists and is a git repo.
func repoPathAccessible(repoPath string) bool {
	if repoPath == "" {
		return false
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(abs, ".git"))
	return err == nil
}

func flywheelGitNoteAddCommands(noteType, message, commitSHA string) []string {
	esc := strings.ReplaceAll(message, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	return []string{
		fmt.Sprintf(`flywheel-git note add -t %s -m %q -c %s`, noteType, esc, commitSHA),
	}
}

// --- Claims registry MCP handlers ---

// --- Notification handlers ---
