package dispatch

// merge.go contains the PR management, merge automation, review orchestration,
// and conflict resolution subsystem. These are Dispatcher methods extracted from
// dispatcher.go for maintainability. The dispatcher delegates all PR/merge/review
// operations to functions in this file.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/cost"
	"github.com/gabinante/flywheel/internal/policy"
	"github.com/gabinante/flywheel/internal/ticket"
)

// maxMergeAttempts is the number of merge failures before escalating to a human.
const maxMergeAttempts = 5

// maxReviewAttempts is the number of reviewer failures before escalating to a human.
const maxReviewAttempts = 3

// ---------------------------------------------------------------------------
// Review orchestration
// ---------------------------------------------------------------------------

// spawnWaitingReviewers spawns reviewers for any awaiting_review tickets in the
// project. Called synchronously from the worker exit defer to ensure review work
// gets the freed slot before async reconcile picks up new pending tickets.
func (d *Dispatcher) spawnWaitingReviewers(ctx context.Context, projectID string) {
	reviewing, err := d.tickets.ListByState(ctx, projectID, ticket.StateAwaitingValidation)
	if err != nil {
		return
	}
	for _, t := range reviewing {
		if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
			continue
		}
		d.spawnReviewer(ctx, t)
	}
}

// spawnReviewer launches a reviewer agent for a ticket in awaiting_review.
// Reviewers count against the worker capacity limit.
// Skips review if the PR HEAD commit hasn't changed since the last review
// to prevent duplicate reviews when the executor re-submits without new commits.
func (d *Dispatcher) spawnReviewer(ctx context.Context, t *ticket.Ticket) {
	// Guard: don't re-review the same commit. If the PR HEAD hasn't changed
	// since our last review, the executor failed to address feedback — escalate
	// instead of posting another identical review.
	if prURL, ok := t.Outputs["pr_url"].(string); ok && prURL != "" {
		headSHA := d.prHeadCommit(ctx, prURL)
		if headSHA != "" {
			if lastReviewed, ok := t.Outputs["_last_reviewed_commit"].(string); ok && lastReviewed == headSHA {
				slog.Warn("dispatch: skipping review, no new commits since last review",
					"ticket", t.ID, "commit", headSHA)
				d.escalateReviewFailure(ctx, t,
					fmt.Sprintf("Executor re-submitted without new commits (HEAD still %s). "+
						"Review feedback was not addressed.", headSHA[:min(len(headSHA), 12)]))
				return
			}
		}
	}

	reviewKey := "review:" + t.ID

	workerCtx, _, active, limit, started := d.startActive(ctx, reviewKey, t.ProjectID)
	if !started {
		slog.Info("dispatch: at capacity, deferring review", "ticket", t.ID, "project", t.ProjectID, "active", active, "max", limit)
		return
	}

	ticketID := t.ID
	projectID := t.ProjectID
	prURL, _ := t.Outputs["pr_url"].(string)
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			delete(d.active, reviewKey)
			delete(d.activeProjects, reviewKey)
			d.mu.Unlock()

			// Persist the PR HEAD commit that was reviewed so future spawn
			// attempts can detect no-new-commits re-submissions.
			if prURL != "" {
				if headSHA := d.prHeadCommit(ctx, prURL); headSHA != "" {
					d.persistReviewedCommit(ctx, ticketID, projectID, headSHA)
				}
			}

			d.handleReviewerExit(ctx, ticketID)

			go d.reconcile(ctx)
		}()

		if err := d.runReviewer(workerCtx, t); err != nil {
			slog.Error("dispatch: reviewer failed", "ticket", t.ID, "error", err)
		}
	}()

	slog.Info("dispatch: spawned reviewer", "ticket", t.ID, "project", t.ProjectID, "active", active, "max", limit)
}

// prHeadCommit returns the HEAD commit SHA of a PR, or "" on error.
func (d *Dispatcher) prHeadCommit(ctx context.Context, prURL string) string {
	cmd := d.ghCommand(ctx, "pr", "view", prURL, "--json", "headRefOid", "--jq", ".headRefOid")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// persistReviewedCommit records which commit SHA the reviewer last reviewed.
func (d *Dispatcher) persistReviewedCommit(ctx context.Context, ticketID, projectID, commitSHA string) {
	if d.outputPatcher == nil {
		return
	}
	_ = d.outputPatcher.PatchOutputs(ctx, ticketID, map[string]any{
		"_last_reviewed_commit": commitSHA,
	})
}

// runReviewer spawns a validator worker that reviews the ticket's PR and approves or rejects.
func (d *Dispatcher) runReviewer(ctx context.Context, t *ticket.Ticket) error {
	proj, err := d.projects.GetProject(ctx, t.ProjectID)
	if err != nil {
		return err
	}

	// Use the typed validator prompt for consistency.
	depOutputs := make(map[string]map[string]any)
	prompt := AssembleTypedWorkerPrompt(WorkerTypeValidator, proj, t, depOutputs, d.cfg.ServerURL, d.cfg.AgentID, nil)

	// Reviewer works in the repo dir (needs access to the code for `gh` and `make test`).
	// Use the existing worktree if available (the worker's branch), otherwise use
	// the project's isolated clone — never the server's own codebase.
	// For repo-less projects, use a temp directory.
	workDir := d.worktrees.Path(t.ID)
	if workDir == "" {
		if proj.RepoURL == "" {
			workDir, err = os.MkdirTemp("", "flywheel-review-"+t.ID+"-")
			if err != nil {
				return fmt.Errorf("dispatch: reviewer create temp dir: %w", err)
			}
		} else {
			workDir, err = d.resolveProjectRepoDir(ctx, t.ProjectID)
			if err != nil {
				return fmt.Errorf("dispatch: reviewer resolve repo: %w", err)
			}
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}

	slog.Info("dispatch: running validator worker", "ticket", t.ID)

	taskMsg := buildTypedTaskPrompt(WorkerTypeValidator, t.ID, t.ProjectID)
	result, selected, err := d.spawnWorker(ctx, proj, t.ID, t.ProjectID, string(WorkerTypeValidator), WorkerTypeValidator, prompt, taskMsg, workDir)
	if err != nil {
		return err
	}
	d.recordUsage(ctx, selected.Config, t.ProjectID, t.ID, "review", cost.OpReview, prompt, taskMsg, result)

	if !result.Success {
		slog.Error("dispatch: validator completed with error", "ticket", t.ID, "error", result.Error, "output", result.Output)
	} else {
		slog.Info("dispatch: validator completed", "ticket", t.ID)
	}

	return nil
}

// handleReviewerExit is called when a reviewer agent exits. It checks whether
// the reviewer acted (transitioned the ticket) and if not, checks GitHub for
// a review decision. If still no decision, tracks the attempt and escalates
// after maxReviewAttempts failures.
func (d *Dispatcher) handleReviewerExit(ctx context.Context, ticketID string) {
	bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	t, err := d.tickets.GetTicket(bgCtx, ticketID)
	if err != nil || t == nil {
		return
	}
	if t.State != ticket.StateAwaitingValidation {
		return // Reviewer acted (approve/reject already moved the state)
	}

	// Immediate GitHub check — don't wait 60s for reconcile.
	d.reconcileGitHubReviewStatus(bgCtx, []*ticket.Ticket{t})

	// Re-fetch: reconcileGitHubReviewStatus may have transitioned the ticket.
	t, err = d.tickets.GetTicket(bgCtx, ticketID)
	if err != nil || t == nil || t.State != ticket.StateAwaitingValidation {
		return
	}

	// Still stuck — no review on GitHub either. Track the attempt.
	attempts := 0
	if v, ok := t.Outputs["_review_attempts"]; ok {
		switch n := v.(type) {
		case float64:
			attempts = int(n)
		case int:
			attempts = n
		}
	}
	attempts++

	slog.Warn("dispatch: reviewer exited without acting, no GitHub review found",
		"ticket", ticketID, "review_attempts", attempts)

	d.persistReviewAttempt(bgCtx, ticketID, attempts)

	if attempts >= maxReviewAttempts {
		d.escalateReviewFailure(bgCtx, t, fmt.Sprintf(
			"Reviewer failed to post a review %d times. Ticket needs manual review.", attempts))
	}
}

// persistReviewAttempt writes review attempt metadata into the ticket's outputs.
func (d *Dispatcher) persistReviewAttempt(ctx context.Context, ticketID string, attempts int) {
	if d.outputPatcher == nil {
		return
	}
	_ = d.outputPatcher.PatchOutputs(ctx, ticketID, map[string]any{
		"_review_attempts":        attempts,
		"_review_last_attempt_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// escalateReviewFailure publishes an escalation event when review attempts exceed the threshold.
// It records the escalation reason in ticket outputs so the same reason is never published twice
// (survives restarts, unlike in-memory dedup).
func (d *Dispatcher) escalateReviewFailure(ctx context.Context, t *ticket.Ticket, reason string) {
	// Skip if already escalated for this exact reason (persisted in ticket outputs).
	if prev, ok := t.Outputs["_escalation_reason"].(string); ok && prev == reason {
		slog.Debug("dispatch: skipping duplicate escalation", "ticket", t.ID)
		return
	}

	slog.Warn("dispatch: escalating review failure", "ticket", t.ID, "reason", reason)

	// Persist the escalation reason so future reconcile cycles don't re-fire.
	if d.outputPatcher != nil {
		_ = d.outputPatcher.PatchOutputs(ctx, t.ID, map[string]any{
			"_escalation_reason": reason,
		})
		// Update in-memory copy so later calls in the same cycle see it.
		if t.Outputs == nil {
			t.Outputs = make(map[string]any)
		}
		t.Outputs["_escalation_reason"] = reason
	}

	_ = d.bus.Publish(ctx, events.Event{
		Type: events.EventTicketEscalated,
		Payload: map[string]any{
			"ticket_id":  t.ID,
			"project_id": t.ProjectID,
			"reason":     reason,
			"source":     "auto_review",
		},
	})
}

// reconcileGitHubReviewStatus reads the PR review decision from GitHub for
// awaiting_validation tickets. If the PR has been approved or has changes
// requested (by a human or by the reviewer agent), Flywheel acts on it
// regardless of whether the agent called approve/reject via MCP.
func (d *Dispatcher) reconcileGitHubReviewStatus(ctx context.Context, tickets []*ticket.Ticket) {
	if d.ticketTransitioner == nil {
		return
	}
	for _, t := range tickets {
		if ctx.Err() != nil {
			return
		}
		prURL, ok := t.Outputs["pr_url"].(string)
		if !ok || prURL == "" {
			continue
		}

		repoDir, err := d.resolveProjectRepoDir(ctx, t.ProjectID)
		if err != nil {
			continue
		}

		cmd := d.ghCommand(ctx, "pr", "view", prURL,
			"--json", "reviewDecision", "--jq", ".reviewDecision")
		cmd.Dir = repoDir
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		decision := strings.TrimSpace(string(out))

		actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}

		switch decision {
		case "APPROVED":
			slog.Info("dispatch: PR approved on GitHub, auto-approving ticket",
				"ticket", t.ID, "pr_url", prURL)
			if err := d.ticketTransitioner.TransitionTicket(
				ctx, t.ID, ticket.TriggerApprove, actor, nil,
			); err != nil {
				slog.Warn("dispatch: auto-approve from GitHub failed",
					"ticket", t.ID, "error", err)
				continue
			}
			updated, _ := d.tickets.GetTicket(ctx, t.ID)
			if updated != nil {
				d.autoMergePR(ctx, updated, prURL)
			}

		case "CHANGES_REQUESTED":
			slog.Info("dispatch: PR has changes requested on GitHub, auto-rejecting ticket",
				"ticket", t.ID, "pr_url", prURL)
			if err := d.ticketTransitioner.TransitionTicket(
				ctx, t.ID, ticket.TriggerReject, actor,
				map[string]any{"notes": "Changes requested on GitHub PR review"},
			); err != nil {
				slog.Warn("dispatch: auto-reject from GitHub failed",
					"ticket", t.ID, "error", err)
				continue
			}
			updated, _ := d.tickets.GetTicket(ctx, t.ID)
			if updated != nil {
				d.spawn(ctx, updated)
			}
		}
	}
}

// reconcileOrphanedPRs checks draft tickets for open PRs that exist on GitHub
// (e.g. from a rollback after submit). If found, recovers the ticket to
// awaiting_validation and spawns a reviewer.
func (d *Dispatcher) reconcileOrphanedPRs(ctx context.Context, drafts []*ticket.Ticket) map[string]bool {
	recovered := make(map[string]bool)
	if d.ticketTransitioner == nil {
		return recovered
	}
	for _, t := range drafts {
		if ctx.Err() != nil {
			return recovered
		}
		if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
			continue
		}

		prURL, _ := t.Outputs["pr_url"].(string)

		// If no pr_url but worktree exists, check GitHub for PR on this branch.
		if prURL == "" && d.worktrees.Path(t.ID) != "" {
			repoDir, err := d.resolveProjectRepoDir(ctx, t.ProjectID)
			if err != nil {
				continue
			}
			branch := d.branchForTicket(ctx, t)
			cmd := d.ghCommand(ctx, "pr", "list", "--head", branch,
				"--state", "open", "--json", "url", "--jq", ".[0].url")
			cmd.Dir = repoDir
			out, err := cmd.Output()
			if err != nil {
				continue
			}
			prURL = strings.TrimSpace(string(out))
		}

		if !isValidPRURL(prURL) {
			continue
		}

		// Verify PR is still open.
		repoDir, err := d.resolveProjectRepoDir(ctx, t.ProjectID)
		if err != nil {
			continue
		}
		cmd := d.ghCommand(ctx, "pr", "view", prURL, "--json", "state", "--jq", ".state")
		cmd.Dir = repoDir
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(out)) != "OPEN" {
			continue
		}

		slog.Info("dispatch: draft ticket has open PR, recovering to review",
			"ticket", t.ID, "pr_url", prURL)
		if d.outputPatcher != nil {
			_ = d.outputPatcher.PatchOutputs(ctx, t.ID, map[string]any{"pr_url": prURL})
		}
		actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}
		if err := d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerRecoverPR, actor, nil); err != nil {
			slog.Warn("dispatch: orphan PR recovery failed", "ticket", t.ID, "error", err)
			continue
		}
		updated, _ := d.tickets.GetTicket(ctx, t.ID)
		if updated != nil {
			d.spawnReviewer(ctx, updated)
		}
		recovered[t.ID] = true
	}
	return recovered
}

// ---------------------------------------------------------------------------
// PR reconciliation (combined state + review check)
// ---------------------------------------------------------------------------

// reconcileAwaitingValidationPRs performs a single pass over awaiting_validation
// tickets, fetching both state and reviewDecision in one gh call per PR.
// This replaces the old reconcileExternalMerges + reconcileGitHubReviewStatus.
func (d *Dispatcher) reconcileAwaitingValidationPRs(ctx context.Context, reviewing []*ticket.Ticket) {
	if d.ticketTransitioner == nil {
		return
	}
	for _, t := range reviewing {
		if ctx.Err() != nil {
			return
		}
		if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
			continue
		}
		prURL, ok := t.Outputs["pr_url"].(string)
		if !ok || prURL == "" {
			continue
		}

		// Single gh call fetches both state and reviewDecision.
		cmd := d.ghCommand(ctx, "pr", "view", prURL,
			"--json", "state,reviewDecision",
			"--jq", "[.state, .reviewDecision] | @tsv")
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		parts := strings.Split(strings.TrimSpace(string(out)), "\t")
		prState := ""
		reviewDecision := ""
		if len(parts) >= 1 {
			prState = parts[0]
		}
		if len(parts) >= 2 {
			reviewDecision = parts[1]
		}

		actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}

		// Handle externally-merged PRs (was reconcileExternalMerges).
		if prState == "MERGED" {
			slog.Info("dispatch: PR merged externally while awaiting review, auto-advancing ticket",
				"ticket", t.ID, "pr_url", prURL)
			if err := d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerApprove, actor, nil); err != nil {
				slog.Warn("dispatch: auto-approve for external merge failed", "ticket", t.ID, "error", err)
				continue
			}
			updated, err := d.tickets.GetTicket(ctx, t.ID)
			if err != nil {
				slog.Warn("dispatch: re-fetch after auto-approve failed", "ticket", t.ID, "error", err)
				continue
			}
			d.persistMergeState(ctx, t.ID, 0, "merged", "PR merged externally before review")
			d.cleanupTicketBranch(ctx, t.ID, t.ProjectID)
			d.publishMergedEvent(ctx, t, prURL)
			d.closeMergedTicket(ctx, updated)
			continue
		}

		// Handle GitHub review decisions (was reconcileGitHubReviewStatus).
		switch reviewDecision {
		case "APPROVED":
			slog.Info("dispatch: PR approved on GitHub, auto-approving ticket",
				"ticket", t.ID, "pr_url", prURL)
			if err := d.ticketTransitioner.TransitionTicket(
				ctx, t.ID, ticket.TriggerApprove, actor, nil,
			); err != nil {
				slog.Warn("dispatch: auto-approve from GitHub failed",
					"ticket", t.ID, "error", err)
				continue
			}
			updated, _ := d.tickets.GetTicket(ctx, t.ID)
			if updated != nil {
				d.autoMergePR(ctx, updated, prURL)
			}

		case "CHANGES_REQUESTED":
			slog.Info("dispatch: PR has changes requested on GitHub, auto-rejecting ticket",
				"ticket", t.ID, "pr_url", prURL)
			if err := d.ticketTransitioner.TransitionTicket(
				ctx, t.ID, ticket.TriggerReject, actor,
				map[string]any{"notes": "Changes requested on GitHub PR review"},
			); err != nil {
				slog.Warn("dispatch: auto-reject from GitHub failed",
					"ticket", t.ID, "error", err)
				continue
			}
			updated, _ := d.tickets.GetTicket(ctx, t.ID)
			if updated != nil {
				d.spawn(ctx, updated)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Merge automation
// ---------------------------------------------------------------------------

// isValidPRURL returns true if the string looks like an actual PR URL.
func isValidPRURL(prURL string) bool {
	return strings.HasPrefix(prURL, "http://") || strings.HasPrefix(prURL, "https://")
}

// autoMergePR merges the PR after a ticket is approved/validated.
// It first validates that CI checks pass, then merges. If the merge fails due
// to conflicts, it spawns a conflict resolver worker. On success, it advances
// the ticket through deploying → observing → closed.
func (d *Dispatcher) autoMergePR(ctx context.Context, t *ticket.Ticket, prURL string) {
	// State guard: only merge from validated state. Prevents re-entrancy when
	// ticket.closed event re-enters handleTicketDone.
	if t.State != ticket.StateValidated {
		return
	}

	if !isValidPRURL(prURL) {
		slog.Warn("dispatch: skipping merge, invalid pr_url", "ticket", t.ID, "pr_url", prURL)
		return
	}

	// Read merge attempts from persisted outputs (survives restarts).
	attempts := 0
	if v, ok := t.Outputs["_merge_attempts"]; ok {
		switch n := v.(type) {
		case float64:
			attempts = int(n)
		case int:
			attempts = n
		}
	}
	if attempts >= maxMergeAttempts {
		d.escalateMergeFailure(ctx, t, fmt.Sprintf("merge failed %d times", attempts))
		return
	}

	// Check if the PR is already merged before attempting any work.
	// gh pr view works with just a URL — no local clone needed.
	checkCmd := d.ghCommand(ctx, "pr", "view", prURL, "--json", "state", "--jq", ".state")
	if stateOut, checkErr := checkCmd.Output(); checkErr == nil {
		prState := strings.TrimSpace(string(stateOut))
		if prState == "MERGED" {
			slog.Info("dispatch: PR already merged, closing ticket", "ticket", t.ID)
			d.persistMergeState(ctx, t.ID, attempts, "merged", "")
			d.cleanupTicketBranch(ctx, t.ID, t.ProjectID)
			d.publishMergedEvent(ctx, t, prURL)
			d.closeMergedTicket(ctx, t)
			return
		}
		if prState == "CLOSED" {
			slog.Warn("dispatch: PR closed without merge, skipping", "ticket", t.ID)
			d.persistMergeState(ctx, t.ID, attempts, "pr_closed", "PR was closed without merging")
			return
		}
	}

	d.persistMergeState(ctx, t.ID, attempts, "merging", "")

	_ = d.worktrees.Remove(t.ID)

	repoDir, err := d.resolveProjectRepoDir(ctx, t.ProjectID)
	if err != nil {
		newAttempts := attempts + 1
		slog.Error("dispatch: cannot resolve repo for auto-merge", "ticket", t.ID, "error", err, "attempt", newAttempts)
		d.persistMergeState(ctx, t.ID, newAttempts, "repo_error", truncate(err.Error(), 500))
		if newAttempts >= maxMergeAttempts {
			d.escalateMergeFailure(ctx, t, fmt.Sprintf("cannot resolve repo after %d attempts: %v", newAttempts, err))
		}
		return
	}

	// Validate CI checks before attempting merge.
	if !d.validatePRChecks(ctx, t, prURL, repoDir) {
		newAttempts := attempts + 1
		d.persistMergeState(ctx, t.ID, newAttempts, "checks_failing", "CI checks not passing")
		if newAttempts >= maxMergeAttempts {
			d.escalateMergeFailure(ctx, t, fmt.Sprintf("CI checks not passing after %d attempts", newAttempts))
		}
		return
	}

	cmd := d.ghCommand(ctx, "pr", "merge", prURL, "--squash")
	cmd.Dir = repoDir
	out, mergeErr := cmd.CombinedOutput()
	if mergeErr != nil {
		output := string(out)
		slog.Error("dispatch: auto-merge failed", "ticket", t.ID, "error", mergeErr, "output", output)

		newAttempts := attempts + 1
		status := "conflict"
		d.persistMergeState(ctx, t.ID, newAttempts, status, truncate(output, 500))

		if strings.Contains(output, "not mergeable") || strings.Contains(output, "CONFLICT") || strings.Contains(output, "cannot be cleanly created") {
			d.spawnConflictResolver(ctx, t, prURL)
		}
	} else {
		slog.Info("dispatch: auto-merged PR", "ticket", t.ID)
		d.persistMergeState(ctx, t.ID, attempts, "merged", "")

		// Delete remote branch (best-effort) using persisted branch name.
		d.cleanupTicketBranch(ctx, t.ID, t.ProjectID)

		d.publishMergedEvent(ctx, t, prURL)
		d.closeMergedTicket(ctx, t)
	}
}

// publishMergedEvent emits a ticket.merged event so the orchestrator knows a PR landed.
func (d *Dispatcher) publishMergedEvent(ctx context.Context, t *ticket.Ticket, prURL string) {
	_ = d.bus.Publish(ctx, events.Event{
		Type: events.EventTicketMerged,
		Payload: map[string]any{
			"ticket_id":  t.ID,
			"project_id": t.ProjectID,
			"pr_url":     prURL,
		},
	})
}

// closeMergedTicket advances a ticket through the post-merge lifecycle.
// Instead of a synchronous for-loop, advances the current phase with outcome
// "success" (which sets the next phase to status=ready), then processes one
// phase synchronously as a fast path. Remaining phases are picked up by the
// reconcile loop via processReadyWorkflowPhases.
// Nil-safe: if ticketTransitioner is nil, logs and returns.
func (d *Dispatcher) closeMergedTicket(ctx context.Context, t *ticket.Ticket) {
	if d.ticketTransitioner == nil {
		slog.Warn("dispatch: ticket transitioner not set, cannot close merged ticket", "ticket", t.ID)
		return
	}

	// Workflow-aware: advance the current phase, then process the next one.
	if t.WorkflowID != "" && d.workflowEngine != nil && t.WorkflowPhase != "" {
		next := d.advanceWorkflowIfNeeded(ctx, t, "success")
		if next == nil {
			// Workflow complete — close the ticket.
			actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}
			if err := d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerClose, actor, nil); err != nil {
				slog.Warn("dispatch: close after workflow complete failed", "ticket", t.ID, "error", err)
			}
			slog.Info("dispatch: ticket closed after merge (workflow complete)", "ticket", t.ID)
			return
		}
		// Fast path: process the ready phase immediately.
		d.processReadyPhase(ctx, t)
		slog.Info("dispatch: ticket advanced through workflow after merge", "ticket", t.ID)
		return
	}

	// Legacy path: close validated ticket after merge.
	actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}
	if err := d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerClose, actor, nil); err != nil {
		slog.Error("dispatch: post-merge close failed", "ticket", t.ID, "error", err)
		return
	}
	slog.Info("dispatch: ticket closed after merge", "ticket", t.ID)
}

// ---------------------------------------------------------------------------
// Merge helpers
// ---------------------------------------------------------------------------

// escalateMergeFailure publishes an escalation event when merge attempts exceed the threshold.
// The ticket stays in validated state for manual intervention.
func (d *Dispatcher) escalateMergeFailure(ctx context.Context, t *ticket.Ticket, reason string) {
	slog.Warn("dispatch: escalating merge failure", "ticket", t.ID, "reason", reason)
	d.persistMergeState(ctx, t.ID, maxMergeAttempts, "escalated", reason)
	_ = d.bus.Publish(ctx, events.Event{
		Type: events.EventTicketEscalated,
		Payload: map[string]any{
			"ticket_id":  t.ID,
			"project_id": t.ProjectID,
			"reason":     reason,
			"source":     "auto_merge",
		},
	})
}

// persistMergeState writes merge metadata into the ticket's outputs JSONB.
// Keys are prefixed with _ to distinguish system metadata from worker outputs.
func (d *Dispatcher) persistMergeState(ctx context.Context, ticketID string, attempts int, status, lastErr string) {
	if d.outputPatcher == nil {
		return
	}
	_ = d.outputPatcher.PatchOutputs(ctx, ticketID, map[string]any{
		"_merge_attempts":       attempts,
		"_merge_status":         status,
		"_merge_last_error":     lastErr,
		"_merge_last_attempt_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// branchForTicket returns the branch name for a ticket, using the project's
// GitPolicy if configured, falling back to "ticket/<id>".
func (d *Dispatcher) branchForTicket(ctx context.Context, t *ticket.Ticket) string {
	if d.projects != nil {
		if proj, err := d.projects.GetProject(ctx, t.ProjectID); err == nil && proj != nil {
			if gp := proj.DispatchConfig.Normalized().GitPolicy; gp != nil {
				return gp.BranchNameForTicket(t.ID)
			}
		}
	}
	return "ticket/" + t.ID
}

// branchFromOutputs reads the persisted branch name from ticket outputs,
// falling back to "ticket/<id>" if absent.
func branchFromOutputs(t *ticket.Ticket) string {
	if b, ok := t.Outputs["_branch"].(string); ok && b != "" {
		return b
	}
	return "ticket/" + t.ID
}

// cleanupTicketBranch removes the worktree and deletes the remote branch.
// Only called for safe terminal states (cancelled, merged).
func (d *Dispatcher) cleanupTicketBranch(ctx context.Context, ticketID, projectID string) {
	_ = d.worktrees.Remove(ticketID)

	repoDir, err := d.resolveProjectRepoDir(ctx, projectID)
	if err != nil {
		return
	}

	// Read branch name from outputs; fall back to convention.
	branch := "ticket/" + ticketID
	if t, err := d.tickets.GetTicket(ctx, ticketID); err == nil && t != nil {
		branch = branchFromOutputs(t)
	}

	cmd := exec.Command("git", "push", "origin", "--delete", branch)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "remote ref does not exist") {
		slog.Warn("dispatch: branch cleanup failed", "ticket", ticketID, "branch", branch, "error", err)
	}
}

// validatePRChecks verifies CI checks pass on the PR before merging.
// Returns true if checks pass (or no checks exist), false if failing/pending.
// Emits EventTestsFailed when checks fail.
// Uses policy.ParseGHPRChecks for the actual `gh pr checks` parsing.
func (d *Dispatcher) validatePRChecks(ctx context.Context, t *ticket.Ticket, prURL, repoDir string) bool {
	status, reason := policy.ParseGHPRChecks(prURL)
	switch status {
	case policy.ChecksFailed:
		slog.Warn("dispatch: CI checks failed", "ticket", t.ID, "reason", reason)
		_ = d.bus.Publish(ctx, events.Event{
			Type: events.EventTestsFailed,
			Payload: map[string]any{
				"ticket_id":  t.ID,
				"project_id": t.ProjectID,
				"pr_url":     prURL,
				"output":     reason,
			},
		})
		return false
	case policy.ChecksPending:
		slog.Info("dispatch: CI checks pending, will retry later", "ticket", t.ID)
		return false
	default:
		return true
	}
}

// truncate returns s truncated to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// ---------------------------------------------------------------------------
// Conflict resolution
// ---------------------------------------------------------------------------

// spawnConflictResolver launches a worker to rebase a PR branch onto main and resolve conflicts.
func (d *Dispatcher) spawnConflictResolver(ctx context.Context, t *ticket.Ticket, prURL string) {
	resolveKey := "resolve:" + t.ID

	workerCtx, _, active, limit, started := d.startActive(ctx, resolveKey, t.ProjectID)
	if !started {
		slog.Info("dispatch: at capacity, deferring conflict resolution", "ticket", t.ID, "project", t.ProjectID, "active", active, "max", limit)
		return
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			delete(d.active, resolveKey)
			delete(d.activeProjects, resolveKey)
			d.mu.Unlock()
			go d.reconcile(ctx)
		}()

		if err := d.runConflictResolver(workerCtx, t, prURL); err != nil {
			slog.Error("dispatch: conflict resolver failed", "ticket", t.ID, "error", err)
		}
	}()

	slog.Info("dispatch: spawned conflict resolver", "ticket", t.ID, "project", t.ProjectID, "active", active, "max", limit)
}

// runConflictResolver rebases a ticket's branch onto main and retries the merge.
func (d *Dispatcher) runConflictResolver(ctx context.Context, t *ticket.Ticket, prURL string) error {
	branch := branchFromOutputs(t)

	proj, err := d.projects.GetProject(ctx, t.ProjectID)
	if err != nil {
		return err
	}

	// Resolve the repo dir from the project (not the server's cwd).
	repoDir, err := d.resolveProjectRepoDir(ctx, t.ProjectID)
	if err != nil {
		return fmt.Errorf("resolve repo for conflict resolver: %w", err)
	}

	workDir, err := d.worktrees.CreateFromRepo(t.ID, branch, repoDir, proj.DefaultBranch)
	if err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}

	prompt := assembleConflictResolverPrompt(t, prURL, branch)
	taskMsg := fmt.Sprintf(
		"Rebase branch %s onto main and resolve any merge conflicts. "+
			"Then force-push the result. The goal is to make PR %s mergeable.",
		branch, prURL,
	)

	result, selected, err := d.spawnWorker(ctx, proj, t.ID, t.ProjectID, WorkerRoleConflictResolver, WorkerType(WorkerRoleConflictResolver), prompt, taskMsg, workDir)
	if err != nil {
		return err
	}
	d.recordUsage(ctx, selected.Config, t.ProjectID, t.ID, "conflict_resolution", cost.OpCodeGeneration, prompt, taskMsg, result)

	if !result.Success {
		slog.Error("dispatch: conflict resolver failed", "ticket", t.ID, "error", result.Error, "output", result.Output)
		return fmt.Errorf("resolver failed: %s", result.Error)
	}

	slog.Info("dispatch: conflict resolver completed, retrying merge", "ticket", t.ID)

	_ = d.worktrees.Remove(t.ID)
	cmd := d.ghCommand(ctx, "pr", "merge", prURL, "--squash")
	cmd.Dir = repoDir
	out, mergeErr := cmd.CombinedOutput()
	if mergeErr != nil {
		output := string(out)
		slog.Error("dispatch: retry merge still failed", "ticket", t.ID, "error", mergeErr, "output", output)

		// Read current attempts from outputs for accurate count.
		attempts := 0
		if fresh, ferr := d.tickets.GetTicket(ctx, t.ID); ferr == nil && fresh != nil {
			if v, ok := fresh.Outputs["_merge_attempts"]; ok {
				switch n := v.(type) {
				case float64:
					attempts = int(n)
				case int:
					attempts = n
				}
			}
		}
		d.persistMergeState(ctx, t.ID, attempts+1, "conflict", truncate(output, 500))

		return fmt.Errorf("retry merge: %w", mergeErr)
	}

	slog.Info("dispatch: auto-merged PR after conflict resolution", "ticket", t.ID)
	d.persistMergeState(ctx, t.ID, 0, "merged", "")
	d.cleanupTicketBranch(ctx, t.ID, t.ProjectID)

	d.publishMergedEvent(ctx, t, prURL)
	d.closeMergedTicket(ctx, t)
	return nil
}
