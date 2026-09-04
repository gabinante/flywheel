package codereview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/harness"
	"github.com/gabinante/flywheel/internal/sessions"
)

// FeedbackConfig controls the address-feedback workflow on the operator's own PRs.
type FeedbackConfig struct {
	PromptPrefix string // worker base prompt prepended to the feedback system prompt
	Harness      string // claude (default) or codex
	Model        string
	Effort       string
	AutoAddress  bool          // dispatch automatically when a review lands; otherwise wait for the operator
	Timeout      time.Duration // default 45m
}

// FeedbackSystemPrompt is the operator's "address feedback on the PR" instruction, generalized.
const FeedbackSystemPrompt = `You are addressing review feedback on a pull request you (the operator) authored. You are in a git worktree
checked out on the PR branch with the remote tracking branch set; the gh CLI is authenticated.

Work like a careful engineer landing their own PR:
1. Read the PR and every unresolved review thread (gh pr view, gh api repos/{owner}/{repo}/pulls/{n}/comments and
   /reviews). Include bot reviews (Bugbot, CI annotations) — they are real feedback.
2. For each thread decide: fix it, or explain briefly why not. Prefer fixing. Keep the change scoped to the feedback.
3. Run the relevant tests and linters for the files you touched. Do not push red.
4. Commit with clear messages, push to the PR branch (never force-push unless the branch is yours alone and the
   history rewrite is deliberate), and reply on each addressed thread with what changed. Resolve threads you fixed
   where the tooling allows; otherwise reply and leave them for the reviewer.
5. If the PR description needs updating (it must start with an Abstract section), update it with gh pr edit.
6. Re-request review from the reviewers whose feedback you addressed (gh pr edit --add-reviewer).

Follow AGENTS.md / CLAUDE.md in the repository. Finish with a short summary of what you changed and anything you
deliberately left open.`

// BuildFeedbackPrompt renders the task for one landed review.
func BuildFeedbackPrompt(round *FeedbackRound, pr *PR) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Address the review that %s left on %s#%d (%s): %s\n", round.Reviewer, round.Repo, round.Number, round.ReviewState, pr.Title)
	fmt.Fprintf(&b, "PR: %s\nBranch: %s (base %s)\n", pr.URL, pr.HeadRef, pr.BaseRef)
	if round.CommentCount > 0 {
		fmt.Fprintf(&b, "The review has %d inline comment(s).\n", round.CommentCount)
	}
	if body := strings.TrimSpace(round.Body); body != "" {
		fmt.Fprintf(&b, "\nReview body:\n%s\n", truncateStr(body, 4000))
	}
	b.WriteString("\nAlso address any other unresolved review threads on this PR while you are there. Push your changes, reply on the threads, and re-request review.")
	return b.String()
}

// AddressFeedback runs the harness on the PR branch to resolve a landed review.
func (s *Service) AddressFeedback(ctx context.Context, roundID string) (*FeedbackRound, error) {
	round, err := s.store.GetFeedbackRound(ctx, roundID)
	if err != nil {
		return nil, err
	}
	if round == nil {
		return nil, nil
	}
	if round.State == "dispatched" {
		return round, errors.New("feedback is already being addressed")
	}
	// One run addresses every unresolved thread on the PR, so all of its new rounds go along.
	_, _ = s.store.UpdateFeedbackRoundsByPR(ctx, round.Repo, round.Number, "new", "dispatched", "")
	_ = s.store.UpdateFeedbackRound(ctx, round.ID, "dispatched", "", "")
	round.State = "dispatched"
	go s.runAddressFeedback(context.WithoutCancel(ctx), round)
	return round, nil
}

func (s *Service) runAddressFeedback(ctx context.Context, round *FeedbackRound) {
	fail := func(err error) {
		slog.Warn("codereview: address feedback failed", "pr", round.Ref(), "error", err)
		_, _ = s.store.UpdateFeedbackRoundsByPR(ctx, round.Repo, round.Number, "dispatched", "new", "")
	}
	login, _ := s.gh.Login(ctx)
	pr, err := s.gh.ViewPR(ctx, round.Repo, round.Number, login)
	if err != nil {
		fail(err)
		return
	}
	if pr.State != "OPEN" {
		_, _ = s.store.UpdateFeedbackRoundsByPR(ctx, round.Repo, round.Number, "dispatched", "ignored", "")
		return
	}
	wt, err := s.ws.PrepareBranch(ctx, round.Repo, pr.HeadRef, round.Number)
	if err != nil {
		fail(fmt.Errorf("prepare branch worktree: %w", err))
		return
	}
	defer s.ws.Remove(ctx, round.Repo, wt)

	fb := s.feedback()
	kind, err := harness.ParseKind(firstNonEmpty(fb.Harness, "claude"))
	if err != nil {
		fail(err)
		return
	}
	prompt := BuildFeedbackPrompt(round, pr)
	started := time.Now()
	res, runErr := s.runner.Run(ctx, harness.Spec{
		Harness: kind, Model: fb.Model, Effort: fb.Effort, WorkDir: wt,
		SystemPrompt: withPrefix(fb.PromptPrefix, FeedbackSystemPrompt), Prompt: prompt, Sandbox: harness.SandboxWorkspaceWrite, Timeout: fb.Timeout,
	})
	sessionID := ""
	if s.sessions != nil && res != nil && res.ExternalSessionID != "" {
		h := sessions.HarnessClaudeCode
		if kind == harness.Codex {
			h = sessions.HarnessCodex
		}
		ended := time.Now()
		sess := &sessions.Session{
			Harness: h, ExternalID: res.ExternalSessionID, Origin: sessions.OriginDispatched, CWD: wt, Repo: round.Repo, Branch: pr.HeadRef,
			Model: firstNonEmpty(res.Model, fb.Model), ReasoningEffort: fb.Effort,
			Title: fmt.Sprintf("Address %s review on %s: %s", round.Reviewer, round.Ref(), pr.Title), FirstPrompt: truncateStr(prompt, 500),
			TokensIn: res.TokensIn, TokensOut: res.TokensOut, PromptCount: 1, StartedAt: started, LastActivityAt: ended, EndedAt: &ended,
			Metadata: map[string]any{"flywheel_role": "address_feedback", "feedback_round_id": round.ID, "cost_usd": res.CostUSD},
		}
		if err := s.sessions.Record(ctx, sess); err == nil {
			sessionID = sess.ID
			_ = s.sessions.Link(ctx, sess.ID, sessions.LinkPR, round.Ref(), sessions.LinkSourceDispatch)
		}
	}
	if runErr != nil {
		_, _ = s.store.UpdateFeedbackRoundsByPR(ctx, round.Repo, round.Number, "dispatched", "new", sessionID)
		slog.Warn("codereview: address feedback run failed", "pr", round.Ref(), "error", runErr)
		return
	}
	_, _ = s.store.UpdateFeedbackRoundsByPR(ctx, round.Repo, round.Number, "dispatched", "addressed", sessionID)
	slog.Info("codereview: addressed review feedback", "pr", round.Ref(), "reviewer", round.Reviewer, "session", sessionID)
}

// PrepareBranch creates a worktree tracking the PR's head branch (for pushing fixes),
// at <root>/<repo>-worktrees/feedback-<n>.
func (w *Workspaces) PrepareBranch(ctx context.Context, repo, branch string, number int) (string, error) {
	repoDir, err := w.RepoDir(ctx, repo)
	if err != nil {
		return "", err
	}
	if _, err := gitRun(ctx, repoDir, "fetch", "--quiet", "origin", branch); err != nil {
		return "", err
	}
	name := filepath.Base(repoDir)
	if strings.Contains(name, "__") {
		name = name[strings.Index(name, "__")+2:]
	}
	wt := filepath.Join(w.Root, name+"-worktrees", fmt.Sprintf("feedback-%d", number))
	if _, err := os.Stat(wt); err == nil {
		_, _ = gitRun(ctx, repoDir, "worktree", "remove", "--force", wt)
		_ = os.RemoveAll(wt)
	}
	_, _ = gitRun(ctx, repoDir, "worktree", "prune")
	if err := os.MkdirAll(filepath.Dir(wt), 0o755); err != nil {
		return "", err
	}
	// Check out the branch (creating a local branch tracking origin when needed). If the branch is
	// already checked out elsewhere (the operator's own worktree), git refuses; fall back to a detached
	// checkout of the same commit and push explicitly by refspec.
	if _, err := gitRun(ctx, repoDir, "worktree", "add", "--quiet", wt, branch); err != nil {
		if _, err2 := gitRun(ctx, repoDir, "worktree", "add", "--quiet", "--track", "-b", branch, wt, "origin/"+branch); err2 != nil {
			if _, err3 := gitRun(ctx, repoDir, "worktree", "add", "--quiet", "--detach", wt, "origin/"+branch); err3 != nil {
				return "", err3
			}
		}
	}
	return wt, nil
}

// withPrefix prepends a worker's base prompt to a service system prompt.
func withPrefix(prefix, base string) string {
	if strings.TrimSpace(prefix) == "" {
		return base
	}
	return strings.TrimSpace(prefix) + "\n\n" + base
}
