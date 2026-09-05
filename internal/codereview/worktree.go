package codereview

import (
	"context"
	"fmt"
	"github.com/gabinante/flywheel/internal/gitworkspace"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Workspaces prepares detached git worktrees at a PR's head so reviews never run
// in the operator's primary checkout on whatever branch happens to be there.
//
// Layout follows the operator's convention: the clone lives at <root>/<name>
// (or <root>/.flywheel/clones/<owner>__<name> when Flywheel had to clone), and
// worktrees live at <root>/<name>-worktrees/review-<number>.
type Workspaces struct {
	Root string // e.g. ~/git
}

func gitRun(ctx context.Context, dir string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %s", strings.Join(args[:min(len(args), 2)], " "), truncateStr(strings.TrimSpace(string(out)), 400))
	}
	return string(out), nil
}

// RepoDir returns the local clone for owner/name, cloning it if needed.
func (w *Workspaces) RepoDir(ctx context.Context, repo string) (string, error) {
	name := repo
	if i := strings.Index(repo, "/"); i >= 0 {
		name = repo[i+1:]
	}
	primary := filepath.Join(w.Root, name)
	if isGitRepo(primary) && gitworkspace.Matches(ctx, primary, repo) {
		return primary, nil
	}
	managed := filepath.Join(w.Root, ".flywheel", "clones", strings.ReplaceAll(repo, "/", "__"))
	if isGitRepo(managed) {
		if !gitworkspace.Matches(ctx, managed, repo) {
			return "", fmt.Errorf("repository origin mismatch: %s", managed)
		}
		return managed, nil
	}
	if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
		return "", err
	}
	if _, err := gitRun(ctx, "", "clone", "--quiet", "git@github.com:"+repo+".git", managed); err != nil {
		return "", err
	}
	return managed, nil
}

func isGitRepo(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && (st.IsDir() || st.Mode().IsRegular())
}

// Prepare fetches the PR head and creates a detached worktree at it. It returns
// the worktree path and the resolved head SHA.
func (w *Workspaces) Prepare(ctx context.Context, repo string, number int) (string, string, error) {
	repoDir, err := w.RepoDir(ctx, repo)
	if err != nil {
		return "", "", err
	}
	ref := fmt.Sprintf("refs/flywheel/pr-%d", number)
	if _, err := gitRun(ctx, repoDir, "fetch", "--quiet", "origin", fmt.Sprintf("+refs/pull/%d/head:%s", number, ref)); err != nil {
		return "", "", err
	}
	sha, err := gitRun(ctx, repoDir, "rev-parse", ref)
	if err != nil {
		return "", "", err
	}
	sha = strings.TrimSpace(sha)
	name := filepath.Base(repoDir)
	if strings.Contains(name, "__") { // managed clone: owner__name
		name = name[strings.Index(name, "__")+2:]
	}
	wt, err := gitworkspace.CreateAt(ctx, repoDir, filepath.Join(w.Root, name+"-worktrees"), fmt.Sprintf("review-%d", number), repo, sha, "")
	if err != nil {
		return "", "", err
	}
	return wt, sha, nil
}

// Remove deletes a review worktree.
func (w *Workspaces) Remove(ctx context.Context, repo, wt string) {
	if wt == "" {
		return
	}
	_ = gitworkspace.Remove(ctx, wt)
}
