package mcp

import (
	"context"
	"time"

	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterReportTools adds Linear reporting tools (project status updates, weekly roundup).
func RegisterReportTools(s *mcp.Server, b *Backend) {
	if b == nil || b.Reports == nil {
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
	mcp.AddTool(s, &mcp.Tool{
		Name:        "preview_project_update",
		Description: "Render the delta status update for a Linear-linked project (done, merged, in review, blocked, review feedback, agent activity) since the last posted update, without posting. Params: project_id.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"project_id": map[string]any{"type": "string"}}, "required": []string{"project_id"}, "additionalProperties": false},
	}, wrap(func(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
		pid, err := requireString(args, "project_id")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		r, err := b.Reports.ComposeProjectUpdate(ctx, pid)
		if err != nil {
			return toolErrTriple(apierrors.MapError(err))
		}
		return jsonResult(r)
	}))
	mcp.AddTool(s, &mcp.Tool{
		Name:        "post_project_update",
		Description: "Post the delta status update to the project's Linear project. Params: project_id; optional body (edited Markdown), health (onTrack|atRisk|offTrack).",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"project_id": map[string]any{"type": "string"}, "body": map[string]any{"type": "string"}, "health": map[string]any{"type": "string", "enum": []string{"onTrack", "atRisk", "offTrack"}},
		}, "required": []string{"project_id"}, "additionalProperties": false},
	}, wrap(func(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
		pid, err := requireString(args, "project_id")
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		r, err := b.Reports.PostProjectUpdate(ctx, pid, getString(args, "body", ""), getString(args, "health", ""))
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		return jsonResult(r)
	}))
	mcp.AddTool(s, &mcp.Tool{
		Name:        "preview_weekly_roundup",
		Description: "Render this week's roundup (merged PRs, per-project ticket table, agent activity) without posting. Params: optional week_of (YYYY-MM-DD).",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"week_of": map[string]any{"type": "string"}}, "additionalProperties": false},
	}, wrap(func(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
		r, err := b.Reports.ComposeWeeklyRoundup(ctx, weekOfArg(args))
		if err != nil {
			return toolErrTriple(apierrors.MapError(err))
		}
		return jsonResult(r)
	}))
	mcp.AddTool(s, &mcp.Tool{
		Name:        "post_weekly_roundup",
		Description: "Post the weekly roundup to the rolling Linear document (REPORT_ROUNDUP_DOCUMENT_ID) and the roundup project. Params: optional week_of (YYYY-MM-DD), body (edited Markdown).",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"week_of": map[string]any{"type": "string"}, "body": map[string]any{"type": "string"}}, "additionalProperties": false},
	}, wrap(func(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
		r, err := b.Reports.PostWeeklyRoundup(ctx, weekOfArg(args), getString(args, "body", ""))
		if err != nil {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
		}
		return jsonResult(r)
	}))
}

func weekOfArg(args map[string]any) time.Time {
	if v := getString(args, "week_of", ""); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
			return t
		}
	}
	return time.Now()
}
