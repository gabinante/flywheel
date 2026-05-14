package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/workflow"
)

func registerWorkflowTools(s *mcp.Server, b *Backend, wrap wrapFn) {
	if b.Workflow == nil {
		return
	}

	mcp.AddTool(s, &mcp.Tool{Name: "get_workflow_position", Description: "Get the current workflow position for a ticket — current phase, progress, and phase history. Returns null if the ticket has no workflow assigned.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
		},
		"required":             []string{"ticket_id"},
		"additionalProperties": false,
	}}, wrap(getWorkflowPositionHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "list_workflow_templates", Description: "List built-in workflow templates (Standard SDLC, Fast Track, Full Pipeline). Templates can be used as starting points for project workflow configuration.", InputSchema: map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}}, wrap(listWorkflowTemplatesHandler))
}

func getWorkflowPositionHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	ticketID, err := requireString(args, "ticket_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	t, err := b.Ticket.GetTicket(ctx, ticketID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	if t.WorkflowID == "" {
		return jsonResult(map[string]any{"position": nil, "message": "ticket has no workflow assigned"})
	}
	pos, err := b.Workflow.GetPosition(ctx, ticketID, t.WorkflowID, t.WorkflowPhase, t.WorkflowVersion)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"position": pos})
}

func listWorkflowTemplatesHandler(_ *Backend, _ context.Context, _ map[string]any) (*mcp.CallToolResult, any, error) {
	templates := workflow.BuiltinTemplates()
	return jsonResult(map[string]any{"templates": templates})
}
