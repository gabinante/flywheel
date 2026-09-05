package dispatch

import (
	"context"
	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/project"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewGlobalWorkerLimit(t *testing.T) {
	d := New(Config{MaxWorkers: 1, WorktreeDir: t.TempDir()}, events.NewInProcessBus(), newMockTicketGetter(), newMockProjectGetter(&project.Project{ID: "a"}, &project.Project{ID: "b"}))
	_, cancelA, _, _, ok := d.startActive(context.Background(), "ticket-a", "a")
	if !ok {
		t.Fatal("first worker did not start")
	}
	defer cancelA()
	_, cancelB, _, _, ok := d.startActive(context.Background(), "ticket-b", "b")
	if ok {
		defer cancelB()
		t.Fatal("MaxWorkers=1 admitted two concurrent workers across projects")
	}
}

func TestReviewCodexWorkerParsesOutput(t *testing.T) {
	var driver AgentDriver = NewCodexDriver(DriverConfig{})
	if _, ok := driver.(OutputParsingDriver); !ok {
		t.Fatal("Codex dispatch has no structured output parser, unlike Claude")
	}
}

func TestReviewWorkerCredentialMustNotBeStaged(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "--quiet", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	script := filepath.Join(t.TempDir(), "fake-codex")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ngit add .\ngit diff --cached --name-only\n"), 0700); err != nil {
		t.Fatal(err)
	}
	w := &CLIWorker{Driver: NewCodexDriver(DriverConfig{CLIPath: script}), APIKey: "FAKE-REVIEW-KEY"}
	result, err := w.Spawn(context.Background(), "t", "p", "system", "task", dir, "http://localhost:8090")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Output, ".flywheel-mcp-config.json") {
		t.Fatal("ordinary git add . staged the MCP credential file")
	}
}
