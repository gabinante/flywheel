package mcp

import (
	"context"

	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterRepoTools adds multi-repo management MCP tools.
func RegisterRepoTools(s *mcp.Server, b *Backend) {
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

	mcp.AddTool(s, &mcp.Tool{
		Name: "add_project_repository",
		Description: "Add a repository to a multi-repo project. Each repo has a unique alias (e.g. 'backend', 'frontend', 'infra'). " +
			"One repo can be marked as primary (corresponds to the legacy project.repo_url). " +
			"Tickets can target specific repos using the alias. Params: project_id, alias, repo_url, default_branch (optional, default 'main'), is_primary (optional, default false).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":     map[string]any{"type": "string", "description": "Project ID"},
				"alias":          map[string]any{"type": "string", "description": "Unique alias for this repo within the project (e.g. 'backend', 'frontend')"},
				"repo_url":       map[string]any{"type": "string", "description": "Git clone URL for the repository"},
				"default_branch": map[string]any{"type": "string", "description": "Default branch name (optional, default 'main')"},
				"is_primary":     map[string]any{"type": "boolean", "description": "Whether this is the primary repo (optional, default false)"},
				"agent_id":       map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
			},
			"required":             []string{"project_id", "alias", "repo_url"},
			"additionalProperties": false,
		},
	}, wrap(addProjectRepositoryHandler))

	mcp.AddTool(s, &mcp.Tool{
		Name: "list_project_repositories",
		Description: "List all repositories for a project. Returns alias, repo_url, default_branch, and is_primary for each. " +
			"Primary repo is listed first. Use this to discover available target_repo aliases for ticket creation.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project ID"},
				"agent_id":   map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
			},
			"required":             []string{"project_id"},
			"additionalProperties": false,
		},
	}, wrap(listProjectRepositoriesHandler))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "remove_project_repository",
		Description: "Remove a repository from a project by alias. Cannot remove the primary repo while other repos exist.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project ID"},
				"alias":      map[string]any{"type": "string", "description": "Repository alias to remove"},
				"agent_id":   map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
			},
			"required":             []string{"project_id", "alias"},
			"additionalProperties": false,
		},
	}, wrap(removeProjectRepositoryHandler))
}

func addProjectRepositoryHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.Repos == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "multi-repo support is not configured", false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	alias, err := requireString(args, "alias")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	repoURL, err := requireString(args, "repo_url")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	defaultBranch := getString(args, "default_branch", "main")
	isPrimary := getBool(args, "is_primary", false)

	repo, err := b.Repos.AddRepository(ctx, projectID, alias, repoURL, defaultBranch, isPrimary)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{
		"repository": repo,
	})
}

func listProjectRepositoriesHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.Repos == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "multi-repo support is not configured", false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	repos, err := b.Repos.ListRepositories(ctx, projectID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{
		"repositories": repos,
	})
}

func removeProjectRepositoryHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	if b.Repos == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "multi-repo support is not configured", false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	alias, err := requireString(args, "alias")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	if err := b.Repos.RemoveRepository(ctx, projectID, alias); err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(map[string]any{
		"ok": true,
	})
}

// getBool is defined in tools.go — reuse from there.
