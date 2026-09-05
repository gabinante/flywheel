package mcp

import (
	"context"
	"strings"

	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/sessions"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterSessionTools adds tools for querying and linking tracked harness sessions.
func RegisterSessionTools(s *mcp.Server, b *Backend) {
	if b == nil || b.Sessions == nil {
		return
	}
	wrap := func(f func(*Backend, context.Context, map[string]any) (*mcp.CallToolResult, any, error)) func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
		return func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			if req != nil && req.Session != nil {
				ctx = context.WithValue(ctx, sessionContextKey{}, req.Session)
			}
			if req != nil && req.Params != nil {
				if err := authorizeRun(ctx, req.Params.Name, args); err != nil {
					return toolErrTriple(apierrors.New(apierrors.CodeForbidden, err.Error(), false))
				}
			}
			return f(b, ctx, args)
		}
	}

	mcp.AddTool(s, &mcp.Tool{
		Name: "list_sessions",
		Description: "List tracked Claude Code and Codex sessions (interactive, dispatched, automation, subagent), most recent first. " +
			"Use it to answer 'what have I been working on', find the session that touched a PR or Linear issue, or check what is running now. " +
			"Params (all optional): harness (claude_code|codex), origin, repo (owner/name or bare name), branch, status (active|idle|ended), q (full-text), limit.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"harness": map[string]any{"type": "string", "enum": []string{"claude_code", "codex"}},
				"origin":  map[string]any{"type": "string", "enum": []string{"interactive", "dispatched", "automation", "subagent"}},
				"repo":    map[string]any{"type": "string"},
				"branch":  map[string]any{"type": "string"},
				"status":  map[string]any{"type": "string", "enum": []string{"active", "idle", "ended"}},
				"q":       map[string]any{"type": "string", "description": "Search prompts, titles, repos, branches, and linked refs"},
				"limit":   map[string]any{"type": "integer", "description": "Max results (default 20)"},
			},
			"additionalProperties": false,
		},
	}, wrap(listSessionsHandler))

	mcp.AddTool(s, &mcp.Tool{
		Name: "link_session",
		Description: "Link a tracked session to a pull request (owner/repo#123), Linear issue (KEY-123), Flywheel ticket id, or review request id. " +
			"Identify the session by session_id, or by harness + external_id (the Claude Code sessionId or Codex thread id). Params: kind, ref, session_id | (harness, external_id).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"session_id":  map[string]any{"type": "string"},
				"harness":     map[string]any{"type": "string", "enum": []string{"claude_code", "codex"}},
				"external_id": map[string]any{"type": "string"},
				"kind":        map[string]any{"type": "string", "enum": []string{"pr", "linear_issue", "ticket", "review"}},
				"ref":         map[string]any{"type": "string"},
			},
			"required":             []string{"kind", "ref"},
			"additionalProperties": false,
		},
	}, wrap(linkSessionHandler))
}

func listSessionsHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	f := sessions.Filter{
		Harness: sessions.Harness(getString(args, "harness", "")),
		Origin:  sessions.Origin(getString(args, "origin", "")),
		Repo:    getString(args, "repo", ""),
		Branch:  getString(args, "branch", ""),
		Status:  sessions.Status(getString(args, "status", "")),
		Query:   getString(args, "q", ""),
		Limit:   getInt(args, "limit", 20),
	}
	list, total, err := b.Sessions.List(ctx, f)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"sessions": list, "total": total})
}

func linkSessionHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	kind, err := requireString(args, "kind")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	ref, err := requireString(args, "ref")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	var sess *sessions.Session
	if id := getString(args, "session_id", ""); id != "" {
		sess, err = b.Sessions.Get(ctx, id)
	} else if ext := getString(args, "external_id", ""); ext != "" {
		sess, err = b.Sessions.GetByExternal(ctx, sessions.Harness(getString(args, "harness", "")), ext)
	} else {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "session_id or harness+external_id is required", false))
	}
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	if sess == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "session not found", false))
	}
	if err := b.Sessions.Link(ctx, sess.ID, kind, strings.TrimSpace(ref), sessions.LinkSourceExplicit); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"session_id": sess.ID, "kind": kind, "ref": ref, "linked": true})
}
