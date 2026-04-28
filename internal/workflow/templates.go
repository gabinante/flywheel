package workflow

// BuiltinTemplates returns the built-in workflow templates.
func BuiltinTemplates() []Definition {
	return []Definition{
		StandardSDLC(),
		FastTrack(),
		FullPipeline(),
	}
}

// StandardSDLC returns the default workflow matching the current hardcoded state machine.
func StandardSDLC() Definition {
	return Definition{
		Name:        "Standard SDLC",
		Description: "Default workflow: agent executes, human reviews, deploy, observe.",
		Phases: []Phase{
			{
				ID: "execute", Name: "Execution", Type: PhaseAgent,
				Description: "Agent claims, codes, and submits work.",
				Config:      map[string]any{"role": "executor"},
			},
			{
				ID: "review", Name: "Review", Type: PhaseGate,
				Description: "Human reviews and approves the work.",
				Config:      map[string]any{"prompt": "Review the submitted code and approve or reject."},
			},
			{
				ID: "deploy", Name: "Deploy", Type: PhaseExternal,
				Description: "Deploy the validated changes.",
				Config:      map[string]any{"mode": "async"},
			},
			{
				ID: "observe", Name: "Observe", Type: PhaseExternal,
				Description: "Post-deploy monitoring window.",
				Config:      map[string]any{"mode": "poll", "poll_interval": "30s", "poll_timeout": "5m"},
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

// FullPipeline returns the most comprehensive workflow with testing gates.
func FullPipeline() Definition {
	return Definition{
		Name:        "Full Pipeline",
		Description: "Complete pipeline: execute → review → PR → test → merge → deploy → post-deploy test → observe.",
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
