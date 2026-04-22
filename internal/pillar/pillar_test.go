package pillar

import (
	"context"
	"testing"
	"time"
)

// ── In-memory store for testing ──────────────────────────────────────────────

type memStore struct {
	entries     map[string]*Entry
	claims      map[string]*Claim
	evaluations map[string][]*Evaluation
}

func newMemStore() *memStore {
	return &memStore{
		entries:     make(map[string]*Entry),
		claims:      make(map[string]*Claim),
		evaluations: make(map[string][]*Evaluation),
	}
}

func (s *memStore) CreateEntry(_ context.Context, e *Entry) error {
	s.entries[e.ID] = e
	return nil
}
func (s *memStore) GetEntry(_ context.Context, id string) (*Entry, error) {
	e, ok := s.entries[id]
	if !ok {
		return nil, ErrEntryNotFound
	}
	return e, nil
}
func (s *memStore) ListEntries(_ context.Context, projectID, entityID, pillarType string, limit int) ([]*Entry, error) {
	var result []*Entry
	for _, e := range s.entries {
		if e.ProjectID != projectID {
			continue
		}
		if entityID != "" && e.EntityID != entityID {
			continue
		}
		if pillarType != "" && string(e.PillarType) != pillarType {
			continue
		}
		result = append(result, e)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}
func (s *memStore) UpdateEntry(_ context.Context, e *Entry) error {
	s.entries[e.ID] = e
	return nil
}
func (s *memStore) DeleteEntry(_ context.Context, id string) error {
	delete(s.entries, id)
	// Also delete claims and evaluations
	for cid, c := range s.claims {
		if c.PillarEntryID == id {
			delete(s.claims, cid)
		}
	}
	delete(s.evaluations, id)
	return nil
}
func (s *memStore) CreateClaim(_ context.Context, c *Claim) error {
	s.claims[c.ID] = c
	return nil
}
func (s *memStore) GetClaim(_ context.Context, id string) (*Claim, error) {
	c, ok := s.claims[id]
	if !ok {
		return nil, ErrClaimNotFound
	}
	return c, nil
}
func (s *memStore) ListClaims(_ context.Context, pillarEntryID string) ([]*Claim, error) {
	var result []*Claim
	for _, c := range s.claims {
		if c.PillarEntryID == pillarEntryID {
			result = append(result, c)
		}
	}
	return result, nil
}
func (s *memStore) UpdateClaim(_ context.Context, c *Claim) error {
	s.claims[c.ID] = c
	return nil
}
func (s *memStore) DeleteClaim(_ context.Context, id string) error {
	delete(s.claims, id)
	return nil
}
func (s *memStore) CreateEvaluation(_ context.Context, ev *Evaluation) error {
	s.evaluations[ev.PillarEntryID] = append(s.evaluations[ev.PillarEntryID], ev)
	return nil
}
func (s *memStore) ListEvaluations(_ context.Context, pillarEntryID string, limit int) ([]*Evaluation, error) {
	evals := s.evaluations[pillarEntryID]
	if len(evals) > limit {
		evals = evals[:limit]
	}
	return evals, nil
}
func (s *memStore) ListDueForReview(_ context.Context, projectID string, before time.Time, limit int) ([]*Entry, error) {
	var result []*Entry
	for _, e := range s.entries {
		if e.ProjectID != projectID {
			continue
		}
		if e.NextReviewAt != nil && !e.NextReviewAt.After(before) {
			result = append(result, e)
		}
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestAllPillarTypes(t *testing.T) {
	types := AllPillarTypes()
	if len(types) != 7 {
		t.Fatalf("expected 7 pillar types, got %d", len(types))
	}
	expected := []PillarType{
		PillarObservability, PillarMutability, PillarScalability,
		PillarAvailability, PillarSecurity, PillarResiliency, PillarCost,
	}
	for i, pt := range expected {
		if types[i] != pt {
			t.Errorf("type[%d] = %s, want %s", i, types[i], pt)
		}
	}
}

func TestIsValidPillarType(t *testing.T) {
	for _, pt := range AllPillarTypes() {
		if !IsValidPillarType(string(pt)) {
			t.Errorf("expected %s to be valid", pt)
		}
	}
	if IsValidPillarType("notapillar") {
		t.Error("expected 'notapillar' to be invalid")
	}
}

func TestDefaultTemplates(t *testing.T) {
	templates := DefaultTemplates()
	if len(templates) != 7 {
		t.Fatalf("expected 7 templates, got %d", len(templates))
	}
	for _, tmpl := range templates {
		if tmpl.Description == "" {
			t.Errorf("template %s has empty description", tmpl.PillarType)
		}
		if tmpl.StrategyPrompt == "" {
			t.Errorf("template %s has empty strategy prompt", tmpl.PillarType)
		}
		if len(tmpl.ExampleClaims) == 0 {
			t.Errorf("template %s has no example claims", tmpl.PillarType)
		}
		if len(tmpl.CommonGaps) == 0 {
			t.Errorf("template %s has no common gaps", tmpl.PillarType)
		}
		if tmpl.DefaultCadence == "" {
			t.Errorf("template %s has empty default cadence", tmpl.PillarType)
		}
	}
}

func TestTemplateByType(t *testing.T) {
	tmpl := TemplateByType(PillarSecurity)
	if tmpl == nil {
		t.Fatal("expected security template, got nil")
	}
	if tmpl.PillarType != PillarSecurity {
		t.Errorf("got type %s, want security", tmpl.PillarType)
	}
	if TemplateByType("nonexistent") != nil {
		t.Error("expected nil for nonexistent type")
	}
}

func TestCreateEntry(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	entry, err := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "observability",
		"Full observability through logs, metrics, and traces", "agent-1",
		[]Gap{{Description: "No tracing", Severity: "high"}}, "monthly")
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if entry.ID == "" {
		t.Error("expected non-empty ID")
	}
	if entry.PillarType != PillarObservability {
		t.Errorf("got type %s, want observability", entry.PillarType)
	}
	if entry.Strategy != "Full observability through logs, metrics, and traces" {
		t.Errorf("wrong strategy: %s", entry.Strategy)
	}
	if len(entry.Gaps) != 1 {
		t.Fatalf("expected 1 gap, got %d", len(entry.Gaps))
	}
	if entry.Version != 1 {
		t.Errorf("expected version 1, got %d", entry.Version)
	}
	if entry.NextReviewAt == nil {
		t.Error("expected next_review_at to be set")
	}
}

func TestCreateEntry_InvalidPillarType(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	_, err := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "invalid",
		"test", "agent-1", nil, "monthly")
	if err != ErrInvalidPillarType {
		t.Fatalf("expected ErrInvalidPillarType, got %v", err)
	}
}

func TestCreateEntry_DefaultCadence(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	entry, err := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "scalability",
		"Scales horizontally", "agent-1", nil, "")
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	// Scalability template defaults to quarterly
	if entry.ReviewCadence != CadenceQuarterly {
		t.Errorf("expected quarterly cadence (from template), got %s", entry.ReviewCadence)
	}
}

func TestUpdateEntry(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	entry, _ := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "security",
		"Secure by default", "agent-1", nil, "monthly")

	updated, err := svc.UpdateEntry(ctx, entry.ID, "Defense in depth",
		[]Gap{{Description: "No pen testing", Severity: "medium"}}, "quarterly")
	if err != nil {
		t.Fatalf("UpdateEntry: %v", err)
	}
	if updated.Strategy != "Defense in depth" {
		t.Errorf("strategy not updated: %s", updated.Strategy)
	}
	if updated.ReviewCadence != CadenceQuarterly {
		t.Errorf("cadence not updated: %s", updated.ReviewCadence)
	}
	if updated.Version != 2 {
		t.Errorf("expected version 2, got %d", updated.Version)
	}
}

func TestDeleteEntry(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	entry, _ := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "cost",
		"Minimize waste", "agent-1", nil, "quarterly")

	if err := svc.DeleteEntry(ctx, entry.ID); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}
	_, err := svc.GetEntry(ctx, entry.ID)
	if err != ErrEntryNotFound {
		t.Fatalf("expected ErrEntryNotFound, got %v", err)
	}
}

func TestCreateClaim(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	entry, _ := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "observability",
		"Observable", "agent-1", nil, "monthly")

	claim, err := svc.CreateClaim(ctx, entry.ID,
		"Structured logging enabled", "entity-2", "infrastructure",
		"Fluentd sidecar deployed to all pods")
	if err != nil {
		t.Fatalf("CreateClaim: %v", err)
	}
	if claim.Status != ClaimUnverified {
		t.Errorf("expected unverified status, got %s", claim.Status)
	}
	if claim.EntityRefID != "entity-2" {
		t.Errorf("wrong entity ref: %s", claim.EntityRefID)
	}
}

func TestCreateClaim_EntryNotFound(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	_, err := svc.CreateClaim(ctx, "nonexistent", "claim", "ref-1", "service", "evidence")
	if err != ErrEntryNotFound {
		t.Fatalf("expected ErrEntryNotFound, got %v", err)
	}
}

func TestUpdateClaimStatus(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	entry, _ := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "security",
		"Secure", "agent-1", nil, "monthly")
	claim, _ := svc.CreateClaim(ctx, entry.ID, "TLS enabled", "entity-3", "infrastructure", "cert verified")

	updated, err := svc.UpdateClaimStatus(ctx, claim.ID, "verified", "Confirmed via probe")
	if err != nil {
		t.Fatalf("UpdateClaimStatus: %v", err)
	}
	if updated.Status != ClaimVerified {
		t.Errorf("expected verified, got %s", updated.Status)
	}
	if updated.Evidence != "Confirmed via probe" {
		t.Errorf("evidence not updated: %s", updated.Evidence)
	}
	if updated.LastEvaluatedAt == nil {
		t.Error("expected last_evaluated_at to be set")
	}
}

func TestUpdateClaimStatus_Invalid(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	entry, _ := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "security",
		"Secure", "agent-1", nil, "monthly")
	claim, _ := svc.CreateClaim(ctx, entry.ID, "TLS", "ref-1", "service", "")

	_, err := svc.UpdateClaimStatus(ctx, claim.ID, "bogus", "")
	if err != ErrInvalidClaimStatus {
		t.Fatalf("expected ErrInvalidClaimStatus, got %v", err)
	}
}

func TestEvaluateEntry_NoClaims(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	entry, _ := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "availability",
		"Always up", "agent-1", nil, "monthly")

	evals, err := svc.EvaluateEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("EvaluateEntry: %v", err)
	}
	if len(evals) != 3 {
		t.Fatalf("expected 3 evaluation results, got %d", len(evals))
	}
	// With no claims, evidence_intact should warn
	if evals[0].Outcome != EvalWarn {
		t.Errorf("evidence_intact should warn with no claims, got %s", evals[0].Outcome)
	}
}

func TestEvaluateEntry_AllVerified(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	entry, _ := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "resiliency",
		"Resilient design", "agent-1", nil, "monthly")

	claim, _ := svc.CreateClaim(ctx, entry.ID,
		"Circuit breaker enabled", "entity-2", "service",
		"Hystrix configured")
	svc.UpdateClaimStatus(ctx, claim.ID, "verified", "Tested in staging")

	evals, err := svc.EvaluateEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("EvaluateEntry: %v", err)
	}
	// All checks should pass
	for _, ev := range evals {
		if ev.Outcome != EvalPass {
			t.Errorf("check %s should pass, got %s: %s", ev.CheckType, ev.Outcome, ev.Details)
		}
	}
}

func TestEvaluateEntry_CriticalGap(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	entry, _ := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "security",
		"Basic security", "agent-1",
		[]Gap{{Description: "No encryption at rest", Severity: "critical"}}, "monthly")

	svc.CreateClaim(ctx, entry.ID, "Auth enabled", "ref-1", "service", "JWT verified")

	evals, err := svc.EvaluateEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("EvaluateEntry: %v", err)
	}
	// Strategy check should fail due to critical unmitigated gap
	strategyEval := evals[2] // strategy_appropriate is the 3rd check
	if strategyEval.Outcome != EvalFail {
		t.Errorf("strategy_appropriate should fail with critical gap, got %s: %s",
			strategyEval.Outcome, strategyEval.Details)
	}
}

func TestMarkReviewed(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	entry, _ := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "observability",
		"Observable", "agent-1", nil, "monthly")

	reviewed, err := svc.MarkReviewed(ctx, entry.ID)
	if err != nil {
		t.Fatalf("MarkReviewed: %v", err)
	}
	if reviewed.LastReviewedAt == nil {
		t.Error("expected last_reviewed_at to be set")
	}
	if reviewed.NextReviewAt == nil {
		t.Error("expected next_review_at to be set")
	}
	// next should be ~1 month from now (monthly cadence)
	if reviewed.NextReviewAt.Before(time.Now().AddDate(0, 0, 25)) {
		t.Error("next_review_at should be at least 25 days from now for monthly cadence")
	}
}

func TestListDueForReview(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	ctx := context.Background()

	// Create an entry with next_review_at in the past
	entry, _ := svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "cost",
		"Cost aware", "agent-1", nil, "weekly")
	// Manually set next_review_at to past
	past := time.Now().AddDate(0, 0, -1)
	entry.NextReviewAt = &past
	store.entries[entry.ID] = entry

	due, err := svc.ListDueForReview(ctx, "proj-1", time.Now(), 50)
	if err != nil {
		t.Fatalf("ListDueForReview: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("expected 1 due entry, got %d", len(due))
	}
}

func TestGetTemplates(t *testing.T) {
	svc := NewService(newMemStore())
	templates := svc.GetTemplates()
	if len(templates) != 7 {
		t.Fatalf("expected 7 templates, got %d", len(templates))
	}
}

func TestGetTemplate(t *testing.T) {
	svc := NewService(newMemStore())
	tmpl, err := svc.GetTemplate("cost")
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if tmpl.PillarType != PillarCost {
		t.Errorf("expected cost, got %s", tmpl.PillarType)
	}

	_, err = svc.GetTemplate("invalid")
	if err != ErrInvalidPillarType {
		t.Fatalf("expected ErrInvalidPillarType, got %v", err)
	}
}

func TestListEntries_Filters(t *testing.T) {
	svc := NewService(newMemStore())
	ctx := context.Background()

	svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "security", "s1", "a", nil, "monthly")
	svc.CreateEntry(ctx, "proj-1", "entity-1", "service", "cost", "s2", "a", nil, "monthly")
	svc.CreateEntry(ctx, "proj-1", "entity-2", "datastore", "security", "s3", "a", nil, "monthly")

	// Filter by entity_id
	entries, _ := svc.ListEntries(ctx, "proj-1", "entity-1", "", 50)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries for entity-1, got %d", len(entries))
	}

	// Filter by pillar_type
	entries, _ = svc.ListEntries(ctx, "proj-1", "", "security", 50)
	if len(entries) != 2 {
		t.Errorf("expected 2 security entries, got %d", len(entries))
	}

	// Filter by both
	entries, _ = svc.ListEntries(ctx, "proj-1", "entity-1", "security", 50)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry for entity-1+security, got %d", len(entries))
	}
}

func TestNextReviewTime(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	weekly := nextReviewTime(now, CadenceWeekly)
	if !weekly.Equal(now.AddDate(0, 0, 7)) {
		t.Errorf("weekly: got %v", weekly)
	}

	biweekly := nextReviewTime(now, CadenceBiweekly)
	if !biweekly.Equal(now.AddDate(0, 0, 14)) {
		t.Errorf("biweekly: got %v", biweekly)
	}

	monthly := nextReviewTime(now, CadenceMonthly)
	if !monthly.Equal(now.AddDate(0, 1, 0)) {
		t.Errorf("monthly: got %v", monthly)
	}

	quarterly := nextReviewTime(now, CadenceQuarterly)
	if !quarterly.Equal(now.AddDate(0, 3, 0)) {
		t.Errorf("quarterly: got %v", quarterly)
	}
}
