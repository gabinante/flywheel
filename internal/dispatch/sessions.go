package dispatch

import (
	"context"
	"github.com/gabinante/flywheel/internal/sessions"
	"log/slog"
)

type SessionRecorder interface {
	Record(context.Context, *sessions.Session) error
	Link(context.Context, string, string, string, string) error
}

func RecordWorkerSession(ctx context.Context, recorder SessionRecorder, result *WorkerResult, worker RoutedWorker, projectID, ticketID, role, workDir, prompt string) {
	if recorder == nil || result == nil || result.SessionID == "" {
		return
	}
	h := sessions.HarnessClaudeCode
	if worker.Config.AgentDriver == "codex" {
		h = sessions.HarnessCodex
	}
	end := result.EndedAt
	sess := &sessions.Session{Harness: h, ExternalID: result.SessionID, Origin: sessions.OriginDispatched, CWD: workDir, Title: role + " · " + ticketID, FirstPrompt: truncate(prompt, 500), Model: worker.Config.AgentModel, ReasoningEffort: worker.Config.AgentReasoningEffort, PromptCount: 1, StartedAt: result.StartedAt, LastActivityAt: end, EndedAt: &end, Metadata: map[string]any{"project_id": projectID, "flywheel_role": role, "cost_usd": result.CostUSD, "success": result.Success}}
	if err := recorder.Record(ctx, sess); err != nil {
		slog.Warn("dispatch: record session failed", "error", err)
		return
	}
	if role != "orchestrator" {
		if err := recorder.Link(ctx, sess.ID, sessions.LinkTicket, ticketID, sessions.LinkSourceDispatch); err != nil {
			slog.Warn("dispatch: link session failed", "error", err)
		}
	}
}
func (d *Dispatcher) SetSessionRecorder(s SessionRecorder) { d.sessions = s }
