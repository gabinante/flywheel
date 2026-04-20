package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/gabinante/flywheel/api/rest"
	"github.com/gabinante/flywheel/internal/auth"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/gitnotes"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/queue"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/workstream"
)

type sessionContextKey struct{}

// RegisterTools adds all Warrant MCP tools to the MCP server (official go-sdk).
func RegisterTools(s *mcp.Server, b *Backend) {
	if b == nil {
		return
	}
	wrap := func(f func(*Backend, context.Context, map[string]any) (*mcp.CallToolResult, any, error)) func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
		return func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			toolName := "unknown"
			if req != nil && req.Params.Name != "" {
				toolName = req.Params.Name
			}
			log.Printf("MCP tool call: %s args=%v", toolName, args)
			if req != nil && req.Session != nil {
				ctx = context.WithValue(ctx, sessionContextKey{}, req.Session)
			}
			result, elicit, err := f(b, ctx, args)
			if err != nil {
				log.Printf("MCP tool %s error: %v", toolName, err)
			} else if result != nil && result.IsError {
				log.Printf("MCP tool %s returned error result", toolName)
			} else {
				log.Printf("MCP tool %s completed ok", toolName)
			}
			return result, elicit, err
		}
	}

	mcp.AddTool(s, &mcp.Tool{Name: "list_orgs", Description: "List organizations you belong to. Returns id, name, slug for each. Requires OAuth. Use this to see your workspaces (e.g. personal org, or teams you were added to).", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
	}}, wrap(listOrgsHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "create_project", Description: "Create a project in your default (first) organization. Use for initiatives, epics, or any work container. You do not pass org_id; the project is created in an org you belong to.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":     map[string]any{"type": "string", "description": "Project name"},
			"slug":     map[string]any{"type": "string", "description": "URL-friendly slug (optional, auto-generated if omitted)"},
			"agent_id": map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"name"},
	}}, wrap(createProjectHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "create_work_stream", Description: "Create a work stream in a project. Work streams group tickets toward a goal (e.g. 'Productionize feature A'). When the project has repo_url: Warrant does NOT create a Git branch. The response includes git_instruction—follow it immediately: create or checkout the branch in the repo, then call update_work_stream with branch (required in that session; do not stop after only setting plan text). Until branch is set, claim_ticket/get_ticket repeat create_or_set_branch. Params: project_id, name (required), slug (optional), plan (optional Markdown). Returns work_stream and optional git_instruction.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project ID"},
			"name":       map[string]any{"type": "string", "description": "Work stream name"},
			"slug":       map[string]any{"type": "string", "description": "URL-friendly slug (optional, auto-generated if omitted)"},
			"plan":       map[string]any{"type": "string", "description": "Markdown plan for the work stream (optional)"},
			"agent_id":   map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id", "name"},
	}}, wrap(createWorkStreamHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "list_work_streams", Description: "List work streams for a project. Params: project_id, optional status (active, closed, all).", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project ID"},
			"status":     map[string]any{"type": "string", "description": "Filter by status: active, closed, or all (default: active)"},
			"agent_id":   map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id"},
	}}, wrap(listWorkStreamsHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "get_work_stream", Description: "Get a work stream by ID. Params: project_id, work_stream_id. When project has repo_url, response includes git_instruction: if branch is empty, you must create/checkout the branch and call update_work_stream with branch (plan updates alone are insufficient).", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project ID"},
			"work_stream_id": map[string]any{"type": "string", "description": "Work stream ID"},
			"agent_id":       map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id", "work_stream_id"},
	}}, wrap(getWorkStreamHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "update_work_stream", Description: "Update a work stream (name, plan, branch, status). When project has repo_url: you MUST set branch (pass the real Git branch name) as soon as you create or checkout that branch—this is easy to forget if you only update the Markdown plan. update_work_stream_plan does NOT set branch. Omit branch only to leave the stored branch unchanged. When closing (status=closed) and project has repo_url, returns git_instruction to checkout default branch. Params: project_id, work_stream_id, optional name, plan, branch, status (active|closed).", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project ID"},
			"work_stream_id": map[string]any{"type": "string", "description": "Work stream ID"},
			"name":           map[string]any{"type": "string", "description": "New name (optional, keeps current if omitted)"},
			"plan":           map[string]any{"type": "string", "description": "Markdown plan (optional, keeps current if omitted)"},
			"branch":         map[string]any{"type": "string", "description": "Git branch name (optional, keeps current if omitted)"},
			"status":         map[string]any{"type": "string", "description": "Status: active or closed (optional, keeps current if omitted)"},
			"agent_id":       map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id", "work_stream_id"},
	}}, wrap(updateWorkStreamHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "update_work_stream_plan", Description: "Replace only the work stream's Markdown plan. Does NOT set or change the Git branch—if repo_url is set and branch is still empty, you must still call update_work_stream with branch after creating/checking out the branch. Params: project_id, work_stream_id, plan (required). Returns work_stream and optional git_instruction when repo_url is set and stream is active.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project ID"},
			"work_stream_id": map[string]any{"type": "string", "description": "Work stream ID"},
			"plan":           map[string]any{"type": "string", "description": "New Markdown plan content"},
			"agent_id":       map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id", "work_stream_id", "plan"},
	}}, wrap(updateWorkStreamPlanHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "create_ticket", Description: "Create a ticket in a project. Response JSON has **ticket** (the new ticket) and **workflow** (next_steps + note) to remind you to claim → start → log_step → submit. The ticket is created as pending; agents claim via claim_ticket. created_by is set to your agent identity. **work_stream_id:** pass when the work belongs to a stream—otherwise the ticket will not appear in the web UI when that stream is filtered (use **update_ticket** later to attach). If set and project has repo_url, the work stream must already have **branch** set via **update_work_stream** after you create/checkout that branch (plan-only updates do not count). Optional idempotency_key.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":       map[string]any{"type": "string", "description": "Project ID"},
			"title":            map[string]any{"type": "string", "description": "Ticket title"},
			"description":      map[string]any{"type": "string", "description": "Ticket description / objective"},
			"ticket_type":      map[string]any{"type": "string", "description": "Ticket type: task, bug, spike, or review (default: task)"},
			"priority":         map[string]any{"type": "integer", "description": "Priority 0-3 (0=P0 highest, default: 2)"},
			"success_criteria": map[string]any{"type": "string", "description": "JSON array of success criteria strings (optional)"},
			"acceptance_test":  map[string]any{"type": "string", "description": "Acceptance test description (optional)"},
			"idempotency_key":  map[string]any{"type": "string", "description": "Idempotency key to prevent duplicate creation (optional)"},
			"work_stream_id":   map[string]any{"type": "string", "description": "Work stream ID to attach this ticket to (optional)"},
			"depends_on":       map[string]any{"type": "string", "description": "JSON array of ticket IDs this ticket depends on (optional)"},
			"agent_id":         map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id", "title", "description"},
	}}, wrap(createTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "list_projects", Description: "List projects for the authenticated user's organization(s). Requires OAuth (agent linked to a user). Returns only active projects by default. Pass include_closed: true to include closed projects. Optionally pass org_id to limit to one org (must be an org you belong to).", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"org_id":         map[string]any{"type": "string", "description": "Organization ID to filter by (optional, must be an org you belong to)"},
			"include_closed": map[string]any{"type": "boolean", "description": "Include closed projects (default: false)"},
			"agent_id":       map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
	}}, wrap(listProjectsHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "get_project_context", Description: "Return the full context pack for a project: conventions, key files, system prompt, and extra hints. Call this after list_projects to load the project's context before claiming or inspecting tickets.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project ID"},
		},
		"required": []string{"project_id"},
	}}, wrap(getProjectContextHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "update_project_context", Description: "Update a project's context pack. Pass project_id and any of: conventions (string), system_prompt (string), key_files (JSON array of {path, snippet}), extra (JSON object of string key-value pairs). Merges with existing context pack. Returns the updated context pack.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project ID"},
			"conventions":   map[string]any{"type": "string", "description": "Conventions text (optional, merges with existing)"},
			"system_prompt": map[string]any{"type": "string", "description": "System prompt (optional, replaces existing)"},
			"key_files":     map[string]any{"type": "array", "description": "JSON array of {path, snippet} objects (optional, replaces existing)", "items": map[string]any{"type": "object"}},
			"extra":         map[string]any{"type": "object", "description": "JSON object of string key-value pairs (optional, replaces existing)"},
			"agent_id":      map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id"},
	}}, wrap(updateProjectContextHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "update_project_status", Description: "Set a project's status to active or closed. Use to close a project when work is done, or reopen it (set to active) for follow-up. Requires OAuth and org access. Pass project_id and status (\"active\" or \"closed\"). Returns the updated project.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project ID"},
			"status":     map[string]any{"type": "string", "description": "New status: active or closed"},
			"agent_id":   map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id", "status"},
	}}, wrap(updateProjectStatusHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "list_tickets", Description: "List tickets for a project. Response JSON has **tickets** (array). When you omit **state** or set state=pending, **workflow** (next_steps + note) is included to nudge claim → start → submit. Optionally filter by state, priority (0–3), or work_stream_id. Use after get_project_context to see what work is available.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project ID"},
			"work_stream_id": map[string]any{"type": "string", "description": "Filter by work stream ID (optional)"},
			"state":          map[string]any{"type": "string", "description": "Filter by state: pending, claimed, executing, awaiting_review, done, needs_human (optional)"},
			"priority":       map[string]any{"type": "integer", "description": "Filter by priority 0-3 (optional)"},
		},
		"required": []string{"project_id"},
	}}, wrap(listTicketsHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "get_ticket", Description: "Get the full ticket payload: objective, success criteria, acceptance test, context pack, dependency outputs (from tickets this one depends on), prior attempts, and human answers. This is the main input for doing the work. Call after claim_ticket and before start_ticket to load everything you need. If the ticket has a work_stream and the project has repo_url, the response may include git_instruction (checkout branch, or create branch + update_work_stream if branch is not set yet).", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
		},
		"required": []string{"ticket_id"},
	}}, wrap(getTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "update_ticket", Description: "Update ticket metadata. Pass project_id and ticket_id. Optional: depends_on (JSON array string), work_stream_id. For title/objective text, use REST PATCH /tickets/{id} with JSON body (title, objective partial merge) or recreate the ticket.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project ID"},
			"ticket_id":      map[string]any{"type": "string", "description": "Ticket ID"},
			"depends_on":     map[string]any{"type": "string", "description": "JSON array of ticket ID strings this ticket depends on (optional)"},
			"work_stream_id": map[string]any{"type": "string", "description": "Work stream ID to attach this ticket to (optional)"},
			"agent_id":       map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id", "ticket_id"},
	}}, wrap(updateTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "claim_ticket", Description: "Claim the next available ticket in the queue for a project. Returns the ticket, lease (lease_token, expires_at), and **workflow** (next_steps + note). If the ticket has a work_stream and the project has repo_url, the response may include git_instruction (checkout branch, or create branch + update_work_stream if branch is not set yet). Optional idempotency_key: retries with the same key return the same ticket/lease (renewed if still valid) so the same agent does not claim a different ticket. You must start_ticket and then either submit_ticket or escalate_ticket before the lease expires, or renew_lease to extend. agent_id is inferred from OAuth when using URL auth.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":      map[string]any{"type": "string", "description": "Project ID"},
			"priority":        map[string]any{"type": "integer", "description": "Claim only tickets at this priority level 0-3 (optional, claims next available if omitted)"},
			"idempotency_key": map[string]any{"type": "string", "description": "Idempotency key to prevent claiming multiple tickets (optional)"},
			"agent_id":        map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id"},
	}}, wrap(claimTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "start_ticket", Description: "Move the ticket from claimed to executing. Response includes **workflow** (next_steps + note). Call after claim_ticket and get_ticket when you are ready to do the work. Requires the lease_token from claim_ticket. agent_id is inferred from OAuth when using URL auth.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id":   map[string]any{"type": "string", "description": "Ticket ID"},
			"lease_token": map[string]any{"type": "string", "description": "Lease token from claim_ticket"},
			"agent_id":    map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"ticket_id", "lease_token"},
	}}, wrap(startTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "log_step", Description: "Append a step to the execution trace. Call this as you work—after each significant tool use (step_type tool_call, payload as object e.g. {\"name\":\"write\",\"input\":{\"path\":\"...\"}}), for key observations or decisions (observation/thought), and on errors (error). payload can be a JSON object or JSON string. Reviewers see this trace when approving the ticket; call it regularly so they know what was done.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id":   map[string]any{"type": "string", "description": "Ticket ID"},
			"lease_token": map[string]any{"type": "string", "description": "Lease token from claim_ticket"},
			"step_type":   map[string]any{"type": "string", "description": "Step type: tool_call, observation, thought, or error"},
			"payload":     map[string]any{"type": "object", "description": "Step payload as a JSON object (or JSON string)"},
		},
		"required": []string{"ticket_id", "lease_token", "step_type"},
	}}, wrap(logStepHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "submit_ticket", Description: "Submit your outputs and move the ticket to awaiting_review. outputs must be a JSON object (e.g. {\"summary\":\"...\", \"artifacts\":[...]}). A human will approve or reject via the REST API. Call when the work is done.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id":   map[string]any{"type": "string", "description": "Ticket ID"},
			"lease_token": map[string]any{"type": "string", "description": "Lease token from claim_ticket"},
			"outputs":     map[string]any{"type": "string", "description": "JSON object string with outputs (e.g. {\"summary\":\"...\", \"artifacts\":[...]})"},
		},
		"required": []string{"ticket_id", "lease_token", "outputs"},
	}}, wrap(submitTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "escalate_ticket", Description: "Escalate to a human when you need help. Moves the ticket to needs_human. Provide a reason and a specific question; the human's answer is stored and the ticket returns to executing so you can continue. Use when blocked or when the objective is ambiguous.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id":   map[string]any{"type": "string", "description": "Ticket ID"},
			"lease_token": map[string]any{"type": "string", "description": "Lease token from claim_ticket"},
			"reason":      map[string]any{"type": "string", "description": "Reason for escalation"},
			"question":    map[string]any{"type": "string", "description": "Specific question for the human"},
		},
		"required": []string{"ticket_id", "lease_token", "reason", "question"},
	}}, wrap(escalateTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "renew_lease", Description: "Extend the lease TTL so the ticket is not returned to the queue. Call periodically while working if the job takes longer than the lease duration. Returns the new expires_at.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id":   map[string]any{"type": "string", "description": "Ticket ID"},
			"lease_token": map[string]any{"type": "string", "description": "Lease token from claim_ticket"},
		},
		"required": []string{"ticket_id", "lease_token"},
	}}, wrap(renewLeaseHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "force_release_lease", Description: "Force-release a ticket's lease (no token needed). Use when the user directs you to release a stuck ticket so you can claim it in this session (e.g. 'release agent-reliability-3 and claim it'). Caller must have access to the ticket's project. Ticket returns to pending; then use claim_ticket to claim it.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID to force-release"},
			"agent_id":  map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"ticket_id"},
	}}, wrap(forceReleaseLeaseHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "list_pending_reviews", Description: "List tickets in awaiting_review for a project. Use this when the user asks 'what needs my review?' or 'show pending reviews'. Returns full tickets so you can summarize them in chat; use get_trace(ticket_id) to show execution steps for each.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project ID"},
			"agent_id":   map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id"},
	}}, wrap(listPendingReviewsHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "get_trace", Description: "Get the execution trace for a ticket (all log_step entries). Use when summarizing a ticket for review so the user can see what was done before approving or rejecting.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
		},
		"required": []string{"ticket_id"},
	}}, wrap(getTraceHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "approve_ticket", Description: "Approve a ticket in awaiting_review. Moves it to done. Call when the user says to approve, ship it, looks good, etc. reviewer_id is inferred from OAuth.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
			"notes":     map[string]any{"type": "string", "description": "Reviewer notes (optional)"},
			"agent_id":  map[string]any{"type": "string", "description": "Reviewer ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"ticket_id"},
	}}, wrap(approveTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "reject_ticket", Description: "Reject a ticket in awaiting_review. Returns it to executing with your notes appended so the agent can fix and resubmit. Call when the user says reject, needs changes, etc. reviewer_id is inferred from OAuth.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
			"notes":     map[string]any{"type": "string", "description": "Rejection notes explaining what to fix"},
			"agent_id":  map[string]any{"type": "string", "description": "Reviewer ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"ticket_id", "notes"},
	}}, wrap(rejectTicketHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "reopen_ticket", Description: "Move a ticket from done back to awaiting_review (e.g. mistaken approval). Preserves outputs. Only call when the user explicitly asks to reopen or return a completed ticket for review. reviewer_id is inferred from OAuth.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
			"notes":     map[string]any{"type": "string", "description": "Notes explaining why the ticket is being reopened (optional)"},
			"agent_id":  map[string]any{"type": "string", "description": "Reviewer ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"ticket_id"},
	}}, wrap(reopenTicketHandler))

	// Git notes (Warrant integration): if repo_path provided and server has access, run git notes; else return commands for warrant-git CLI.
	mcp.AddTool(s, &mcp.Tool{Name: "warrant_add_git_note", Description: "Add a git note to a commit (refs/notes/warrant/decision|trace|intent). Params: message (required), type (decision|trace|intent, default decision), commit_sha (default HEAD), optional repo_path, ticket_id, project_id. If server has repo_path, adds note; else returns commands to run warrant-git note add locally.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message":    map[string]any{"type": "string", "description": "Note message content"},
			"type":       map[string]any{"type": "string", "description": "Note type: decision, trace, or intent (default: decision)"},
			"commit_sha": map[string]any{"type": "string", "description": "Commit SHA to attach note to (default: HEAD)"},
			"repo_path":  map[string]any{"type": "string", "description": "Path to git repo (optional, for server-side execution)"},
			"ticket_id":  map[string]any{"type": "string", "description": "Associated ticket ID (optional, stored in note metadata)"},
			"project_id": map[string]any{"type": "string", "description": "Associated project ID (optional, stored in note metadata)"},
			"agent_id":   map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"message"},
	}}, wrap(warrantAddGitNoteHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "warrant_show_git_notes", Description: "Show git note(s) for a commit. Params: commit_sha (default HEAD), optional repo_path, type (decision|trace|intent, or omit for all). Returns note body or commands for warrant-git note show.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"commit_sha": map[string]any{"type": "string", "description": "Commit SHA to show notes for (default: HEAD)"},
			"repo_path":  map[string]any{"type": "string", "description": "Path to git repo (optional, for server-side execution)"},
			"type":       map[string]any{"type": "string", "description": "Note type filter: decision, trace, or intent (optional, omit for all)"},
		},
	}}, wrap(warrantShowGitNotesHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "warrant_log_git_notes", Description: "Log commits with notes (last N). Params: limit (default 20), optional repo_path, type (default decision). Returns list of {commit_sha, ref, body} or commands for warrant-git note log.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit":     map[string]any{"type": "integer", "description": "Number of entries to return (default: 20)"},
			"repo_path": map[string]any{"type": "string", "description": "Path to git repo (optional, for server-side execution)"},
			"type":      map[string]any{"type": "string", "description": "Note type: decision, trace, or intent (default: decision)"},
		},
	}}, wrap(warrantLogGitNotesHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "warrant_diff_git_notes", Description: "Notes on commits in base..head. Params: base, head (required), optional repo_path, type (default decision). Returns entries or commands for warrant-git note diff.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"base":      map[string]any{"type": "string", "description": "Base commit SHA or ref"},
			"head":      map[string]any{"type": "string", "description": "Head commit SHA or ref"},
			"repo_path": map[string]any{"type": "string", "description": "Path to git repo (optional, for server-side execution)"},
			"type":      map[string]any{"type": "string", "description": "Note type: decision, trace, or intent (default: decision)"},
		},
		"required": []string{"base", "head"},
	}}, wrap(warrantDiffGitNotesHandler))
	mcp.AddTool(s, &mcp.Tool{Name: "warrant_sync_git_notes", Description: "Push/pull refs/notes/warrant/*. Params: optional repo_path, direction (push|pull|both). Usually returns commands to run warrant-git sync locally.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"repo_path":  map[string]any{"type": "string", "description": "Path to git repo (optional)"},
			"direction":  map[string]any{"type": "string", "description": "Sync direction: push, pull, or both (default: both)"},
		},
	}}, wrap(warrantSyncGitNotesHandler))
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
	if id := getString(args, "agent_id", ""); id != "" {
		return id, nil
	}
	if id := rest.GetAgentID(ctx); id != "" {
		return id, nil
	}
	// Stdio fallback: resolve agent ID from WARRANT_TOKEN env var (JWT).
	if token := os.Getenv("WARRANT_TOKEN"); token != "" {
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
	claimed, _ := b.Ticket.ListByState(ctx, projectID, ticket.StateClaimed)
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
	return jsonResult(p)
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
			"message":          "**Required:** Check out branch '" + ws.Branch + "' before start_ticket or committing. If it does not exist: `git checkout -b " + ws.Branch + "`. The work stream is already linked to this branch in Warrant.",
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
				"enabled":  true,
				"action":   "checkout_default_branch",
				"branch":   defaultBranch,
				"message":  "Work stream closed. Checkout default branch with `git checkout " + defaultBranch + "`",
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
		projectID, err := requireString(args,"project_id")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		title, err := requireString(args,"title")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		description, err := requireString(args,"description")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
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
		if t := getString(args,"ticket_type", ""); t != "" {
			switch t {
			case "task", "bug", "spike", "review":
				typ = ticket.TicketType(t)
			}
		}
		prio := ticket.P2
		if p := getInt(args,"priority", -1); p >= 0 && p <= 3 {
			prio = ticket.Priority(p)
		}
		var successCriteria []string
		if s := getString(args,"success_criteria", ""); s != "" {
			_ = json.Unmarshal([]byte(s), &successCriteria)
		}
		acceptanceTest := getString(args,"acceptance_test", "")
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
		dependsOn := []string{}
		if d := getString(args, "depends_on", ""); d != "" {
			_ = json.Unmarshal([]byte(d), &dependsOn)
		}
		t, err := b.Ticket.CreateTicket(ctx, projectID, title, typ, prio, agentID, dependsOn, workStreamID, objective, ticket.TicketContext{}, idempotencyKey)
		if err != nil {
			return toolErrTriple(apierrors.MapError(err))
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
		filterOrgID := getString(args,"org_id", "")
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
		projectID, err := requireString(args,"project_id")
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
		projectID, err := requireString(args,"project_id")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		workStreamID := getString(args, "work_stream_id", "")
		state := ticket.State(getString(args, "state", ""))
		list, err := b.Ticket.ListTickets(ctx, projectID, workStreamID, state)
		if err != nil {
			return toolErrTriple(apierrors.MapError(err))
		}
		if p := getInt(args,"priority", -1); p >= 0 && p <= 3 {
			filtered := make([]*ticket.Ticket, 0)
			for _, t := range list {
				if int(t.Priority) == p {
					filtered = append(filtered, t)
				}
			}
			list = filtered
		}
		stateStr := getString(args, "state", "")
		out := map[string]any{"tickets": list}
		if stateStr == "" || ticket.State(stateStr) == ticket.StatePending {
			out["workflow"] = workflowAfterListTicketsPending()
		}
		return jsonResult(out)
}

func getTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
		ticketID, err := requireString(args,"ticket_id")
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
			"ticket":              t,
			"context_pack":        pack,
			"dependency_outputs":  depOutputs,
			"prior_attempts":      t.Context.PriorAttempts,
			"human_answers":       t.Context.HumanAnswers,
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
		return jsonResult(out)
}

func updateTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
		agentID, err := getAgentIDFromArgs(ctx, args)
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		projectID, err := requireString(args,"project_id")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		ticketID, err := requireString(args,"ticket_id")
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
		dependsOnStr := getString(args,"depends_on", "")
		if dependsOnStr != "" {
			var dependsOn []string
			if err := json.Unmarshal([]byte(dependsOnStr), &dependsOn); err != nil {
				return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "depends_on must be a JSON array of ticket ID strings", false))
			}
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
	log.Printf("MCP claim_ticket called with args: %v", args)
	projectID, err := requireString(args, "project_id")
	if err != nil {
		log.Printf("MCP claim_ticket: missing project_id: %v", err)
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		log.Printf("MCP claim_ticket: agent error: %v", err)
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
	log.Printf("MCP claim_ticket: attempting claim for agent=%s project=%s", agentID, projectID)
	t, lease, err := b.Queue.ClaimTicket(ctx, agentID, projectID, priority, idempotencyKey)
	if err != nil {
		log.Printf("MCP claim_ticket: error: %v", err)
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
		ticketID, err := requireString(args,"ticket_id")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		leaseToken, err := requireString(args,"lease_token")
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
		ticketID, err := requireString(args,"ticket_id")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		leaseToken, err := requireString(args,"lease_token")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		stepType, err := requireString(args,"step_type")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		payload := getPayloadMap(args, "payload")
		step := execution.Step{Type: execution.StepType(stepType), Payload: payload}
		if err := b.Trace.LogStep(ctx, ticketID, leaseToken, step); err != nil {
			return toolErrTriple(apierrors.MapError(err))
		}
		return jsonResult(map[string]any{"ok": true})
}

func submitTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
		ticketID, err := requireString(args,"ticket_id")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		leaseToken, err := requireString(args,"lease_token")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		outputsStr, err := requireString(args,"outputs")
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
		return jsonResult(map[string]any{"ok": true})
}

func escalateTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
		ticketID, err := requireString(args,"ticket_id")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		leaseToken, err := requireString(args,"lease_token")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		reason, _ := requireString(args,"reason")
		question, _ := requireString(args,"question")
		if err := b.Ticket.EscalateTicket(ctx, ticketID, leaseToken, reason, question); err != nil {
			return toolErrTriple(apierrors.MapError(err))
		}
		return jsonResult(map[string]any{"ok": true})
}

func renewLeaseHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
		ticketID, err := requireString(args,"ticket_id")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		leaseToken, err := requireString(args,"lease_token")
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
		projectID, err := requireString(args,"project_id")
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
		ticketID, err := requireString(args,"ticket_id")
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
		ticketID, err := requireString(args,"ticket_id")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		reviewerID, err := getAgentIDFromArgs(ctx, args)
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		notes := getString(args,"notes", "")
		if err := b.Review.ApproveTicket(ctx, ticketID, reviewerID, notes); err != nil {
			return toolErrTriple(apierrors.MapError(err))
		}
		return jsonResult(map[string]any{"ok": true, "decision": review.DecisionApproved})
}

func rejectTicketHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
		if b.Review == nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInternal, "reviews not configured", false))
		}
		ticketID, err := requireString(args,"ticket_id")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		reviewerID, err := getAgentIDFromArgs(ctx, args)
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		notes, err := requireString(args,"notes")
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

func warrantAddGitNoteHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	message, err := requireString(args, "message")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	noteType := getString(args, "type", gitnotes.TypeDecision)
	ref := gitnotes.RefForType(noteType)
	if ref == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "type must be decision, trace, or intent", false))
	}
	commitSHA := getString(args, "commit_sha", "HEAD")
	repoPath := getString(args, "repo_path", "")
	ticketID := getString(args, "ticket_id", "")
	projectID := getString(args, "project_id", "")
	agentID, _ := getAgentIDFromArgs(ctx, args)

	payload := map[string]any{
		"v":         1,
		"type":      noteType,
		"message":   message,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}
	if agentID != "" {
		payload["agent_id"] = agentID
	}
	if ticketID != "" {
		payload["ticket_id"] = ticketID
	}
	if projectID != "" {
		payload["project_id"] = projectID
	}
	bodyBytes, _ := json.Marshal(payload)
	body := string(bodyBytes)

	if repoPathAccessible(repoPath) {
		if err := gitnotes.AddNote(repoPath, ref, commitSHA, body); err != nil {
			return jsonResult(map[string]any{"ok": false, "error": err.Error(), "commands": warrantGitNoteAddCommands(noteType, message, commitSHA)})
		}
		return jsonResult(map[string]any{"ok": true, "message": "Note added."})
	}
	return jsonResult(map[string]any{"ok": true, "commands": warrantGitNoteAddCommands(noteType, message, commitSHA), "hint": "Run these in your repo (or install warrant-git and run the first)."})
}

func warrantGitNoteAddCommands(noteType, message, commitSHA string) []string {
	esc := strings.ReplaceAll(message, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	return []string{
		fmt.Sprintf(`warrant-git note add -t %s -m %q -c %s`, noteType, esc, commitSHA),
	}
}

func warrantShowGitNotesHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	commitSHA := getString(args, "commit_sha", "HEAD")
	repoPath := getString(args, "repo_path", "")
	noteType := getString(args, "type", "")

	if noteType != "" && gitnotes.RefForType(noteType) == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "type must be decision, trace, or intent", false))
	}

	if repoPathAccessible(repoPath) {
		if noteType != "" {
			ref := gitnotes.RefForType(noteType)
			if ref == "" {
				return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "type must be decision, trace, or intent", false))
			}
			body, err := gitnotes.ShowNote(repoPath, ref, commitSHA)
			if err != nil {
				return toolErrTriple(apierrors.MapError(err))
			}
			return jsonResult(map[string]any{"commit_sha": commitSHA, "ref": ref, "body": body})
		}
		out := make(map[string]any)
		out["commit_sha"] = commitSHA
		notes := make(map[string]string)
		for _, ref := range gitnotes.AllRefs() {
			body, _ := gitnotes.ShowNote(repoPath, ref, commitSHA)
			if body != "" {
				notes[filepath.Base(ref)] = body
			}
		}
		out["notes"] = notes
		return jsonResult(out)
	}
	cmd := fmt.Sprintf("warrant-git note show -c %s", commitSHA)
	if noteType != "" {
		cmd += " -t " + noteType
	}
	return jsonResult(map[string]any{"commands": []string{cmd}})
}

func warrantLogGitNotesHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	limit := getInt(args, "limit", 20)
	repoPath := getString(args, "repo_path", "")
	noteType := getString(args, "type", gitnotes.TypeDecision)
	ref := gitnotes.RefForType(noteType)
	if ref == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "type must be decision, trace, or intent", false))
	}

	if repoPathAccessible(repoPath) {
		entries, err := gitnotes.Log(repoPath, ref, limit)
		if err != nil {
			return toolErrTriple(apierrors.MapError(err))
		}
		list := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			list = append(list, map[string]any{"commit_sha": e.CommitSHA, "ref": e.Ref, "body": e.Body})
		}
		return jsonResult(map[string]any{"entries": list})
	}
	return jsonResult(map[string]any{"commands": []string{fmt.Sprintf("warrant-git note log -t %s -n %d", noteType, limit)}})
}

func warrantDiffGitNotesHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	base, err := requireString(args, "base")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	head, err := requireString(args, "head")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	repoPath := getString(args, "repo_path", "")
	noteType := getString(args, "type", gitnotes.TypeDecision)
	ref := gitnotes.RefForType(noteType)
	if ref == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "type must be decision, trace, or intent", false))
	}

	if repoPathAccessible(repoPath) {
		entries, err := gitnotes.Diff(repoPath, ref, base, head)
		if err != nil {
			return toolErrTriple(apierrors.MapError(err))
		}
		list := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			list = append(list, map[string]any{"commit_sha": e.CommitSHA, "ref": e.Ref, "body": e.Body})
		}
		return jsonResult(map[string]any{"entries": list})
	}
	return jsonResult(map[string]any{"commands": []string{fmt.Sprintf("warrant-git note diff -t %s %s %s", noteType, base, head)}})
}

func warrantSyncGitNotesHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	direction := getString(args, "direction", "both")
	return jsonResult(map[string]any{
		"commands": []string{fmt.Sprintf("warrant-git sync %s", direction)},
		"hint":     "Run in your repo to push/pull refs/notes/warrant/*.",
	})
}
