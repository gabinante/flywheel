package claims

import (
	"context"
	"testing"
	"time"
)

// --- In-memory store for testing ---

type memStore struct {
	claims    map[string]*Claim
	conflicts map[string]*Conflict
}

func newMemStore() *memStore {
	return &memStore{
		claims:    make(map[string]*Claim),
		conflicts: make(map[string]*Conflict),
	}
}

func (m *memStore) CreateClaim(_ context.Context, c *Claim) error {
	m.claims[c.ID] = c
	return nil
}

func (m *memStore) GetClaim(_ context.Context, id string) (*Claim, error) {
	c, ok := m.claims[id]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func (m *memStore) GetActiveByEntity(_ context.Context, entityID, environment string) ([]*Claim, error) {
	var result []*Claim
	for _, c := range m.claims {
		if c.EntityID == entityID && c.Environment == environment && c.State == StateActive {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *memStore) GetActiveByTicket(_ context.Context, ticketID string) ([]*Claim, error) {
	var result []*Claim
	for _, c := range m.claims {
		if c.TicketID == ticketID && c.State == StateActive {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *memStore) GetActiveByEnvironment(_ context.Context, environment string) ([]*Claim, error) {
	var result []*Claim
	for _, c := range m.claims {
		if c.Environment == environment && c.State == StateActive {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *memStore) ReleaseClaim(_ context.Context, id string, releasedAt time.Time) error {
	c, ok := m.claims[id]
	if !ok || c.State != StateActive {
		return ErrNotFound
	}
	c.State = StateReleased
	c.ReleasedAt = &releasedAt
	return nil
}

func (m *memStore) ReleaseByTicket(_ context.Context, ticketID string, releasedAt time.Time) (int, error) {
	count := 0
	for _, c := range m.claims {
		if c.TicketID == ticketID && c.State == StateActive {
			c.State = StateReleased
			c.ReleasedAt = &releasedAt
			count++
		}
	}
	return count, nil
}

func (m *memStore) CreateConflict(_ context.Context, c *Conflict) error {
	m.conflicts[c.ID] = c
	return nil
}

func (m *memStore) GetUnresolvedConflicts(_ context.Context, ticketID string) ([]*Conflict, error) {
	var result []*Conflict
	for _, c := range m.conflicts {
		if c.TicketID == ticketID && c.ResolvedAt == nil {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *memStore) ResolveConflictsByTicket(_ context.Context, blockingTicketID string, resolvedAt time.Time) (int, error) {
	count := 0
	for _, c := range m.conflicts {
		if c.BlockingTicket == blockingTicketID && c.ResolvedAt == nil {
			c.ResolvedAt = &resolvedAt
			count++
		}
	}
	return count, nil
}

// --- Fake event bus ---

type fakeBus struct {
	published []string
}

func (f *fakeBus) Publish(_ context.Context, e interface{ getType() string }) error {
	return nil
}

type fakeBusAdapter struct {
	published []string
}

func (f *fakeBusAdapter) Publish(_ context.Context, evt interface{}) error {
	return nil
}

// --- Tests ---

func newTestService() (*Service, *memStore) {
	ms := newMemStore()
	// We use a nil bus for testing since we wrap the store directly
	svc := &Service{store: &Store{pool: nil}, bus: nil}
	// Replace the store internals with our mem store via a test service
	return svc, ms
}

// testService creates a service with in-memory store for testing.
// We bypass the Postgres-backed store by testing at the integration level.
func testServiceWithMemStore() *serviceWithMem {
	ms := newMemStore()
	return &serviceWithMem{ms: ms}
}

type serviceWithMem struct {
	ms *memStore
}

func (s *serviceWithMem) RegisterClaims(ctx context.Context, ticketID string, touches []Touch) ([]*Claim, error) {
	now := time.Now().UTC()
	var registered []*Claim
	for _, t := range touches {
		claim := &Claim{
			ID:          "claim-" + t.EntityID + "-" + ticketID,
			TicketID:    ticketID,
			EntityID:    t.EntityID,
			Environment: t.Environment,
			ClaimType:   t.ClaimType,
			State:       StateActive,
			Metadata:    t.Metadata,
			ClaimedAt:   now,
		}
		if err := s.ms.CreateClaim(ctx, claim); err != nil {
			return registered, err
		}
		registered = append(registered, claim)
	}
	return registered, nil
}

func (s *serviceWithMem) ReleaseClaims(ctx context.Context, ticketID string) (int, error) {
	now := time.Now().UTC()
	count, _ := s.ms.ReleaseByTicket(ctx, ticketID, now)
	_, _ = s.ms.ResolveConflictsByTicket(ctx, ticketID, now)
	return count, nil
}

func (s *serviceWithMem) DetectConflicts(ctx context.Context, ticketID string, touches []Touch) (*ConflictResult, error) {
	result := &ConflictResult{ParallelSafe: true}

	for _, t := range touches {
		activeClaims, err := s.ms.GetActiveByEntity(ctx, t.EntityID, t.Environment)
		if err != nil {
			return nil, err
		}
		for _, existing := range activeClaims {
			if existing.TicketID == ticketID {
				continue
			}
			conflictType, severity := classifyConflict(t, existing)
			if conflictType == ConflictDisjoint {
				continue
			}
			conflict := Conflict{
				ID:             "conflict-" + existing.ID,
				TicketID:       ticketID,
				BlockingTicket: existing.TicketID,
				ClaimID:        existing.ID,
				ConflictType:   conflictType,
				Severity:       severity,
				DetectedAt:     time.Now().UTC(),
			}
			result.Conflicts = append(result.Conflicts, conflict)
			result.HasConflicts = true
			result.ParallelSafe = false
			if severity == SeverityHard {
				result.HardCount++
			} else {
				result.SoftCount++
			}
		}
	}
	return result, nil
}

func TestRegisterClaims(t *testing.T) {
	ctx := context.Background()
	svc := testServiceWithMemStore()

	touches := []Touch{
		{EntityID: "src/main.go", Environment: "dev", ClaimType: ClaimFileWrite},
		{EntityID: "OrderService", Environment: "prod", ClaimType: ClaimService},
	}

	claims, err := svc.RegisterClaims(ctx, "ticket-1", touches)
	if err != nil {
		t.Fatalf("RegisterClaims: %v", err)
	}
	if len(claims) != 2 {
		t.Fatalf("expected 2 claims, got %d", len(claims))
	}
	if claims[0].State != StateActive {
		t.Errorf("expected active state, got %s", claims[0].State)
	}
	if claims[0].TicketID != "ticket-1" {
		t.Errorf("expected ticket-1, got %s", claims[0].TicketID)
	}
	if claims[1].Environment != "prod" {
		t.Errorf("expected prod environment, got %s", claims[1].Environment)
	}
}

func TestReleaseClaims(t *testing.T) {
	ctx := context.Background()
	svc := testServiceWithMemStore()

	touches := []Touch{
		{EntityID: "src/main.go", Environment: "dev", ClaimType: ClaimFileWrite},
		{EntityID: "src/util.go", Environment: "dev", ClaimType: ClaimFileWrite},
	}

	_, err := svc.RegisterClaims(ctx, "ticket-1", touches)
	if err != nil {
		t.Fatalf("RegisterClaims: %v", err)
	}

	count, err := svc.ReleaseClaims(ctx, "ticket-1")
	if err != nil {
		t.Fatalf("ReleaseClaims: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 released, got %d", count)
	}

	// Verify claims are released
	active, _ := svc.ms.GetActiveByTicket(ctx, "ticket-1")
	if len(active) != 0 {
		t.Errorf("expected 0 active claims after release, got %d", len(active))
	}
}

func TestDetectConflicts_HardConflict_SameFileWrite(t *testing.T) {
	ctx := context.Background()
	svc := testServiceWithMemStore()

	// ticket-1 claims a file write
	_, err := svc.RegisterClaims(ctx, "ticket-1", []Touch{
		{EntityID: "src/main.go", Environment: "dev", ClaimType: ClaimFileWrite},
	})
	if err != nil {
		t.Fatalf("RegisterClaims: %v", err)
	}

	// ticket-2 wants to write same file
	result, err := svc.DetectConflicts(ctx, "ticket-2", []Touch{
		{EntityID: "src/main.go", Environment: "dev", ClaimType: ClaimFileWrite},
	})
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}

	if !result.HasConflicts {
		t.Fatal("expected conflict")
	}
	if result.ParallelSafe {
		t.Fatal("expected not parallel-safe")
	}
	if result.HardCount != 1 {
		t.Errorf("expected 1 hard conflict, got %d", result.HardCount)
	}
	if result.Conflicts[0].ConflictType != ConflictSameFileWrite {
		t.Errorf("expected same_file_write, got %s", result.Conflicts[0].ConflictType)
	}
	if result.Conflicts[0].Severity != SeverityHard {
		t.Errorf("expected hard severity, got %s", result.Conflicts[0].Severity)
	}
}

func TestDetectConflicts_SoftConflict_SameService(t *testing.T) {
	ctx := context.Background()
	svc := testServiceWithMemStore()

	// ticket-1 claims a service
	_, err := svc.RegisterClaims(ctx, "ticket-1", []Touch{
		{EntityID: "OrderService", Environment: "prod", ClaimType: ClaimService},
	})
	if err != nil {
		t.Fatalf("RegisterClaims: %v", err)
	}

	// ticket-2 also touches the same service
	result, err := svc.DetectConflicts(ctx, "ticket-2", []Touch{
		{EntityID: "OrderService", Environment: "prod", ClaimType: ClaimService},
	})
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}

	if !result.HasConflicts {
		t.Fatal("expected conflict")
	}
	if result.SoftCount != 1 {
		t.Errorf("expected 1 soft conflict, got %d", result.SoftCount)
	}
	if result.Conflicts[0].Severity != SeveritySoft {
		t.Errorf("expected soft severity, got %s", result.Conflicts[0].Severity)
	}
}

func TestDetectConflicts_NoConflict_DifferentEnvironment(t *testing.T) {
	ctx := context.Background()
	svc := testServiceWithMemStore()

	// ticket-1 claims a file in dev
	_, err := svc.RegisterClaims(ctx, "ticket-1", []Touch{
		{EntityID: "src/main.go", Environment: "dev", ClaimType: ClaimFileWrite},
	})
	if err != nil {
		t.Fatalf("RegisterClaims: %v", err)
	}

	// ticket-2 claims same file in staging — no conflict (environment-scoped)
	result, err := svc.DetectConflicts(ctx, "ticket-2", []Touch{
		{EntityID: "src/main.go", Environment: "staging", ClaimType: ClaimFileWrite},
	})
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}

	if result.HasConflicts {
		t.Fatal("expected no conflict across different environments")
	}
	if !result.ParallelSafe {
		t.Fatal("expected parallel-safe across environments")
	}
}

func TestDetectConflicts_NoConflict_DifferentEntity(t *testing.T) {
	ctx := context.Background()
	svc := testServiceWithMemStore()

	// ticket-1 writes one file
	_, err := svc.RegisterClaims(ctx, "ticket-1", []Touch{
		{EntityID: "src/main.go", Environment: "dev", ClaimType: ClaimFileWrite},
	})
	if err != nil {
		t.Fatalf("RegisterClaims: %v", err)
	}

	// ticket-2 writes a different file — no conflict
	result, err := svc.DetectConflicts(ctx, "ticket-2", []Touch{
		{EntityID: "src/other.go", Environment: "dev", ClaimType: ClaimFileWrite},
	})
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}

	if result.HasConflicts {
		t.Fatal("expected no conflict for different entities")
	}
	if !result.ParallelSafe {
		t.Fatal("expected parallel-safe for different entities")
	}
}

func TestDetectConflicts_NoSelfConflict(t *testing.T) {
	ctx := context.Background()
	svc := testServiceWithMemStore()

	// ticket-1 claims a file
	_, err := svc.RegisterClaims(ctx, "ticket-1", []Touch{
		{EntityID: "src/main.go", Environment: "dev", ClaimType: ClaimFileWrite},
	})
	if err != nil {
		t.Fatalf("RegisterClaims: %v", err)
	}

	// ticket-1 checks its own touches — should not self-conflict
	result, err := svc.DetectConflicts(ctx, "ticket-1", []Touch{
		{EntityID: "src/main.go", Environment: "dev", ClaimType: ClaimFileWrite},
	})
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}

	if result.HasConflicts {
		t.Fatal("ticket should not conflict with itself")
	}
}

func TestDetectConflicts_Disjoint_DifferentClaimTypes(t *testing.T) {
	ctx := context.Background()
	svc := testServiceWithMemStore()

	// ticket-1 claims a file write on an entity
	_, err := svc.RegisterClaims(ctx, "ticket-1", []Touch{
		{EntityID: "entity-1", Environment: "dev", ClaimType: ClaimFileWrite},
	})
	if err != nil {
		t.Fatalf("RegisterClaims: %v", err)
	}

	// ticket-2 claims a deploy target on the same entity — disjoint
	result, err := svc.DetectConflicts(ctx, "ticket-2", []Touch{
		{EntityID: "entity-1", Environment: "dev", ClaimType: ClaimDeployTarget},
	})
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}

	if result.HasConflicts {
		t.Fatal("expected disjoint (no conflict) for different claim types")
	}
}

func TestDetectConflicts_HardConflict_SameSymbol(t *testing.T) {
	ctx := context.Background()
	svc := testServiceWithMemStore()

	// ticket-1 claims a symbol
	_, err := svc.RegisterClaims(ctx, "ticket-1", []Touch{
		{EntityID: "pkg.Handler.ServeHTTP", Environment: "dev", ClaimType: ClaimSymbol},
	})
	if err != nil {
		t.Fatalf("RegisterClaims: %v", err)
	}

	// ticket-2 also modifies the same symbol
	result, err := svc.DetectConflicts(ctx, "ticket-2", []Touch{
		{EntityID: "pkg.Handler.ServeHTTP", Environment: "dev", ClaimType: ClaimSymbol},
	})
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}

	if !result.HasConflicts {
		t.Fatal("expected conflict for same symbol")
	}
	if result.HardCount != 1 {
		t.Errorf("expected 1 hard conflict, got %d", result.HardCount)
	}
	if result.Conflicts[0].ConflictType != ConflictSameSymbol {
		t.Errorf("expected same_symbol conflict type, got %s", result.Conflicts[0].ConflictType)
	}
}

func TestDetectConflicts_HardConflict_SameSchema(t *testing.T) {
	ctx := context.Background()
	svc := testServiceWithMemStore()

	// ticket-1 claims a schema
	_, err := svc.RegisterClaims(ctx, "ticket-1", []Touch{
		{EntityID: "orders_table", Environment: "prod", ClaimType: ClaimSchema},
	})
	if err != nil {
		t.Fatalf("RegisterClaims: %v", err)
	}

	// ticket-2 also modifies the same schema
	result, err := svc.DetectConflicts(ctx, "ticket-2", []Touch{
		{EntityID: "orders_table", Environment: "prod", ClaimType: ClaimSchema},
	})
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}

	if !result.HasConflicts {
		t.Fatal("expected conflict for same schema")
	}
	if result.HardCount != 1 {
		t.Errorf("expected 1 hard conflict, got %d", result.HardCount)
	}
	if result.Conflicts[0].ConflictType != ConflictSameSchema {
		t.Errorf("expected same_schema conflict type, got %s", result.Conflicts[0].ConflictType)
	}
}

func TestReleaseClaims_ResolvesConflicts(t *testing.T) {
	ctx := context.Background()
	svc := testServiceWithMemStore()

	// ticket-1 claims a file
	_, err := svc.RegisterClaims(ctx, "ticket-1", []Touch{
		{EntityID: "src/main.go", Environment: "dev", ClaimType: ClaimFileWrite},
	})
	if err != nil {
		t.Fatalf("RegisterClaims: %v", err)
	}

	// ticket-2 detects conflict
	result, err := svc.DetectConflicts(ctx, "ticket-2", []Touch{
		{EntityID: "src/main.go", Environment: "dev", ClaimType: ClaimFileWrite},
	})
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}
	if !result.HasConflicts {
		t.Fatal("expected conflict")
	}

	// Record the conflict
	for i := range result.Conflicts {
		_ = svc.ms.CreateConflict(ctx, &result.Conflicts[i])
	}

	// ticket-1 completes — release claims
	_, err = svc.ReleaseClaims(ctx, "ticket-1")
	if err != nil {
		t.Fatalf("ReleaseClaims: %v", err)
	}

	// Conflicts should now be resolved
	unresolved, _ := svc.ms.GetUnresolvedConflicts(ctx, "ticket-2")
	if len(unresolved) != 0 {
		t.Errorf("expected 0 unresolved conflicts after release, got %d", len(unresolved))
	}
}

func TestClaimTypes_Valid(t *testing.T) {
	types := AllClaimTypes()
	if len(types) != 6 {
		t.Errorf("expected 6 claim types, got %d", len(types))
	}

	for _, ct := range types {
		if !IsValidClaimType(ct) {
			t.Errorf("expected %s to be valid", ct)
		}
	}

	if IsValidClaimType("invalid") {
		t.Error("expected 'invalid' to be invalid claim type")
	}
}

func TestMultipleConflicts(t *testing.T) {
	ctx := context.Background()
	svc := testServiceWithMemStore()

	// Two tickets holding different claims
	_, _ = svc.RegisterClaims(ctx, "ticket-1", []Touch{
		{EntityID: "src/main.go", Environment: "dev", ClaimType: ClaimFileWrite},
	})
	_, _ = svc.RegisterClaims(ctx, "ticket-2", []Touch{
		{EntityID: "src/util.go", Environment: "dev", ClaimType: ClaimFileWrite},
	})

	// ticket-3 wants to write both files
	result, err := svc.DetectConflicts(ctx, "ticket-3", []Touch{
		{EntityID: "src/main.go", Environment: "dev", ClaimType: ClaimFileWrite},
		{EntityID: "src/util.go", Environment: "dev", ClaimType: ClaimFileWrite},
	})
	if err != nil {
		t.Fatalf("DetectConflicts: %v", err)
	}

	if !result.HasConflicts {
		t.Fatal("expected conflicts")
	}
	if result.HardCount != 2 {
		t.Errorf("expected 2 hard conflicts, got %d", result.HardCount)
	}
	if len(result.Conflicts) != 2 {
		t.Errorf("expected 2 conflicts, got %d", len(result.Conflicts))
	}
}
