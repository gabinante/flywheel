// Package gitworkspace owns disposable checkouts without ever deleting an
// operator's directory. Ownership records live outside the checkout.
package gitworkspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Record struct{ Dir, Repo, Key string }

func run(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(string(b)), err)
	}
	return strings.TrimSpace(string(b)), nil
}

// CreateAt creates at a resolved commit, optionally on a new branch.
func CreateAt(ctx context.Context, repo, parent, prefix, key, ref, branch string) (string, error) {
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(parent, prefix+"-")
	if err != nil {
		return "", err
	}
	if err = os.Remove(dir); err != nil {
		return "", err
	}
	args := []string{"worktree", "add", "--quiet"}
	if branch == "" {
		args = append(args, "--detach")
	} else {
		args = append(args, "-b", branch)
	}
	args = append(args, dir, ref)
	if _, err = run(ctx, repo, args...); err != nil {
		return "", err
	}
	data, _ := json.Marshal(Record{Dir: dir, Repo: repo, Key: key})
	if err = os.WriteFile(dir+".flywheel-owner.json", data, 0600); err != nil {
		_, _ = run(ctx, repo, "worktree", "remove", dir)
		return "", err
	}
	return dir, nil
}

func Read(dir string) (*Record, error) {
	data, err := os.ReadFile(dir + ".flywheel-owner.json")
	if err != nil {
		return nil, err
	}
	var r Record
	if err = json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if r.Dir != dir || r.Repo == "" {
		return nil, fmt.Errorf("invalid workspace ownership")
	}
	return &r, nil
}

// Remove refuses unknown or dirty workspaces. A failed run's work is preserved.
func Remove(ctx context.Context, dir string) error {
	r, err := Read(dir)
	if err != nil {
		return fmt.Errorf("unowned workspace %s: %w", dir, err)
	}
	status, err := run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("workspace %s has uncommitted work; preserved", dir)
	}
	if _, err = run(ctx, r.Repo, "worktree", "remove", dir); err != nil {
		return err
	}
	return os.Remove(dir + ".flywheel-owner.json")
}

func CanonicalRemote(remote string) string {
	remote = strings.TrimSpace(remote)
	remote = strings.TrimPrefix(remote, "git@github.com:")
	remote = strings.TrimPrefix(remote, "https://github.com/")
	remote = strings.TrimPrefix(remote, "ssh://git@github.com/")
	return strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(remote, "/"), ".git"))
}

func Matches(ctx context.Context, repo, remote string) bool {
	actual, err := run(ctx, repo, "remote", "get-url", "origin")
	return err == nil && CanonicalRemote(actual) == CanonicalRemote(remote)
}
