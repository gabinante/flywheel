// Package catalog implements the Layer 14 project map: a lightweight built-in
// service catalog with typed entities, edges, and declared-vs-observed data.
// Pluggable via MCP — Backstage, Cortex, Port as alternatives.
package catalog

import (
	"errors"
	"time"
)

// Sentinel errors.
var (
	ErrEntityNotFound = errors.New("entity not found")
	ErrEdgeNotFound   = errors.New("edge not found")
	ErrInvalidType    = errors.New("invalid entity type")
	ErrInvalidEdge    = errors.New("invalid edge type")
	ErrDuplicateEdge  = errors.New("duplicate edge")
)

// EntityType classifies catalog entities.
type EntityType string

const (
	EntityService        EntityType = "service"
	EntityDatastore      EntityType = "datastore"
	EntityIntegration    EntityType = "integration"
	EntityInfrastructure EntityType = "infrastructure"
	EntityRepository     EntityType = "repository"
	EntityEnvironment    EntityType = "environment"
)

// ValidEntityTypes returns all valid entity types.
func ValidEntityTypes() []EntityType {
	return []EntityType{
		EntityService, EntityDatastore, EntityIntegration,
		EntityInfrastructure, EntityRepository, EntityEnvironment,
	}
}

// IsValidEntityType checks whether t is a valid entity type.
func IsValidEntityType(t string) bool {
	for _, v := range ValidEntityTypes() {
		if string(v) == t {
			return true
		}
	}
	return false
}

// EdgeType classifies relationships between entities.
type EdgeType string

const (
	EdgeDependsOn  EdgeType = "depends_on"
	EdgeProvides   EdgeType = "provides"
	EdgeConsumes   EdgeType = "consumes"
	EdgeDeployedTo EdgeType = "deployed_to"
	EdgeBackedBy   EdgeType = "backed_by"
	EdgeMonitors   EdgeType = "monitors"
	EdgeOwns       EdgeType = "owns"
)

// ValidEdgeTypes returns all valid edge types.
func ValidEdgeTypes() []EdgeType {
	return []EdgeType{
		EdgeDependsOn, EdgeProvides, EdgeConsumes,
		EdgeDeployedTo, EdgeBackedBy, EdgeMonitors, EdgeOwns,
	}
}

// IsValidEdgeType checks whether t is a valid edge type.
func IsValidEdgeType(t string) bool {
	for _, v := range ValidEdgeTypes() {
		if string(v) == t {
			return true
		}
	}
	return false
}

// Source distinguishes declared (human intent) from observed (system-derived) facts.
type Source string

const (
	SourceDeclared Source = "declared"
	SourceObserved Source = "observed"
)

// Entity is a node in the project map: a service, datastore, integration, etc.
type Entity struct {
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id"`
	Type        EntityType        `json:"type"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Source      Source            `json:"source"` // declared or observed
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Edge is a typed, directed relationship between two entities.
type Edge struct {
	ID        string            `json:"id"`
	ProjectID string            `json:"project_id"`
	FromID    string            `json:"from_id"`
	ToID      string            `json:"to_id"`
	Type      EdgeType          `json:"type"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Source    Source            `json:"source"` // declared or observed
	CreatedAt time.Time         `json:"created_at"`
}

// DeploymentEntry represents one cell in the deployment matrix:
// which version of a service is deployed to which environment.
type DeploymentEntry struct {
	ServiceID     string    `json:"service_id"`
	ServiceName   string    `json:"service_name"`
	EnvironmentID string    `json:"environment_id"`
	EnvName       string    `json:"environment_name"`
	Version       string    `json:"version"`
	Source        Source    `json:"source"`
	ObservedAt    time.Time `json:"observed_at"`
}

// ScanResult holds the output of a bootstrap scan.
type ScanResult struct {
	Entities []Entity `json:"entities"`
	Edges    []Edge   `json:"edges"`
	Errors   []string `json:"errors,omitempty"`
}
