package dispatch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeManager handles git worktree lifecycle for concurrent ticket execution.
type WorktreeManager struct {
	BaseDir string // base directory for worktrees (e.g. /tmp/warrant-worktrees)
	RepoDir string // path to the main git repository
}

// Create creates a git worktree for a ticket branch and returns the worktree path.
// The branch is created from the current HEAD if it doesn't exist.
func (m *WorktreeManager) Create(ticketID, branch string) (string, error) {
	dir := m.worktreePath(ticketID)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", fmt.Errorf("worktree mkdir: %w", err)
	}

	// Prune stale worktree entries first.
	prune := exec.Command("git", "worktree", "prune")
	prune.Dir = m.RepoDir
	_ = prune.Run()

	// Remove existing directory if present (leftover from crash).
	_ = os.RemoveAll(dir)

	// Try creating a new branch; if it already exists, just check it out.
	cmd := exec.Command("git", "worktree", "add", "-b", branch, dir)
	cmd.Dir = m.RepoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		// Branch may already exist — try without -b, with -f to force.
		cmd2 := exec.Command("git", "worktree", "add", "-f", dir, branch)
		cmd2.Dir = m.RepoDir
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			return "", fmt.Errorf("worktree add: %s / %s: %w", strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)), err2)
		}
	}
	return dir, nil
}

// Remove cleans up a worktree for a ticket.
func (m *WorktreeManager) Remove(ticketID string) error {
	dir := m.worktreePath(ticketID)
	cmd := exec.Command("git", "worktree", "remove", "--force", dir)
	cmd.Dir = m.RepoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree remove: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (m *WorktreeManager) worktreePath(ticketID string) string {
	// Sanitize ticket ID for filesystem use (e.g. "proj-42" → "proj-42").
	safe := strings.ReplaceAll(ticketID, "/", "-")
	return filepath.Join(m.BaseDir, safe)
}
