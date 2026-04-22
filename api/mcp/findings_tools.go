package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterFindingsTools registers the findings layer MCP tools on the server.
// The tools use the FindingsProvider interface, so the backend (Weaviate, pgvector,
// in-memory) is transparent to tool callers.
func RegisterFindingsTools(s *mcp.Server, b *Backend) {
	if b == nil || b.Findings == nil {
		return // no findings provider configured — skip tool registration
	}

	wrap := func(f func(*Backend, context.Context, map[string]any) (*mcp.CallToolResult, any, error)) func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
		return func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			return f(b, ctx, args)
		}
	}

	// findings_save — Save a new finding with provenance.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "findings_save",
		Description: "Save a semantic finding with provenance. A finding is a claim about code (quality, architecture, security, performance) with tracked origin and symbol/ticket references.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":         map[string]any{"type": "string", "description": "Project ID"},
				"claim":              map[string]any{"type": "string", "description": "The finding's claim text"},
				"finding_type":       map[string]any{"type": "string", "description": "Category of finding", "enum": []string{"code_quality", "architecture", "security", "performance", "design_pattern", "investigation"}},
				"confidence":         map[string]any{"type": "number", "description": "Confidence score 0.0–1.0", "minimum": 0, "maximum": 1},
				"commit_sha":         map[string]any{"type": "string", "description": "Git commit SHA when finding was made"},
				"function_body_hash": map[string]any{"type": "string", "description": "Hash of the function body for invalidation (optional)"},
				"source_type":        map[string]any{"type": "string", "description": "How the finding was produced", "enum": []string{"agent_analysis", "test_result", "review_comment"}},
				"source_ticket_id":   map[string]any{"type": "string", "description": "Ticket that produced this finding (optional)"},
				"source_agent_id":    map[string]any{"type": "string", "description": "Agent that produced this finding (optional)"},
				"symbol_refs":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Symbol IDs this finding relates to (optional)"},
				"ticket_refs":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Ticket IDs this finding relates to (optional)"},
				"file_refs":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "File paths this finding relates to (optional)"},
				"tags":               map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Free-form tags (optional)"},
				"summary":            map[string]any{"type": "string", "description": "Short summary of the finding"},
			},
			"required":             []string{"project_id", "claim", "finding_type", "confidence", "commit_sha", "source_type", "summary"},
			"additionalProperties": false,
		},
	}, wrap(handleFindingsSave))

	// findings_query — Semantic query for findings.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "findings_query",
		Description: "Semantic query for findings. Uses natural language to find relevant findings via vector similarity search.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":   map[string]any{"type": "string", "description": "Project ID"},
				"query":        map[string]any{"type": "string", "description": "Natural language query (e.g., 'performance issues in the API layer')"},
				"finding_type": map[string]any{"type": "string", "description": "Filter by finding type (optional)", "enum": []string{"code_quality", "architecture", "security", "performance", "design_pattern", "investigation"}},
				"valid_only":   map[string]any{"type": "boolean", "description": "Return only valid (non-invalidated) findings (default true)"},
				"limit":        map[string]any{"type": "integer", "description": "Max results (default 20)", "minimum": 1, "maximum": 100},
			},
			"required":             []string{"project_id", "query"},
			"additionalProperties": false,
		},
	}, wrap(handleFindingsQuery))

	// findings_by_symbol — Get findings for a symbol.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "findings_by_symbol",
		Description: "Get findings associated with a specific code symbol. Structural navigation for understanding what's known about a symbol.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project ID"},
				"symbol_ref": map[string]any{"type": "string", "description": "Symbol ID or name"},
				"limit":      map[string]any{"type": "integer", "description": "Max results (default 20)", "minimum": 1, "maximum": 100},
			},
			"required":             []string{"project_id", "symbol_ref"},
			"additionalProperties": false,
		},
	}, wrap(handleFindingsBySymbol))

	// findings_by_ticket — Get findings for a ticket.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "findings_by_ticket",
		Description: "Get findings associated with a specific ticket. Structural navigation for understanding what a ticket discovered or relates to.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project ID"},
				"ticket_ref": map[string]any{"type": "string", "description": "Ticket ID"},
				"limit":      map[string]any{"type": "integer", "description": "Max results (default 20)", "minimum": 1, "maximum": 100},
			},
			"required":             []string{"project_id", "ticket_ref"},
			"additionalProperties": false,
		},
	}, wrap(handleFindingsByTicket))

	// findings_invalidate — Invalidate findings when code changes.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "findings_invalidate",
		Description: "Invalidate findings when underlying code changes. Marks findings as stale based on commit SHA and changed file paths.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":    map[string]any{"type": "string", "description": "Project ID"},
				"commit_sha":    map[string]any{"type": "string", "description": "New commit SHA that changed the code"},
				"changed_files": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "File paths that changed in this commit"},
			},
			"required":             []string{"project_id", "commit_sha", "changed_files"},
			"additionalProperties": false,
		},
	}, wrap(handleFindingsInvalidate))

	// findings_get — Get a specific finding by ID.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "findings_get",
		Description: "Get a specific finding by ID. Returns full finding details with provenance.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project ID"},
				"finding_id": map[string]any{"type": "string", "description": "Finding ID"},
			},
			"required":             []string{"project_id", "finding_id"},
			"additionalProperties": false,
		},
	}, wrap(handleFindingsGet))
}

// --------------------------------------------------------------------------
// Handler implementations
// --------------------------------------------------------------------------

func handleFindingsSave(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	finding := Finding{
		ProjectID:        argStr(args, "project_id"),
		Claim:            argStr(args, "claim"),
		FindingType:      argStr(args, "finding_type"),
		Summary:          argStr(args, "summary"),
		CommitSHA:        argStr(args, "commit_sha"),
		SourceType:       argStr(args, "source_type"),
		FunctionBodyHash: argStr(args, "function_body_hash"),
		SourceTicketID:   argStr(args, "source_ticket_id"),
		SourceAgentID:    argStr(args, "source_agent_id"),
		SymbolRefs:       argStrSlice(args, "symbol_refs"),
		TicketRefs:       argStrSlice(args, "ticket_refs"),
		FileRefs:         argStrSlice(args, "file_refs"),
		Tags:             argStrSlice(args, "tags"),
	}

	if v, ok := args["confidence"].(float64); ok {
		finding.Confidence = v
	}

	saved, err := b.Findings.SaveFinding(ctx, finding)
	if err != nil {
		return nil, nil, fmt.Errorf("saving finding: %w", err)
	}

	return nil, saved, nil
}

func handleFindingsQuery(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := argStr(args, "project_id")
	query := argStr(args, "query")

	opts := FindingsQueryOpts{
		FindingType: argStr(args, "finding_type"),
		ValidOnly:   true, // default
	}
	if v, ok := args["valid_only"].(bool); ok {
		opts.ValidOnly = v
	}
	if v, ok := args["limit"].(float64); ok {
		opts.Limit = int(v)
	}

	findings, err := b.Findings.QueryFindings(ctx, projectID, query, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("querying findings: %w", err)
	}

	return nil, map[string]any{
		"findings": findings,
		"count":    len(findings),
		"query":    query,
	}, nil
}

func handleFindingsBySymbol(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := argStr(args, "project_id")
	symbolRef := argStr(args, "symbol_ref")

	limit := 20
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}

	findings, err := b.Findings.FindingsBySymbol(ctx, projectID, symbolRef, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("findings by symbol: %w", err)
	}

	return nil, map[string]any{
		"findings":   findings,
		"count":      len(findings),
		"symbol_ref": symbolRef,
	}, nil
}

func handleFindingsByTicket(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := argStr(args, "project_id")
	ticketRef := argStr(args, "ticket_ref")

	limit := 20
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}

	findings, err := b.Findings.FindingsByTicket(ctx, projectID, ticketRef, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("findings by ticket: %w", err)
	}

	return nil, map[string]any{
		"findings":   findings,
		"count":      len(findings),
		"ticket_ref": ticketRef,
	}, nil
}

func handleFindingsInvalidate(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := argStr(args, "project_id")
	commitSHA := argStr(args, "commit_sha")
	changedFiles := argStrSlice(args, "changed_files")

	if len(changedFiles) == 0 {
		return nil, nil, fmt.Errorf("changed_files is required and must not be empty")
	}

	count, err := b.Findings.InvalidateFindings(ctx, projectID, commitSHA, changedFiles)
	if err != nil {
		return nil, nil, fmt.Errorf("invalidating findings: %w", err)
	}

	return nil, map[string]any{
		"invalidated_count": count,
		"commit_sha":        commitSHA,
		"changed_files":     changedFiles,
	}, nil
}

func handleFindingsGet(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := argStr(args, "project_id")
	findingID := argStr(args, "finding_id")

	finding, err := b.Findings.GetFinding(ctx, projectID, findingID)
	if err != nil {
		return nil, nil, fmt.Errorf("getting finding: %w", err)
	}

	return nil, finding, nil
}

// --------------------------------------------------------------------------
// Argument helpers
// --------------------------------------------------------------------------

func argStr(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func argStrSlice(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	switch arr := v.(type) {
	case []string:
		return arr
	case []any:
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}
