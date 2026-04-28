package dispatch

import (
	"testing"

	"github.com/gabinante/flywheel/internal/project"
)

func TestProjectWorkerRouter_DefaultWorkerWhenNoProfiles(t *testing.T) {
	router := NewProjectWorkerRouter(Config{
		AgentRunner: RunnerCLI,
		AgentDriver: "claude",
	})

	candidates := router.Candidates(&project.Project{}, string(WorkerTypeExecutor))
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if !candidates[0].UseDefault {
		t.Fatalf("expected default worker candidate")
	}
	if candidates[0].ID != DefaultProjectWorkerID {
		t.Fatalf("expected default worker id, got %q", candidates[0].ID)
	}
}

func TestProjectWorkerRouter_OrderedPolicyUsesConfiguredWorkers(t *testing.T) {
	t.Setenv("CODEX_REVIEW_KEY", "review-key")

	router := NewProjectWorkerRouter(Config{
		AgentRunner: RunnerCLI,
		AgentDriver: "claude",
		AgentAPIKey: "anthropic-key",
	})
	proj := &project.Project{
		DispatchConfig: project.DispatchConfig{
			Workers: []project.DispatchWorkerProfile{
				{
					ID:               "claude-impl",
					Name:             "Claude impl",
					Enabled:          true,
					Driver:           "claude",
					CredentialEnvVar: "ANTHROPIC_API_KEY",
				},
				{
					ID:               "codex-review",
					Name:             "Codex review",
					Enabled:          true,
					Driver:           "codex",
					CredentialEnvVar: "CODEX_REVIEW_KEY",
				},
			},
			Policies: map[string]project.DispatchRolePolicy{
				"review": {
					SelectionMode: "ordered",
					WorkerIDs:     []string{"codex-review", "claude-impl"},
				},
			},
		},
	}

	candidates := router.Candidates(proj, "validator")
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].ID != "codex-review" || candidates[1].ID != "claude-impl" {
		t.Fatalf("unexpected candidate order: %#v", candidates)
	}
	if candidates[0].Config.AgentDriver != "codex" {
		t.Fatalf("expected codex driver, got %q", candidates[0].Config.AgentDriver)
	}
	if candidates[0].Config.AgentAPIKey != "review-key" {
		t.Fatalf("expected env-derived key, got %q", candidates[0].Config.AgentAPIKey)
	}
}

func TestProjectWorkerRouter_CustomRolePolicy(t *testing.T) {
	router := NewProjectWorkerRouter(Config{
		AgentRunner: RunnerCLI,
		AgentDriver: "claude",
	})
	proj := &project.Project{
		DispatchConfig: project.DispatchConfig{
			Workers: []project.DispatchWorkerProfile{
				{ID: "codex-sec", Name: "Codex security", Enabled: true, Driver: "codex"},
			},
			Policies: map[string]project.DispatchRolePolicy{
				"security_review": {
					SelectionMode: "ordered",
					WorkerIDs:     []string{"codex-sec"},
				},
			},
		},
	}

	candidates := router.Candidates(proj, "security-review")
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].ID != "codex-sec" {
		t.Fatalf("expected codex-sec, got %q", candidates[0].ID)
	}
	if candidates[0].Config.AgentDriver != "codex" {
		t.Fatalf("expected codex driver, got %q", candidates[0].Config.AgentDriver)
	}
}

func TestProjectWorkerRouter_AnyModeRotatesAcrossWorkers(t *testing.T) {
	router := NewProjectWorkerRouter(Config{
		AgentRunner: RunnerCLI,
		AgentDriver: "claude",
	})
	proj := &project.Project{
		DispatchConfig: project.DispatchConfig{
			Workers: []project.DispatchWorkerProfile{
				{ID: "worker-a", Name: "A", Enabled: true, Driver: "claude"},
				{ID: "worker-b", Name: "B", Enabled: true, Driver: "codex"},
			},
			Policies: map[string]project.DispatchRolePolicy{
				string(WorkerTypeExecutor): {
					SelectionMode: "any",
					WorkerIDs:     []string{"worker-a", "worker-b"},
				},
			},
		},
	}

	first := router.Candidates(proj, string(WorkerTypeExecutor))
	second := router.Candidates(proj, string(WorkerTypeExecutor))
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("expected 2 candidates in both runs, got %d and %d", len(first), len(second))
	}
	if first[0].ID == second[0].ID {
		t.Fatalf("expected rotation, got %q then %q", first[0].ID, second[0].ID)
	}
}

func TestProjectWorkerRouter_OpenAICompatibleUsesOpenAIFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-openai")

	router := NewProjectWorkerRouter(Config{
		AgentRunner: RunnerCLI,
		AgentDriver: "claude",
	})
	proj := &project.Project{
		DispatchConfig: project.DispatchConfig{
			Workers: []project.DispatchWorkerProfile{
				{
					ID:      "local-compatible",
					Name:    "Local compatible",
					Enabled: true,
					Runner:  RunnerOpenAICompatible,
					Model:   "qwen2.5-coder",
				},
			},
			Policies: map[string]project.DispatchRolePolicy{
				string(WorkerTypeExecutor): {
					SelectionMode: "ordered",
					WorkerIDs:     []string{"local-compatible"},
				},
			},
		},
	}

	candidates := router.Candidates(proj, string(WorkerTypeExecutor))
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Config.AgentRunner != RunnerOpenAICompatible {
		t.Fatalf("expected openai-compatible runner, got %q", candidates[0].Config.AgentRunner)
	}
	if candidates[0].Config.AgentAPIKey != "sk-openai" {
		t.Fatalf("expected OPENAI_API_KEY fallback, got %q", candidates[0].Config.AgentAPIKey)
	}
}
