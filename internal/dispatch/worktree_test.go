package dispatch

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorktreePath(t *testing.T) {
	m := &WorktreeManager{BaseDir: "/tmp/wt", RepoDir: "/repo"}

	tests := []struct {
		name     string
		ticketID string
		want     string
	}{
		{"simple id", "proj-42", "/tmp/wt/proj-42"},
		{"slash sanitized", "org/proj-1", "/tmp/wt/org-proj-1"},
		{"multiple slashes", "a/b/c", "/tmp/wt/a-b-c"},
		{"no slashes", "ticket-1", "/tmp/wt/ticket-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.worktreePath(tt.ticketID)
			if got != tt.want {
				t.Errorf("worktreePath(%q) = %q, want %q", tt.ticketID, got, tt.want)
			}
		})
	}
}

func TestWorktreeCreateAndRemove(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Set up a bare git repo to act as the "main" repository.
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "--initial-branch=main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	baseDir := t.TempDir()
	m := &WorktreeManager{BaseDir: baseDir, RepoDir: repoDir}

	// Create a worktree.
	dir, err := m.Create("test-ticket", "ticket/test-ticket")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	expected := filepath.Join(baseDir, "test-ticket")
	if dir != expected {
		t.Errorf("Create returned %q, want %q", dir, expected)
	}

	// Verify the directory exists.
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("worktree directory does not exist: %s", dir)
	}

	// Verify it's a git worktree (has .git file, not directory).
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		t.Fatalf("stat .git: %v", err)
	}
	if info.IsDir() {
		t.Error("expected .git to be a file (worktree), not a directory")
	}

	// Remove the worktree.
	if err := m.Remove("test-ticket"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify the directory no longer exists.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("worktree directory still exists after Remove")
	}
}

func TestWorktreeCreateExistingBranch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "--initial-branch=main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")
	// Pre-create the branch so the -b flag fails and the fallback path is taken.
	runGit(t, repoDir, "branch", "ticket/existing")

	baseDir := t.TempDir()
	m := &WorktreeManager{BaseDir: baseDir, RepoDir: repoDir}

	dir, err := m.Create("existing", "ticket/existing")
	if err != nil {
		t.Fatalf("Create with existing branch: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("worktree directory does not exist after fallback: %s", dir)
	}

	// Cleanup.
	_ = m.Remove("existing")
}

func TestWorktreeCreateCleansUpLeftover(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "--initial-branch=main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	baseDir := t.TempDir()
	m := &WorktreeManager{BaseDir: baseDir, RepoDir: repoDir}

	// Create a leftover directory simulating a crash.
	leftoverDir := filepath.Join(baseDir, "leftover")
	if err := os.MkdirAll(leftoverDir, 0o755); err != nil {
		t.Fatalf("mkdir leftover: %v", err)
	}

	// Create should succeed despite the leftover directory.
	dir, err := m.Create("leftover", "ticket/leftover")
	if err != nil {
		t.Fatalf("Create with leftover dir: %v", err)
	}
	if dir != leftoverDir {
		t.Errorf("expected %q, got %q", leftoverDir, dir)
	}

	_ = m.Remove("leftover")
}

func TestWorktreeRemoveNonexistent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "--initial-branch=main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	m := &WorktreeManager{BaseDir: t.TempDir(), RepoDir: repoDir}

	// Removing a non-existent worktree should return an error.
	err := m.Remove("does-not-exist")
	if err == nil {
		t.Error("expected error when removing non-existent worktree, got nil")
	}
}

// runGit is a test helper that runs a git command in the given directory.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
}
