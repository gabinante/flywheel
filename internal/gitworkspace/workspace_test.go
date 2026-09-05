package gitworkspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnershipAndIsolation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	os.Mkdir(repo, 0700)
	for _, args := range [][]string{{"init", "--quiet"}, {"-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "--allow-empty", "-m", "base"}} {
		if _, err := run(ctx, repo, args...); err != nil {
			t.Fatal(err)
		}
	}
	first, err := CreateAt(ctx, repo, root, "review-1", "a", "HEAD", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := CreateAt(ctx, repo, root, "review-1", "b", "HEAD", "")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("runs shared a workspace")
	}
	if err := os.WriteFile(filepath.Join(first, "work.txt"), []byte("unfinished"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Remove(ctx, first); err == nil {
		t.Fatal("dirty worktree removed")
	}
	if err := Remove(ctx, repo); err == nil {
		t.Fatal("unowned checkout removed")
	}
	if err := Remove(ctx, second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(first, "work.txt")); err != nil {
		t.Fatal("other run's cleanup removed unfinished work")
	}
}
