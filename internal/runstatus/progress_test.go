package runstatus

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProgressDistinguishesOutputWorkAndSilence(t *testing.T) {
	r := New()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ctx, finish := r.Begin(ctx, Run{})
	if r.Snapshot()[0].Progress.WorkerState != "waiting" {
		t.Fatal("queued run claimed to be running")
	}
	Started(ctx, 42)
	for _, line := range []string{
		`{"type":"thread.started","thread_id":"native"}`,
		`{"type":"item.started","item":{"type":"command_execution","command":"secret command"}}`,
		`{"type":"item.completed","item":{"type":"reasoning","text":"secret reasoning"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"secret output"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":120,"output_tokens":17}}`,
	} {
		ParseOutput(ctx, []byte(line))
	}
	p := r.Snapshot()[0].Progress
	if p.PID != 42 || p.DeadlineAt == nil || p.ToolCalls != 1 || p.ReasoningUpdates != 1 || p.AssistantMessages != 1 || *p.TokensIn != 120 || *p.TokensOut != 17 {
		t.Fatalf("progress=%+v", p)
	}
	encoded, _ := json.Marshal(p)
	if strings.Contains(string(encoded), "secret") {
		t.Fatalf("raw content leaked: %s", encoded)
	}
	if p.At(p.LastOutputAt.Add(QuietAfter+time.Second)).Health != "quiet" {
		t.Fatal("silent run was shown as active")
	}
	Output(ctx) // stderr indicates output, not a new tool or model turn
	next := r.Snapshot()[0].Progress
	if next.OutputEvents != p.OutputEvents+1 || !next.LastActivityAt.Equal(*p.LastActivityAt) {
		t.Fatal("stderr was counted as work")
	}
	if next.At(time.Now()).Health != "active" {
		t.Fatal("new output did not clear quiet status")
	}
	finish()
	Output(ctx)
	Running(ctx)
	if len(r.Snapshot()) != 0 {
		t.Fatal("late output resurrected a finished run")
	}
}

func TestClaudeUsageDoesNotDoubleCountContentBlocks(t *testing.T) {
	r := New()
	ctx, finish := r.Begin(context.Background(), Run{})
	defer finish()
	if r.Snapshot()[0].Progress.TokensIn != nil {
		t.Fatal("unknown usage should not be zero")
	}
	for _, line := range []string{
		`{"type":"assistant","message":{"id":"one","content":[],"usage":{"input_tokens":10,"cache_read_input_tokens":30,"output_tokens":4}}}`,
		`{"type":"assistant","message":{"id":"one","content":[],"usage":{"input_tokens":10,"cache_read_input_tokens":30,"output_tokens":6}}}`,
		`{"type":"assistant","message":{"id":"two","content":[],"usage":{"input_tokens":2,"output_tokens":3}}}`,
	} {
		ParseOutput(ctx, []byte(line))
	}
	p := r.Snapshot()[0].Progress
	if *p.TokensIn != 42 || *p.TokensOut != 9 {
		t.Fatalf("usage=%+v", p)
	}
	for range 50 {
		ParseOutput(ctx, []byte(`{"type":"stream_event","event":{"type":"content_block_delta"}}`))
	}
	p = r.Snapshot()[0].Progress
	if len(p.Recent) != 4 {
		t.Fatalf("partial tokens flooded history: %d", len(p.Recent))
	}
	for range 50 {
		ParseOutput(ctx, []byte(`{"type":"turn.started"}`))
	}
	if len(r.Snapshot()[0].Progress.Recent) != recentLimit {
		t.Fatal("unbounded event history")
	}
	ParseOutput(ctx, []byte(`{"type":"result","usage":{"input_tokens":2,"cache_read_input_tokens":40,"output_tokens":9}}`))
	p = r.Snapshot()[0].Progress
	if *p.TokensIn != 42 || *p.TokensOut != 9 {
		t.Fatal("final totals were added twice")
	}
}
