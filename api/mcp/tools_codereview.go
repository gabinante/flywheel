package mcp

import (
	"context"

	"github.com/gabinante/flywheel/internal/codereview"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterCodeReviewTools adds tools to queue and inspect PR reviews. Queue fan-out
// ("review these five PRs") is the caller's job: pass all the URLs in one call.
func RegisterCodeReviewTools(s *mcp.Server, b *Backend) {
	if b == nil || b.CodeReview == nil {
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
		Name: "request_code_review",
		Description: "Queue in-depth code reviews for one or more pull requests. Pass free text containing GitHub PR URLs or owner/repo#N " +
			"references. Each PR is reviewed in a detached worktree at its head by the configured harness (Codex by default); findings are " +
			"posted as one GitHub review with inline conversational comments, requesting changes on any P0/P1 and approving otherwise. " +
			"Set dry_run=true to record findings without posting. Params: text (required), dry_run, watch, harness (codex|claude).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":    map[string]any{"type": "string"},
				"dry_run": map[string]any{"type": "boolean"},
				"watch":   map[string]any{"type": "boolean", "description": "Re-review on new commits or dismissed reviews (default true)"},
				"harness": map[string]any{"type": "string", "enum": []string{"codex", "claude"}},
			},
			"required":             []string{"text"},
			"additionalProperties": false,
		},
	}, wrap(requestCodeReviewHandler))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_code_reviews",
		Description: "List PR review requests and their state, verdict, and finding counts. Params (optional): state, repo, limit.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"state": map[string]any{"type": "string"},
				"repo":  map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
			},
			"additionalProperties": false,
		},
	}, wrap(listCodeReviewsHandler))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_code_review",
		Description: "Get one PR review request with its findings. Params: review_id.",
		InputSchema: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"review_id": map[string]any{"type": "string"}},
			"required":             []string{"review_id"},
			"additionalProperties": false,
		},
	}, wrap(getCodeReviewHandler))
}

func requestCodeReviewHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	text, err := requireString(args, "text")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	opts := codereview.EnqueueOptions{Harness: getString(args, "harness", "")}
	if v, ok := args["dry_run"].(bool); ok {
		opts.DryRun = &v
	}
	if v, ok := args["watch"].(bool); ok {
		opts.Watch = &v
	}
	reqs, err := b.CodeReview.Enqueue(ctx, text, codereview.OriginMCP, opts)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	return jsonResult(map[string]any{"requests": reqs, "count": len(reqs)})
}

func listCodeReviewsHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	list, total, err := b.CodeReview.List(ctx, codereview.Filter{State: codereview.State(getString(args, "state", "")), Repo: getString(args, "repo", ""), Limit: getInt(args, "limit", 25)})
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{"requests": list, "total": total})
}

func getCodeReviewHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	id, err := requireString(args, "review_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	r, err := b.CodeReview.Get(ctx, id)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	if r == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "review request not found", false))
	}
	return jsonResult(r)
}
