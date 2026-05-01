package workflow

import (
	"context"
	"testing"
)

type stubTicketUpdater struct {
	phases map[string]string // ticketID → workflowPhase
}

func (s *stubTicketUpdater) UpdateWorkflowPhase(_ context.Context, id string, phase string) error {
	s.phases[id] = phase
	return nil
}

func newTestEngine(defs ...*Definition) (*Engine, *stubTicketUpdater) {
	store := newInMemoryStore()
	for _, d := range defs {
		store.defs[d.ID] = d
	}
	updater := &stubTicketUpdater{phases: make(map[string]string)}
	return NewEngine(store, updater), updater
}

// inMemoryStore is a minimal in-memory Store for tests.
type inMemoryStore struct {
	defs        map[string]*Definition
	completions map[string][]PhaseCompletion
}

func newInMemoryStore() *inMemoryStore {
	return &inMemoryStore{
		defs:        make(map[string]*Definition),
		completions: make(map[string][]PhaseCompletion),
	}
}

func (s *inMemoryStore) GetByID(_ context.Context, id string) (*Definition, error) {
	d, ok := s.defs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return d, nil
}

func (s *inMemoryStore) GetByScope(_ context.Context, scope, scopeID string) (*Definition, error) {
	for _, d := range s.defs {
		if d.Scope == scope && d.ScopeID == scopeID && d.IsActive {
			return d, nil
		}
	}
	return nil, nil
}

func (s *inMemoryStore) ListCompletions(_ context.Context, ticketID string) ([]PhaseCompletion, error) {
	return s.completions[ticketID], nil
}

func (s *inMemoryStore) RecordCompletion(_ context.Context, c *PhaseCompletion) error {
	s.completions[c.TicketID] = append(s.completions[c.TicketID], *c)
	return nil
}

func TestResolve_ProjectOverridesOrg(t *testing.T) {
	orgDef := &Definition{ID: "org-wf", Scope: "org", ScopeID: "org-1", IsActive: true, Name: "Org Default"}
	projDef := &Definition{ID: "proj-wf", Scope: "project", ScopeID: "proj-1", IsActive: true, Name: "Project Override"}

	engine, _ := newTestEngine(orgDef, projDef)

	got, err := engine.Resolve(context.Background(), "org-1", "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != "proj-wf" {
		t.Fatalf("expected project override, got %+v", got)
	}
}

func TestResolve_FallsBackToOrg(t *testing.T) {
	orgDef := &Definition{ID: "org-wf", Scope: "org", ScopeID: "org-1", IsActive: true, Name: "Org Default"}
	engine, _ := newTestEngine(orgDef)

	got, err := engine.Resolve(context.Background(), "org-1", "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != "org-wf" {
		t.Fatalf("expected org fallback, got %+v", got)
	}
}

func TestResolve_FallsBackToSystem(t *testing.T) {
	sysDef := &Definition{ID: "sys-wf", Scope: "system", ScopeID: "", IsActive: true, Name: "System Default"}
	engine, _ := newTestEngine(sysDef)

	got, err := engine.Resolve(context.Background(), "org-1", "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != "sys-wf" {
		t.Fatalf("expected system fallback, got %+v", got)
	}
}

func TestResolve_NoActiveWorkflow(t *testing.T) {
	engine, _ := newTestEngine()

	got, err := engine.Resolve(context.Background(), "org-1", "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestAdvancePhase_HappyPath(t *testing.T) {
	def := &Definition{
		ID:   "wf-1",
		Name: "Test",
		Phases: []Phase{
			{ID: "execute", Name: "Execute", Type: PhaseAgent},
			{ID: "review", Name: "Review", Type: PhaseGate},
			{ID: "deploy", Name: "Deploy", Type: PhaseExternal},
		},
	}
	engine, updater := newTestEngine(def)

	// Advance from execute → review
	next, err := engine.AdvancePhase(context.Background(), "t-1", "wf-1", "execute", "success", nil)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "review" {
		t.Fatalf("expected review phase, got %+v", next)
	}
	if updater.phases["t-1"] != "review" {
		t.Fatalf("expected phase updated to review, got %s", updater.phases["t-1"])
	}

	// Advance from review → deploy
	next, err = engine.AdvancePhase(context.Background(), "t-1", "wf-1", "review", "success", nil)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "deploy" {
		t.Fatalf("expected deploy phase, got %+v", next)
	}

	// Advance from deploy → complete
	next, err = engine.AdvancePhase(context.Background(), "t-1", "wf-1", "deploy", "success", nil)
	if err != nil {
		t.Fatal(err)
	}
	if next != nil {
		t.Fatalf("expected nil (complete), got %+v", next)
	}
	if updater.phases["t-1"] != "" {
		t.Fatalf("expected phase cleared, got %s", updater.phases["t-1"])
	}
}

func TestAdvancePhase_OnFailureJump(t *testing.T) {
	def := &Definition{
		ID:   "wf-1",
		Name: "Test",
		Phases: []Phase{
			{ID: "execute", Name: "Execute", Type: PhaseAgent},
			{ID: "test", Name: "Test", Type: PhaseExternal, OnFailure: "execute"},
			{ID: "deploy", Name: "Deploy", Type: PhaseExternal},
		},
	}
	engine, updater := newTestEngine(def)

	// Fail the test phase → should jump back to execute
	next, err := engine.AdvancePhase(context.Background(), "t-1", "wf-1", "test", "failed", nil)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "execute" {
		t.Fatalf("expected jump to execute, got %+v", next)
	}
	if updater.phases["t-1"] != "execute" {
		t.Fatalf("expected phase updated to execute, got %s", updater.phases["t-1"])
	}
}

func TestGetPosition(t *testing.T) {
	def := &Definition{
		ID:   "wf-1",
		Name: "Standard",
		Phases: []Phase{
			{ID: "execute", Name: "Execute", Type: PhaseAgent},
			{ID: "review", Name: "Review", Type: PhaseGate},
		},
	}
	engine, _ := newTestEngine(def)

	pos, err := engine.GetPosition(context.Background(), "t-1", "wf-1", "review")
	if err != nil {
		t.Fatal(err)
	}
	if pos.PhaseIndex != 1 {
		t.Fatalf("expected phase index 1, got %d", pos.PhaseIndex)
	}
	if pos.TotalPhases != 2 {
		t.Fatalf("expected 2 total phases, got %d", pos.TotalPhases)
	}
	if pos.CurrentPhase == nil || pos.CurrentPhase.ID != "review" {
		t.Fatal("expected current phase to be review")
	}
}

func TestGetPosition_NoWorkflow(t *testing.T) {
	engine, _ := newTestEngine()

	pos, err := engine.GetPosition(context.Background(), "t-1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if pos != nil {
		t.Fatalf("expected nil, got %+v", pos)
	}
}

func TestTemplates(t *testing.T) {
	templates := BuiltinTemplates()
	if len(templates) != 3 {
		t.Fatalf("expected 3 templates, got %d", len(templates))
	}
	names := map[string]bool{}
	for _, tmpl := range templates {
		names[tmpl.Name] = true
		if len(tmpl.Phases) == 0 {
			t.Fatalf("template %s has no phases", tmpl.Name)
		}
	}
	for _, name := range []string{"Standard SDLC", "Fast Track", "Full Pipeline"} {
		if !names[name] {
			t.Fatalf("missing template: %s", name)
		}
	}
}

func TestValidPhaseType(t *testing.T) {
	for _, pt := range ValidPhaseTypes() {
		if !IsValidPhaseType(pt) {
			t.Fatalf("expected %s to be valid", pt)
		}
	}
	if IsValidPhaseType("invalid") {
		t.Fatal("expected 'invalid' to be invalid")
	}
}

func TestStandardSDLC_HasSixPhases(t *testing.T) {
	sdlc := StandardSDLC()
	if len(sdlc.Phases) != 6 {
		t.Fatalf("expected 6 phases, got %d", len(sdlc.Phases))
	}
	expectedIDs := []string{"decompose", "execute", "agentic-review", "quality-gate", "merge", "deploy-dev"}
	for i, id := range expectedIDs {
		if sdlc.Phases[i].ID != id {
			t.Fatalf("phase %d: expected %s, got %s", i, id, sdlc.Phases[i].ID)
		}
	}
	if sdlc.Phases[2].OnFailure != "execute" {
		t.Fatalf("agentic-review on_failure: expected 'execute', got %q", sdlc.Phases[2].OnFailure)
	}
}

func TestAdvancePhase_MaxIterationsExhausted(t *testing.T) {
	def := &Definition{
		ID:   "wf-1",
		Name: "Test",
		Phases: []Phase{
			{ID: "execute", Name: "Execute", Type: PhaseAgent},
			{ID: "review", Name: "Review", Type: PhaseAgent,
				Config:    map[string]any{"role": "validator", "max_iterations": 3},
				OnFailure: "execute",
			},
			{ID: "gate", Name: "Gate", Type: PhaseGate},
		},
	}
	engine, updater := newTestEngine(def)

	// First failure — count=1, under max=3 → jump to execute
	next, err := engine.AdvancePhase(context.Background(), "t-1", "wf-1", "review", "failed", nil)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "execute" {
		t.Fatalf("first failure: expected jump to execute, got %+v", next)
	}

	// Second failure — count=2, still under max=3 → jump to execute
	next, err = engine.AdvancePhase(context.Background(), "t-1", "wf-1", "review", "failed", nil)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "execute" {
		t.Fatalf("second failure: expected jump to execute, got %+v", next)
	}

	// Third failure — count=3 >= max=3, should auto-succeed to gate
	next, err = engine.AdvancePhase(context.Background(), "t-1", "wf-1", "review", "failed", nil)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "gate" {
		t.Fatalf("third failure: expected auto-advance to gate, got %+v", next)
	}
	if updater.phases["t-1"] != "gate" {
		t.Fatalf("expected phase updated to gate, got %s", updater.phases["t-1"])
	}
}

func TestAdvancePhase_OnFailureLoop(t *testing.T) {
	def := &Definition{
		ID:   "wf-1",
		Name: "Test",
		Phases: []Phase{
			{ID: "execute", Name: "Execute", Type: PhaseAgent},
			{ID: "review", Name: "Review", Type: PhaseAgent, OnFailure: "execute"},
			{ID: "deploy", Name: "Deploy", Type: PhaseExternal},
		},
	}
	engine, updater := newTestEngine(def)

	// Fail review → should jump to execute
	next, err := engine.AdvancePhase(context.Background(), "t-1", "wf-1", "review", "failed", nil)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "execute" {
		t.Fatalf("expected jump to execute, got %+v", next)
	}
	if updater.phases["t-1"] != "execute" {
		t.Fatalf("expected phase execute, got %s", updater.phases["t-1"])
	}

	// Success from execute → should advance to review
	next, err = engine.AdvancePhase(context.Background(), "t-1", "wf-1", "execute", "success", nil)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "review" {
		t.Fatalf("expected advance to review, got %+v", next)
	}
}

func TestParseAgentConfig_MaxIterations(t *testing.T) {
	cfg, err := ParseAgentConfig(map[string]any{
		"role":           "validator",
		"max_iterations": 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Role != "validator" {
		t.Fatalf("expected role validator, got %s", cfg.Role)
	}
	if cfg.MaxIterations != 5 {
		t.Fatalf("expected max_iterations 5, got %d", cfg.MaxIterations)
	}
}

func TestParseGateConfig_Requirements(t *testing.T) {
	cfg, err := ParseGateConfig(map[string]any{
		"prompt":       "Check CI",
		"requirements": []any{"github_checks"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Prompt != "Check CI" {
		t.Fatalf("expected prompt 'Check CI', got %s", cfg.Prompt)
	}
	if len(cfg.Requirements) != 1 || cfg.Requirements[0] != "github_checks" {
		t.Fatalf("expected requirements [github_checks], got %v", cfg.Requirements)
	}
}
