package harness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunCodexParsesEventsAndStructuredOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts")
	}
	dir := t.TempDir()
	// Fake codex: echo the prompt to a file, emit the real event shapes, write -o.
	bin := writeScript(t, dir, "codex", `
out=""
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2;;
    *) shift;;
  esac
done
cat > "$(dirname "$out")/prompt.txt"
echo '{"type":"thread.started","thread_id":"thr_123"}'
echo '{"type":"turn.started"}'
echo '{"type":"item.completed","item":{"type":"agent_message","text":"{\"summary\":\"fine\",\"findings\":[]}"}}'
echo '{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":50,"output_tokens":7}}'
printf '{"summary":"fine","findings":[]}' > "$out"
`)
	r := New(Config{})
	res, err := r.Run(context.Background(), Spec{
		Harness: Codex, Binary: bin, WorkDir: dir, Prompt: "review this", SystemPrompt: "be terse",
		OutputSchema: json.RawMessage(`{"type":"object"}`), Timeout: time.Minute,
	})
	if err != nil {
		t.Fatalf("Run: %v (stderr=%s)", err, res.Stderr)
	}
	if res.ExternalSessionID != "thr_123" {
		t.Errorf("session id = %q", res.ExternalSessionID)
	}
	if res.TokensIn != 150 || res.TokensOut != 7 {
		t.Errorf("tokens = %d/%d", res.TokensIn, res.TokensOut)
	}
	var s struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(res.Structured, &s); err != nil || s.Summary != "fine" {
		t.Errorf("structured = %s (%v)", res.Structured, err)
	}
}

func TestRunClaudeParsesResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts")
	}
	dir := t.TempDir()
	bin := writeScript(t, dir, "claude", `
echo '{"type":"result","subtype":"success","is_error":false,"result":"{\"answer\":\"ok\"}","session_id":"sess-9","structured_output":{"answer":"ok"},"total_cost_usd":0.42,"duration_ms":10,"usage":{"input_tokens":2,"output_tokens":5,"cache_read_input_tokens":40,"cache_creation_input_tokens":0}}'
`)
	r := New(Config{})
	res, err := r.Run(context.Background(), Spec{Harness: ClaudeCode, Binary: bin, Prompt: "hi", OutputSchema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExternalSessionID != "sess-9" || res.TokensIn != 42 || res.TokensOut != 5 || res.CostUSD != 0.42 {
		t.Errorf("unexpected result: %+v", res)
	}
	if string(res.Structured) != `{"answer":"ok"}` {
		t.Errorf("structured = %s", res.Structured)
	}
}

func TestRunFailureSurfacesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts")
	}
	dir := t.TempDir()
	bin := writeScript(t, dir, "codex", "echo 'boom: bad auth' >&2; exit 3")
	r := New(Config{})
	res, err := r.Run(context.Background(), Spec{Harness: Codex, Binary: bin, Prompt: "x"})
	if err == nil || res.ExitCode != 3 {
		t.Fatalf("expected exit 3 error, got err=%v res=%+v", err, res)
	}
}

func TestParseKind(t *testing.T) {
	for in, want := range map[string]Kind{"codex": Codex, "claude": ClaudeCode, "claude_code": ClaudeCode} {
		if got, err := ParseKind(in); err != nil || got != want {
			t.Errorf("ParseKind(%q) = %v,%v", in, got, err)
		}
	}
	if _, err := ParseKind("gemini"); err == nil {
		t.Error("expected error for unknown harness")
	}
}
