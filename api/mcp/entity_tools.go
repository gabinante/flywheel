package mcp

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/entity"
)

// RegisterEntityTools adds entity identity MCP tools to the server.
func RegisterEntityTools(s *mcp.Server, b *Backend) {
	if b == nil || b.Entity == nil {
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

	mcp.AddTool(s, &mcp.Tool{Name: "create_entity", Description: "Create a new entity (stable identity for a service, datastore, integration, ticket, or finding). Returns entity with minted UUID. The ID is stable and never reused.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":   map[string]any{"type": "string", "description": "Project ID"},
			"type":         map[string]any{"type": "string", "enum": []string{"service", "datastore", "integration", "ticket", "finding"}, "description": "Entity type"},
			"logical_name": map[string]any{"type": "string", "description": "Human-readable name (mutable)"},
			"description":  map[string]any{"type": "string", "description": "Description of the entity (optional)"},
			"attributes":   map[string]any{"type": "object", "description": "Key-value attributes (external system IDs, metadata). Optional."},
			"agent_id":     map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id", "type", "logical_name"},
	}}, wrap(createEntityHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "get_entity", Description: "Get an entity by its stable UUID. Returns full entity with type, logical name, attributes, and timestamps.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entity_id": map[string]any{"type": "string", "description": "Entity UUID"},
			"agent_id":  map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"entity_id"},
	}}, wrap(getEntityHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "list_entities", Description: "List entities for a project. Optionally filter by type and include retired entities.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":      map[string]any{"type": "string", "description": "Project ID"},
			"type":            map[string]any{"type": "string", "enum": []string{"service", "datastore", "integration", "ticket", "finding"}, "description": "Filter by entity type (optional)"},
			"include_retired": map[string]any{"type": "boolean", "description": "Include retired entities (default: false)"},
			"agent_id":        map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"project_id"},
	}}, wrap(listEntitiesHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "rename_entity", Description: "Rename an entity's logical name. The UUID remains stable; only the human-readable name changes.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entity_id":    map[string]any{"type": "string", "description": "Entity UUID"},
			"logical_name": map[string]any{"type": "string", "description": "New logical name"},
			"agent_id":     map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"entity_id", "logical_name"},
	}}, wrap(renameEntityHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "retire_entity", Description: "Retire (soft-delete) an entity. It remains queryable but is marked inactive. IDs are never reused.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entity_id": map[string]any{"type": "string", "description": "Entity UUID"},
			"agent_id":  map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"entity_id"},
	}}, wrap(retireEntityHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "update_entity_attributes", Description: "Merge key-value attributes into an entity. Use for external system IDs (e.g. github_repo_id, pagerduty_service_id). External IDs become attributes, not primary keys.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entity_id":  map[string]any{"type": "string", "description": "Entity UUID"},
			"attributes": map[string]any{"type": "object", "description": "Key-value pairs to merge into entity attributes"},
			"agent_id":   map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"entity_id", "attributes"},
	}}, wrap(updateEntityAttributesHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "create_entity_instance", Description: "Create an environment-qualified instance of an entity (e.g. OrderService@prod, OrderService@dev). Each entity can have one instance per environment.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entity_id":   map[string]any{"type": "string", "description": "Entity UUID"},
			"environment": map[string]any{"type": "string", "description": "Environment name (e.g. prod, staging, dev)"},
			"attributes":  map[string]any{"type": "object", "description": "Instance-specific attributes (endpoints, versions, etc.). Optional."},
			"agent_id":    map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"entity_id", "environment"},
	}}, wrap(createEntityInstanceHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "list_entity_instances", Description: "List environment instances for an entity.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entity_id":       map[string]any{"type": "string", "description": "Entity UUID"},
			"include_retired": map[string]any{"type": "boolean", "description": "Include retired instances (default: false)"},
			"agent_id":        map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"entity_id"},
	}}, wrap(listEntityInstancesHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "retire_entity_instance", Description: "Retire (soft-delete) an entity instance.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"instance_id": map[string]any{"type": "string", "description": "Instance UUID"},
			"agent_id":    map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"instance_id"},
	}}, wrap(retireEntityInstanceHandler))

	mcp.AddTool(s, &mcp.Tool{Name: "get_entity_stream", Description: "Get the append-only event stream for an entity (create, rename, retire events). Returns events in chronological order.", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entity_id": map[string]any{"type": "string", "description": "Entity UUID"},
			"limit":     map[string]any{"type": "integer", "description": "Max events to return (default: 100)"},
			"agent_id":  map[string]any{"type": "string", "description": "Agent ID (optional, inferred from OAuth when using URL auth)"},
		},
		"required": []string{"entity_id"},
	}}, wrap(getEntityStreamHandler))
}

// --- Handler implementations ---

func createEntityHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	projectID, err := requireString(args, "project_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entityType, err := requireString(args, "type")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	logicalName, err := requireString(args, "logical_name")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	if err := checkProjectAccess(ctx, b, agentID, projectID); err != nil {
		return toolErrTriple(err)
	}
	description := getString(args, "description", "")
	var attributes map[string]any
	if a, ok := args["attributes"]; ok && a != nil {
		if m, ok := a.(map[string]any); ok {
			attributes = m
		}
	}

	e, createErr := b.Entity.CreateEntity(ctx, projectID, entity.Type(entityType), logicalName, description, attributes, agentID)
	if createErr != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, createErr.Error(), false))
	}
	return jsonResult(e)
}

func getEntityHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	entityID, err := requireString(args, "entity_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	e, err := b.Entity.GetEntity(ctx, entityID)
	if err != nil {
		if errors.Is(err, entity.ErrNotFound) {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "entity not found", false))
		}
		return toolErrTriple(apierrors.MapError(err))
	}
	return jsonResult(e)
}

func listEntitiesHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
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
	entityType := entity.Type(getString(args, "type", ""))
	includeRetired := getBool(args, "include_retired", false)

	entities, listErr := b.Entity.ListEntities(ctx, projectID, entityType, includeRetired)
	if listErr != nil {
		return toolErrTriple(apierrors.MapError(listErr))
	}
	if entities == nil {
		entities = []*entity.Entity{}
	}
	return jsonResult(entities)
}

func renameEntityHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entityID, err := requireString(args, "entity_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	logicalName, err := requireString(args, "logical_name")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	if renameErr := b.Entity.RenameEntity(ctx, entityID, logicalName, agentID); renameErr != nil {
		if errors.Is(renameErr, entity.ErrNotFound) {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "entity not found", false))
		}
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, renameErr.Error(), false))
	}
	// Return updated entity
	e, _ := b.Entity.GetEntity(ctx, entityID)
	return jsonResult(e)
}

func retireEntityHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entityID, err := requireString(args, "entity_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	if retireErr := b.Entity.RetireEntity(ctx, entityID, agentID); retireErr != nil {
		if errors.Is(retireErr, entity.ErrNotFound) {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "entity not found or already retired", false))
		}
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, retireErr.Error(), false))
	}
	return jsonResult(map[string]any{"ok": true, "entity_id": entityID})
}

func updateEntityAttributesHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entityID, err := requireString(args, "entity_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	var attributes map[string]any
	if a, ok := args["attributes"]; ok && a != nil {
		if m, ok := a.(map[string]any); ok {
			attributes = m
		}
	}
	if attributes == nil || len(attributes) == 0 {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "attributes object required", false))
	}
	if updateErr := b.Entity.UpdateAttributes(ctx, entityID, attributes, agentID); updateErr != nil {
		if errors.Is(updateErr, entity.ErrNotFound) {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "entity not found", false))
		}
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, updateErr.Error(), false))
	}
	e, _ := b.Entity.GetEntity(ctx, entityID)
	return jsonResult(e)
}

func createEntityInstanceHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	entityID, err := requireString(args, "entity_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	environment, err := requireString(args, "environment")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	var attributes map[string]any
	if a, ok := args["attributes"]; ok && a != nil {
		if m, ok := a.(map[string]any); ok {
			attributes = m
		}
	}
	inst, createErr := b.Entity.CreateInstance(ctx, entityID, environment, attributes, agentID)
	if createErr != nil {
		if errors.Is(createErr, entity.ErrNotFound) {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "entity not found", false))
		}
		if errors.Is(createErr, entity.ErrDuplicateEnv) {
			return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "instance for this environment already exists", false))
		}
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, createErr.Error(), false))
	}
	return jsonResult(inst)
}

func listEntityInstancesHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	entityID, err := requireString(args, "entity_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	includeRetired := getBool(args, "include_retired", false)
	instances, listErr := b.Entity.ListInstances(ctx, entityID, includeRetired)
	if listErr != nil {
		return toolErrTriple(apierrors.MapError(listErr))
	}
	if instances == nil {
		instances = []*entity.Instance{}
	}
	return jsonResult(instances)
}

func retireEntityInstanceHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	agentID, err := getAgentIDFromArgs(ctx, args)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	instanceID, err := requireString(args, "instance_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	if retireErr := b.Entity.RetireInstance(ctx, instanceID, agentID); retireErr != nil {
		if errors.Is(retireErr, entity.ErrNotFound) {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "instance not found or already retired", false))
		}
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, retireErr.Error(), false))
	}
	return jsonResult(map[string]any{"ok": true, "instance_id": instanceID})
}

func getEntityStreamHandler(b *Backend, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	entityID, err := requireString(args, "entity_id")
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, err.Error(), false))
	}
	limit := getInt(args, "limit", 100)
	events, streamErr := b.Entity.GetStream(ctx, entityID, limit)
	if streamErr != nil {
		return toolErrTriple(apierrors.MapError(streamErr))
	}
	if events == nil {
		events = []*entity.StreamEvent{}
	}
	return jsonResult(events)
}

// getEntityStreamSinceHandler could be added for tailing the global stream.
// For now, per-entity stream suffices.
var _ = time.Time{} // keep time import for future use
