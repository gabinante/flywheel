package codereview

import (
	"context"
	"errors"
	"fmt"
	"github.com/gabinante/flywheel/internal/prompts"
	"github.com/gabinante/flywheel/internal/runstatus"
	"log/slog"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/harness"
	"github.com/gabinante/flywheel/internal/sessions"
)

// AskSystemPrompt frames a follow-up conversation about a review the agent produced.
const AskSystemPrompt = `You performed the code review described below and are now talking with the operator (the human reviewer) about it.
Answer their questions directly and concretely, citing files and lines. If they ask you to post something on the pull request
(a comment, a reply on a thread, a review), do it with the gh CLI from this checkout and then confirm what you posted. Do not
invent findings you did not verify in the code. Keep answers short unless asked for depth.`

// Ask sends the operator's message to the agent that reviewed the PR and returns its reply.
// The agent's own harness session is resumed when we still have its id; otherwise a fresh
// session is started with the review as context. The exchange is stored on the review.
func (s *Service) Ask(ctx context.Context, reviewID, message string) (*Message, *Message, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, nil, errors.New("message is required")
	}
	req, err := s.store.Get(ctx, reviewID)
	if err != nil {
		return nil, nil, err
	}
	if req == nil {
		return nil, nil, errors.New("review not found")
	}
	cfg := s.conf()
	kind, err := harness.ParseKind(firstNonEmpty(req.Harness, cfg.Harness, "codex"))
	if err != nil {
		return nil, nil, err
	}

	userMsg := &Message{ReviewID: req.ID, Role: "user", Content: message}
	if err := s.store.AddMessage(ctx, userMsg); err != nil {
		return nil, nil, err
	}

	// A checkout of the PR head so the agent can look at the code (and post with gh).
	ws := &Workspaces{Root: s.conf().RepoRoot}
	wt, _, err := ws.Prepare(ctx, req.Repo, req.Number)
	if err != nil {
		return userMsg, nil, fmt.Errorf("prepare worktree: %w", err)
	}
	defer ws.Remove(context.WithoutCancel(ctx), req.Repo, wt)

	resume := req.SessionExternalID
	prompt := message
	if resume == "" {
		prompt = s.recapPrompt(ctx, req, message)
	}
	spec := harness.Spec{
		Harness: kind, Model: firstNonEmpty(req.Model, cfg.Model), Effort: firstNonEmpty(req.ReasoningEffort, cfg.Effort), WorkDir: wt,
		SystemPrompt: prompts.Text("review_conversation"), Prompt: prompt, Sandbox: harness.SandboxFull, Timeout: 20 * time.Minute, Resume: resume,
	}
	ctx = runstatus.WithInfo(ctx, runstatus.Run{Kind: "conversation", ReviewID: req.ID, Ref: req.Ref(), Title: req.Title})
	res, runErr := s.runner.Run(ctx, spec)
	if runErr != nil && (res == nil || strings.TrimSpace(res.Output) == "") && resume != "" {
		// The old session may be gone (pruned, different machine). Forget it and retry once
		// with the review recap as context.
		slog.Warn("codereview: resume failed, retrying fresh", "review", req.ID, "session", resume, "error", runErr, "stderr", tail(resStderr(res), 400))
		_ = s.store.ClearSession(ctx, req.ID)
		spec.Resume = ""
		spec.Prompt = s.recapPrompt(ctx, req, message)
		res, runErr = s.runner.Run(ctx, spec)
	}
	if runErr != nil && (res == nil || strings.TrimSpace(res.Output) == "") {
		return userMsg, nil, fmt.Errorf("%w: %s", runErr, tail(resStderr(res), 600))
	}
	reply := &Message{ReviewID: req.ID, Role: "assistant", Content: strings.TrimSpace(res.Output)}
	if res.ExternalSessionID != "" {
		reply.SessionID = res.ExternalSessionID
		_ = s.store.SetSession(ctx, req.ID, "", res.ExternalSessionID)
		if res.ExternalSessionID != resume {
			s.recordAskSession(ctx, req, kind, res, wt)
		}
	}
	if err := s.store.AddMessage(ctx, reply); err != nil {
		return userMsg, nil, err
	}
	return userMsg, reply, nil
}

// recapPrompt rebuilds the review context for a fresh session (nothing to resume).
func (s *Service) recapPrompt(ctx context.Context, req *Request, message string) string {
	history, _ := s.store.ListMessages(ctx, req.ID)
	var b strings.Builder
	fmt.Fprintf(&b, "Pull request: %s#%d — %s (%s)\nHead: %s\nYour verdict: %s\nYour summary: %s\n", req.Repo, req.Number, req.Title, req.URL, req.HeadSHA, req.Verdict, req.Summary)
	if findings, err := s.store.ListFindings(ctx, req.ID, req.Attempt); err == nil && len(findings) > 0 {
		b.WriteString("Your findings:\n")
		for _, f := range findings {
			fmt.Fprintf(&b, "- [%s] %s:%d %s — %s\n", f.Severity, f.Path, f.Line, f.Title, f.Body)
		}
	}
	if len(history) > 1 {
		b.WriteString("\nConversation so far:\n")
		for _, m := range history[:len(history)-1] {
			fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Content)
		}
	}
	b.WriteString("\nOperator: " + message)
	return b.String()
}

func (s *Service) recordAskSession(ctx context.Context, req *Request, kind harness.Kind, res *harness.Result, wt string) {
	if s.sessions == nil {
		return
	}
	h := sessions.HarnessClaudeCode
	if kind == harness.Codex {
		h = sessions.HarnessCodex
	}
	now := time.Now()
	sess := &sessions.Session{
		Harness: h, ExternalID: res.ExternalSessionID, Origin: sessions.OriginDispatched, CWD: wt, Repo: req.Repo, Branch: req.HeadRef,
		Model: res.Model, Title: fmt.Sprintf("Review conversation %s#%d", req.Repo, req.Number), StartedAt: now.Add(-res.Duration), EndedAt: &now,
		TokensIn: res.TokensIn, TokensOut: res.TokensOut,
	}
	if err := s.sessions.Record(ctx, sess); err == nil {
		_ = s.sessions.Link(ctx, sess.ID, "pr", fmt.Sprintf("%s#%d", req.Repo, req.Number), "review")
	}
}

// Messages returns the conversation on a review.
func (s *Service) Messages(ctx context.Context, reviewID string) ([]Message, error) {
	return s.store.ListMessages(ctx, reviewID)
}

func resStderr(r *harness.Result) string {
	if r == nil {
		return ""
	}
	return r.Stderr
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func init() {
	prompts.Register(prompts.Prompt{ID: "review_conversation", Name: "Reviewer conversation", Order: 30,
		Description: "Frames the follow-up chat with the agent that reviewed a PR (Talk to the reviewer), including permission to post via gh when asked.",
		UsedBy:      "Code review detail", Default: AskSystemPrompt})
}
