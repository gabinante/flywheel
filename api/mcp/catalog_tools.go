package mcp

import (
	"context"

	"github.com/gabinante/flywheel/internal/catalog"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterCatalogTools adds Layer 14 project map MCP tools to the server.
// These implement the CatalogContract from contracts.go.
func RegisterCatalogTools(s *mcp.Server, b *Backend) {
	if b == nil || b.Catalog == nil {
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
		Name:        "catalog_list_entities",
		Description: "List entities in the catalog, optionally filtered by type, project, or label.",
		InputSchema: CatalogContract.Tools[0].InputSchema,
	}, wrap(catalogListEntitiesHandler))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "catalog_get_entity",
		Description: "Get a single entity by ID with all its metadata, labels, and relationships.",
		InputSchema: CatalogContract.Tools[1].InputSchema,
	}, wrap(catalogGetEntityHandler))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "catalog_create_entity",
		Description: "Create a new entity in the catalog. Returns the entity with generated ID.",
		InputSchema: CatalogContract.Tools[2].InputSchema,
	}, wrap(catalogCreateEntityHandler))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "catalog_update_entity",
		Description: "Update an existing entity's metadata, labels, or description.",
		InputSchema: CatalogContract.Tools[3].InputSchema,
	}, wrap(catalogUpdateEntityHandler))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "catalog_list_edges",
		Description: "List relationships (edges) for an entity or between entities.",
		InputSchema: CatalogContract.Tools[4].InputSchema,
	}, wrap(catalogListEdgesHandler))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "catalog_create_edge",
		Description: "Create a typed relationship between two entities.",
		InputSchema: CatalogContract.Tools[5].InputSchema,
	}, wrap(catalogCreateEdgeHandler))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "catalog_bootstrap_scan",
		Description: "Trigger a bootstrap scan of the repository to auto-discover entities from Dockerfiles, docker-compose, K8s manifests, Terraform, and varlock schemas.",
		InputSchema: CatalogContract.Tools[6].InputSchema,
	}, wrap(catalogBootstrapScanHandler))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "catalog_deployment_matrix",
		Description: "Get the deployment matrix: service x environment x version. Shows what's deployed where.",
		InputSchema: CatalogContract.Tools[7].InputSchema,
	}, wrap(catalogDeploymentMatrixHandler))
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func catalogListEntitiesHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := getString(args, "project_id", "")
	if projectID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id required", false))
	}
	entityType := getString(args, "entity_type", "")
	label := getString(args, "label", "")
	limit := getInt(args, "limit", 50)

	entities, err := b.Catalog.ListEntities(ctx, projectID, entityType, label, limit)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	if entities == nil {
		entities = []*catalog.Entity{}
	}
	return jsonResult(map[string]any{"entities": entities})
}

func catalogGetEntityHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := getString(args, "project_id", "")
	entityID := getString(args, "entity_id", "")
	if projectID == "" || entityID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id and entity_id required", false))
	}

	entity, err := b.Catalog.GetEntity(ctx, projectID, entityID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}

	// Also fetch edges for this entity.
	edges, _ := b.Catalog.ListEdges(ctx, projectID, entityID, "", "both")
	if edges == nil {
		edges = []*catalog.Edge{}
	}
	return jsonResult(map[string]any{"entity": entity, "edges": edges})
}

func catalogCreateEntityHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := getString(args, "project_id", "")
	entityType := getString(args, "entity_type", "")
	name := getString(args, "name", "")
	description := getString(args, "description", "")
	source := getString(args, "source", "declared")

	if projectID == "" || entityType == "" || name == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id, entity_type, and name required", false))
	}

	labels := getStringStringMap(args, "labels")
	metadata := getStringStringMap(args, "metadata")

	entity, err := b.Catalog.CreateEntity(ctx, projectID, entityType, name, description, labels, metadata, source)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(entity)
}

func catalogUpdateEntityHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := getString(args, "project_id", "")
	entityID := getString(args, "entity_id", "")
	if projectID == "" || entityID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id and entity_id required", false))
	}

	name := getString(args, "name", "")
	description := getString(args, "description", "")
	labels := getStringStringMap(args, "labels")
	metadata := getStringStringMap(args, "metadata")

	entity, err := b.Catalog.UpdateEntity(ctx, projectID, entityID, name, description, labels, metadata)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(entity)
}

func catalogListEdgesHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := getString(args, "project_id", "")
	entityID := getString(args, "entity_id", "")
	if projectID == "" || entityID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id and entity_id required", false))
	}
	edgeType := getString(args, "edge_type", "")
	direction := getString(args, "direction", "both")

	edges, err := b.Catalog.ListEdges(ctx, projectID, entityID, edgeType, direction)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	if edges == nil {
		edges = []*catalog.Edge{}
	}
	return jsonResult(map[string]any{"edges": edges})
}

func catalogCreateEdgeHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := getString(args, "project_id", "")
	fromID := getString(args, "from_id", "")
	toID := getString(args, "to_id", "")
	edgeType := getString(args, "edge_type", "")
	source := getString(args, "source", "declared")

	if projectID == "" || fromID == "" || toID == "" || edgeType == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id, from_id, to_id, and edge_type required", false))
	}

	metadata := getStringStringMap(args, "metadata")
	edge, err := b.Catalog.CreateEdge(ctx, projectID, fromID, toID, edgeType, metadata, source)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(edge)
}

func catalogBootstrapScanHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := getString(args, "project_id", "")
	if projectID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id required", false))
	}
	repoPath := getString(args, "repo_path", "")
	if repoPath == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "repo_path required for bootstrap scan", false))
	}

	if b.CatalogScanner == nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, "scanner not configured", false))
	}
	scanResult, err := b.CatalogScanner.ScanRepo(repoPath)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}

	imported, err := b.Catalog.ImportScanResult(ctx, projectID, scanResult)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}

	return jsonResult(map[string]any{
		"entities_created": len(imported.Entities),
		"edges_created":    len(imported.Edges),
		"errors":           imported.Errors,
	})
}

func catalogDeploymentMatrixHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID := getString(args, "project_id", "")
	if projectID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id required", false))
	}
	serviceID := getString(args, "service_id", "")

	deployments, err := b.Catalog.DeploymentMatrix(ctx, projectID, serviceID)
	if err != nil {
		return toolErrTriple(apierrors.MapError(err))
	}
	if deployments == nil {
		deployments = []*catalog.DeploymentEntry{}
	}
	return jsonResult(map[string]any{"deployments": deployments})
}

// getStringStringMap extracts a map[string]string from args, handling the
// map[string]any that MCP JSON deserialization produces.
func getStringStringMap(args map[string]any, key string) map[string]string {
	if args == nil {
		return nil
	}
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, val := range m {
		if s, ok := val.(string); ok {
			result[k] = s
		}
	}
	return result
}
