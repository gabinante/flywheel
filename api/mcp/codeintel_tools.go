package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	apierrors "github.com/gabinante/flywheel/internal/errors"
)

// RegisterCodeIntelTools registers all code intelligence MCP tools (Layer 3).
// Tools are only registered when b.CodeIntel is non-nil.
func RegisterCodeIntelTools(s *sdkmcp.Server, b *Backend) {
	if b == nil || b.CodeIntel == nil {
		return
	}

	wrap := func(f func(*Backend, context.Context, map[string]any) (*sdkmcp.CallToolResult, any, error)) func(context.Context, *sdkmcp.CallToolRequest, map[string]any) (*sdkmcp.CallToolResult, any, error) {
		return func(ctx context.Context, req *sdkmcp.CallToolRequest, args map[string]any) (*sdkmcp.CallToolResult, any, error) {
			return f(b, ctx, args)
		}
	}

	// --- Required tools ---

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "code_symbol_lookup",
		Description: CodeIntelligenceContract.Tools[0].Description,
		InputSchema: CodeIntelligenceContract.Tools[0].InputSchema,
	}, wrap(codeSymbolLookupHandler))

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "code_callers",
		Description: CodeIntelligenceContract.Tools[1].Description,
		InputSchema: CodeIntelligenceContract.Tools[1].InputSchema,
	}, wrap(codeCallersHandler))

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "code_callees",
		Description: CodeIntelligenceContract.Tools[2].Description,
		InputSchema: CodeIntelligenceContract.Tools[2].InputSchema,
	}, wrap(codeCalleesHandler))

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "code_blast_radius",
		Description: CodeIntelligenceContract.Tools[3].Description,
		InputSchema: CodeIntelligenceContract.Tools[3].InputSchema,
	}, wrap(codeBlastRadiusHandler))

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "code_importers",
		Description: CodeIntelligenceContract.Tools[4].Description,
		InputSchema: CodeIntelligenceContract.Tools[4].InputSchema,
	}, wrap(codeImportersHandler))

	// --- Optional tools (only if extended interface is implemented) ---

	if ext, ok := b.CodeIntel.(CodeIntelligenceExtended); ok && ext != nil {
		sdkmcp.AddTool(s, &sdkmcp.Tool{
			Name:        "code_reindex",
			Description: CodeIntelligenceContract.Tools[5].Description,
			InputSchema: CodeIntelligenceContract.Tools[5].InputSchema,
		}, wrap(codeReindexHandler))

		sdkmcp.AddTool(s, &sdkmcp.Tool{
			Name:        "code_symbol_metadata",
			Description: CodeIntelligenceContract.Tools[6].Description,
			InputSchema: CodeIntelligenceContract.Tools[6].InputSchema,
		}, wrap(codeSymbolMetadataHandler))
	}
}

// --- Handler implementations ---

func codeSymbolLookupHandler(b *Backend, ctx context.Context, args map[string]any) (*sdkmcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	query, err := requireString(args, "query")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}

	opts := SymbolLookupOpts{
		Language: getString(args, "language", ""),
		Kind:     getString(args, "kind", ""),
		Limit:    getInt(args, "limit", 20),
	}

	symbols, err := b.CodeIntel.SymbolLookup(ctx, projectID, query, opts)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	if symbols == nil {
		symbols = []SymbolInfo{}
	}
	return jsonResult(map[string]any{"symbols": symbols, "count": len(symbols)})
}

func codeCallersHandler(b *Backend, ctx context.Context, args map[string]any) (*sdkmcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	symbol, err := requireString(args, "symbol")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}

	depth := getInt(args, "depth", 1)
	limit := getInt(args, "limit", 50)

	callers, err := b.CodeIntel.Callers(ctx, projectID, symbol, depth, limit)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	if callers == nil {
		callers = []CallSite{}
	}
	return jsonResult(map[string]any{"callers": callers, "count": len(callers)})
}

func codeCalleesHandler(b *Backend, ctx context.Context, args map[string]any) (*sdkmcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	symbol, err := requireString(args, "symbol")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}

	depth := getInt(args, "depth", 1)
	limit := getInt(args, "limit", 50)

	callees, err := b.CodeIntel.Callees(ctx, projectID, symbol, depth, limit)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	if callees == nil {
		callees = []CallSite{}
	}
	return jsonResult(map[string]any{"callees": callees, "count": len(callees)})
}

func codeBlastRadiusHandler(b *Backend, ctx context.Context, args map[string]any) (*sdkmcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	symbol, err := requireString(args, "symbol")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}

	depth := getInt(args, "depth", 2)

	result, err := b.CodeIntel.BlastRadius(ctx, projectID, symbol, depth)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	return jsonResult(result)
}

func codeImportersHandler(b *Backend, ctx context.Context, args map[string]any) (*sdkmcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	pkg, err := requireString(args, "package")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}

	limit := getInt(args, "limit", 50)

	importers, err := b.CodeIntel.Importers(ctx, projectID, pkg, limit)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	if importers == nil {
		importers = []string{}
	}
	return jsonResult(map[string]any{"importers": importers, "count": len(importers)})
}

func codeReindexHandler(b *Backend, ctx context.Context, args map[string]any) (*sdkmcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}

	ext, ok := b.CodeIntel.(CodeIntelligenceExtended)
	if !ok {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "code intelligence provider does not support reindex", false))
	}

	commitSHA := getString(args, "commit_sha", "")

	// Extract paths from args (optional array of strings).
	var paths []string
	if v, ok := args["paths"]; ok && v != nil {
		if arr, ok := v.([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok {
					paths = append(paths, s)
				}
			}
		}
	}

	if err := ext.Reindex(ctx, projectID, paths, commitSHA); err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}

	status, err := b.CodeIntel.Status(ctx, projectID)
	if err != nil {
		return jsonResult(map[string]any{"reindexed": true})
	}
	return jsonResult(map[string]any{"reindexed": true, "status": status})
}

func codeSymbolMetadataHandler(b *Backend, ctx context.Context, args map[string]any) (*sdkmcp.CallToolResult, any, error) {
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	symbol, err := requireString(args, "symbol")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}

	ext, ok := b.CodeIntel.(CodeIntelligenceExtended)
	if !ok {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "code intelligence provider does not support symbol metadata", false))
	}

	info, err := ext.SymbolMetadata(ctx, projectID, symbol)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	return jsonResult(info)
}
