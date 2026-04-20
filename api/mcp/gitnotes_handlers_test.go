package mcp

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH, skipping git-notes handler tests that need repo")
	}
}

// makeTempGitRepo creates a temp git repo and returns its path.
func makeTempGitRepoForMCP(t *testing.T) string {
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

func getResultJSON(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(*mcpsdk.TextContent); ok {
		return tc.Text
	}
	return ""
}

func TestFlywheelAddGitNote_MissingMessage(t *testing.T) {
	b := &Backend{}
	ctx := context.Background()
	res, _, err := flywheelAddGitNoteHandler(b, ctx, map[string]any{"type": "decision"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Error("expected error result when message missing")
	}
	if res != nil {
		if text := getResultJSON(t, res); text != "" && !strings.Contains(text, "message") {
			t.Errorf("error should mention message: %s", text)
		}
	}
}

func TestFlywheelAddGitNote_InvalidType(t *testing.T) {
	b := &Backend{}
	ctx := context.Background()
	res, _, err := flywheelAddGitNoteHandler(b, ctx, map[string]any{"message": "x", "type": "invalid"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Error("expected error result for invalid type")
	}
}

func TestFlywheelAddGitNote_NoRepoPath_ReturnsCommands(t *testing.T) {
	b := &Backend{}
	ctx := context.Background()
	res, _, err := flywheelAddGitNoteHandler(b, ctx, map[string]any{
		"message": "hello",
		"type":    "decision",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatal("expected success with commands")
	}
	text := getResultJSON(t, res)
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	cmds, _ := m["commands"].([]interface{})
	if len(cmds) == 0 {
		t.Error("expected commands when repo_path empty")
	}
	if _, hasHint := m["hint"]; !hasHint {
		t.Error("expected hint when returning commands")
	}
}

func TestFlywheelAddGitNote_WithRepoPath_AddsNote(t *testing.T) {
	requireGit(t)
	dir := makeTempGitRepoForMCP(t)
	b := &Backend{}
	ctx := context.Background()
	res, _, err := flywheelAddGitNoteHandler(b, ctx, map[string]any{
		"message":   "mcp test note",
		"type":      "decision",
		"repo_path": dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatal("expected success")
	}
	text := getResultJSON(t, res)
	var m map[string]any
	json.Unmarshal([]byte(text), &m)
	if m["ok"] != true {
		t.Errorf("expected ok true: %v", m)
	}
}

func TestFlywheelShowGitNotes_InvalidType_ReturnsError(t *testing.T) {
	b := &Backend{}
	ctx := context.Background()
	res, _, err := flywheelShowGitNotesHandler(b, ctx, map[string]any{
		"type":       "invalid",
		"repo_path": "/tmp",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Error("expected error for invalid type")
	}
}

func TestFlywheelShowGitNotes_NoRepoPath_ReturnsCommands(t *testing.T) {
	b := &Backend{}
	ctx := context.Background()
	res, _, err := flywheelShowGitNotesHandler(b, ctx, map[string]any{"commit_sha": "HEAD"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatal("expected success with commands")
	}
	text := getResultJSON(t, res)
	var m map[string]any
	json.Unmarshal([]byte(text), &m)
	if _, ok := m["commands"]; !ok {
		t.Error("expected commands when repo_path empty")
	}
}

func TestFlywheelShowGitNotes_WithRepoPath_ReturnsBody(t *testing.T) {
	requireGit(t)
	dir := makeTempGitRepoForMCP(t)
	// Add a note via handler first
	flywheelAddGitNoteHandler(&Backend{}, context.Background(), map[string]any{
		"message":   "show test",
		"type":      "decision",
		"repo_path": dir,
	})
	res, _, err := flywheelShowGitNotesHandler(&Backend{}, context.Background(), map[string]any{
		"repo_path":  dir,
		"type":       "decision",
		"commit_sha": "HEAD",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatal("expected success")
	}
	text := getResultJSON(t, res)
	var m map[string]any
	json.Unmarshal([]byte(text), &m)
	if body, _ := m["body"].(string); !strings.Contains(body, "show test") {
		t.Errorf("body should contain note: %v", m)
	}
}

func TestFlywheelLogGitNotes_InvalidType_ReturnsError(t *testing.T) {
	b := &Backend{}
	ctx := context.Background()
	res, _, err := flywheelLogGitNotesHandler(b, ctx, map[string]any{"type": "bad"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Error("expected error for invalid type")
	}
}

func TestFlywheelLogGitNotes_NoRepoPath_ReturnsCommands(t *testing.T) {
	b := &Backend{}
	ctx := context.Background()
	res, _, err := flywheelLogGitNotesHandler(b, ctx, map[string]any{"limit": 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatal("expected success with commands")
	}
	text := getResultJSON(t, res)
	var m map[string]any
	json.Unmarshal([]byte(text), &m)
	if _, ok := m["commands"]; !ok {
		t.Error("expected commands")
	}
}

func TestFlywheelDiffGitNotes_MissingBase_ReturnsError(t *testing.T) {
	b := &Backend{}
	ctx := context.Background()
	res, _, err := flywheelDiffGitNotesHandler(b, ctx, map[string]any{"head": "HEAD"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Error("expected error when base missing")
	}
}

func TestFlywheelDiffGitNotes_NoRepoPath_ReturnsCommands(t *testing.T) {
	b := &Backend{}
	ctx := context.Background()
	res, _, err := flywheelDiffGitNotesHandler(b, ctx, map[string]any{
		"base": "main",
		"head": "HEAD",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatal("expected success with commands")
	}
	text := getResultJSON(t, res)
	var m map[string]any
	json.Unmarshal([]byte(text), &m)
	if _, ok := m["commands"]; !ok {
		t.Error("expected commands")
	}
}

func TestFlywheelSyncGitNotes_ReturnsCommands(t *testing.T) {
	b := &Backend{}
	ctx := context.Background()
	res, _, err := flywheelSyncGitNotesHandler(b, ctx, map[string]any{"direction": "push"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatal("expected success")
	}
	text := getResultJSON(t, res)
	var m map[string]any
	json.Unmarshal([]byte(text), &m)
	cmds, _ := m["commands"].([]interface{})
	if len(cmds) == 0 {
		t.Error("expected commands")
	}
	if s, _ := cmds[0].(string); !strings.Contains(s, "sync") {
		t.Errorf("command should contain sync: %s", s)
	}
}
