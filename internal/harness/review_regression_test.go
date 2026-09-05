package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewClaudeEffortIsForwarded(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	bin := writeScript(t, dir, "claude", "printf '%s\\n' \"$@\" > \"$REVIEW_ARGS_PATH\"\necho '{\"type\":\"result\",\"result\":\"ok\"}'\n")
	_, err := New(Config{}).Run(context.Background(), Spec{Harness: ClaudeCode, Binary: bin, Prompt: "task", Effort: "high", Env: []string{"REVIEW_ARGS_PATH=" + argsPath}})
	if err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--effort\nhigh") {
		t.Fatalf("configured effort omitted from Claude args: %s", args)
	}
}
