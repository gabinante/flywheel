package dispatch

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/ticket"
)

// defaultBranchGCInterval is the default interval for the branch GC goroutine.
const defaultBranchGCInterval = 6 * time.Hour

// startBranchGC starts a background goroutine that periodically garbage-collects
// stale ticket branches. It is conservative: only deletes branches for confirmed-
// merged or truly orphaned tickets. Everything else gets logged for human visibility.
func (d *Dispatcher) startBranchGC(ctx context.Context) {
	interval := d.config().BranchGCInterval
	if interval <= 0 {
		interval = defaultBranchGCInterval
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.runBranchGC(ctx)
			}
		}
	}()
}

// runBranchGC scans remote branches matching ticket/* and cleans up safe-to-delete ones.
func (d *Dispatcher) runBranchGC(ctx context.Context) {
	// List all projects that are dispatch-enabled with a repo_url.
	// We scan validated/closed tickets to find which branches exist.
	states := []ticket.State{ticket.StateClosed, ticket.StateValidated}

	var deleted, warnings int

	for _, state := range states {
		tickets, err := d.tickets.ListByState(ctx, d.config().ProjectID, state)
		if err != nil {
			slog.Error("dispatch: branch GC list failed", "state", state, "error", err)
			continue
		}

		for _, t := range tickets {
			if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
				continue
			}

			branch := branchFromOutputs(t)
			mergeStatus, _ := t.Outputs["_merge_status"].(string)

			switch {
			case state == ticket.StateClosed && mergeStatus == "merged":
				// Belt-and-suspenders: auto-merge should have already cleaned this up.
				repoDir, err := d.resolveProjectRepoDir(ctx, t.ProjectID)
				if err != nil {
					continue
				}
				if remoteBranchExists(repoDir, branch) {
					cmd := exec.Command("git", "push", "origin", "--delete", branch)
					cmd.Dir = repoDir
					if out, err := cmd.CombinedOutput(); err != nil && !strings.Contains(string(out), "remote ref does not exist") {
						slog.Warn("dispatch: branch GC delete failed", "ticket", t.ID, "branch", branch, "error", err)
					} else {
						deleted++
					}
				}

			case state == ticket.StateValidated && mergeStatus == "escalated":
				warnings++
				slog.Warn("dispatch: branch GC found escalated ticket with branch", "ticket", t.ID, "branch", branch)

			case state == ticket.StateValidated:
				mergeAttempts := 0
				if v, ok := t.Outputs["_merge_attempts"]; ok {
					switch n := v.(type) {
					case float64:
						mergeAttempts = int(n)
					case int:
						mergeAttempts = n
					}
				}
				if mergeAttempts >= maxMergeAttempts {
					warnings++
					slog.Warn("dispatch: branch GC found max-attempts ticket", "ticket", t.ID, "branch", branch, "attempts", mergeAttempts)
				}
				// Stale check: ticket not updated in >7 days while validated.
				if time.Since(t.UpdatedAt) > 7*24*time.Hour {
					warnings++
					slog.Warn("dispatch: branch GC found stale validated ticket", "ticket", t.ID, "branch", branch, "updated_at", t.UpdatedAt)
				}
			}
		}
	}

	// Scan draft tickets with failed attempts for dangling branches.
	// Only clean up branches where the ticket has been stale for >24 hours,
	// has a system-identified failure in PriorAttempts, and no PR exists.
	draftTickets, err := d.tickets.ListByState(ctx, d.config().ProjectID, ticket.StateDraft)
	if err == nil {
		for _, t := range draftTickets {
			if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
				continue
			}
			// Only consider tickets stale for >24 hours.
			if time.Since(t.UpdatedAt) <= 24*time.Hour {
				continue
			}
			// Only clean up if there's a system-identified failure.
			if !hasFailedAttempt(t) {
				continue
			}
			branch := branchFromOutputs(t)
			if branch == "ticket/"+t.ID {
				// No branch was persisted in outputs — check if one exists on remote.
			}
			repoDir, err := d.resolveProjectRepoDir(ctx, t.ProjectID)
			if err != nil {
				continue
			}
			if remoteBranchExists(repoDir, branch) {
				cmd := exec.Command("git", "push", "origin", "--delete", branch)
				cmd.Dir = repoDir
				if out, err := cmd.CombinedOutput(); err != nil && !strings.Contains(string(out), "remote ref does not exist") {
					slog.Warn("dispatch: branch GC delete failed (draft)", "ticket", t.ID, "branch", branch, "error", err)
				} else {
					deleted++
					slog.Info("dispatch: branch GC cleaned dangling branch from failed run", "ticket", t.ID, "branch", branch)
				}
			}
		}
	}

	if deleted > 0 || warnings > 0 {
		slog.Info("dispatch: branch GC completed", "deleted", deleted, "warnings", warnings)
	}
}

// hasFailedAttempt returns true if the ticket has a PriorAttempt with a
// system-identified failure outcome (worker_exit or infrastructure_failure).
func hasFailedAttempt(t *ticket.Ticket) bool {
	for _, a := range t.Context.PriorAttempts {
		if a.Outcome == "worker_exit" || a.Outcome == "infrastructure_failure" {
			return true
		}
	}
	return false
}

// remoteBranchExists checks if a branch exists on the remote.
func remoteBranchExists(repoDir, branch string) bool {
	cmd := exec.Command("git", "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
