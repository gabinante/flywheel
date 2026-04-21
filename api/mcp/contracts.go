// Package mcp provides MCP server integration for Flywheel.
// This file defines versioned plugin contracts for pluggable layers.
// Each contract specifies the MCP tools and resources a plugin must expose,
// enabling operators to swap bundled defaults for external MCP servers
// with no code changes (per spec v0.2 section 2.6, principle 1.9).
package mcp

import "fmt"

// ContractVersion represents a semantic version for contract compatibility checking.
type ContractVersion struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
	Patch int `json:"patch"`
}

// String returns the semver string representation.
func (v ContractVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compatible returns true if provider version `other` satisfies this contract version.
// A provider is compatible if major versions match and the provider's minor >= contract's minor.
func (v ContractVersion) Compatible(other ContractVersion) bool {
	return v.Major == other.Major && other.Minor >= v.Minor
}

// ToolSpec describes an MCP tool that a plugin contract requires or optionally provides.
type ToolSpec struct {
	// Name is the MCP tool name (e.g. "code_callers").
	Name string `json:"name"`

	// Description explains what the tool does.
	Description string `json:"description"`

	// Required indicates whether the tool must be present for contract compliance.
	// Optional tools provide enhanced functionality but the system works without them.
	Required bool `json:"required"`

	// InputSchema is the JSON Schema for tool parameters.
	InputSchema map[string]any `json:"input_schema"`
}

// ResourceSpec describes an MCP resource that a plugin contract requires or optionally exposes.
type ResourceSpec struct {
	// URIPattern is the resource URI pattern (e.g. "flywheel://code-intel/{project_id}/symbols").
	URIPattern string `json:"uri_pattern"`

	// Name is the human-readable resource name.
	Name string `json:"name"`

	// Description explains what the resource provides.
	Description string `json:"description"`

	// Required indicates whether the resource must be present for contract compliance.
	Required bool `json:"required"`

	// MIMEType is the expected content type of the resource.
	MIMEType string `json:"mime_type"`
}

// PluginContract defines the full specification for a pluggable layer's MCP interface.
// Operators can implement this contract as an external MCP server to replace bundled defaults.
type PluginContract struct {
	// Name identifies the contract (e.g. "code_intelligence").
	Name string `json:"name"`

	// Version is the contract version for compatibility checking.
	Version ContractVersion `json:"version"`

	// Layer is the spec layer number this contract implements.
	Layer int `json:"layer"`

	// Description explains the contract's purpose and scope.
	Description string `json:"description"`

	// Tools lists all MCP tools this contract specifies (required and optional).
	Tools []ToolSpec `json:"tools"`

	// Resources lists all MCP resources this contract specifies (required and optional).
	Resources []ResourceSpec `json:"resources"`
}

// RequiredTools returns only the required tools from the contract.
func (c *PluginContract) RequiredTools() []ToolSpec {
	var result []ToolSpec
	for _, t := range c.Tools {
		if t.Required {
			result = append(result, t)
		}
	}
	return result
}

// OptionalTools returns only the optional tools from the contract.
func (c *PluginContract) OptionalTools() []ToolSpec {
	var result []ToolSpec
	for _, t := range c.Tools {
		if !t.Required {
			result = append(result, t)
		}
	}
	return result
}

// RequiredResources returns only the required resources from the contract.
func (c *PluginContract) RequiredResources() []ResourceSpec {
	var result []ResourceSpec
	for _, r := range c.Resources {
		if r.Required {
			result = append(result, r)
		}
	}
	return result
}

// --------------------------------------------------------------------------
// Layer Contracts
// --------------------------------------------------------------------------

// CodeIntelligenceContract defines the MCP contract for Layer 3 (Code Knowledge).
// Implementations provide structural code analysis: symbol lookup, caller/callee
// graphs, import relationships, and blast radius estimation.
//
// Required capabilities: symbol lookup, callers, callees, blast radius.
// Optional capabilities: full re-index trigger, language-specific metadata.
//
// Bundled default: Tree-sitter based analysis (see codeintel_default.go).
// Alternative backends: Sourcegraph, GitNexus, Language servers.
var CodeIntelligenceContract = PluginContract{
	Name:    "code_intelligence",
	Version: ContractVersion{Major: 1, Minor: 0, Patch: 0},
	Layer:   3,
	Description: "Structural code knowledge: symbol lookup, caller/callee graphs, " +
		"import relationships, and blast radius estimation. Agents use this to " +
		"understand code structure before making changes.",
	Tools: []ToolSpec{
		{
			Name:        "code_symbol_lookup",
			Description: "Look up a symbol by name or path. Returns symbol metadata including type, location, visibility, and documentation.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"query":      map[string]any{"type": "string", "description": "Symbol name, partial name, or file:line reference"},
					"language":   map[string]any{"type": "string", "description": "Filter by language (optional)"},
					"kind":       map[string]any{"type": "string", "description": "Filter by symbol kind: function, type, variable, constant, method, interface (optional)", "enum": []string{"function", "type", "variable", "constant", "method", "interface"}},
					"limit":      map[string]any{"type": "integer", "description": "Max results (default 20)", "minimum": 1, "maximum": 100},
				},
				"required":             []string{"project_id", "query"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "code_callers",
			Description: "Find all callers of a given symbol. Returns call sites with file, line, and calling context.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"symbol":     map[string]any{"type": "string", "description": "Fully qualified symbol name or ID"},
					"depth":      map[string]any{"type": "integer", "description": "Traversal depth (default 1, max 5)", "minimum": 1, "maximum": 5},
					"limit":      map[string]any{"type": "integer", "description": "Max results (default 50)", "minimum": 1, "maximum": 200},
				},
				"required":             []string{"project_id", "symbol"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "code_callees",
			Description: "Find all functions/methods called by a given symbol. Returns callees with file, line, and target info.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"symbol":     map[string]any{"type": "string", "description": "Fully qualified symbol name or ID"},
					"depth":      map[string]any{"type": "integer", "description": "Traversal depth (default 1, max 5)", "minimum": 1, "maximum": 5},
					"limit":      map[string]any{"type": "integer", "description": "Max results (default 50)", "minimum": 1, "maximum": 200},
				},
				"required":             []string{"project_id", "symbol"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "code_blast_radius",
			Description: "Estimate the blast radius of modifying a symbol. Returns affected symbols, files, and services ranked by impact.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"symbol":     map[string]any{"type": "string", "description": "Fully qualified symbol name or ID to analyze"},
					"depth":      map[string]any{"type": "integer", "description": "Analysis depth (default 2, max 5)", "minimum": 1, "maximum": 5},
				},
				"required":             []string{"project_id", "symbol"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "code_importers",
			Description: "Find all files/modules that import a given package or module.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"package":    map[string]any{"type": "string", "description": "Package/module path to find importers for"},
					"limit":      map[string]any{"type": "integer", "description": "Max results (default 50)", "minimum": 1, "maximum": 200},
				},
				"required":             []string{"project_id", "package"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "code_reindex",
			Description: "Trigger incremental re-indexing for a project or specific files. Returns job status.",
			Required:    false,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"paths":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Specific file paths to re-index (optional, indexes all if omitted)"},
					"commit_sha": map[string]any{"type": "string", "description": "Commit SHA to index at (optional, uses HEAD if omitted)"},
				},
				"required":             []string{"project_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "code_symbol_metadata",
			Description: "Get detailed metadata for a symbol: documentation, type signature, visibility, complexity metrics.",
			Required:    false,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"symbol":     map[string]any{"type": "string", "description": "Fully qualified symbol name or ID"},
				},
				"required":             []string{"project_id", "symbol"},
				"additionalProperties": false,
			},
		},
	},
	Resources: []ResourceSpec{
		{
			URIPattern:  "flywheel://code-intel/{project_id}/index-status",
			Name:        "Code index status",
			Description: "Current indexing status: last indexed commit, coverage, staleness.",
			Required:    true,
			MIMEType:    "application/json",
		},
		{
			URIPattern:  "flywheel://code-intel/{project_id}/languages",
			Name:        "Supported languages",
			Description: "List of languages the code intelligence backend supports with feature matrix.",
			Required:    false,
			MIMEType:    "application/json",
		},
	},
}

// CatalogContract defines the MCP contract for Layer 14 (Project Map / Service Catalog).
// Implementations provide entity management: services, datastores, integrations,
// infrastructure, repositories, environments, and their relationships.
//
// Required capabilities: entity CRUD, relationship management, entity listing.
// Optional capabilities: bootstrap scanning, deployment matrix view.
//
// Bundled default: Postgres-backed lightweight catalog.
// Alternative backends: Backstage, Cortex, Port.
var CatalogContract = PluginContract{
	Name:    "catalog",
	Version: ContractVersion{Major: 1, Minor: 0, Patch: 0},
	Layer:   14,
	Description: "Project map and service catalog: entities (services, datastores, " +
		"integrations, infrastructure, repositories, environments) and typed " +
		"relationships between them. Supports declared vs observed data distinction.",
	Tools: []ToolSpec{
		{
			Name:        "catalog_list_entities",
			Description: "List entities in the catalog, optionally filtered by type, project, or label.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "string", "description": "Project ID"},
					"entity_type": map[string]any{"type": "string", "description": "Filter by type (optional)", "enum": []string{"service", "datastore", "integration", "infrastructure", "repository", "environment"}},
					"label":       map[string]any{"type": "string", "description": "Filter by label (optional)"},
					"limit":       map[string]any{"type": "integer", "description": "Max results (default 50)", "minimum": 1, "maximum": 200},
				},
				"required":             []string{"project_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "catalog_get_entity",
			Description: "Get a single entity by ID with all its metadata, labels, and relationships.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"entity_id":  map[string]any{"type": "string", "description": "Entity ID"},
				},
				"required":             []string{"project_id", "entity_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "catalog_create_entity",
			Description: "Create a new entity in the catalog. Returns the entity with generated ID.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "string", "description": "Project ID"},
					"entity_type": map[string]any{"type": "string", "description": "Entity type", "enum": []string{"service", "datastore", "integration", "infrastructure", "repository", "environment"}},
					"name":        map[string]any{"type": "string", "description": "Entity name"},
					"description": map[string]any{"type": "string", "description": "Entity description (optional)"},
					"labels":      map[string]any{"type": "object", "description": "Key-value labels (optional)", "additionalProperties": map[string]any{"type": "string"}},
					"metadata":    map[string]any{"type": "object", "description": "Arbitrary metadata (optional)", "additionalProperties": map[string]any{"type": "string"}},
					"source":      map[string]any{"type": "string", "description": "Data source: declared or observed", "enum": []string{"declared", "observed"}},
				},
				"required":             []string{"project_id", "entity_type", "name"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "catalog_update_entity",
			Description: "Update an existing entity's metadata, labels, or description.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "string", "description": "Project ID"},
					"entity_id":   map[string]any{"type": "string", "description": "Entity ID"},
					"name":        map[string]any{"type": "string", "description": "New name (optional)"},
					"description": map[string]any{"type": "string", "description": "New description (optional)"},
					"labels":      map[string]any{"type": "object", "description": "Labels to merge (optional)", "additionalProperties": map[string]any{"type": "string"}},
					"metadata":    map[string]any{"type": "object", "description": "Metadata to merge (optional)", "additionalProperties": map[string]any{"type": "string"}},
				},
				"required":             []string{"project_id", "entity_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "catalog_list_edges",
			Description: "List relationships (edges) for an entity or between entities.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"entity_id":  map[string]any{"type": "string", "description": "Entity ID to find edges for"},
					"edge_type":  map[string]any{"type": "string", "description": "Filter by edge type (optional)", "enum": []string{"depends_on", "provides", "consumes", "deployed_to", "backed_by", "monitors", "owns"}},
					"direction":  map[string]any{"type": "string", "description": "Edge direction from entity: outgoing, incoming, or both (default: both)", "enum": []string{"outgoing", "incoming", "both"}},
				},
				"required":             []string{"project_id", "entity_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "catalog_create_edge",
			Description: "Create a typed relationship between two entities.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"from_id":    map[string]any{"type": "string", "description": "Source entity ID"},
					"to_id":      map[string]any{"type": "string", "description": "Target entity ID"},
					"edge_type":  map[string]any{"type": "string", "description": "Relationship type", "enum": []string{"depends_on", "provides", "consumes", "deployed_to", "backed_by", "monitors", "owns"}},
					"metadata":   map[string]any{"type": "object", "description": "Edge metadata (optional)", "additionalProperties": map[string]any{"type": "string"}},
				},
				"required":             []string{"project_id", "from_id", "to_id", "edge_type"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "catalog_bootstrap_scan",
			Description: "Trigger a bootstrap scan of the repository to auto-discover entities from Dockerfiles, docker-compose, K8s manifests, Terraform, and varlock schemas.",
			Required:    false,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"repo_path":  map[string]any{"type": "string", "description": "Repository path to scan (optional, uses project default)"},
				},
				"required":             []string{"project_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "catalog_deployment_matrix",
			Description: "Get the deployment matrix: service x environment x version. Shows what's deployed where.",
			Required:    false,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"service_id": map[string]any{"type": "string", "description": "Filter to specific service (optional)"},
				},
				"required":             []string{"project_id"},
				"additionalProperties": false,
			},
		},
	},
	Resources: []ResourceSpec{
		{
			URIPattern:  "flywheel://catalog/{project_id}/topology",
			Name:        "Service topology",
			Description: "Full entity-relationship graph for the project as JSON.",
			Required:    true,
			MIMEType:    "application/json",
		},
		{
			URIPattern:  "flywheel://catalog/{project_id}/entity/{entity_id}",
			Name:        "Entity detail",
			Description: "Full entity detail including all edges and metadata.",
			Required:    false,
			MIMEType:    "application/json",
		},
	},
}

// StateIndexContract defines the MCP contract for Layer 10 (Observed State Index).
// Implementations provide a continuous, queryable ground-truth view of infrastructure state.
// Tracks what actually exists (observed) vs what should exist (declared), with
// staleness tracking and change attribution.
//
// Required capabilities: state queries, staleness tracking, drift detection.
// Optional capabilities: attribution, historical snapshots.
//
// Bundled default: Steampipe-based cloud state ingestion.
// Alternative backends: CloudQuery, CSPM tools, custom collectors.
var StateIndexContract = PluginContract{
	Name:    "state_index",
	Version: ContractVersion{Major: 1, Minor: 0, Patch: 0},
	Layer:   10,
	Description: "Observed infrastructure state index: continuous ground-truth view of " +
		"what actually exists in cloud/infra environments. Supports SQL-like queries, " +
		"staleness tracking, and drift detection between declared and observed state.",
	Tools: []ToolSpec{
		{
			Name:        "state_query",
			Description: "Query the observed state index. Accepts structured queries against known resource types. Returns current state with observation timestamps.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":    map[string]any{"type": "string", "description": "Project ID"},
					"resource_type": map[string]any{"type": "string", "description": "Resource type to query (e.g. 'container', 'service', 'database', 'loadbalancer')"},
					"environment":   map[string]any{"type": "string", "description": "Environment filter (optional)"},
					"filter":        map[string]any{"type": "object", "description": "Key-value filters on resource properties (optional)", "additionalProperties": map[string]any{"type": "string"}},
					"limit":         map[string]any{"type": "integer", "description": "Max results (default 50)", "minimum": 1, "maximum": 500},
				},
				"required":             []string{"project_id", "resource_type"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "state_get_resource",
			Description: "Get the full observed state for a specific resource by ID. Includes all properties, observation timestamps, and attribution.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "string", "description": "Project ID"},
					"resource_id": map[string]any{"type": "string", "description": "Resource ID (stable cross-layer entity ID)"},
				},
				"required":             []string{"project_id", "resource_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "state_staleness",
			Description: "Check staleness of observed state for resources. Returns resources where observation is older than threshold.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":       map[string]any{"type": "string", "description": "Project ID"},
					"resource_type":    map[string]any{"type": "string", "description": "Resource type filter (optional)"},
					"environment":      map[string]any{"type": "string", "description": "Environment filter (optional)"},
					"threshold_seconds": map[string]any{"type": "integer", "description": "Staleness threshold in seconds (default 300)", "minimum": 1},
				},
				"required":             []string{"project_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "state_drift",
			Description: "Detect drift between declared state (from config/IaC) and observed state. Returns mismatches with severity.",
			Required:    true,
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
		},
		{
			Name:        "state_attribute_change",
			Description: "Attribute an observed state change to a ticket, external actor, or mark as unattributed.",
			Required:    false,
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
		},
		{
			Name:        "state_snapshot",
			Description: "Get a point-in-time snapshot of resource state for freshness stamping in plans.",
			Required:    false,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":    map[string]any{"type": "string", "description": "Project ID"},
					"resource_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Resource IDs to snapshot"},
					"environment":   map[string]any{"type": "string", "description": "Environment (optional)"},
				},
				"required":             []string{"project_id", "resource_ids"},
				"additionalProperties": false,
			},
		},
	},
	Resources: []ResourceSpec{
		{
			URIPattern:  "flywheel://state-index/{project_id}/summary",
			Name:        "State index summary",
			Description: "Summary of observed resources: counts by type, staleness distribution, last full scan timestamp.",
			Required:    true,
			MIMEType:    "application/json",
		},
		{
			URIPattern:  "flywheel://state-index/{project_id}/unattributed",
			Name:        "Unattributed changes",
			Description: "List of state changes that have no attribution (no ticket or known actor caused them).",
			Required:    false,
			MIMEType:    "application/json",
		},
	},
}

// SignalIngestionContract defines the MCP contract for Layer 11 (Signal Ingestion).
// Implementations provide production signal collection: alerts, metrics thresholds,
// error spikes, deployment events from external observability platforms.
//
// Required capabilities: signal ingestion, source listing, recent signals query.
// Optional capabilities: webhook registration, correlation.
//
// Bundled default: Polling-based multi-source collector.
// Alternative backends: Prometheus Alertmanager, Datadog, PagerDuty, Sentry.
var SignalIngestionContract = PluginContract{
	Name:    "signal_ingestion",
	Version: ContractVersion{Major: 1, Minor: 0, Patch: 0},
	Layer:   11,
	Description: "Production signal ingestion: collects alerts, metric threshold breaches, " +
		"error spikes, and deployment events from external observability platforms. " +
		"Normalizes signals for attribution and observation window evaluation.",
	Tools: []ToolSpec{
		{
			Name:        "signal_ingest",
			Description: "Ingest a signal (alert, event, metric threshold breach) into the signal store. Normalizes and indexes for attribution.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "string", "description": "Project ID"},
					"source":      map[string]any{"type": "string", "description": "Signal source identifier (e.g. 'prometheus', 'datadog', 'sentry')"},
					"signal_type": map[string]any{"type": "string", "description": "Signal type", "enum": []string{"alert", "metric_breach", "error_spike", "deploy_event", "status_change", "custom"}},
					"severity":    map[string]any{"type": "string", "description": "Signal severity", "enum": []string{"critical", "warning", "info"}},
					"title":       map[string]any{"type": "string", "description": "Signal title/summary"},
					"description": map[string]any{"type": "string", "description": "Detailed description (optional)"},
					"entity_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Related entity IDs from catalog (optional)"},
					"environment": map[string]any{"type": "string", "description": "Environment where signal originated (optional)"},
					"metadata":    map[string]any{"type": "object", "description": "Source-specific metadata (optional)", "additionalProperties": map[string]any{"type": "string"}},
				},
				"required":             []string{"project_id", "source", "signal_type", "severity", "title"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "signal_list_recent",
			Description: "List recent signals for a project, optionally filtered by source, severity, or entity.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "string", "description": "Project ID"},
					"source":      map[string]any{"type": "string", "description": "Filter by source (optional)"},
					"severity":    map[string]any{"type": "string", "description": "Filter by severity (optional)", "enum": []string{"critical", "warning", "info"}},
					"signal_type": map[string]any{"type": "string", "description": "Filter by type (optional)", "enum": []string{"alert", "metric_breach", "error_spike", "deploy_event", "status_change", "custom"}},
					"entity_id":   map[string]any{"type": "string", "description": "Filter by related entity (optional)"},
					"since":       map[string]any{"type": "string", "description": "RFC3339 timestamp to filter signals after (optional)"},
					"limit":       map[string]any{"type": "integer", "description": "Max results (default 50)", "minimum": 1, "maximum": 200},
				},
				"required":             []string{"project_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "signal_list_sources",
			Description: "List configured signal sources and their status (connected, polling interval, last poll timestamp).",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
				},
				"required":             []string{"project_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "signal_correlate",
			Description: "Find signals that correlate with a time window (e.g. around a deployment or ticket execution). Used for observation window evaluation.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "string", "description": "Project ID"},
					"start_time":  map[string]any{"type": "string", "description": "Window start (RFC3339)"},
					"end_time":    map[string]any{"type": "string", "description": "Window end (RFC3339)"},
					"entity_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Entity IDs to correlate against (optional)"},
					"ticket_id":   map[string]any{"type": "string", "description": "Ticket ID to find signals around (optional, uses ticket execution window)"},
				},
				"required":             []string{"project_id", "start_time", "end_time"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "signal_register_webhook",
			Description: "Register a webhook endpoint for a signal source to push alerts/events to.",
			Required:    false,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":  map[string]any{"type": "string", "description": "Project ID"},
					"source":      map[string]any{"type": "string", "description": "Source name to register webhook for"},
					"webhook_url": map[string]any{"type": "string", "description": "URL to receive webhook payloads (optional, auto-generated if omitted)"},
				},
				"required":             []string{"project_id", "source"},
				"additionalProperties": false,
			},
		},
	},
	Resources: []ResourceSpec{
		{
			URIPattern:  "flywheel://signals/{project_id}/dashboard",
			Name:        "Signal dashboard",
			Description: "Signal summary: active alerts, recent events, source health.",
			Required:    true,
			MIMEType:    "application/json",
		},
	},
}

// NotificationContract defines the MCP contract for Layer 12 (Notification Channels).
// Implementations provide policy-driven async push to operators: urgent decisions,
// autonomous action notifications, calibration reviews, and anomaly alerts.
//
// Required capabilities: send notification, list channels, notification preferences.
// Optional capabilities: dismissal tracking, digest routing, template management.
//
// Bundled default: Slack incoming webhooks.
// Alternative backends: Email, SMS, PagerDuty, custom push channels.
var NotificationContract = PluginContract{
	Name:    "notification",
	Version: ContractVersion{Major: 1, Minor: 0, Patch: 0},
	Layer:   12,
	Description: "Notification and push layer: policy-driven async notifications to operators. " +
		"Supports urgent decisions, autonomous action notifications, calibration reviews, " +
		"and anomaly alerts. Routes by urgency to appropriate channels.",
	Tools: []ToolSpec{
		{
			Name:        "notify_send",
			Description: "Send a notification to the configured channel(s) based on urgency and category.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"category":   map[string]any{"type": "string", "description": "Notification category", "enum": []string{"urgent_decision", "autonomous_action", "calibration_review", "anomaly_alert", "status_update"}},
					"title":      map[string]any{"type": "string", "description": "Notification title"},
					"body":       map[string]any{"type": "string", "description": "Notification body (markdown supported)"},
					"urgency":    map[string]any{"type": "string", "description": "Urgency level (affects routing)", "enum": []string{"immediate", "soon", "digest"}},
					"ticket_id":  map[string]any{"type": "string", "description": "Related ticket ID (optional)"},
					"actions":    map[string]any{"type": "array", "description": "Available actions the operator can take (optional)", "items": map[string]any{"type": "object", "properties": map[string]any{"label": map[string]any{"type": "string"}, "action_id": map[string]any{"type": "string"}}, "required": []string{"label", "action_id"}}},
					"timeout_seconds": map[string]any{"type": "integer", "description": "Auto-action timeout in seconds (optional, for decisions with defaults)", "minimum": 60},
					"default_action":  map[string]any{"type": "string", "description": "Action to take on timeout (optional, requires timeout_seconds)"},
				},
				"required":             []string{"project_id", "category", "title", "body", "urgency"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "notify_list_channels",
			Description: "List configured notification channels and their routing rules.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
				},
				"required":             []string{"project_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "notify_configure_channel",
			Description: "Configure a notification channel (Slack webhook, email, SMS endpoint).",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":   map[string]any{"type": "string", "description": "Project ID"},
					"channel_type": map[string]any{"type": "string", "description": "Channel type", "enum": []string{"slack", "email", "sms", "webhook"}},
					"name":         map[string]any{"type": "string", "description": "Channel display name"},
					"config":       map[string]any{"type": "object", "description": "Channel-specific config (webhook_url, email, phone, etc.)", "additionalProperties": map[string]any{"type": "string"}},
					"categories":   map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"urgent_decision", "autonomous_action", "calibration_review", "anomaly_alert", "status_update"}}, "description": "Which notification categories route to this channel (optional, all if omitted)"},
				},
				"required":             []string{"project_id", "channel_type", "name", "config"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "notify_preferences",
			Description: "Get or update notification preferences (quiet hours, digest schedule, escalation rules).",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":     map[string]any{"type": "string", "description": "Project ID"},
					"quiet_start":    map[string]any{"type": "string", "description": "Quiet hours start (HH:MM, optional)"},
					"quiet_end":      map[string]any{"type": "string", "description": "Quiet hours end (HH:MM, optional)"},
					"digest_schedule": map[string]any{"type": "string", "description": "Digest frequency (optional)", "enum": []string{"hourly", "every_4h", "daily", "never"}},
					"timezone":       map[string]any{"type": "string", "description": "Timezone for quiet hours (optional, e.g. 'America/New_York')"},
				},
				"required":             []string{"project_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "notify_dismiss",
			Description: "Record a notification dismissal for classifier tuning. Tracks which notifications operators ignore.",
			Required:    false,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":      map[string]any{"type": "string", "description": "Project ID"},
					"notification_id": map[string]any{"type": "string", "description": "Notification ID that was dismissed"},
					"reason":          map[string]any{"type": "string", "description": "Dismissal reason (optional)", "enum": []string{"not_relevant", "already_handled", "too_noisy", "wrong_urgency"}},
				},
				"required":             []string{"project_id", "notification_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "notify_dismissal_stats",
			Description: "Get dismissal rate statistics for classifier tuning. Shows which categories/sources are over-notifying.",
			Required:    false,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"days":       map[string]any{"type": "integer", "description": "Look-back window in days (default 30)", "minimum": 1, "maximum": 90},
				},
				"required":             []string{"project_id"},
				"additionalProperties": false,
			},
		},
	},
	Resources: []ResourceSpec{
		{
			URIPattern:  "flywheel://notifications/{project_id}/config",
			Name:        "Notification configuration",
			Description: "Current notification channels, routing rules, and preferences.",
			Required:    true,
			MIMEType:    "application/json",
		},
	},
}

// --------------------------------------------------------------------------
// Layer 4: Findings
// --------------------------------------------------------------------------

// FindingsContract defines the MCP contract for Layer 4 (Findings).
// Implementations provide a semantic findings store for claims with provenance,
// symbol-level summaries, and investigation outputs. Supports semantic retrieval
// (natural language query → relevant findings) and structural navigation
// (findings about this symbol/ticket). Findings track provenance and are
// invalidated when underlying code changes via commit SHA + function-body hash.
//
// Required capabilities: save, query, by-symbol, by-ticket, invalidate, get.
// Optional capabilities: delete, by-file.
//
// Bundled default: Weaviate vector store (see findings_weaviate.go).
// Alternative backends: pgvector, Qdrant.
var FindingsContract = PluginContract{
	Name:    "findings",
	Version: ContractVersion{Major: 1, Minor: 0, Patch: 0},
	Layer:   4,
	Description: "Semantic findings store: claims with provenance, symbol-level " +
		"summaries, and investigation outputs. Supports semantic retrieval " +
		"(natural language → findings) and structural navigation (findings " +
		"about a symbol or ticket). Provenance tracks commit SHA and function " +
		"body hash for automatic invalidation on code changes.",
	Tools: []ToolSpec{
		{
			Name:        "findings_save",
			Description: "Save a semantic finding with provenance. A finding is a claim about code (quality, architecture, security, performance) with tracked origin and symbol/ticket references.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id":         map[string]any{"type": "string", "description": "Project ID"},
					"claim":              map[string]any{"type": "string", "description": "The finding's claim text (e.g., 'This function has O(n²) complexity due to nested loops')"},
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
		},
		{
			Name:        "findings_query",
			Description: "Semantic query for findings. Uses natural language to find relevant findings via vector similarity search.",
			Required:    true,
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
		},
		{
			Name:        "findings_by_symbol",
			Description: "Get findings associated with a specific code symbol. Structural navigation for understanding what's known about a symbol.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"symbol_ref": map[string]any{"type": "string", "description": "Symbol ID or name to find findings for"},
					"limit":      map[string]any{"type": "integer", "description": "Max results (default 20)", "minimum": 1, "maximum": 100},
				},
				"required":             []string{"project_id", "symbol_ref"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "findings_by_ticket",
			Description: "Get findings associated with a specific ticket. Structural navigation for understanding what a ticket discovered or relates to.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"ticket_ref": map[string]any{"type": "string", "description": "Ticket ID to find findings for"},
					"limit":      map[string]any{"type": "integer", "description": "Max results (default 20)", "minimum": 1, "maximum": 100},
				},
				"required":             []string{"project_id", "ticket_ref"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "findings_invalidate",
			Description: "Invalidate findings when underlying code changes. Marks findings as stale based on commit SHA and changed file paths.",
			Required:    true,
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
		},
		{
			Name:        "findings_get",
			Description: "Get a specific finding by ID. Returns full finding details with provenance.",
			Required:    true,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"finding_id": map[string]any{"type": "string", "description": "Finding ID"},
				},
				"required":             []string{"project_id", "finding_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "findings_delete",
			Description: "Permanently delete a finding.",
			Required:    false,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"finding_id": map[string]any{"type": "string", "description": "Finding ID to delete"},
				},
				"required":             []string{"project_id", "finding_id"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "findings_by_file",
			Description: "Get findings associated with a specific file path.",
			Required:    false,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "Project ID"},
					"file_path":  map[string]any{"type": "string", "description": "File path to find findings for"},
					"limit":      map[string]any{"type": "integer", "description": "Max results (default 20)", "minimum": 1, "maximum": 100},
				},
				"required":             []string{"project_id", "file_path"},
				"additionalProperties": false,
			},
		},
	},
	Resources: []ResourceSpec{
		{
			URIPattern:  "flywheel://findings/{project_id}/summary",
			Name:        "Findings summary",
			Description: "Summary of findings counts by type, validity status, and recent activity.",
			Required:    true,
			MIMEType:    "application/json",
		},
	},
}

// AllContracts returns all defined plugin contracts.
func AllContracts() []PluginContract {
	return []PluginContract{
		CodeIntelligenceContract,
		CatalogContract,
		StateIndexContract,
		FindingsContract,
		SignalIngestionContract,
		NotificationContract,
	}
}
