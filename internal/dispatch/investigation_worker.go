package dispatch

import (
	"context"

	"github.com/gabinante/flywheel/internal/investigation"
)

// NewInvestigationWorker creates a Worker suitable for investigation dispatch.
// It uses the same CLI worker infrastructure as the dispatcher but operates
// independently — no lease management, no event bus, no worktrees.
// The returned worker satisfies investigation.Worker via an adapter.
func NewInvestigationWorker(cfg Config) investigation.Worker {
	return &investigationWorkerAdapter{worker: NewWorker(cfg)}
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
