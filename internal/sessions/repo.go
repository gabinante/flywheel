package sessions

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var originURLRe = regexp.MustCompile(`(?:github\.com[:/])([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`)

// RepoFromOriginURL converts a git remote URL into "owner/name". Non-GitHub
// remotes fall back to the last path component.
func RepoFromOriginURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if m := originURLRe.FindStringSubmatch(u); m != nil {
		return m[1] + "/" + m[2]
	}
	u = strings.TrimSuffix(strings.TrimSuffix(u, "/"), ".git")
	if i := strings.LastIndexAny(u, "/:"); i >= 0 {
		return u[i+1:]
	}
	return u
}

// repoResolver maps working directories to repositories, preferring the git
// remote when the directory still exists and falling back to path heuristics
// for worktrees that have since been removed.
type repoResolver struct {
	mu    sync.Mutex
	cache map[string]string
}

func newRepoResolver() *repoResolver {
	return &repoResolver{cache: map[string]string{}}
}

// Resolve returns the repo for cwd. originHint (e.g. Codex's git_origin_url) wins when set.
func (r *repoResolver) Resolve(ctx context.Context, cwd, originHint string) string {
	if originHint != "" {
		return RepoFromOriginURL(originHint)
	}
	if cwd == "" {
		return ""
	}
	r.mu.Lock()
	if v, ok := r.cache[cwd]; ok {
		r.mu.Unlock()
		return v
	}
	r.mu.Unlock()

	repo := ""
	if st, err := os.Stat(cwd); err == nil && st.IsDir() {
		cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		out, err := exec.CommandContext(cctx, "git", "-C", cwd, "config", "--get", "remote.origin.url").Output()
		cancel()
		if err == nil {
			repo = RepoFromOriginURL(string(out))
		}
	}
	if repo == "" {
		repo = RepoFromPath(cwd)
	}
	r.mu.Lock()
	r.cache[cwd] = repo
	r.mu.Unlock()
	return repo
}

// RepoFromPath guesses a repository name from a checkout or worktree path:
// ~/git/joinera → joinera, ~/git/rle-dp-worktrees/deid → rle-dp,
// ~/git/joinera-worktrees/x → joinera, ~/git/joinera/.claude/worktrees/y → joinera.
func RepoFromPath(cwd string) string {
	cwd = filepath.Clean(cwd)
	parts := strings.Split(cwd, string(filepath.Separator))
	for i := len(parts) - 1; i >= 0; i-- {
		p := parts[i]
		if strings.HasSuffix(p, "-worktrees") {
			return strings.TrimSuffix(p, "-worktrees")
		}
		if p == "worktrees" && i >= 2 && parts[i-1] == ".claude" {
			return parts[i-2]
		}
	}
	// Direct checkout under a code root such as ~/git/<repo>.
	for i := len(parts) - 1; i >= 1; i-- {
		if parts[i-1] == "git" || parts[i-1] == "src" || parts[i-1] == "code" || parts[i-1] == "repos" {
			return parts[i]
		}
	}
	return ""
}
