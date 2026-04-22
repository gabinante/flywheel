package dispatch

import (
	"context"
	"log"

	"github.com/gabinante/flywheel/internal/investigation"
)

// NewInvestigationWorker creates a Worker suitable for investigation dispatch.
// It uses the same CLI worker infrastructure as the dispatcher but operates
// independently — no lease management, no event bus, no worktrees.
// The returned worker satisfies investigation.Worker via an adapter.
func NewInvestigationWorker(cfg Config) investigation.Worker {
	// Resolve the agent driver.
	driverName := cfg.AgentDriver
	if driverName == "" {
		driverName = "claude"
	}
	cliPath := cfg.AgentCLIPath
	if cliPath == "" {
		cliPath = cfg.ClaudePath
	}
	driver, err := LookupDriver(driverName, DriverConfig{
		CLIPath:   cliPath,
		ExtraArgs: cfg.AgentArgs,
	})
	if err != nil {
		log.Printf("dispatch/investigation: %v, falling back to claude driver", err)
		driver = NewClaudeDriver(DriverConfig{CLIPath: cliPath})
	}

	var worker Worker
	if cfg.DockerEnabled {
		worker = &DockerWorker{
			Driver:       driver,
			Image:        cfg.DockerImage,
			APIKey:       cfg.APIKey,
			RepoDir:      cfg.RepoDir,
			AnthropicKey: cfg.AnthropicKey,
			Memory:       cfg.DockerMemory,
			CPUs:         cfg.DockerCPUs,
			Firewall:     cfg.DockerFirewall,
		}
	} else {
		worker = &CLIWorker{
			Driver: driver,
			APIKey: cfg.APIKey,
		}
	}

	return &investigationWorkerAdapter{worker: worker}
}

// investigationWorkerAdapter adapts dispatch.Worker to investigation.Worker.
type investigationWorkerAdapter struct {
	worker Worker
}

func (a *investigationWorkerAdapter) Spawn(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*investigation.WorkerResult, error) {
	result, err := a.worker.Spawn(ctx, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL)
	if err != nil {
		return nil, err
	}
	return &investigation.WorkerResult{
		Success: result.Success,
		Output:  result.Output,
		Error:   result.Error,
	}, nil
}
