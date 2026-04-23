package stateindex

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gabinante/flywheel/events"
)

// Event types emitted by the state index service.
const (
	EventResourceObserved    = "stateindex.resource_observed"
	EventChangeDetected      = "stateindex.change_detected"
	EventDriftDetected       = "stateindex.drift_detected"
	EventAttributionCreated  = "stateindex.attribution_created"
	EventUnattributedChange  = "stateindex.unattributed_change"
)

// DefaultStalenessThreshold is the default age after which an observation is stale.
const DefaultStalenessThreshold = 5 * time.Minute

// Service implements the observed state index business logic.
type Service struct {
	store     Store
	bus       events.Bus
	providers map[string]StateProvider
}

// NewService creates a state index Service.
func NewService(store Store, bus events.Bus) *Service {
	return &Service{
		store:     store,
		bus:       bus,
		providers: make(map[string]StateProvider),
	}
}

// RegisterProvider adds a state provider (Steampipe, CloudQuery, etc.).
func (s *Service) RegisterProvider(provider StateProvider) {
	s.providers[provider.Name()] = provider
}

// GetProvider returns a registered provider by name.
func (s *Service) GetProvider(name string) StateProvider {
	return s.providers[name]
}

// --------------------------------------------------------------------------
// Resource observation
// --------------------------------------------------------------------------

// ObserveResource records a new observation of a resource's state.
// If the resource already exists, detects changes and creates a change record.
func (s *Service) ObserveResource(ctx context.Context, r *ObservedResource) (*StateChange, error) {
	if r.ID == "" {
		r.ID = generateID()
	}
	if r.ObservedAt.IsZero() {
		r.ObservedAt = time.Now().UTC()
	}
	if r.Source == "" {
		r.Source = "manual"
	}

	// Check for existing resource to detect changes.
	existing, err := s.store.GetResource(ctx, r.ID)
	if err != nil && err != ErrResourceNotFound {
		return nil, fmt.Errorf("get existing resource: %w", err)
	}

	// Upsert the resource.
	if err := s.store.UpsertResource(ctx, r); err != nil {
		return nil, fmt.Errorf("upsert resource: %w", err)
	}

	_ = s.bus.Publish(ctx, events.Event{
		Type: EventResourceObserved,
		Payload: map[string]any{
			"resource_id":   r.ID,
			"project_id":    r.ProjectID,
			"resource_type": string(r.ResourceType),
			"source":        r.Source,
		},
	})

	// Detect changes if resource existed before.
	if existing != nil {
		return s.detectChange(ctx, existing, r)
	}

	// New resource — create a "created" change.
	change := &StateChange{
		ID:         generateID(),
		ProjectID:  r.ProjectID,
		ResourceID: r.ID,
		ChangeType: ChangeCreated,
		AfterState: r.Properties,
		DetectedAt: time.Now().UTC(),
	}
	if err := s.store.CreateChange(ctx, change); err != nil {
		return nil, fmt.Errorf("create change: %w", err)
	}

	// Mark as unattributed by default.
	attr := &StateAttribution{
		ID:         generateID(),
		ProjectID:  r.ProjectID,
		ChangeID:   change.ID,
		ResourceID: r.ID,
		ActorType:  ActorUnattributed,
		Confidence: 0.0,
		Rationale:  "New resource observed, no attribution yet",
	}
	if err := s.store.CreateAttribution(ctx, attr); err != nil {
		return nil, fmt.Errorf("create attribution: %w", err)
	}

	_ = s.bus.Publish(ctx, events.Event{
		Type: EventUnattributedChange,
		Payload: map[string]any{
			"change_id":   change.ID,
			"resource_id": r.ID,
			"project_id":  r.ProjectID,
			"change_type": string(ChangeCreated),
		},
	})

	return change, nil
}

// detectChange compares old and new state and records changes.
func (s *Service) detectChange(ctx context.Context, old, new *ObservedResource) (*StateChange, error) {
	diff := computeDiff(old.Properties, new.Properties)
	if len(diff) == 0 {
		return nil, nil // No change detected.
	}

	change := &StateChange{
		ID:          generateID(),
		ProjectID:   new.ProjectID,
		ResourceID:  new.ID,
		ChangeType:  ChangeUpdated,
		BeforeState: old.Properties,
		AfterState:  new.Properties,
		Diff:        diff,
		DetectedAt:  time.Now().UTC(),
	}
	if err := s.store.CreateChange(ctx, change); err != nil {
		return nil, fmt.Errorf("create change: %w", err)
	}

	_ = s.bus.Publish(ctx, events.Event{
		Type: EventChangeDetected,
		Payload: map[string]any{
			"change_id":   change.ID,
			"resource_id": new.ID,
			"project_id":  new.ProjectID,
			"change_type": string(ChangeUpdated),
		},
	})

	// Mark as unattributed by default.
	attr := &StateAttribution{
		ID:         generateID(),
		ProjectID:  new.ProjectID,
		ChangeID:   change.ID,
		ResourceID: new.ID,
		ActorType:  ActorUnattributed,
		Confidence: 0.0,
		Rationale:  "Change detected, no attribution yet",
	}
	if err := s.store.CreateAttribution(ctx, attr); err != nil {
		return nil, fmt.Errorf("create attribution: %w", err)
	}

	_ = s.bus.Publish(ctx, events.Event{
		Type: EventUnattributedChange,
		Payload: map[string]any{
			"change_id":   change.ID,
			"resource_id": new.ID,
			"project_id":  new.ProjectID,
		},
	})

	return change, nil
}

// --------------------------------------------------------------------------
// Queries
// --------------------------------------------------------------------------

// QueryResources queries the state index with filters.
func (s *Service) QueryResources(ctx context.Context, q ResourceQuery) ([]*ObservedResource, error) {
	return s.store.QueryResources(ctx, q)
}

// GetResource returns a single resource by ID.
func (s *Service) GetResource(ctx context.Context, id string) (*ObservedResource, error) {
	return s.store.GetResource(ctx, id)
}

// --------------------------------------------------------------------------
// Staleness
// --------------------------------------------------------------------------

// CheckStaleness returns resources whose observations are older than the threshold.
func (s *Service) CheckStaleness(ctx context.Context, projectID string, resourceType ResourceType, environment string, thresholdSeconds int) (*StalenessReport, error) {
	if thresholdSeconds <= 0 {
		thresholdSeconds = int(DefaultStalenessThreshold.Seconds())
	}
	threshold := time.Duration(thresholdSeconds) * time.Second

	stale, err := s.store.GetStaleResources(ctx, projectID, resourceType, environment, threshold)
	if err != nil {
		return nil, fmt.Errorf("get stale resources: %w", err)
	}

	total, err := s.store.CountResources(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("count resources: %w", err)
	}

	return &StalenessReport{
		ProjectID:        projectID,
		ThresholdSeconds: thresholdSeconds,
		StaleCount:       len(stale),
		TotalCount:       total,
		StaleResources:   stale,
	}, nil
}

// --------------------------------------------------------------------------
// Drift detection
// --------------------------------------------------------------------------

// DetectDrift finds resources where observed state differs from declared state.
func (s *Service) DetectDrift(ctx context.Context, projectID string, resourceID string, environment string) (*DriftReport, error) {
	q := ResourceQuery{
		ProjectID:   projectID,
		Environment: environment,
		Limit:       500,
	}
	resources, err := s.store.QueryResources(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query resources: %w", err)
	}

	report := &DriftReport{ProjectID: projectID}
	for _, r := range resources {
		if resourceID != "" && r.ID != resourceID {
			continue
		}
		if r.DeclaredState == nil || len(r.DeclaredState) == 0 {
			continue
		}
		diffs := computeDiff(r.DeclaredState, r.Properties)
		if len(diffs) == 0 {
			continue
		}

		item := &DriftItem{
			ResourceID:   r.ID,
			ResourceName: r.Name,
			ResourceType: r.ResourceType,
			Environment:  r.Environment,
			Declared:     r.DeclaredState,
			Observed:     r.Properties,
			Differences:  diffs,
		}
		report.Drifts = append(report.Drifts, item)

		// Emit event and create a change record for drift.
		change := &StateChange{
			ID:          generateID(),
			ProjectID:   r.ProjectID,
			ResourceID:  r.ID,
			ChangeType:  ChangeDriftDetected,
			BeforeState: r.DeclaredState,
			AfterState:  r.Properties,
			Diff:        diffs,
			DetectedAt:  time.Now().UTC(),
		}
		_ = s.store.CreateChange(ctx, change)

		_ = s.bus.Publish(ctx, events.Event{
			Type: EventDriftDetected,
			Payload: map[string]any{
				"resource_id": r.ID,
				"project_id":  r.ProjectID,
				"drift_count": len(diffs),
			},
		})
	}

	report.DriftCount = len(report.Drifts)
	return report, nil
}

// --------------------------------------------------------------------------
// Attribution
// --------------------------------------------------------------------------

// AttributeChange assigns a state change to a ticket or external actor.
func (s *Service) AttributeChange(ctx context.Context, projectID, resourceID, ticketID, actor, changeID string) (*StateAttribution, error) {
	actorType := ActorUnattributed
	if ticketID != "" {
		actorType = ActorTicket
	} else if actor != "" {
		actorType = ActorExternal
	}

	confidence := 1.0
	rationale := "Manual attribution"
	if actorType == ActorTicket {
		rationale = fmt.Sprintf("Attributed to ticket %s", ticketID)
	} else if actorType == ActorExternal {
		rationale = fmt.Sprintf("Attributed to external actor: %s", actor)
	}

	// If no specific changeID, find the most recent change for this resource.
	if changeID == "" {
		changes, err := s.store.ListChangesByResource(ctx, resourceID, 1)
		if err != nil {
			return nil, fmt.Errorf("list changes: %w", err)
		}
		if len(changes) == 0 {
			return nil, fmt.Errorf("no changes found for resource %s", resourceID)
		}
		changeID = changes[0].ID
	}

	attr := &StateAttribution{
		ID:         generateID(),
		ProjectID:  projectID,
		ChangeID:   changeID,
		ResourceID: resourceID,
		TicketID:   ticketID,
		Actor:      actor,
		ActorType:  actorType,
		Confidence: confidence,
		Rationale:  rationale,
	}

	if err := s.store.CreateAttribution(ctx, attr); err != nil {
		return nil, fmt.Errorf("create attribution: %w", err)
	}

	_ = s.bus.Publish(ctx, events.Event{
		Type: EventAttributionCreated,
		Payload: map[string]any{
			"attribution_id": attr.ID,
			"resource_id":    resourceID,
			"project_id":     projectID,
			"actor_type":     string(actorType),
		},
	})

	return attr, nil
}

// ListUnattributed returns state changes without attribution.
func (s *Service) ListUnattributed(ctx context.Context, projectID string, limit int) ([]*StateAttribution, error) {
	return s.store.ListUnattributed(ctx, projectID, limit)
}

// --------------------------------------------------------------------------
// Summary & Snapshots
// --------------------------------------------------------------------------

// GetSummary returns a high-level overview of the state index.
func (s *Service) GetSummary(ctx context.Context, projectID string) (*StateSummary, error) {
	return s.store.GetSummary(ctx, projectID, DefaultStalenessThreshold)
}

// GetSnapshot returns a point-in-time snapshot of resource states for freshness stamping.
func (s *Service) GetSnapshot(ctx context.Context, projectID string, resourceIDs []string, environment string) ([]*ObservedResource, error) {
	var result []*ObservedResource
	for _, id := range resourceIDs {
		r, err := s.store.GetResource(ctx, id)
		if err != nil {
			continue // skip resources that don't exist
		}
		if environment != "" && r.Environment != environment {
			continue
		}
		result = append(result, r)
	}
	return result, nil
}

// ListChanges returns recent state changes for a project.
func (s *Service) ListChanges(ctx context.Context, projectID string, since time.Time, limit int) ([]*StateChange, error) {
	return s.store.ListChanges(ctx, projectID, since, limit)
}

// --------------------------------------------------------------------------
// Provider-based ingestion
// --------------------------------------------------------------------------

// IngestFromProvider uses a registered provider to fetch and observe resources.
func (s *Service) IngestFromProvider(ctx context.Context, projectID, providerName string, resourceType ResourceType, environment string) (int, error) {
	provider, ok := s.providers[providerName]
	if !ok {
		return 0, fmt.Errorf("unknown provider: %s", providerName)
	}

	resources, err := provider.QueryState(ctx, projectID, resourceType, environment)
	if err != nil {
		return 0, fmt.Errorf("query state from %s: %w", providerName, err)
	}

	observed := 0
	for _, r := range resources {
		r.Source = providerName
		if _, err := s.ObserveResource(ctx, r); err != nil {
			continue // log and continue on individual failures
		}
		observed++
	}
	return observed, nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// computeDiff returns the keys that differ between two property maps.
func computeDiff(declared, observed map[string]any) map[string]any {
	diff := make(map[string]any)
	for k, dv := range declared {
		ov, exists := observed[k]
		if !exists {
			diff[k] = map[string]any{"declared": dv, "observed": nil, "type": "missing"}
		} else if fmtAny(dv) != fmtAny(ov) {
			diff[k] = map[string]any{"declared": dv, "observed": ov, "type": "changed"}
		}
	}
	for k, ov := range observed {
		if _, exists := declared[k]; !exists {
			diff[k] = map[string]any{"declared": nil, "observed": ov, "type": "extra"}
		}
	}
	return diff
}

// generateID creates a unique identifier using timestamp + hash.
func generateID() string {
	now := time.Now().UTC()
	data := fmt.Sprintf("%d-%d", now.UnixNano(), now.UnixMicro())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("si_%x", hash[:8])
}

// hashResult computes a hash of JSON data for change detection.
func HashResult(data any) string {
	b, _ := json.Marshal(data)
	hash := sha256.Sum256(b)
	return fmt.Sprintf("%x", hash[:16])
}
