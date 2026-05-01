package workflow

// BuiltinTemplates returns the built-in workflow templates.
func BuiltinTemplates() []Definition {
	return []Definition{
		StandardSDLC(),
		FastTrack(),
		FullPipeline(),
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
				Config:      map[string]any{"role": "planner", "goal": "Analyze the ticket scope. If it needs decomposition, create focused child tickets with depends_on edges. If already well-scoped, complete immediately."},
			},
			{
				ID: "execute", Name: "Execution", Type: PhaseAgent,
				Description: "Implement the work and open a PR.",
				Config:      map[string]any{"role": "executor"},
			},
			{
				ID: "agentic-review", Name: "Agentic Code Review", Type: PhaseAgent,
				Description: "Agent reviews code and provides feedback. Loops back to execution on rejection. Auto-passes after max iterations.",
				Config:    map[string]any{"role": "validator", "max_iterations": 3},
				OnFailure: "execute",
			},
			{
				ID: "quality-gate", Name: "Quality Gate", Type: PhaseGate,
				Description: "CI tests must pass before merge.",
				Config: map[string]any{
					"prompt":       "Verify all CI checks pass.",
					"requirements": []any{"github_checks"},
				},
			},
			{
				ID: "merge", Name: "Merge", Type: PhaseExternal,
				Description: "Merge the approved PR.",
				Config: map[string]any{"mode": "sync"},
			},
			{
				ID: "deploy-dev", Name: "Deploy to Dev", Type: PhaseExternal,
				Description: "Deploy merged changes to the dev environment.",
				Config: map[string]any{"mode": "async"},
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
				Config:      map[string]any{"prompt": "Review the code and approve or reject."},
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
