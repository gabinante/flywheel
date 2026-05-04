package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/gabinante/flywheel/api/rest"
	apierrors "github.com/gabinante/flywheel/internal/errors"
)

func registerProjectTemplateTools(s *mcp.Server, b *Backend, wrap wrapFn) {
	if b.ProjectTemplate == nil {
		return
	}

	mcp.AddTool(s, &mcp.Tool{Name: "list_project_templates", Description: "List project templates with their workstream templates expanded. Project templates are named combinations of workstream templates that can be used to seed a new project with pre-defined workstreams and tickets.", InputSchema: map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}}, wrap(listProjectTemplatesHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "list_workstream_templates", Description: "List workstream templates from the composable library. Each template defines a workstream with pre-defined tickets. Use with create_project's template_id and work_stream_template_ids params to seed a new project.", InputSchema: map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}}, wrap(listWorkstreamTemplatesHandler))
}

func listProjectTemplatesHandler(b *Backend, ctx context.Context, _ map[string]any) (*mcp.CallToolResult, any, error) {
	orgID := resolveOrgIDFromContext(b, ctx)
	templates, err := b.ProjectTemplate.ListProjectTemplatesExpanded(ctx, orgID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"templates": templates})
}

func listWorkstreamTemplatesHandler(b *Backend, ctx context.Context, _ map[string]any) (*mcp.CallToolResult, any, error) {
	orgID := resolveOrgIDFromContext(b, ctx)
	templates, err := b.ProjectTemplate.ListWorkstreamTemplates(ctx, orgID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"templates": templates})
}

// resolveOrgIDFromContext extracts the org ID for the authenticated agent.
func resolveOrgIDFromContext(b *Backend, ctx context.Context) string {
	agentID := rest.GetAgentID(ctx)
	if agentID == "" {
		return ""
	}
	ag, err := b.AgentStore.GetByID(ctx, agentID)
	if err != nil || ag == nil || ag.UserID == "" {
		return ""
	}
	orgs, err := b.Org.ListOrgsForUser(ctx, ag.UserID)
	if err != nil || len(orgs) == 0 {
		return ""
	}
	return orgs[0].ID
}
