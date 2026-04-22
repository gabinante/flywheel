package plan

import (
	"testing"
)

func TestValidateGraph_Valid(t *testing.T) {
	g := &PlanGraph{
		ID:       "graph-1",
		TicketID: "ticket-1",
		Name:     "coordinated schema + code change",
		Steps: []PlanStep{
			{StepID: "migrate-db", PlanID: "plan-1", Backend: BackendDatabase, Name: "Schema migration", Ordinal: 1},
			{StepID: "write-code", PlanID: "plan-2", Backend: BackendCode, Name: "Code changes", DependsOn: []string{"migrate-db"}, Ordinal: 2},
			{StepID: "deploy", PlanID: "plan-3", Backend: BackendDeploy, Name: "Deploy", DependsOn: []string{"write-code"}, Ordinal: 3},
		},
		State: GraphStatePending,
	}

	if err := ValidateGraph(g); err != nil {
		t.Fatalf("expected valid graph, got: %v", err)
	}
}

func TestValidateGraph_Empty(t *testing.T) {
	g := &PlanGraph{ID: "graph-1", Steps: []PlanStep{}}
	if err := ValidateGraph(g); err == nil {
		t.Fatal("expected error for empty graph")
	}
}

func TestValidateGraph_DuplicateStepID(t *testing.T) {
	g := &PlanGraph{
		ID: "graph-1",
		Steps: []PlanStep{
			{StepID: "step-1", PlanID: "plan-1", Backend: BackendCode, Name: "First"},
			{StepID: "step-1", PlanID: "plan-2", Backend: BackendCode, Name: "Duplicate"},
		},
	}
	if err := ValidateGraph(g); err == nil {
		t.Fatal("expected error for duplicate step IDs")
	}
}

func TestValidateGraph_InvalidDependency(t *testing.T) {
	g := &PlanGraph{
		ID: "graph-1",
		Steps: []PlanStep{
			{StepID: "step-1", PlanID: "plan-1", Backend: BackendCode, Name: "First"},
			{StepID: "step-2", PlanID: "plan-2", Backend: BackendCode, Name: "Second", DependsOn: []string{"nonexistent"}},
		},
	}
	if err := ValidateGraph(g); err == nil {
		t.Fatal("expected error for invalid dependency reference")
	}
}

func TestValidateGraph_SelfDependency(t *testing.T) {
	g := &PlanGraph{
		ID: "graph-1",
		Steps: []PlanStep{
			{StepID: "step-1", PlanID: "plan-1", Backend: BackendCode, Name: "Self-dep", DependsOn: []string{"step-1"}},
		},
	}
	if err := ValidateGraph(g); err == nil {
		t.Fatal("expected error for self-dependency")
	}
}

func TestValidateGraph_Cycle(t *testing.T) {
	g := &PlanGraph{
		ID: "graph-1",
		Steps: []PlanStep{
			{StepID: "step-a", PlanID: "plan-1", Backend: BackendCode, Name: "A", DependsOn: []string{"step-c"}},
			{StepID: "step-b", PlanID: "plan-2", Backend: BackendCode, Name: "B", DependsOn: []string{"step-a"}},
			{StepID: "step-c", PlanID: "plan-3", Backend: BackendCode, Name: "C", DependsOn: []string{"step-b"}},
		},
	}
	if err := ValidateGraph(g); err == nil {
		t.Fatal("expected error for cyclic graph")
	}
}

func TestValidateGraph_InvalidBackend(t *testing.T) {
	g := &PlanGraph{
		ID: "graph-1",
		Steps: []PlanStep{
			{StepID: "step-1", PlanID: "plan-1", Backend: Backend("invalid"), Name: "Bad backend"},
		},
	}
	if err := ValidateGraph(g); err == nil {
		t.Fatal("expected error for invalid backend")
	}
}

func TestReadySteps_NoDeps(t *testing.T) {
	g := &PlanGraph{
		Steps: []PlanStep{
			{StepID: "step-1", PlanID: "plan-1", Backend: BackendCode, Name: "A", State: StepStatePending},
			{StepID: "step-2", PlanID: "plan-2", Backend: BackendCode, Name: "B", State: StepStatePending},
		},
	}
	ready := ReadySteps(g)
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready steps, got: %d", len(ready))
	}
}

func TestReadySteps_WithDeps(t *testing.T) {
	g := &PlanGraph{
		Steps: []PlanStep{
			{StepID: "step-1", PlanID: "plan-1", Backend: BackendDatabase, Name: "Migrate", State: StepStateCompleted},
			{StepID: "step-2", PlanID: "plan-2", Backend: BackendCode, Name: "Code", DependsOn: []string{"step-1"}, State: StepStatePending},
			{StepID: "step-3", PlanID: "plan-3", Backend: BackendDeploy, Name: "Deploy", DependsOn: []string{"step-2"}, State: StepStatePending},
		},
	}
	ready := ReadySteps(g)
	if len(ready) != 1 {
		t.Fatalf("expected 1 ready step, got: %d", len(ready))
	}
	if ready[0].StepID != "step-2" {
		t.Fatalf("expected step-2 to be ready, got: %s", ready[0].StepID)
	}
}

func TestReadySteps_UnmetDeps(t *testing.T) {
	g := &PlanGraph{
		Steps: []PlanStep{
			{StepID: "step-1", PlanID: "plan-1", Backend: BackendDatabase, Name: "Migrate", State: StepStatePending},
			{StepID: "step-2", PlanID: "plan-2", Backend: BackendCode, Name: "Code", DependsOn: []string{"step-1"}, State: StepStatePending},
		},
	}
	ready := ReadySteps(g)
	// Only step-1 is ready (no deps). step-2 depends on step-1 which is still pending.
	if len(ready) != 1 {
		t.Fatalf("expected 1 ready step, got: %d", len(ready))
	}
	if ready[0].StepID != "step-1" {
		t.Fatalf("expected step-1 to be ready, got: %s", ready[0].StepID)
	}
}

func TestTopologicalOrder(t *testing.T) {
	g := &PlanGraph{
		ID: "graph-1",
		Steps: []PlanStep{
			{StepID: "deploy", PlanID: "plan-3", Backend: BackendDeploy, Name: "Deploy", DependsOn: []string{"code"}},
			{StepID: "code", PlanID: "plan-2", Backend: BackendCode, Name: "Code", DependsOn: []string{"migrate"}},
			{StepID: "migrate", PlanID: "plan-1", Backend: BackendDatabase, Name: "Migrate"},
		},
	}

	order, err := TopologicalOrder(g)
	if err != nil {
		t.Fatalf("expected valid order, got: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("expected 3 steps, got: %d", len(order))
	}
	// migrate must come before code; code before deploy.
	idxMap := make(map[string]int)
	for i, step := range order {
		idxMap[step.StepID] = i
	}
	if idxMap["migrate"] >= idxMap["code"] {
		t.Fatalf("migrate must come before code")
	}
	if idxMap["code"] >= idxMap["deploy"] {
		t.Fatalf("code must come before deploy")
	}
}

func TestFailStep_Cascade(t *testing.T) {
	g := &PlanGraph{
		Steps: []PlanStep{
			{StepID: "step-1", PlanID: "plan-1", Backend: BackendDatabase, Name: "Migrate", State: StepStateCompleted},
			{StepID: "step-2", PlanID: "plan-2", Backend: BackendCode, Name: "Code", DependsOn: []string{"step-1"}, State: StepStateExecuting},
			{StepID: "step-3", PlanID: "plan-3", Backend: BackendDeploy, Name: "Deploy", DependsOn: []string{"step-2"}, State: StepStatePending},
			{StepID: "step-4", PlanID: "plan-4", Backend: BackendShell, Name: "Cleanup", DependsOn: []string{"step-3"}, State: StepStatePending},
		},
	}

	FailStep(g, "step-2")

	if g.Steps[1].State != StepStateFailed {
		t.Fatalf("expected step-2 to be failed, got: %s", g.Steps[1].State)
	}
	if g.Steps[2].State != StepStateSkipped {
		t.Fatalf("expected step-3 to be skipped (depends on failed step-2), got: %s", g.Steps[2].State)
	}
	if g.Steps[3].State != StepStateSkipped {
		t.Fatalf("expected step-4 to be skipped (transitively depends on failed step-2), got: %s", g.Steps[3].State)
	}
	if g.State != GraphStateFailed {
		t.Fatalf("expected graph state to be failed, got: %s", g.State)
	}
}

func TestIsComplete(t *testing.T) {
	tests := []struct {
		name     string
		steps    []PlanStep
		complete bool
	}{
		{
			"all completed",
			[]PlanStep{
				{StepID: "1", State: StepStateCompleted},
				{StepID: "2", State: StepStateCompleted},
			},
			true,
		},
		{
			"mixed terminal",
			[]PlanStep{
				{StepID: "1", State: StepStateCompleted},
				{StepID: "2", State: StepStateFailed},
				{StepID: "3", State: StepStateSkipped},
			},
			true,
		},
		{
			"still executing",
			[]PlanStep{
				{StepID: "1", State: StepStateCompleted},
				{StepID: "2", State: StepStateExecuting},
			},
			false,
		},
		{
			"still pending",
			[]PlanStep{
				{StepID: "1", State: StepStatePending},
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &PlanGraph{Steps: tt.steps}
			if got := IsComplete(g); got != tt.complete {
				t.Errorf("IsComplete() = %v, want %v", got, tt.complete)
			}
		})
	}
}

func TestFanOutGraph(t *testing.T) {
	// Test fan-out pattern: A → {B, C, D}
	g := &PlanGraph{
		ID: "graph-1",
		Steps: []PlanStep{
			{StepID: "setup", PlanID: "p-1", Backend: BackendShell, Name: "Setup"},
			{StepID: "feat-a", PlanID: "p-2", Backend: BackendCode, Name: "Feature A", DependsOn: []string{"setup"}},
			{StepID: "feat-b", PlanID: "p-3", Backend: BackendCode, Name: "Feature B", DependsOn: []string{"setup"}},
			{StepID: "feat-c", PlanID: "p-4", Backend: BackendCode, Name: "Feature C", DependsOn: []string{"setup"}},
		},
	}

	if err := ValidateGraph(g); err != nil {
		t.Fatalf("expected valid fan-out graph, got: %v", err)
	}

	// Initialize states.
	for i := range g.Steps {
		g.Steps[i].State = StepStatePending
	}

	// After setup completes, all three features should be ready.
	g.Steps[0].State = StepStateCompleted
	ready := ReadySteps(g)
	if len(ready) != 3 {
		t.Fatalf("expected 3 ready steps after fan-out, got: %d", len(ready))
	}
}

func TestDiamondGraph(t *testing.T) {
	// Test diamond pattern: A → {B, C} → D
	g := &PlanGraph{
		ID: "graph-1",
		Steps: []PlanStep{
			{StepID: "base", PlanID: "p-1", Backend: BackendDatabase, Name: "Base"},
			{StepID: "left", PlanID: "p-2", Backend: BackendCode, Name: "Left", DependsOn: []string{"base"}},
			{StepID: "right", PlanID: "p-3", Backend: BackendCode, Name: "Right", DependsOn: []string{"base"}},
			{StepID: "merge", PlanID: "p-4", Backend: BackendDeploy, Name: "Merge", DependsOn: []string{"left", "right"}},
		},
	}

	if err := ValidateGraph(g); err != nil {
		t.Fatalf("expected valid diamond graph, got: %v", err)
	}

	// Initialize states.
	for i := range g.Steps {
		g.Steps[i].State = StepStatePending
	}

	// After only left completes, merge is not ready.
	g.Steps[0].State = StepStateCompleted
	g.Steps[1].State = StepStateCompleted
	ready := ReadySteps(g)
	// Only "right" should be ready (base completed, but merge needs both left AND right).
	if len(ready) != 1 || ready[0].StepID != "right" {
		t.Fatalf("expected only 'right' to be ready, got: %v", ready)
	}

	// After both complete, merge is ready.
	g.Steps[2].State = StepStateCompleted
	ready = ReadySteps(g)
	if len(ready) != 1 || ready[0].StepID != "merge" {
		t.Fatalf("expected 'merge' to be ready, got: %v", ready)
	}
}
