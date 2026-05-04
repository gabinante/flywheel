package dispatch

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// WorktreeManager handles git worktree lifecycle for concurrent ticket execution.
type WorktreeManager struct {
	BaseDir string // base directory for worktrees (e.g. /tmp/flywheel-worktrees)
	RepoDir string // path to the main git repository (legacy single-repo)
}

// Create creates a git worktree for a ticket branch and returns the worktree path.
// The branch is created from the current HEAD if it doesn't exist.
func (m *WorktreeManager) Create(ticketID, branch string) (string, error) {
	return m.CreateFromRepo(ticketID, branch, m.RepoDir, "main")
}

// CreateFromRepo creates a git worktree for a ticket branch from a specific repo directory.
// This supports multi-repo projects where different tickets target different repos.
// defaultBranch is the project's target branch (e.g. "main") used for ancestry validation.
func (m *WorktreeManager) CreateFromRepo(ticketID, branch, repoDir, defaultBranch string) (string, error) {
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	dir := m.worktreePath(ticketID)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", fmt.Errorf("worktree mkdir: %w", err)
	}

	// Prune stale worktree entries first.
	prune := exec.Command("git", "worktree", "prune")
	prune.Dir = repoDir
	_ = prune.Run()

	// Fetch latest refs before creating worktree to avoid stale/orphaned branches.
	fetch := exec.Command("git", "fetch", "origin")
	fetch.Dir = repoDir
	_ = fetch.Run()

	// Remove existing directory if present (leftover from crash).
	_ = os.RemoveAll(dir)

	// Try creating a new branch; if it already exists, just check it out.
	cmd := exec.Command("git", "worktree", "add", "-b", branch, dir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		// Branch may already exist — try without -b, with -f to force.
		cmd2 := exec.Command("git", "worktree", "add", "-f", dir, branch)
		cmd2.Dir = repoDir
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			return "", fmt.Errorf("worktree add: %s / %s: %w", strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)), err2)
		}
	}

	// Validate the new worktree shares history with the target branch.
	if err := ValidateAncestry(dir, "origin/"+defaultBranch); err != nil {
		// Cleanup the invalid worktree.
		_ = os.RemoveAll(dir)
		pruneCleanup := exec.Command("git", "worktree", "prune")
		pruneCleanup.Dir = repoDir
		_ = pruneCleanup.Run()
		return "", err
	}

	return dir, nil
}

// ValidateAncestry checks that the worktree HEAD shares common history with the
// given remote branch. Returns an error if the branches are disconnected (e.g. the
// local clone was initialized from the wrong repo or is corrupted).
func ValidateAncestry(worktreeDir, remoteBranch string) error {
	cmd := exec.Command("git", "merge-base", "HEAD", remoteBranch)
	cmd.Dir = worktreeDir
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("branch HEAD has no common history with %s — local clone may be corrupted or initialized from wrong repo", remoteBranch)
	}
	return nil
}

// Remove cleans up a worktree for a ticket. Tries the primary repo first,
// then falls back to searching all known clone dirs.
func (m *WorktreeManager) Remove(ticketID string) error {
	return m.RemoveFromRepo(ticketID, m.RepoDir)
}

// RemoveFromRepo cleans up a worktree for a ticket using the specified repo dir.
func (m *WorktreeManager) RemoveFromRepo(ticketID, repoDir string) error {
	dir := m.worktreePath(ticketID)
	cmd := exec.Command("git", "worktree", "remove", "--force", dir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree remove: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Path returns the worktree directory for a ticket if it exists, or empty string.
func (m *WorktreeManager) Path(ticketID string) string {
	dir := m.worktreePath(ticketID)
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	return ""
}

func (m *WorktreeManager) worktreePath(ticketID string) string {
	// Sanitize ticket ID for filesystem use (e.g. "proj-42" → "proj-42").
	safe := strings.ReplaceAll(ticketID, "/", "-")
	return filepath.Join(m.BaseDir, safe)
}

// MultiRepoCloneManager manages local clones for multi-repo projects.
// When a ticket targets a non-primary repo, we need a local clone to create worktrees from.
type MultiRepoCloneManager struct {
	BaseDir string // base directory for clones (e.g. /tmp/flywheel-clones)
	mu      sync.Mutex
}

// NewMultiRepoCloneManager creates a new clone manager.
func NewMultiRepoCloneManager(baseDir string) *MultiRepoCloneManager {
	return &MultiRepoCloneManager{BaseDir: baseDir}
}

// ResetClone removes an existing clone directory so the next EnsureClone call
// will perform a fresh clone. Used to recover from corrupted or mismatched clones.
func (m *MultiRepoCloneManager) ResetClone(alias string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	safe := strings.ReplaceAll(alias, "/", "-")
	dir := filepath.Join(m.BaseDir, safe)

	slog.Warn("multi-repo: resetting clone", "alias", alias, "dir", dir)
	return os.RemoveAll(dir)
}

// currentRemoteURL returns the current origin remote URL for a git repo directory.
// Returns empty string on any error.
func currentRemoteURL(dir string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// EnsureClone ensures a local clone exists for the given repo URL and returns its path.
// If the clone already exists, it fetches latest changes. If the remote URL has changed
// (e.g. project repo_url was updated), the stale clone is removed and re-cloned.
func (m *MultiRepoCloneManager) EnsureClone(repoURL, alias string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Use alias as directory name for readability.
	safe := strings.ReplaceAll(alias, "/", "-")
	dir := filepath.Join(m.BaseDir, safe)

	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		// Clone exists — check if the remote URL still matches.
		if existing := currentRemoteURL(dir); existing != "" && existing != repoURL {
			slog.Warn("multi-repo: repo URL changed, removing stale clone",
				"alias", alias, "old_url", existing, "new_url", repoURL)
			if err := os.RemoveAll(dir); err != nil {
				return "", fmt.Errorf("remove stale clone: %w", err)
			}
			// Fall through to fresh clone below.
		} else {
			// URL matches (or couldn't be determined) — fetch latest.
			fetch := exec.Command("git", "fetch", "--all")
			fetch.Dir = dir
			if out, err := fetch.CombinedOutput(); err != nil {
				slog.Warn("multi-repo fetch failed", "alias", alias, "output", strings.TrimSpace(string(out)), "error", err)
			}
			return dir, nil
		}
	}

	// Clone the repo.
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", fmt.Errorf("clone mkdir: %w", err)
	}

	cmd := exec.Command("git", "clone", repoURL, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("clone %s: %s: %w", repoURL, strings.TrimSpace(string(out)), err)
	}

	return dir, nil
}
