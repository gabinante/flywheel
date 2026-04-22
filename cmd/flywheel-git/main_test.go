package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH, skipping CLI tests")
	}
}

func makeTempGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "first"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

var flywheelGitBinary string // set by TestMain

// runFlywheelGit runs the flywheel-git CLI in dir with args. Uses a built binary so CWD=dir is the repo.
func runFlywheelGit(t *testing.T, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	if flywheelGitBinary == "" {
		t.Fatal("flywheel-git binary not built (TestMain)")
	}
	cmd := exec.Command(flywheelGitBinary, args...)
	cmd.Dir = dir
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

func TestMain(m *testing.M) {
	// Build flywheel-git binary to a temp location so tests can run it with Dir=tempRepo.
	dir, err := os.MkdirTemp("", "flywheel-git-test-bin-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	flywheelGitBinary = filepath.Join(dir, "flywheel-git")
	if out, err := exec.Command("go", "build", "-o", flywheelGitBinary, ".").CombinedOutput(); err != nil {
		panic("build flywheel-git: " + err.Error() + ": " + string(out))
	}
	os.Exit(m.Run())
}

func TestCLI_Help(t *testing.T) {
	stdout, stderr, err := runFlywheelGit(t, t.TempDir(), "help")
	if err != nil {
		t.Fatalf("help: %v\nstderr: %s", err, stderr)
	}
	if stdout != "" && !strings.Contains(stdout, "flywheel-git") {
		t.Errorf("stdout should contain flywheel-git: %s", stdout)
	}
	if stderr != "" && !strings.Contains(stderr, "flywheel-git") {
		t.Errorf("stderr should contain flywheel-git: %s", stderr)
	}
	// help prints to stderr (printUsage uses Fprintf os.Stderr)
	if !strings.Contains(stderr, "note add") && !strings.Contains(stdout, "note add") {
		t.Errorf("output should contain 'note add': stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestCLI_NoteAdd_Show(t *testing.T) {
	requireGit(t)
	dir := makeTempGitRepo(t)
	_, stderr, err := runFlywheelGit(t, dir, "note", "add", "-t", "decision", "-m", "hello world")
	if err != nil {
		t.Fatalf("note add: %v\nstderr: %s", err, stderr)
	}
	stdout, stderr2, err := runFlywheelGit(t, dir, "note", "show", "-t", "decision")
	if err != nil {
		t.Fatalf("note show: %v\nstderr: %s", err, stderr2)
	}
	if !strings.Contains(stdout, "hello world") {
		t.Errorf("show should contain message: %q", stdout)
	}
	if !strings.Contains(stdout, `"type":"decision"`) {
		t.Errorf("show should contain type: %q", stdout)
	}
}

func TestCLI_NoteAdd_MissingMessage(t *testing.T) {
	requireGit(t)
	dir := makeTempGitRepo(t)
	_, stderr, err := runFlywheelGit(t, dir, "note", "add", "-t", "decision")
	if err == nil {
		t.Error("expected error when -m missing")
	}
	if !strings.Contains(stderr, "-m") {
		t.Errorf("stderr should mention -m: %s", stderr)
	}
}

func TestCLI_NoteAdd_InvalidType(t *testing.T) {
	requireGit(t)
	dir := makeTempGitRepo(t)
	_, stderr, err := runFlywheelGit(t, dir, "note", "add", "-t", "invalid", "-m", "x")
	if err == nil {
		t.Error("expected error for invalid type")
	}
	if !strings.Contains(stderr, "decision") && !strings.Contains(stderr, "trace") && !strings.Contains(stderr, "intent") {
		t.Errorf("stderr should mention valid types: %s", stderr)
	}
}

func TestCLI_NoteLog(t *testing.T) {
	requireGit(t)
	dir := makeTempGitRepo(t)
	_, _, err := runFlywheelGit(t, dir, "note", "add", "-t", "decision", "-m", "log test")
	if err != nil {
		t.Fatalf("note add: %v", err)
	}
	stdout, stderr, err := runFlywheelGit(t, dir, "note", "log", "-t", "decision", "-n", "5")
	if err != nil {
		t.Fatalf("note log: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "log test") {
		t.Errorf("log should contain note body: %q", stdout)
	}
}

func TestCLI_NoteDiff(t *testing.T) {
	requireGit(t)
	dir := makeTempGitRepo(t)
	// Second commit
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", "second")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	cmd = exec.Command("git", "rev-parse", "HEAD~1")
	cmd.Dir = dir
	baseOut, _ := cmd.Output()
	base := strings.TrimSpace(string(baseOut))
	_, _, err := runFlywheelGit(t, dir, "note", "add", "-t", "decision", "-m", "diff test")
	if err != nil {
		t.Fatalf("note add: %v", err)
	}
	stdout, stderr, err := runFlywheelGit(t, dir, "note", "diff", "-t", "decision", base, "HEAD")
	if err != nil {
		t.Fatalf("note diff: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "diff test") {
		t.Errorf("diff should contain note body: %q", stdout)
	}
}
