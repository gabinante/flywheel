package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabinante/flywheel/internal/runstatus"
)

func TestHarnessProgressArrivesBeforeProcessExit(t *testing.T) {
	for _, kind := range []Kind{Codex, ClaudeCode} {
		t.Run(string(kind), func(t *testing.T) {
			dir := t.TempDir()
			initial := `echo '{"type":"thread.started","thread_id":"test-live"}'
echo '{"type":"item.started","item":{"type":"command_execution"}}'
echo '{"type":"turn.completed","usage":{"input_tokens":12,"output_tokens":3}}'`
			final := `echo '{"type":"item.completed","item":{"type":"agent_message","text":"done"}}'`
			if kind == ClaudeCode {
				initial = `echo '{"type":"system","subtype":"init","session_id":"test-live"}'
echo '{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","name":"Read"}],"usage":{"input_tokens":12,"output_tokens":3}}}'`
				final = `echo '{"type":"result","subtype":"success","result":"done","session_id":"test-live","usage":{"input_tokens":12,"output_tokens":3}}'`
			}
			bin := writeScript(t, dir, "worker", initial+"\nwhile [ ! -f \"$RELEASE_FILE\" ]; do sleep 0.05; done\n"+final)
			release := filepath.Join(dir, "release")
			ctx := runstatus.WithInfo(context.Background(), runstatus.Run{Kind: "code_review", Title: dir})
			done := make(chan error, 1)
			go func() {
				res, err := New(Config{}).Run(ctx, Spec{Harness: kind, Binary: bin, WorkDir: dir, Prompt: "test", Timeout: 5 * time.Second, Env: []string{"RELEASE_FILE=" + release}})
				if err == nil && res.Output != "done" {
					err = fmt.Errorf("output=%q", res.Output)
				}
				done <- err
			}()
			var live runstatus.Run
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				for _, run := range runstatus.Default.Snapshot() {
					if run.Title == dir {
						live = run
					}
				}
				if live.Progress.ToolCalls == 1 && live.Progress.TokensIn != nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if live.ExternalSessionID != "test-live" || live.Progress.PID == 0 || live.Progress.ToolCalls != 1 || live.Progress.TokensIn == nil || *live.Progress.TokensIn != 12 {
				t.Errorf("missing live progress: %+v", live)
			}
			if err := os.WriteFile(release, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClaudeRequiresFinalStreamResult(t *testing.T) {
	bin := writeScript(t, t.TempDir(), "claude", `echo '{"type":"system","subtype":"init","session_id":"incomplete"}'`)
	res, err := New(Config{}).Run(context.Background(), Spec{Harness: ClaudeCode, Binary: bin, Prompt: "test"})
	if err == nil {
		t.Fatal("incomplete stream was accepted as a completed review")
	}
	if res.ExternalSessionID != "incomplete" {
		t.Fatal("failed stream lost its session link")
	}
}
