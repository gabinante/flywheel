package stateindex

import (
	"context"
	"time"
)

// Store defines the persistence interface for the observed state index.
// This is the plugin contract boundary — alternative backends (CloudQuery, CSPM tools)
// implement this interface to replace the default Steampipe-based implementation.
type Store interface {
	// Resources
	UpsertResource(ctx context.Context, r *ObservedResource) error
	GetResource(ctx context.Context, id string) (*ObservedResource, error)
	GetResourceByExternalID(ctx context.Context, projectID, externalID string) (*ObservedResource, error)
	QueryResources(ctx context.Context, q ResourceQuery) ([]*ObservedResource, error)
	DeleteResource(ctx context.Context, id string) error

	// Staleness
	GetStaleResources(ctx context.Context, projectID string, resourceType ResourceType, environment string, threshold time.Duration) ([]*ObservedResource, error)
	CountResources(ctx context.Context, projectID string) (int, error)

	// Changes
	CreateChange(ctx context.Context, c *StateChange) error
	GetChange(ctx context.Context, id string) (*StateChange, error)
	ListChanges(ctx context.Context, projectID string, since time.Time, limit int) ([]*StateChange, error)
	ListChangesByResource(ctx context.Context, resourceID string, limit int) ([]*StateChange, error)

	// Attribution
	CreateAttribution(ctx context.Context, a *StateAttribution) error
	GetAttribution(ctx context.Context, id string) (*StateAttribution, error)
	ListUnattributed(ctx context.Context, projectID string, limit int) ([]*StateAttribution, error)
	ListAttributionsByResource(ctx context.Context, resourceID string) ([]*StateAttribution, error)
	ListAttributionsByTicket(ctx context.Context, ticketID string) ([]*StateAttribution, error)

	// Summary
	GetSummary(ctx context.Context, projectID string, stalenessThreshold time.Duration) (*StateSummary, error)

	// Steampipe snapshots
	SaveSnapshot(ctx context.Context, s *SteampipeSnapshot) error
	GetLatestSnapshot(ctx context.Context, projectID, resourceType, environment string) (*SteampipeSnapshot, error)
}

// StateProvider is the plugin interface for alternative cloud state backends.
// Implementations fetch current infrastructure state from their respective sources
// (Steampipe, CloudQuery, CSPM tools, etc.) and return normalized ObservedResource objects.
type StateProvider interface {
	// Name returns the provider identifier (e.g. "steampipe", "cloudquery").
	Name() string

	// QueryState fetches current state for a resource type from the backend.
	QueryState(ctx context.Context, projectID string, resourceType ResourceType, environment string) ([]*ObservedResource, error)

	// SupportedResourceTypes returns the resource types this provider can query.
	SupportedResourceTypes() []ResourceType

	// Healthy returns nil if the provider backend is reachable.
	Healthy(ctx context.Context) error
}
