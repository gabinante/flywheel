package dispatch

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
)

func TestWorkerTypeIsValid(t *testing.T) {
	tests := []struct {
		wt   WorkerType
		want bool
	}{
		{WorkerTypePlanner, true},
		{WorkerTypeExecutor, true},
		{WorkerTypeValidator, true},
		{WorkerTypeDeployer, true},
		{WorkerTypeInvestigator, true},
		{WorkerTypeOperator, true},
		{WorkerTypeDecomposer, true},
		{WorkerType("unknown"), false},
		{WorkerType(""), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.wt), func(t *testing.T) {
			if got := tt.wt.IsValid(); got != tt.want {
				t.Errorf("WorkerType(%q).IsValid() = %v, want %v", tt.wt, got, tt.want)
			}
		})
	}
}

func TestWorkerTypeString(t *testing.T) {
	if WorkerTypeExecutor.String() != "executor" {
		t.Errorf("expected 'executor', got %q", WorkerTypeExecutor.String())
	}
}

func TestAllWorkerTypes(t *testing.T) {
	types := AllWorkerTypes()
	if len(types) != 7 {
		t.Errorf("expected 7 worker types, got %d", len(types))
	}
	for _, wt := range types {
		if !wt.IsValid() {
			t.Errorf("AllWorkerTypes() contains invalid type: %q", wt)
		}
	}
}

func TestGetToolAccess(t *testing.T) {
	// All defined types should have tool access definitions.
	for _, wt := range AllWorkerTypes() {
		ta := GetToolAccess(wt)
		if len(ta.AllowedTools) == 0 && len(ta.DeniedTools) == 0 {
			t.Errorf("WorkerType %q has no tool access definition", wt)
		}
	}

	// Unknown type should return empty (all tools allowed).
	ta := GetToolAccess(WorkerType("unknown"))
	if len(ta.AllowedTools) != 0 || len(ta.DeniedTools) != 0 {
		t.Error("unknown worker type should have empty tool access")
	}
}

func TestIsToolAllowed(t *testing.T) {
	tests := []struct {
		wt   WorkerType
		tool string
		want bool
	}{
		// Executor can claim and submit.
		{WorkerTypeExecutor, "claim_ticket", true},
		{WorkerTypeExecutor, "submit_ticket", true},
		{WorkerTypeExecutor, "log_step", true},
		// Executor cannot approve.
		{WorkerTypeExecutor, "approve_ticket", false},
		{WorkerTypeExecutor, "reject_ticket", false},
		// Validator can approve/reject.
		{WorkerTypeValidator, "approve_ticket", true},
		{WorkerTypeValidator, "reject_ticket", true},
		// Validator cannot claim or submit.
		{WorkerTypeValidator, "claim_ticket", false},
		{WorkerTypeValidator, "submit_ticket", false},
		// Investigator is read-only.
		{WorkerTypeInvestigator, "get_ticket", true},
		{WorkerTypeInvestigator, "log_step", true},
		{WorkerTypeInvestigator, "claim_ticket", false},
		{WorkerTypeInvestigator, "submit_ticket", false},
		// Planner can claim and submit but not approve.
		{WorkerTypePlanner, "claim_ticket", true},
		{WorkerTypePlanner, "submit_ticket", true},
		{WorkerTypePlanner, "approve_ticket", false},
		// Deployer can claim and submit.
		{WorkerTypeDeployer, "claim_ticket", true},
		{WorkerTypeDeployer, "submit_ticket", true},
		{WorkerTypeDeployer, "approve_ticket", false},
		// Operator can claim and submit but not approve.
		{WorkerTypeOperator, "claim_ticket", true},
		{WorkerTypeOperator, "submit_ticket", true},
		{WorkerTypeOperator, "log_step", true},
		{WorkerTypeOperator, "approve_ticket", false},
		{WorkerTypeOperator, "reject_ticket", false},
		// Decomposer can claim, submit, and create_ticket.
		{WorkerTypeDecomposer, "claim_ticket", true},
		{WorkerTypeDecomposer, "submit_ticket", true},
		{WorkerTypeDecomposer, "log_step", true},
		{WorkerTypeDecomposer, "create_ticket", true},
		{WorkerTypeDecomposer, "list_tickets", true},
		{WorkerTypeDecomposer, "approve_ticket", false},
		{WorkerTypeDecomposer, "reject_ticket", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.wt)+"/"+tt.tool, func(t *testing.T) {
			if got := IsToolAllowed(tt.wt, tt.tool); got != tt.want {
				t.Errorf("IsToolAllowed(%q, %q) = %v, want %v", tt.wt, tt.tool, got, tt.want)
			}
		})
	}
}

func TestDetermineWorkerType(t *testing.T) {
	tests := []struct {
		name string
		t    *ticket.Ticket
		want WorkerType
	}{
		{
			name: "default pending ticket is executor",
			t:    &ticket.Ticket{State: ticket.StateDraft},
			want: WorkerTypeExecutor,
		},
		{
			name: "awaiting_validation is validator",
			t:    &ticket.Ticket{State: ticket.StateAwaitingValidation},
			want: WorkerTypeValidator,
		},
		{
			name: "explicit worker_type in inputs overrides",
			t: &ticket.Ticket{
				State:  ticket.StateDraft,
				Inputs: map[string]any{"worker_type": "planner"},
			},
			want: WorkerTypePlanner,
		},
		{
			name: "invalid explicit worker_type falls back",
			t: &ticket.Ticket{
				State:  ticket.StateDraft,
				Inputs: map[string]any{"worker_type": "invalid"},
			},
			want: WorkerTypeExecutor,
		},
		{
			name: "deployer from inputs",
			t: &ticket.Ticket{
				State:  ticket.StateDraft,
				Inputs: map[string]any{"worker_type": "deployer"},
			},
			want: WorkerTypeDeployer,
		},
		{
			name: "investigator from inputs",
			t: &ticket.Ticket{
				State:  ticket.StateDraft,
				Inputs: map[string]any{"worker_type": "investigator"},
			},
			want: WorkerTypeInvestigator,
		},
		{
			name: "operator from inputs",
			t: &ticket.Ticket{
				State:  ticket.StateDraft,
				Inputs: map[string]any{"worker_type": "operator"},
			},
			want: WorkerTypeOperator,
		},
		{
			name: "decomposer from inputs",
			t: &ticket.Ticket{
				State:  ticket.StateDraft,
				Inputs: map[string]any{"worker_type": "decomposer"},
			},
			want: WorkerTypeDecomposer,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineWorkerType(tt.t)
			if got != tt.want {
				t.Errorf("DetermineWorkerType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTicketWorkerRoleCustomRole(t *testing.T) {
	proj := &project.Project{
		ID: "p-1",
		DispatchConfig: project.DispatchConfig{
			Roles: []project.DispatchWorkerRole{
				{
					ID:          "security_review",
					Name:        "Security Review",
					Description: "Focus on auth boundaries.",
					BaseType:    "validator",
				},
			},
		},
	}
	tk := &ticket.Ticket{
		State:  ticket.StateDraft,
		Inputs: map[string]any{"worker_role": "security-review"},
	}

	role, wt := resolveTicketWorkerRole(proj, tk)
	if role != "security_review" {
		t.Fatalf("role = %q, want security_review", role)
	}
	if wt != WorkerTypeValidator {
		t.Fatalf("worker type = %q, want validator", wt)
	}
}

func TestResolveTicketWorkerRoleNoRepo(t *testing.T) {
	// Project with no repo should route default executor to operator.
	proj := &project.Project{
		ID:      "p-norepo",
		RepoURL: "",
	}
	tk := &ticket.Ticket{
		State: ticket.StateDraft,
	}

	role, wt := resolveTicketWorkerRole(proj, tk)
	if wt != WorkerTypeOperator {
		t.Fatalf("worker type = %q, want operator (no repo)", wt)
	}
	if role != "operator" {
		t.Fatalf("role = %q, want operator", role)
	}

	// Explicit worker_type=executor should still work even without repo.
	tk2 := &ticket.Ticket{
		State:  ticket.StateDraft,
		Inputs: map[string]any{"worker_type": "executor"},
	}
	_, wt2 := resolveTicketWorkerRole(proj, tk2)
	if wt2 != WorkerTypeExecutor {
		t.Fatalf("explicit executor should not be overridden, got %q", wt2)
	}
}

func TestOperatorPromptNoGitReferences(t *testing.T) {
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-1",
		Title:     "Triage alert",
		Type:      ticket.TypeTask,
		Priority:  1,
		Objective: ticket.Objective{Description: "Investigate the alert"},
	}

	prompt := AssembleTypedWorkerPrompt(WorkerTypeOperator, proj, tk, nil, "http://localhost", "a-1", nil)
	if !strings.Contains(prompt, "operator") {
		t.Error("operator prompt should identify as operator")
	}
	for _, forbidden := range []string{"branch", "commit", "pull request", "git push"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Errorf("operator prompt should not contain %q", forbidden)
		}
	}
}

func TestAssembleTypedWorkerPrompt(t *testing.T) {
	proj := &project.Project{
		ID:   "p-1",
		Name: "test-project",
		ContextPack: project.ContextPack{
			SystemPrompt: "Be helpful.",
		},
	}
	tk := &ticket.Ticket{
		ID:        "t-1",
		Title:     "Test ticket",
		Type:      ticket.TypeTask,
		Priority:  2,
		Objective: ticket.Objective{Description: "Do stuff", SuccessCriteria: []string{"It works"}},
		Context:   ticket.TicketContext{RelevantFiles: []string{"main.go"}},
	}
	depOutputs := map[string]map[string]any{
		"dep-1": {"summary": "done"},
	}

	for _, wt := range AllWorkerTypes() {
		t.Run(string(wt), func(t *testing.T) {
			prompt := AssembleTypedWorkerPrompt(wt, proj, tk, depOutputs, "http://localhost:8080", "agent-1", nil)
			if prompt == "" {
				t.Fatal("expected non-empty prompt")
			}
			// Should contain the worker type.
			if !strings.Contains(prompt, string(wt)) {
				t.Errorf("prompt should contain worker type %q", wt)
			}
			// Should contain ticket ID.
			if !strings.Contains(prompt, "t-1") {
				t.Error("prompt should contain ticket ID")
			}
			// Should contain project system prompt.
			if !strings.Contains(prompt, "Be helpful") {
				t.Error("prompt should contain project system prompt")
			}
			// Should contain dependency outputs.
			if !strings.Contains(prompt, "dep-1") {
				t.Error("prompt should contain dependency outputs")
			}
		})
	}
}

func TestAssembleTypedWorkerPrompt_PlannerDoesNotCode(t *testing.T) {
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-1",
		Title:     "Plan this",
		Type:      ticket.TypeTask,
		Priority:  1,
		Objective: ticket.Objective{Description: "Plan it"},
	}

	prompt := AssembleTypedWorkerPrompt(WorkerTypePlanner, proj, tk, nil, "http://localhost", "a-1", nil)
	if !strings.Contains(prompt, "do NOT write code") {
		t.Error("planner prompt should instruct not to write code")
	}
	if !strings.Contains(prompt, "planner") {
		t.Error("planner prompt should identify as planner")
	}
}

func TestAssembleTypedWorkerPrompt_ValidatorApproves(t *testing.T) {
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-1",
		Title:     "Review this",
		Type:      ticket.TypeTask,
		Priority:  1,
		Objective: ticket.Objective{Description: "Review it"},
	}

	prompt := AssembleTypedWorkerPrompt(WorkerTypeValidator, proj, tk, nil, "http://localhost", "a-1", nil)
	if !strings.Contains(prompt, "approve") {
		t.Error("validator prompt should mention approve")
	}
	if !strings.Contains(prompt, "reject") {
		t.Error("validator prompt should mention reject")
	}
}

func TestAssembleTypedWorkerPrompt_PhaseOverrides(t *testing.T) {
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-1",
		Title:     "Test with overrides",
		Type:      ticket.TypeTask,
		Priority:  1,
		Objective: ticket.Objective{Description: "Do the thing"},
	}

	overrides := &PhaseOverrides{
		Goal:   "Focus on the database layer",
		Prompt: "Always run migrations before testing",
	}
	prompt := AssembleTypedWorkerPrompt(WorkerTypeExecutor, proj, tk, nil, "http://localhost", "a-1", overrides)

	if !strings.Contains(prompt, "## Phase objective") {
		t.Error("prompt should contain phase objective heading")
	}
	if !strings.Contains(prompt, "Focus on the database layer") {
		t.Error("prompt should contain phase goal text")
	}
	if !strings.Contains(prompt, "## Phase instructions") {
		t.Error("prompt should contain phase instructions heading")
	}
	if !strings.Contains(prompt, "Always run migrations before testing") {
		t.Error("prompt should contain phase prompt text")
	}

	// Nil overrides should not include phase sections.
	promptNil := AssembleTypedWorkerPrompt(WorkerTypeExecutor, proj, tk, nil, "http://localhost", "a-1", nil)
	if strings.Contains(promptNil, "## Phase objective") {
		t.Error("nil overrides should not include phase objective")
	}

	// Empty overrides should not include phase sections.
	promptEmpty := AssembleTypedWorkerPrompt(WorkerTypeExecutor, proj, tk, nil, "http://localhost", "a-1", &PhaseOverrides{})
	if strings.Contains(promptEmpty, "## Phase objective") {
		t.Error("empty overrides should not include phase objective")
	}
}

func TestBuildTypedTaskPrompt(t *testing.T) {
	tests := []struct {
		wt       WorkerType
		contains string
	}{
		{WorkerTypePlanner, "Plan"},
		{WorkerTypeExecutor, "Execute"},
		{WorkerTypeValidator, "Review"},
		{WorkerTypeDeployer, "Deploy"},
		{WorkerTypeInvestigator, "Investigate"},
		{WorkerTypeOperator, "Operate"},
		{WorkerTypeDecomposer, "Decompose"},
		{WorkerType(""), "Execute"}, // default
	}
	for _, tt := range tests {
		t.Run(string(tt.wt), func(t *testing.T) {
			msg := buildTypedTaskPrompt(tt.wt, "t-1", "p-1")
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("buildTypedTaskPrompt(%q) should contain %q, got: %s", tt.wt, tt.contains, msg)
			}
		})
	}
}

func TestAssembleTypedWorkerPrompt_DecomposerCreatesTickets(t *testing.T) {
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-1",
		Title:     "Big feature",
		Type:      ticket.TypeTask,
		Priority:  1,
		Objective: ticket.Objective{Description: "Implement a large feature"},
	}

	prompt := AssembleTypedWorkerPrompt(WorkerTypeDecomposer, proj, tk, nil, "http://localhost", "a-1", nil)
	if !strings.Contains(prompt, "decomposer") {
		t.Error("decomposer prompt should identify as decomposer")
	}
	if !strings.Contains(prompt, "create_ticket") {
		t.Error("decomposer prompt should mention create_ticket")
	}
	if !strings.Contains(prompt, "subticket") {
		t.Error("decomposer prompt should mention subtickets")
	}
	// Should not contain git/code references.
	for _, forbidden := range []string{"commit", "pull request", "git push"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Errorf("decomposer prompt should not contain %q", forbidden)
		}
	}
}

func TestDecomposerInputHint(t *testing.T) {
	tk := &ticket.Ticket{
		State:  ticket.StateDraft,
		Inputs: map[string]any{"worker_type": "decomposer"},
	}
	got := DetermineWorkerType(tk)
	if got != WorkerTypeDecomposer {
		t.Errorf("DetermineWorkerType() = %q, want decomposer", got)
	}
}

func TestBuildMCPConfigForType(t *testing.T) {
	cfg := buildMCPConfigForType(WorkerTypeExecutor, "http://localhost:8080", "key-123")
	srv, ok := cfg.MCPServers["flywheel"]
	if !ok {
		t.Fatal("expected flywheel server in config")
	}
	if !strings.Contains(srv.URL, "worker_type=executor") {
		t.Errorf("expected worker_type=executor in URL, got %q", srv.URL)
	}
	if srv.Headers["X-API-Key"] != "key-123" {
		t.Errorf("expected API key in headers")
	}

	// Empty type should not add query param.
	cfg2 := buildMCPConfigForType("", "http://localhost:8080", "")
	srv2 := cfg2.MCPServers["flywheel"]
	if strings.Contains(srv2.URL, "worker_type") {
		t.Errorf("empty type should not have worker_type param, got %q", srv2.URL)
	}
}

func TestRunTypedWorker(t *testing.T) {
	// Test that runTypedWorker passes the correct worker type through to the prompt.
	proj := &project.Project{
		ID:      "p-1",
		Name:    "test-proj",
		RepoURL: "https://github.com/test/repo.git",
		ContextPack: project.ContextPack{
			SystemPrompt: "Be helpful",
		},
	}
	tk := &ticket.Ticket{
		ID:        "t-typed",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
		Title:     "typed worker test",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "test"},
	}

	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)
	worker := &mockWorker{}

	tmpDir := t.TempDir()
	cfg := Config{
		MaxWorkers:  5,
		WorktreeDir: tmpDir,
		ServerURL:   "http://localhost:8080",
		AgentID:     "agent-test",
	}

	bus := events.NewInProcessBus()
	clones := NewMultiRepoCloneManager(filepath.Join(tmpDir, ".clones"))
	d := &Dispatcher{
		skipWorktrees: true,
		cfg:           cfg,
		bus:           bus,
		tickets:       tg,
		projects:      pg,
		worker:        worker,
		clones:        clones,
		worktrees: &WorktreeManager{
			BaseDir: tmpDir,
		},
		active: make(map[string]context.CancelFunc),
	}
	seedTestClone(t, d, "p-1")

	// Run as planner.
	err := d.runTypedWorker(t_ctx(), tk, WorkerTypePlanner)
	if err != nil {
		t.Fatalf("runTypedWorker(planner): %v", err)
	}
	if worker.callCount() != 1 {
		t.Fatalf("expected 1 call, got %d", worker.callCount())
	}
	worker.mu.Lock()
	call := worker.calls[0]
	worker.mu.Unlock()

	if !strings.Contains(call.SystemPrompt, "planner") {
		t.Error("planner worker prompt should contain 'planner'")
	}
	if !strings.Contains(call.TaskMessage, "Plan") {
		t.Error("planner task message should contain 'Plan'")
	}
}

func TestBuildProgressBlock(t *testing.T) {
	t.Run("no prior attempts returns empty", func(t *testing.T) {
		tk := &ticket.Ticket{}
		if got := buildProgressBlock(tk); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("rejected with branch and PR", func(t *testing.T) {
		tk := &ticket.Ticket{
			Context: ticket.TicketContext{
				PriorAttempts: []ticket.AttemptSummary{
					{AgentID: "a-1", Outcome: "rejected", Summary: "Missing tests"},
				},
			},
			Outputs: map[string]any{
				"_branch": "ticket/test-10",
				"pr_url":  "https://github.com/org/repo/pull/10",
			},
		}
		block := buildProgressBlock(tk)
		for _, want := range []string{
			"### Ticket progress",
			"**Attempt:** 2",
			"1 rejected",
			"`ticket/test-10`",
			"do NOT create a new branch",
			"https://github.com/org/repo/pull/10",
			"push fixes to this PR",
			"**Last outcome:** rejected",
			"Fix all BLOCKING issues",
		} {
			if !strings.Contains(block, want) {
				t.Errorf("progress block should contain %q, got:\n%s", want, block)
			}
		}
	})

	t.Run("worker_exit outcome", func(t *testing.T) {
		tk := &ticket.Ticket{
			Context: ticket.TicketContext{
				PriorAttempts: []ticket.AttemptSummary{
					{AgentID: "a-1", Outcome: "worker_exit", Summary: "Crashed"},
				},
			},
			Outputs: map[string]any{},
		}
		block := buildProgressBlock(tk)
		if !strings.Contains(block, "Previous worker exited without completing") {
			t.Errorf("should contain worker_exit action, got:\n%s", block)
		}
	})

	t.Run("multiple prior attempts", func(t *testing.T) {
		tk := &ticket.Ticket{
			Context: ticket.TicketContext{
				PriorAttempts: []ticket.AttemptSummary{
					{AgentID: "a-1", Outcome: "worker_exit", Summary: "Crashed"},
					{AgentID: "a-2", Outcome: "rejected", Summary: "Missing tests"},
				},
			},
			Outputs: map[string]any{},
		}
		block := buildProgressBlock(tk)
		if !strings.Contains(block, "**Attempt:** 3") {
			t.Errorf("should show attempt 3, got:\n%s", block)
		}
		if !strings.Contains(block, "1 rejected") || !strings.Contains(block, "1 worker_exit") {
			t.Errorf("should show breakdown, got:\n%s", block)
		}
		// Last outcome should be rejected
		if !strings.Contains(block, "**Last outcome:** rejected") {
			t.Errorf("last outcome should be rejected, got:\n%s", block)
		}
	})
}

func TestAssembleTypedWorkerPrompt_ProgressBlockBeforeObjective(t *testing.T) {
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:       "t-1",
		Title:    "Fix the bug",
		Type:     ticket.TypeTask,
		Priority: 1,
		Objective: ticket.Objective{
			Description:     "Fix the broken feature",
			SuccessCriteria: []string{"Tests pass"},
		},
		Context: ticket.TicketContext{
			PriorAttempts: []ticket.AttemptSummary{
				{AgentID: "a-1", Outcome: "rejected", Summary: "7 blocking issues"},
			},
		},
		Outputs: map[string]any{
			"_branch": "ticket/t-1",
			"pr_url":  "https://github.com/org/repo/pull/5",
		},
	}

	prompt := AssembleTypedWorkerPrompt(WorkerTypeExecutor, proj, tk, nil, "http://localhost", "a-1", nil)

	// Progress block and prior attempts should appear before objective
	progressIdx := strings.Index(prompt, "### Ticket progress")
	priorIdx := strings.Index(prompt, "### Prior attempts")
	objectiveIdx := strings.Index(prompt, "### Objective")

	if progressIdx == -1 {
		t.Fatal("prompt should contain '### Ticket progress'")
	}
	if priorIdx == -1 {
		t.Fatal("prompt should contain '### Prior attempts'")
	}
	if objectiveIdx == -1 {
		t.Fatal("prompt should contain '### Objective'")
	}
	if progressIdx > objectiveIdx {
		t.Error("progress block should appear before objective")
	}
	if priorIdx > objectiveIdx {
		t.Error("prior attempts should appear before objective")
	}
	if progressIdx > priorIdx {
		t.Error("progress block should appear before prior attempts")
	}

	// Should NOT have a duplicate prior attempts section
	count := strings.Count(prompt, "### Prior attempts")
	if count != 1 {
		t.Errorf("expected exactly 1 prior attempts section, got %d", count)
	}
}

func TestAssembleTypedWorkerPrompt_NoPriorAttemptsNoProgress(t *testing.T) {
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-1",
		Title:     "Fresh ticket",
		Type:      ticket.TypeTask,
		Priority:  1,
		Objective: ticket.Objective{Description: "Do stuff"},
	}

	prompt := AssembleTypedWorkerPrompt(WorkerTypeExecutor, proj, tk, nil, "http://localhost", "a-1", nil)
	if strings.Contains(prompt, "### Ticket progress") {
		t.Error("fresh ticket should not have progress block")
	}
	if strings.Contains(prompt, "### Prior attempts") {
		t.Error("fresh ticket should not have prior attempts")
	}
}

// helper to create a context for tests.
func t_ctx() context.Context {
	return context.Background()
}
