package dispatch

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// scriptedDriver runs a shell script in place of the claude binary while
// keeping ClaudeDriver's stream-json parser.
type scriptedDriver struct {
	ClaudeDriver
	script string
}

func (d *scriptedDriver) Executable() string { return "sh" }

func (d *scriptedDriver) BuildCLIArgs(_, _ string, _ mcpConnection, _ string) []string {
	return []string{"-c", d.script}
}

func spawnScripted(t *testing.T, script string) (*WorkerResult, []string) {
	t.Helper()
	w := &CLIWorker{Driver: &scriptedDriver{script: script}}
	var got []string
	res, err := w.SpawnStream(context.Background(), "ticket-1", "proj-1", "system", "task", t.TempDir(), "http://localhost:1", func(stream, text string) {
		got = append(got, stream+"|"+text)
	})
	if err != nil {
		t.Fatalf("SpawnStream() error = %v", err)
	}
	return res, got
}

func TestCLIWorkerSpawnStreamParsesStructuredOutput(t *testing.T) {
	script := strings.Join([]string{
		`echo '{"type":"system","subtype":"init","session_id":"s1","model":"m","mcp_servers":[{"name":"flywheel","status":"connected"}],"tools":["Read"]}'`,
		`echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"a.go"}}]}}'`,
		`echo '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"package a"}]}}'`,
		`echo 'stray warning'`,
		`echo '{"type":"result","subtype":"success","is_error":false,"result":"All done.","session_id":"s1","num_turns":1}'`,
	}, "\n")
	res, got := spawnScripted(t, script)

	want := []string{
		"system|Claude Code session started · model m · MCP flywheel connected · 1 tool",
		"tool_call|Read: a.go",
		"tool_result|Read: package a",
		"stdout|stray warning",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("output events mismatch\n got: %q\nwant: %q", got, want)
	}
	if !res.Success || res.Output != "All done." || res.Error != "" {
		t.Fatalf("unexpected result: %#v", res)
	}
}

func TestCLIWorkerSpawnStreamReportsHarnessError(t *testing.T) {
	res, _ := spawnScripted(t, `echo '{"type":"result","subtype":"error_during_execution","is_error":true,"errors":["boom"],"result":""}'`)
	if res.Success || res.Error != "boom" {
		t.Fatalf("expected harness error to fail the run, got %#v", res)
	}
}

func TestCLIWorkerSpawnStreamKeepsPlainTextOnExitFailure(t *testing.T) {
	res, got := spawnScripted(t, `echo 'Failed to authenticate. API Error: 401 API key is invalid.'; exit 1`)
	if res.Success {
		t.Fatal("expected failure on non-zero exit")
	}
	if !strings.Contains(res.Error, "exit status 1") {
		t.Fatalf("expected exit error, got %q", res.Error)
	}
	if !strings.Contains(res.Output, "401 API key is invalid") {
		t.Fatalf("expected plain-text output to be preserved for failover matching, got %q", res.Output)
	}
	if !ShouldFailoverToNextWorker(nil, res) {
		t.Fatal("expected auth failure output to trigger failover")
	}
	if !reflect.DeepEqual(got, []string{"stdout|Failed to authenticate. API Error: 401 API key is invalid."}) {
		t.Fatalf("unexpected events: %q", got)
	}
}

func TestCLIWorkerSpawnStreamFallsBackToTranscriptWithoutResult(t *testing.T) {
	res, _ := spawnScripted(t, `echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Partial thoughts."}]}}'`)
	if !res.Success || res.Output != "Partial thoughts." {
		t.Fatalf("expected transcript fallback, got %#v", res)
	}
}
