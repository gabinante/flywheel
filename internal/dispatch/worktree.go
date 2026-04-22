package dispatch

import (
	"fmt"
	"log"
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
	return m.CreateFromRepo(ticketID, branch, m.RepoDir)
}

// CreateFromRepo creates a git worktree for a ticket branch from a specific repo directory.
// This supports multi-repo projects where different tickets target different repos.
func (m *WorktreeManager) CreateFromRepo(ticketID, branch, repoDir string) (string, error) {
	dir := m.worktreePath(ticketID)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", fmt.Errorf("worktree mkdir: %w", err)
	}

	// Prune stale worktree entries first.
	prune := exec.Command("git", "worktree", "prune")
	prune.Dir = repoDir
	_ = prune.Run()

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
	return dir, nil
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

// EnsureClone ensures a local clone exists for the given repo URL and returns its path.
// If the clone already exists, it fetches latest changes.
func (m *MultiRepoCloneManager) EnsureClone(repoURL, alias string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Use alias as directory name for readability.
	safe := strings.ReplaceAll(alias, "/", "-")
	dir := filepath.Join(m.BaseDir, safe)

	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		// Clone exists — fetch latest.
		fetch := exec.Command("git", "fetch", "--all")
		fetch.Dir = dir
		if out, err := fetch.CombinedOutput(); err != nil {
			log.Printf("multi-repo: fetch %s: %s: %v", alias, strings.TrimSpace(string(out)), err)
		}
		return dir, nil
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
