package stateindex

import (
	"context"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
)

// mockStore implements Store for unit testing (in-memory).
type mockStore struct {
	resources    map[string]*ObservedResource
	changes      []*StateChange
	attributions []*StateAttribution
	snapshots    []*SteampipeSnapshot
}

func newMockStore() *mockStore {
	return &mockStore{
		resources: make(map[string]*ObservedResource),
	}
}

func (s *mockStore) UpsertResource(_ context.Context, r *ObservedResource) error {
	s.resources[r.ID] = r
	return nil
}

func (s *mockStore) GetResource(_ context.Context, id string) (*ObservedResource, error) {
	r, ok := s.resources[id]
	if !ok {
		return nil, ErrResourceNotFound
	}
	return r, nil
}

func (s *mockStore) GetResourceByExternalID(_ context.Context, projectID, externalID string) (*ObservedResource, error) {
	for _, r := range s.resources {
		if r.ProjectID == projectID && r.ExternalID == externalID {
			return r, nil
		}
	}
	return nil, ErrResourceNotFound
}

func (s *mockStore) QueryResources(_ context.Context, q ResourceQuery) ([]*ObservedResource, error) {
	var result []*ObservedResource
	for _, r := range s.resources {
		if r.ProjectID != q.ProjectID {
			continue
		}
		if q.ResourceType != "" && r.ResourceType != q.ResourceType {
			continue
		}
		if q.Environment != "" && r.Environment != q.Environment {
			continue
		}
		result = append(result, r)
	}
	return result, nil
}

func (s *mockStore) DeleteResource(_ context.Context, id string) error {
	delete(s.resources, id)
	return nil
}

func (s *mockStore) GetStaleResources(_ context.Context, projectID string, resourceType ResourceType, environment string, threshold time.Duration) ([]*ObservedResource, error) {
	cutoff := time.Now().UTC().Add(-threshold)
	var result []*ObservedResource
	for _, r := range s.resources {
		if r.ProjectID != projectID {
			continue
		}
		if resourceType != "" && r.ResourceType != resourceType {
			continue
		}
		if environment != "" && r.Environment != environment {
			continue
		}
		if r.ObservedAt.Before(cutoff) {
			result = append(result, r)
		}
	}
	return result, nil
}

func (s *mockStore) CountResources(_ context.Context, projectID string) (int, error) {
	count := 0
	for _, r := range s.resources {
		if r.ProjectID == projectID {
			count++
		}
	}
	return count, nil
}

func (s *mockStore) CreateChange(_ context.Context, c *StateChange) error {
	s.changes = append(s.changes, c)
	return nil
}

func (s *mockStore) GetChange(_ context.Context, id string) (*StateChange, error) {
	for _, c := range s.changes {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, ErrChangeNotFound
}

func (s *mockStore) ListChanges(_ context.Context, projectID string, since time.Time, limit int) ([]*StateChange, error) {
	var result []*StateChange
	for _, c := range s.changes {
		if c.ProjectID == projectID && !c.DetectedAt.Before(since) {
			result = append(result, c)
		}
	}
	return result, nil
}

func (s *mockStore) ListChangesByResource(_ context.Context, resourceID string, limit int) ([]*StateChange, error) {
	var result []*StateChange
	for _, c := range s.changes {
		if c.ResourceID == resourceID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (s *mockStore) CreateAttribution(_ context.Context, a *StateAttribution) error {
	s.attributions = append(s.attributions, a)
	return nil
}

func (s *mockStore) GetAttribution(_ context.Context, id string) (*StateAttribution, error) {
	for _, a := range s.attributions {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, ErrAttributionNotFound
}

func (s *mockStore) ListUnattributed(_ context.Context, projectID string, limit int) ([]*StateAttribution, error) {
	var result []*StateAttribution
	for _, a := range s.attributions {
		if a.ProjectID == projectID && a.ActorType == ActorUnattributed {
			result = append(result, a)
		}
	}
	return result, nil
}

func (s *mockStore) ListAttributionsByResource(_ context.Context, resourceID string) ([]*StateAttribution, error) {
	var result []*StateAttribution
	for _, a := range s.attributions {
		if a.ResourceID == resourceID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (s *mockStore) ListAttributionsByTicket(_ context.Context, ticketID string) ([]*StateAttribution, error) {
	var result []*StateAttribution
	for _, a := range s.attributions {
		if a.TicketID == ticketID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (s *mockStore) GetSummary(_ context.Context, projectID string, stalenessThreshold time.Duration) (*StateSummary, error) {
	summary := &StateSummary{
		ProjectID:       projectID,
		ResourcesByType: make(map[string]int),
		ResourcesByEnv:  make(map[string]int),
		StalenessDistribution: make(map[string]int),
	}
	cutoff := time.Now().UTC().Add(-stalenessThreshold)
	for _, r := range s.resources {
		if r.ProjectID != projectID {
			continue
		}
		summary.TotalResources++
		summary.ResourcesByType[string(r.ResourceType)]++
		summary.ResourcesByEnv[r.Environment]++
		if r.ObservedAt.Before(cutoff) {
			summary.StaleCount++
		}
	}
	for _, a := range s.attributions {
		if a.ProjectID == projectID && a.ActorType == ActorUnattributed {
			summary.UnattributedCount++
		}
	}
	return summary, nil
}

func (s *mockStore) SaveSnapshot(_ context.Context, snap *SteampipeSnapshot) error {
	s.snapshots = append(s.snapshots, snap)
	return nil
}

func (s *mockStore) GetLatestSnapshot(_ context.Context, projectID, resourceType, environment string) (*SteampipeSnapshot, error) {
	for i := len(s.snapshots) - 1; i >= 0; i-- {
		snap := s.snapshots[i]
		if snap.ProjectID == projectID && snap.ResourceType == resourceType && snap.Environment == environment {
			return snap, nil
		}
	}
	return nil, nil
}

// (noopBus is defined at the bottom of this file.)

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

func TestObserveResource_New(t *testing.T) {
	store := newMockStore()
	svc := NewService(store, &noopBus{})

	resource := &ObservedResource{
		ID:           "test-resource-1",
		ProjectID:    "project-1",
		ResourceType: ResourceVM,
		Environment:  "prod",
		Name:         "web-server-1",
		ExternalID:   "i-abc123",
		Provider:     "aws",
		Region:       "us-east-1",
		Properties: map[string]any{
			"instance_type": "t3.medium",
			"status":        "running",
		},
		ObservedAt: time.Now().UTC(),
		Source:     "steampipe",
	}

	change, err := svc.ObserveResource(context.Background(), resource)
	if err != nil {
		t.Fatalf("ObserveResource: %v", err)
	}

	// Should create a "created" change.
	if change == nil {
		t.Fatal("expected a change record for new resource")
	}
	if change.ChangeType != ChangeCreated {
		t.Errorf("expected change type 'created', got '%s'", change.ChangeType)
	}

	// Resource should be stored.
	stored, err := store.GetResource(context.Background(), "test-resource-1")
	if err != nil {
		t.Fatalf("GetResource: %v", err)
	}
	if stored.Name != "web-server-1" {
		t.Errorf("expected name 'web-server-1', got '%s'", stored.Name)
	}

	// Should have an unattributed attribution.
	if len(store.attributions) != 1 {
		t.Fatalf("expected 1 attribution, got %d", len(store.attributions))
	}
	if store.attributions[0].ActorType != ActorUnattributed {
		t.Errorf("expected unattributed, got '%s'", store.attributions[0].ActorType)
	}
}

func TestObserveResource_Change(t *testing.T) {
	store := newMockStore()
	svc := NewService(store, &noopBus{})

	// Observe initial state.
	resource := &ObservedResource{
		ID:           "test-resource-2",
		ProjectID:    "project-1",
		ResourceType: ResourceVM,
		Environment:  "prod",
		Name:         "web-server-2",
		Properties: map[string]any{
			"instance_type": "t3.medium",
			"status":        "running",
		},
		ObservedAt: time.Now().UTC(),
	}
	_, err := svc.ObserveResource(context.Background(), resource)
	if err != nil {
		t.Fatalf("first ObserveResource: %v", err)
	}

	// Observe changed state.
	resource2 := &ObservedResource{
		ID:           "test-resource-2",
		ProjectID:    "project-1",
		ResourceType: ResourceVM,
		Environment:  "prod",
		Name:         "web-server-2",
		Properties: map[string]any{
			"instance_type": "t3.large", // changed
			"status":        "running",
		},
		ObservedAt: time.Now().UTC(),
	}
	change, err := svc.ObserveResource(context.Background(), resource2)
	if err != nil {
		t.Fatalf("second ObserveResource: %v", err)
	}

	if change == nil {
		t.Fatal("expected a change record for updated resource")
	}
	if change.ChangeType != ChangeUpdated {
		t.Errorf("expected change type 'updated', got '%s'", change.ChangeType)
	}
	if change.Diff == nil {
		t.Fatal("expected diff in change record")
	}
	if _, ok := change.Diff["instance_type"]; !ok {
		t.Error("expected 'instance_type' in diff")
	}
}

func TestObserveResource_NoChange(t *testing.T) {
	store := newMockStore()
	svc := NewService(store, &noopBus{})

	resource := &ObservedResource{
		ID:           "test-resource-3",
		ProjectID:    "project-1",
		ResourceType: ResourceVM,
		Environment:  "prod",
		Name:         "web-server-3",
		Properties:   map[string]any{"status": "running"},
		ObservedAt:   time.Now().UTC(),
	}
	_, err := svc.ObserveResource(context.Background(), resource)
	if err != nil {
		t.Fatalf("first ObserveResource: %v", err)
	}

	// Same state — should not create a change.
	resource2 := &ObservedResource{
		ID:           "test-resource-3",
		ProjectID:    "project-1",
		ResourceType: ResourceVM,
		Environment:  "prod",
		Name:         "web-server-3",
		Properties:   map[string]any{"status": "running"},
		ObservedAt:   time.Now().UTC(),
	}
	change, err := svc.ObserveResource(context.Background(), resource2)
	if err != nil {
		t.Fatalf("second ObserveResource: %v", err)
	}
	if change != nil {
		t.Error("expected no change for identical state")
	}
}

func TestCheckStaleness(t *testing.T) {
	store := newMockStore()
	svc := NewService(store, &noopBus{})

	// Add a stale resource.
	store.resources["stale-1"] = &ObservedResource{
		ID:           "stale-1",
		ProjectID:    "project-1",
		ResourceType: ResourceVM,
		Environment:  "prod",
		Name:         "stale-server",
		Properties:   map[string]any{},
		ObservedAt:   time.Now().UTC().Add(-10 * time.Minute),
	}
	// Add a fresh resource.
	store.resources["fresh-1"] = &ObservedResource{
		ID:           "fresh-1",
		ProjectID:    "project-1",
		ResourceType: ResourceVM,
		Environment:  "prod",
		Name:         "fresh-server",
		Properties:   map[string]any{},
		ObservedAt:   time.Now().UTC(),
	}

	report, err := svc.CheckStaleness(context.Background(), "project-1", "", "", 300)
	if err != nil {
		t.Fatalf("CheckStaleness: %v", err)
	}
	if report.StaleCount != 1 {
		t.Errorf("expected 1 stale resource, got %d", report.StaleCount)
	}
	if report.TotalCount != 2 {
		t.Errorf("expected 2 total resources, got %d", report.TotalCount)
	}
}

func TestDetectDrift(t *testing.T) {
	store := newMockStore()
	svc := NewService(store, &noopBus{})

	store.resources["drift-1"] = &ObservedResource{
		ID:            "drift-1",
		ProjectID:     "project-1",
		ResourceType:  ResourceVM,
		Environment:   "prod",
		Name:          "drifted-server",
		Properties:    map[string]any{"instance_type": "t3.large", "status": "running"},
		DeclaredState: map[string]any{"instance_type": "t3.medium", "status": "running"},
		ObservedAt:    time.Now().UTC(),
	}
	store.resources["nodrift-1"] = &ObservedResource{
		ID:            "nodrift-1",
		ProjectID:     "project-1",
		ResourceType:  ResourceVM,
		Environment:   "prod",
		Name:          "aligned-server",
		Properties:    map[string]any{"instance_type": "t3.medium"},
		DeclaredState: map[string]any{"instance_type": "t3.medium"},
		ObservedAt:    time.Now().UTC(),
	}

	report, err := svc.DetectDrift(context.Background(), "project-1", "", "")
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if report.DriftCount != 1 {
		t.Errorf("expected 1 drift, got %d", report.DriftCount)
	}
	if report.Drifts[0].ResourceID != "drift-1" {
		t.Errorf("expected drift on 'drift-1', got '%s'", report.Drifts[0].ResourceID)
	}
}

func TestAttributeChange(t *testing.T) {
	store := newMockStore()
	svc := NewService(store, &noopBus{})

	// Create a resource and observe it to generate a change.
	resource := &ObservedResource{
		ID:           "attr-resource-1",
		ProjectID:    "project-1",
		ResourceType: ResourceVM,
		Environment:  "prod",
		Name:         "attr-server",
		Properties:   map[string]any{"status": "running"},
		ObservedAt:   time.Now().UTC(),
	}
	_, _ = svc.ObserveResource(context.Background(), resource)

	// Attribute the change to a ticket.
	attr, err := svc.AttributeChange(context.Background(), "project-1", "attr-resource-1", "ticket-123", "", "")
	if err != nil {
		t.Fatalf("AttributeChange: %v", err)
	}
	if attr.ActorType != ActorTicket {
		t.Errorf("expected actor type 'ticket', got '%s'", attr.ActorType)
	}
	if attr.TicketID != "ticket-123" {
		t.Errorf("expected ticket_id 'ticket-123', got '%s'", attr.TicketID)
	}
	if attr.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", attr.Confidence)
	}
}

func TestAttributeChangeExternal(t *testing.T) {
	store := newMockStore()
	svc := NewService(store, &noopBus{})

	resource := &ObservedResource{
		ID:           "attr-resource-2",
		ProjectID:    "project-1",
		ResourceType: ResourceVM,
		Name:         "ext-server",
		Properties:   map[string]any{"status": "running"},
		ObservedAt:   time.Now().UTC(),
	}
	_, _ = svc.ObserveResource(context.Background(), resource)

	attr, err := svc.AttributeChange(context.Background(), "project-1", "attr-resource-2", "", "deploy-bot", "")
	if err != nil {
		t.Fatalf("AttributeChange: %v", err)
	}
	if attr.ActorType != ActorExternal {
		t.Errorf("expected actor type 'external', got '%s'", attr.ActorType)
	}
}

func TestHasDrift(t *testing.T) {
	r := &ObservedResource{
		Properties:    map[string]any{"instance_type": "t3.large"},
		DeclaredState: map[string]any{"instance_type": "t3.medium"},
	}
	if !r.HasDrift() {
		t.Error("expected drift when declared != observed")
	}

	r2 := &ObservedResource{
		Properties:    map[string]any{"instance_type": "t3.medium"},
		DeclaredState: map[string]any{"instance_type": "t3.medium"},
	}
	if r2.HasDrift() {
		t.Error("expected no drift when declared == observed")
	}

	r3 := &ObservedResource{
		Properties:    map[string]any{"instance_type": "t3.medium"},
		DeclaredState: nil,
	}
	if r3.HasDrift() {
		t.Error("expected no drift when no declared state")
	}
}

func TestIsStale(t *testing.T) {
	r := &ObservedResource{
		ObservedAt: time.Now().UTC().Add(-10 * time.Minute),
	}
	if !r.IsStale(5 * time.Minute) {
		t.Error("expected stale for 10m old observation with 5m threshold")
	}
	if r.IsStale(15 * time.Minute) {
		t.Error("expected not stale for 10m old observation with 15m threshold")
	}
}

func TestComputeDiff(t *testing.T) {
	declared := map[string]any{"a": "1", "b": "2", "c": "3"}
	observed := map[string]any{"a": "1", "b": "changed", "d": "extra"}

	diff := computeDiff(declared, observed)

	if _, ok := diff["a"]; ok {
		t.Error("'a' should not be in diff (unchanged)")
	}
	if _, ok := diff["b"]; !ok {
		t.Error("'b' should be in diff (changed)")
	}
	if _, ok := diff["c"]; !ok {
		t.Error("'c' should be in diff (missing from observed)")
	}
	if _, ok := diff["d"]; !ok {
		t.Error("'d' should be in diff (extra in observed)")
	}
}

func TestGetSummary(t *testing.T) {
	store := newMockStore()
	svc := NewService(store, &noopBus{})

	store.resources["r1"] = &ObservedResource{
		ID: "r1", ProjectID: "project-1", ResourceType: ResourceVM, Environment: "prod",
		Properties: map[string]any{}, ObservedAt: time.Now().UTC(),
	}
	store.resources["r2"] = &ObservedResource{
		ID: "r2", ProjectID: "project-1", ResourceType: ResourceDatabase, Environment: "prod",
		Properties: map[string]any{}, ObservedAt: time.Now().UTC().Add(-10 * time.Minute),
	}

	summary, err := svc.GetSummary(context.Background(), "project-1")
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if summary.TotalResources != 2 {
		t.Errorf("expected 2 total resources, got %d", summary.TotalResources)
	}
	if summary.ResourcesByType["vm"] != 1 {
		t.Errorf("expected 1 vm, got %d", summary.ResourcesByType["vm"])
	}
	if summary.ResourcesByType["database"] != 1 {
		t.Errorf("expected 1 database, got %d", summary.ResourcesByType["database"])
	}
}

// noopBus is a no-op event bus for testing.
type noopBus struct{}

func (b *noopBus) Publish(_ context.Context, _ events.Event) error { return nil }
func (b *noopBus) Subscribe(_ string, _ events.HandlerFn)          {}
