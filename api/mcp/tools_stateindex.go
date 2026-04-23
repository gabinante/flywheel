package mcp

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	apierrors "github.com/gabinante/flywheel/internal/errors"
	"github.com/gabinante/flywheel/internal/stateindex"
)

// RegisterStateIndexTools adds Layer 10 (Observed State Index) MCP tools to the server.
// These tools implement the StateIndexContract for state queries, staleness tracking,
// drift detection, and change attribution.
func RegisterStateIndexTools(s *mcp.Server, svc *stateindex.Service) {
	if svc == nil {
		return
	}

	wrap := func(f func(*stateindex.Service, context.Context, map[string]any) (*mcp.CallToolResult, any, error)) func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
		return func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			return f(svc, ctx, args)
		}
	}

	// state_query — Query the observed state index (required by contract).
	mcp.AddTool(s, &mcp.Tool{
		Name:        "state_query",
		Description: "Query the observed state index. Accepts structured queries against known resource types. Returns current state with observation timestamps.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":    map[string]any{"type": "string", "description": "Project ID"},
				"resource_type": map[string]any{"type": "string", "description": "Resource type to query (e.g. 'container', 'service', 'database', 'loadbalancer', 'vm', 'bucket')"},
				"environment":   map[string]any{"type": "string", "description": "Environment filter (optional)"},
				"filter":        map[string]any{"type": "object", "description": "Key-value filters on resource properties (optional)", "additionalProperties": map[string]any{"type": "string"}},
				"limit":         map[string]any{"type": "integer", "description": "Max results (default 50)", "minimum": 1, "maximum": 500},
			},
			"required":             []string{"project_id", "resource_type"},
			"additionalProperties": false,
		},
	}, wrap(stateQueryHandler))

	// state_get_resource — Get full observed state for a resource (required by contract).
	mcp.AddTool(s, &mcp.Tool{
		Name:        "state_get_resource",
		Description: "Get the full observed state for a specific resource by ID. Includes all properties, observation timestamps, and attribution.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":  map[string]any{"type": "string", "description": "Project ID"},
				"resource_id": map[string]any{"type": "string", "description": "Resource ID (stable cross-layer entity ID)"},
			},
			"required":             []string{"project_id", "resource_id"},
			"additionalProperties": false,
		},
	}, wrap(stateGetResourceHandler))

	// state_staleness — Check staleness of observed state (required by contract).
	mcp.AddTool(s, &mcp.Tool{
		Name:        "state_staleness",
		Description: "Check staleness of observed state for resources. Returns resources where observation is older than threshold.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":        map[string]any{"type": "string", "description": "Project ID"},
				"resource_type":     map[string]any{"type": "string", "description": "Resource type filter (optional)"},
				"environment":       map[string]any{"type": "string", "description": "Environment filter (optional)"},
				"threshold_seconds": map[string]any{"type": "integer", "description": "Staleness threshold in seconds (default 300)", "minimum": 1},
			},
			"required":             []string{"project_id"},
			"additionalProperties": false,
		},
	}, wrap(stateStalenessHandler))

	// state_drift — Detect drift between declared and observed state (required by contract).
	mcp.AddTool(s, &mcp.Tool{
		Name:        "state_drift",
		Description: "Detect drift between declared state (from config/IaC) and observed state. Returns mismatches with severity.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":  map[string]any{"type": "string", "description": "Project ID"},
				"resource_id": map[string]any{"type": "string", "description": "Check drift for specific resource (optional, checks all if omitted)"},
				"environment": map[string]any{"type": "string", "description": "Environment filter (optional)"},
			},
			"required":             []string{"project_id"},
			"additionalProperties": false,
		},
	}, wrap(stateDriftHandler))

	// state_attribute_change — Attribute a state change (optional by contract).
	mcp.AddTool(s, &mcp.Tool{
		Name:        "state_attribute_change",
		Description: "Attribute an observed state change to a ticket, external actor, or mark as unattributed.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":  map[string]any{"type": "string", "description": "Project ID"},
				"resource_id": map[string]any{"type": "string", "description": "Resource that changed"},
				"ticket_id":   map[string]any{"type": "string", "description": "Ticket that caused the change (optional)"},
				"actor":       map[string]any{"type": "string", "description": "External actor identifier (optional)"},
				"change_id":   map[string]any{"type": "string", "description": "Change event ID from change stream (optional)"},
			},
			"required":             []string{"project_id", "resource_id"},
			"additionalProperties": false,
		},
	}, wrap(stateAttributeChangeHandler))

	// state_snapshot — Get a point-in-time snapshot (optional by contract).
	mcp.AddTool(s, &mcp.Tool{
		Name:        "state_snapshot",
		Description: "Get a point-in-time snapshot of resource state for freshness stamping in plans.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id":   map[string]any{"type": "string", "description": "Project ID"},
				"resource_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Resource IDs to snapshot"},
				"environment":  map[string]any{"type": "string", "description": "Environment (optional)"},
			},
			"required":             []string{"project_id", "resource_ids"},
			"additionalProperties": false,
		},
	}, wrap(stateSnapshotHandler))

	// state_summary — Get state index summary.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "state_summary",
		Description: "Get a summary of the observed state index: resource counts by type, staleness distribution, drift count, unattributed changes.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project ID"},
			},
			"required":             []string{"project_id"},
			"additionalProperties": false,
		},
	}, wrap(stateSummaryHandler))

	// state_unattributed — List unattributed changes.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "state_unattributed",
		Description: "List state changes that have no attribution (no ticket or known actor). These represent unmanaged infrastructure changes.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "Project ID"},
				"limit":      map[string]any{"type": "integer", "description": "Max results (default 50)", "minimum": 1, "maximum": 200},
			},
			"required":             []string{"project_id"},
			"additionalProperties": false,
		},
	}, wrap(stateUnattributedHandler))
}

// --------------------------------------------------------------------------
// Tool handlers
// --------------------------------------------------------------------------

func stateQueryHandler(svc *stateindex.Service, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, _ := args["project_id"].(string)
	resourceType, _ := args["resource_type"].(string)
	if projectID == "" || resourceType == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id and resource_type required", false))
	}

	q := stateindex.ResourceQuery{
		ProjectID:    projectID,
		ResourceType: stateindex.ResourceType(resourceType),
	}
	if env, ok := args["environment"].(string); ok {
		q.Environment = env
	}
	if filter, ok := args["filter"].(map[string]any); ok {
		q.Filter = make(map[string]string)
		for k, v := range filter {
			if vs, ok := v.(string); ok {
				q.Filter[k] = vs
			}
		}
	}
	if limit, ok := args["limit"].(float64); ok {
		q.Limit = int(limit)
	}

	resources, err := svc.QueryResources(ctx, q)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	if resources == nil {
		resources = []*stateindex.ObservedResource{}
	}
	return jsonResult(resources)
}

func stateGetResourceHandler(svc *stateindex.Service, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	resourceID, _ := args["resource_id"].(string)
	if resourceID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "resource_id required", false))
	}

	resource, err := svc.GetResource(ctx, resourceID)
	if err != nil {
		if err == stateindex.ErrResourceNotFound {
			return toolErrTriple(apierrors.New(apierrors.CodeNotFound, "resource not found", false))
		}
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	return jsonResult(resource)
}

func stateStalenessHandler(svc *stateindex.Service, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, _ := args["project_id"].(string)
	if projectID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id required", false))
	}

	thresholdSeconds := 300
	if ts, ok := args["threshold_seconds"].(float64); ok {
		thresholdSeconds = int(ts)
	}
	resourceType := stateindex.ResourceType("")
	if rt, ok := args["resource_type"].(string); ok {
		resourceType = stateindex.ResourceType(rt)
	}
	environment := ""
	if env, ok := args["environment"].(string); ok {
		environment = env
	}

	report, err := svc.CheckStaleness(ctx, projectID, resourceType, environment, thresholdSeconds)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	return jsonResult(report)
}

func stateDriftHandler(svc *stateindex.Service, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, _ := args["project_id"].(string)
	if projectID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id required", false))
	}

	resourceID := ""
	if rid, ok := args["resource_id"].(string); ok {
		resourceID = rid
	}
	environment := ""
	if env, ok := args["environment"].(string); ok {
		environment = env
	}

	report, err := svc.DetectDrift(ctx, projectID, resourceID, environment)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	return jsonResult(report)
}

func stateAttributeChangeHandler(svc *stateindex.Service, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, _ := args["project_id"].(string)
	resourceID, _ := args["resource_id"].(string)
	if projectID == "" || resourceID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id and resource_id required", false))
	}

	ticketID, _ := args["ticket_id"].(string)
	actor, _ := args["actor"].(string)
	changeID, _ := args["change_id"].(string)

	attr, err := svc.AttributeChange(ctx, projectID, resourceID, ticketID, actor, changeID)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	return jsonResult(attr)
}

func stateSnapshotHandler(svc *stateindex.Service, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, _ := args["project_id"].(string)
	if projectID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id required", false))
	}

	var resourceIDs []string
	if ids, ok := args["resource_ids"].([]any); ok {
		for _, id := range ids {
			if s, ok := id.(string); ok {
				resourceIDs = append(resourceIDs, s)
			}
		}
	}
	if len(resourceIDs) == 0 {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "resource_ids required", false))
	}

	environment, _ := args["environment"].(string)

	resources, err := svc.GetSnapshot(ctx, projectID, resourceIDs, environment)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}

	snapshot := map[string]any{
		"project_id":     projectID,
		"snapshot_at":    time.Now().UTC().Format(time.RFC3339),
		"resource_count": len(resources),
		"resources":      resources,
		"hash":           stateindex.HashResult(resources),
	}
	return jsonResult(snapshot)
}

func stateSummaryHandler(svc *stateindex.Service, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, _ := args["project_id"].(string)
	if projectID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id required", false))
	}

	summary, err := svc.GetSummary(ctx, projectID)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	return jsonResult(summary)
}

func stateUnattributedHandler(svc *stateindex.Service, ctx context.Context, args map[string]any) (*mcp.CallToolResult, any, error) {
	projectID, _ := args["project_id"].(string)
	if projectID == "" {
		return toolErrTriple(apierrors.New(apierrors.CodeInvalidInput, "project_id required", false))
	}

	limit := 50
	if l, ok := args["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
	}

	attrs, err := svc.ListUnattributed(ctx, projectID, limit)
	if err != nil {
		return toolErrTriple(apierrors.New(apierrors.CodeInternal, err.Error(), true))
	}
	if attrs == nil {
		attrs = []*stateindex.StateAttribution{}
	}
	return jsonResult(attrs)
}
