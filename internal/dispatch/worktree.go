package dispatch

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/gabinante/flywheel/internal/gitworkspace"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WorktreeManager handles git worktree lifecycle for concurrent ticket execution.
//
// Worktrees follow the operator's convention: <BaseDir>/<repo>-worktrees/<name>, where
// BaseDir is the code root (e.g. ~/git), <repo> is the repository's directory name, and
// <name> is the ticket's branch slug (e.g. rlep-3488-review-fixes). Legacy callers that
// only know the ticket ID still resolve through the recorded mapping.
type WorktreeManager struct {
	BaseDir string // code root (e.g. ~/git); worktrees live under <BaseDir>/<repo>-worktrees/
	RepoDir string // path to the main git repository (legacy single-repo)

	mu    sync.Mutex
	paths map[string]string // ticket ID → worktree dir created in this process
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

	if dir := m.lookup(ticketID); dir != "" {
		return dir, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	fetch := exec.CommandContext(ctx, "git", "fetch", "origin", defaultBranch)
	fetch.Dir = repoDir
	if out, err := fetch.CombinedOutput(); err != nil {
		return "", fmt.Errorf("fetch base: %s: %w", out, err)
	}
	base := "origin/" + defaultBranch
	ref := exec.CommandContext(ctx, "git", "rev-parse", "--verify", base+"^{commit}")
	ref.Dir = repoDir
	sha, err := ref.Output()
	if err != nil {
		return "", fmt.Errorf("resolve base: %w", err)
	}
	desired := m.worktreePathFor(ticketID, branch, repoDir)
	dir, err := gitworkspace.CreateAt(ctx, repoDir, filepath.Dir(desired), filepath.Base(desired), ticketID, strings.TrimSpace(string(sha)), branch)
	if err != nil {
		return "", err
	}
	m.remember(ticketID, dir)
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
	dir := m.lookup(ticketID)
	if dir == "" {
		dir = m.worktreePath(ticketID)
	}
	if err := gitworkspace.Remove(context.Background(), dir); err != nil {
		return err
	}
	m.forget(ticketID)
	return nil
}

// Path returns the worktree directory for a ticket if it exists, or empty string.
func (m *WorktreeManager) Path(ticketID string) string {
	if dir := m.lookup(ticketID); dir != "" {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	dir := m.worktreePath(ticketID)
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	return ""
}

// worktreePath is the legacy flat layout (<BaseDir>/<ticketID>), used when the repo is unknown.
func (m *WorktreeManager) worktreePath(ticketID string) string {
	safe := strings.ReplaceAll(ticketID, "/", "-")
	return filepath.Join(m.BaseDir, safe)
}

// worktreePathFor places the worktree under <BaseDir>/<repo>-worktrees/<slug>.
func (m *WorktreeManager) worktreePathFor(ticketID, branch, repoDir string) string {
	repo := repoDisplayName(repoDir)
	if repo == "" {
		return m.worktreePath(ticketID)
	}
	name := branch
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = sanitizeDirName(name)
	if name == "" {
		name = sanitizeDirName(ticketID)
	}
	return filepath.Join(m.BaseDir, repo+"-worktrees", name)
}

func (m *WorktreeManager) remember(ticketID, dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.paths == nil {
		m.paths = map[string]string{}
	}
	m.paths[ticketID] = dir
}

func (m *WorktreeManager) lookup(ticketID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if dir, ok := m.paths[ticketID]; ok {
		return dir
	}
	matches, _ := filepath.Glob(filepath.Join(m.BaseDir, "*-worktrees", "*.flywheel-owner.json"))
	for _, path := range matches {
		dir := strings.TrimSuffix(path, ".flywheel-owner.json")
		r, err := gitworkspace.Read(dir)
		if err == nil && r.Key == ticketID {
			if _, err := os.Stat(dir); err == nil {
				return dir
			}
		}
	}
	return ""
}

func (m *WorktreeManager) forget(ticketID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.paths, ticketID)
}

// repoDisplayName returns the repository directory name for a clone: the basename for
// a normal checkout, or the name derived from the remote URL for Flywheel-managed clones.
func repoDisplayName(repoDir string) string {
	if repoDir == "" {
		return ""
	}
	base := filepath.Base(repoDir)
	if filepath.Base(filepath.Dir(repoDir)) == ".clones" || strings.Contains(base, "__") {
		if url := currentRemoteURL(repoDir); url != "" {
			return repoNameFromURL(url)
		}
		if i := strings.Index(base, "__"); i >= 0 {
			return base[i+2:]
		}
	}
	return base
}

// repoNameFromURL extracts "name" from git@github.com:owner/name.git or https URLs.
func repoNameFromURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimSuffix(strings.TrimSuffix(u, "/"), ".git")
	if i := strings.LastIndexAny(u, "/:"); i >= 0 {
		u = u[i+1:]
	}
	return u
}

// sanitizeDirName keeps a branch slug filesystem-safe.
func sanitizeDirName(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-.")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-.")
	}
	return out
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
// Existing matching clones are fetched. A changed repository uses a separate path;
// the previous checkout and any worktrees or uncommitted work are preserved.
func (m *MultiRepoCloneManager) EnsureClone(repoURL, alias string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Use alias as directory name for readability.
	safe := strings.ReplaceAll(alias, "/", "-")
	dir := filepath.Join(m.BaseDir, safe)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := os.Lstat(dir); err == nil && !gitworkspace.Matches(ctx, dir, repoURL) {
		hash := sha256.Sum256([]byte(repoURL))
		dir = filepath.Join(m.BaseDir, fmt.Sprintf("%s-%x", safe, hash[:6]))
	}
	if _, err := os.Lstat(dir); err == nil {
		if !gitworkspace.Matches(ctx, dir, repoURL) {
			return "", fmt.Errorf("existing clone path has unknown repository identity; preserved: %s", dir)
		}
		fetch := exec.CommandContext(ctx, "git", "fetch", "--all")
		fetch.Dir = dir
		if out, err := fetch.CombinedOutput(); err != nil {
			return "", fmt.Errorf("fetch clone: %s: %w", out, err)
		}
		return dir, nil
	}

	// Clone the repo.
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", fmt.Errorf("clone mkdir: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "clone", repoURL, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("clone %s: %s: %w", repoURL, strings.TrimSpace(string(out)), err)
	}

	return dir, nil
}
