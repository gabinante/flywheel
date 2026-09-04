package workflow

// BuiltinTemplates returns the built-in workflow templates.
func BuiltinTemplates() []Definition {
	return []Definition{
		Implement(),
		StandardSDLC(),
		FastTrack(),
		FullPipeline(),
		Triage(),
		GenericTask(),
		SubticketSDLC(),
	}
}

// Implement is the operator's implementation loop for a Linear-backed ticket:
// plan, implement in a repo worktree and open a PR (Abstract body, Linear
// identifier in the branch), pass CI, land it. The Linear issue is attached to
// the PR and moved along by the Linear syncer; landed review comments re-enter
// the loop through the feedback watcher.
func Implement() Definition {
	return Definition{
		Name:        "Implement",
		Description: "Plan, implement in a worktree, open a PR, get CI green, merge. Linear issue follows automatically.",
		Phases: []Phase{
			{
				ID: "plan", Name: "Plan", Type: PhaseAgent,
				Description: "Read the ticket and the repo; write a short implementation plan into the ticket outputs.",
				Config:      map[string]any{"role": "planner"},
			},
			{
				ID: "implement", Name: "Implement", Type: PhaseAgent,
				Description: "Implement the change in the ticket worktree, run the relevant tests, open a PR whose description starts with an Abstract, and submit with pr_url.",
				Config:      map[string]any{"role": "executor"},
			},
			{
				ID: "ci", Name: "CI green", Type: PhaseGate,
				Description: "GitHub checks must pass. Failures loop back to implementation to fix tests or conflicts.",
				Config: map[string]any{
					"conditions": []any{
						map[string]any{"type": "github_checks"},
					},
				},
				OnFailure: "implement",
			},
			{
				ID: "merge", Name: "Merge", Type: PhaseExternal,
				Description: "Merge once approved; the Linear issue moves to Done.",
				Config:      map[string]any{"mode": "sync"},
			},
		},
	}
}

// StandardSDLC returns the default workflow: decompose, execute, agentic review loop, quality gate, merge, deploy.
func StandardSDLC() Definition {
	return Definition{
		Name:        "Standard SDLC",
		Description: "Decompose, execute, agentic review loop, quality gate, merge, deploy to dev.",
		Phases: []Phase{
			{
				ID: "decompose", Name: "Decomposition", Type: PhaseAgent,
				Description: "Break the ticket into well-scoped child tickets with clear acceptance criteria. If already well-scoped, proceed directly.",
				Config:      map[string]any{"role": "decomposer", "goal": "Analyze the ticket scope. If it needs decomposition, create focused child tickets with depends_on edges and assign them a simpler workflow. If already well-scoped, complete immediately."},
			},
			{
				ID: "execute", Name: "Execution", Type: PhaseAgent,
				Description: "Implement the work and open a PR.",
				Config:      map[string]any{"role": "executor"},
			},
			{
				ID: "agentic-review", Name: "Agentic Code Review", Type: PhaseAgent,
				Description: "Agent reviews code and provides feedback. Loops back to execution on rejection. Auto-passes after max iterations.",
				Config:      map[string]any{"role": "validator", "max_iterations": 3},
				OnFailure:   "execute",
			},
			{
				ID: "quality-gate", Name: "Quality Gate", Type: PhaseGate,
				Description: "CI tests must pass before merge.",
				Config: map[string]any{
					"conditions": []any{
						map[string]any{"type": "github_checks"},
					},
				},
			},
			{
				ID: "merge", Name: "Merge", Type: PhaseExternal,
				Description: "Merge the approved PR.",
				Config:      map[string]any{"mode": "sync"},
			},
			{
				ID: "deploy-dev", Name: "Deploy to Dev", Type: PhaseExternal,
				Description: "Deploy merged changes to the dev environment.",
				Config:      map[string]any{"mode": "async"},
			},
		},
	}
}

// FastTrack returns a minimal workflow: execute → merge → done.
func FastTrack() Definition {
	return Definition{
		Name:        "Fast Track",
		Description: "Minimal pipeline: agent executes, auto-merge, done. No deploy/observe phases.",
		Phases: []Phase{
			{
				ID: "execute", Name: "Execution", Type: PhaseAgent,
				Description: "Agent claims, codes, and submits work.",
				Config:      map[string]any{"role": "executor"},
			},
			{
				ID: "merge", Name: "Auto-Merge", Type: PhaseExternal,
				Description: "Automatically merge the PR.",
				Config:      map[string]any{"mode": "sync"},
			},
		},
	}
}

// Triage returns a workflow for incident/alert triage: analyze → diagnose → human review → respond.
// No code, no repos — purely operational.
func Triage() Definition {
	return Definition{
		Name:        "Triage",
		Description: "Operational triage: analyze, diagnose, human review gate, then respond. No code required.",
		Phases: []Phase{
			{
				ID: "analyze", Name: "Analyze", Type: PhaseAgent,
				Description: "Initial analysis of the alert or incident.",
				Config:      map[string]any{"role": "operator", "goal": "Analyze the incoming alert or incident. Gather context, check state, and classify severity."},
			},
			{
				ID: "diagnose", Name: "Diagnose", Type: PhaseAgent,
				Description: "Deep diagnosis and root cause analysis.",
				Config:      map[string]any{"role": "operator", "goal": "Investigate root cause. Query system state, correlate with recent changes, and document findings."},
			},
			{
				ID: "human-review", Name: "Human Review", Type: PhaseGate,
				Description: "Human reviews the diagnosis before response.",
				Config: map[string]any{
					"conditions": []any{
						map[string]any{"type": "human_approval"},
					},
				},
			},
			{
				ID: "respond", Name: "Respond", Type: PhaseAgent,
				Description: "Execute the approved response. Loops back to diagnose on failure.",
				Config:      map[string]any{"role": "operator", "goal": "Execute the approved response plan. Log all actions taken."},
				OnFailure:   "diagnose",
			},
		},
	}
}

// GenericTask returns a minimal non-code workflow: execute → human review.
func GenericTask() Definition {
	return Definition{
		Name:        "Generic Task",
		Description: "Minimal workflow for non-code tasks: operator executes, human reviews.",
		Phases: []Phase{
			{
				ID: "execute", Name: "Execute", Type: PhaseAgent,
				Description: "Operator executes the task.",
				Config:      map[string]any{"role": "operator"},
			},
			{
				ID: "review", Name: "Review", Type: PhaseGate,
				Description: "Human reviews the result.",
				Config: map[string]any{
					"conditions": []any{
						map[string]any{"type": "human_approval"},
					},
				},
			},
		},
	}
}

// SubticketSDLC returns a streamlined pipeline for well-scoped subtickets created by decomposition.
// Uses a fast executor role (routable to smaller/cheaper models via project dispatch config),
// lightweight single-iteration review, and merge.
func SubticketSDLC() Definition {
	return Definition{
		Name:        "Subticket SDLC",
		Description: "Streamlined pipeline for well-scoped subtickets: execute with fast model, lightweight review, merge.",
		Phases: []Phase{
			{
				ID: "execute", Name: "Execution", Type: PhaseAgent,
				Description: "Execute the well-scoped subticket.",
				Config:      map[string]any{"role": "fast-executor"},
			},
			{
				ID: "review", Name: "Review", Type: PhaseAgent,
				Description: "Lightweight review of subticket work. Single iteration.",
				Config:      map[string]any{"role": "validator", "max_iterations": 1},
				OnFailure:   "execute",
			},
			{
				ID: "merge", Name: "Merge", Type: PhaseExternal,
				Description: "Merge the approved subticket PR.",
				Config:      map[string]any{"mode": "sync"},
			},
		},
	}
}

// FullPipeline returns the extended workflow with pre/post-merge testing gates and observation.
func FullPipeline() Definition {
	return Definition{
		Name:        "Full Pipeline",
		Description: "Extended pipeline: execute → review → PR → test → merge → deploy → post-deploy test → observe.",
		Phases: []Phase{
			{
				ID: "execute", Name: "Execution", Type: PhaseAgent,
				Description: "Agent claims, codes, and submits work.",
				Config:      map[string]any{"role": "executor"},
			},
			{
				ID: "review", Name: "Review", Type: PhaseGate,
				Description: "Human reviews and approves the work.",
				Config: map[string]any{
					"conditions": []any{
						map[string]any{"type": "human_approval"},
					},
				},
			},
			{
				ID: "open-pr", Name: "Open PR", Type: PhaseExternal,
				Description: "Open a pull request.",
				Config:      map[string]any{"mode": "sync"},
			},
			{
				ID: "pre-merge-test", Name: "Pre-Merge Tests", Type: PhaseExternal,
				Description: "Run CI checks before merge.",
				Config:      map[string]any{"mode": "poll", "poll_interval": "30s", "poll_timeout": "30m"},
				OnFailure:   "execute",
			},
			{
				ID: "merge", Name: "Merge", Type: PhaseExternal,
				Description: "Merge the PR.",
				Config:      map[string]any{"mode": "sync"},
			},
			{
				ID: "deploy", Name: "Deploy", Type: PhaseExternal,
				Description: "Deploy the validated changes.",
				Config:      map[string]any{"mode": "async"},
			},
			{
				ID: "post-deploy-test", Name: "Post-Deploy Tests", Type: PhaseExternal,
				Description: "Run tests after deployment.",
				Config:      map[string]any{"mode": "poll", "poll_interval": "30s", "poll_timeout": "10m"},
			},
			{
				ID: "observe", Name: "Observe", Type: PhaseExternal,
				Description: "Post-deploy monitoring window.",
				Config:      map[string]any{"mode": "poll", "poll_interval": "30s", "poll_timeout": "5m"},
			},
		},
	}
}
