package sessions

import (
	"context"
	"github.com/gabinante/flywheel/internal/runstatus"
	"strings"
	"time"
)

// RecordRun makes a managed session addressable as soon as the harness announces
// its native ID, and closes the same record when the process exits.
func (s *Service) RecordRun(ctx context.Context, run runstatus.Run) (string, error) {
	h := HarnessCodex
	if run.Harness == "claude" || run.Harness == "claude_code" {
		h = HarnessClaudeCode
	}
	prompt := run.Prompt
	if chars := []rune(prompt); len(chars) > 500 {
		prompt = string(chars[:500])
	}
	title := run.Title
	if title == "" {
		title = strings.TrimSpace(run.Kind + " " + run.TicketID)
	}
	meta := map[string]any{"flywheel_role": run.Kind, "flywheel_running": run.FinishedAt == nil, "flywheel_run_id": run.ID, "flywheel_progress": run.Progress}
	if run.ProjectID != "" {
		meta["project_id"] = run.ProjectID
	}
	sess := &Session{Harness: h, ExternalID: run.ExternalSessionID, Origin: OriginDispatched, CWD: run.WorkDir, Title: title, FirstPrompt: prompt, Model: run.Model, StartedAt: run.StartedAt, LastActivityAt: time.Now().UTC(), EndedAt: run.FinishedAt, Metadata: meta}
	if run.Progress.TokensIn != nil {
		sess.TokensIn = *run.Progress.TokensIn
	}
	if run.Progress.TokensOut != nil {
		sess.TokensOut = *run.Progress.TokensOut
	}
	if err := s.Record(ctx, sess); err != nil {
		return "", err
	}
	for kind, ref := range map[string]string{LinkTicket: run.TicketID, LinkReview: run.ReviewID, LinkPR: run.Ref} {
		if ref != "" {
			if err := s.Link(ctx, sess.ID, kind, ref, LinkSourceDispatch); err != nil {
				return "", err
			}
		}
	}
	return sess.ID, nil
}

// RecoverManagedRuns clears running markers left by an interrupted server.
func (s *Service) RecoverManagedRuns(ctx context.Context) error {
	_, err := s.store.pool.Exec(ctx, `UPDATE agent_sessions SET metadata=metadata||'{"flywheel_running":false}'::jsonb, ended_at=COALESCE(ended_at,now()) WHERE metadata->>'flywheel_running'='true'`)
	return err
}
