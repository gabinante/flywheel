// Package stateindex implements the observed state index (spec v0.2 Layer 10).
// It provides a continuous, queryable ground-truth view of infrastructure state
// with attribution tracking and staleness detection.
package stateindex

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors.
var (
	ErrResourceNotFound    = errors.New("observed resource not found")
	ErrChangeNotFound      = errors.New("state change not found")
	ErrAttributionNotFound = errors.New("state attribution not found")
)

// ResourceType classifies observed infrastructure resources.
type ResourceType string

const (
	ResourceContainer    ResourceType = "container"
	ResourceService      ResourceType = "service"
	ResourceDatabase     ResourceType = "database"
	ResourceLoadBalancer ResourceType = "loadbalancer"
	ResourceBucket       ResourceType = "bucket"
	ResourceVM           ResourceType = "vm"
	ResourceNetwork      ResourceType = "network"
	ResourceDNS          ResourceType = "dns"
	ResourceQueue        ResourceType = "queue"
	ResourceFunction     ResourceType = "function"
	ResourceCustom       ResourceType = "custom"
)

// ChangeType classifies the kind of state change detected.
type ChangeType string

const (
	ChangeCreated      ChangeType = "created"
	ChangeUpdated      ChangeType = "updated"
	ChangeDeleted      ChangeType = "deleted"
	ChangeDriftDetected ChangeType = "drift_detected"
)

// ActorType classifies who caused a state change.
type ActorType string

const (
	ActorTicket       ActorType = "ticket"
	ActorExternal     ActorType = "external"
	ActorUnattributed ActorType = "unattributed"
)

// ObservedResource represents a single infrastructure resource whose state
// is continuously observed and tracked.
type ObservedResource struct {
	ID            string         `json:"id"`
	ProjectID     string         `json:"project_id"`
	ResourceType  ResourceType   `json:"resource_type"`
	Environment   string         `json:"environment"`
	Name          string         `json:"name"`
	ExternalID    string         `json:"external_id,omitempty"`
	Provider      string         `json:"provider,omitempty"`
	Region        string         `json:"region,omitempty"`
	Properties    map[string]any `json:"properties"`
	DeclaredState map[string]any `json:"declared_state,omitempty"`
	ObservedAt    time.Time      `json:"observed_at"`
	Source        string         `json:"source"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// IsStale returns true if the resource hasn't been observed within the threshold.
func (r *ObservedResource) IsStale(threshold time.Duration) bool {
	return time.Since(r.ObservedAt) > threshold
}

// HasDrift returns true if declared state exists and differs from observed properties.
func (r *ObservedResource) HasDrift() bool {
	if r.DeclaredState == nil || len(r.DeclaredState) == 0 {
		return false
	}
	// Simple key-level drift check; deeper comparison done in service layer.
	for k, declared := range r.DeclaredState {
		observed, exists := r.Properties[k]
		if !exists {
			return true
		}
		// Compare string representations for simplicity.
		if fmtAny(declared) != fmtAny(observed) {
			return true
		}
	}
	return false
}

// StateChange records an observed change to a resource's state.
type StateChange struct {
	ID          string         `json:"id"`
	ProjectID   string         `json:"project_id"`
	ResourceID  string         `json:"resource_id"`
	ChangeType  ChangeType     `json:"change_type"`
	BeforeState map[string]any `json:"before_state,omitempty"`
	AfterState  map[string]any `json:"after_state,omitempty"`
	Diff        map[string]any `json:"diff,omitempty"`
	DetectedAt  time.Time      `json:"detected_at"`
	CreatedAt   time.Time      `json:"created_at"`
}

// StateAttribution links a state change to its cause: a ticket, external actor,
// or marks it as unattributed (unknown cause).
type StateAttribution struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	ChangeID   string    `json:"change_id"`
	ResourceID string    `json:"resource_id"`
	TicketID   string    `json:"ticket_id,omitempty"`
	Actor      string    `json:"actor,omitempty"`
	ActorType  ActorType `json:"actor_type"`
	Confidence float64   `json:"confidence"`
	Rationale  string    `json:"rationale,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// SteampipeSnapshot records the result of a Steampipe query execution.
type SteampipeSnapshot struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	Query        string    `json:"query"`
	ResourceType string    `json:"resource_type"`
	Environment  string    `json:"environment,omitempty"`
	ResultHash   string    `json:"result_hash"`
	RowCount     int       `json:"row_count"`
	ExecutedAt   time.Time `json:"executed_at"`
}

// ResourceQuery contains parameters for querying the observed state index.
type ResourceQuery struct {
	ProjectID    string            `json:"project_id"`
	ResourceType ResourceType      `json:"resource_type,omitempty"`
	Environment  string            `json:"environment,omitempty"`
	Filter       map[string]string `json:"filter,omitempty"`
	Limit        int               `json:"limit,omitempty"`
}

// StalenessReport summarizes which resources have stale observations.
type StalenessReport struct {
	ProjectID        string              `json:"project_id"`
	ThresholdSeconds int                 `json:"threshold_seconds"`
	StaleCount       int                 `json:"stale_count"`
	TotalCount       int                 `json:"total_count"`
	StaleResources   []*ObservedResource `json:"stale_resources"`
}

// DriftReport describes drift between declared and observed state.
type DriftReport struct {
	ProjectID  string       `json:"project_id"`
	DriftCount int          `json:"drift_count"`
	Drifts     []*DriftItem `json:"drifts"`
}

// DriftItem describes a single drift between declared and observed state.
type DriftItem struct {
	ResourceID   string         `json:"resource_id"`
	ResourceName string         `json:"resource_name"`
	ResourceType ResourceType   `json:"resource_type"`
	Environment  string         `json:"environment"`
	Declared     map[string]any `json:"declared"`
	Observed     map[string]any `json:"observed"`
	Differences  map[string]any `json:"differences"`
}

// StateSummary provides a high-level overview of the state index.
type StateSummary struct {
	ProjectID          string            `json:"project_id"`
	TotalResources     int               `json:"total_resources"`
	ResourcesByType    map[string]int    `json:"resources_by_type"`
	ResourcesByEnv     map[string]int    `json:"resources_by_env"`
	StaleCount         int               `json:"stale_count"`
	DriftCount         int               `json:"drift_count"`
	UnattributedCount  int               `json:"unattributed_count"`
	LastFullScan       *time.Time        `json:"last_full_scan,omitempty"`
	StalenessDistribution map[string]int `json:"staleness_distribution"` // bucket -> count
}

// fmtAny converts any value to string for comparison.
func fmtAny(v any) string {
	return fmt.Sprintf("%v", v)
}
