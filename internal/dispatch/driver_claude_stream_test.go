package dispatch

import (
	"reflect"
	"testing"
)

func TestClaudeStreamParserTracesToolsAndResult(t *testing.T) {
	p := NewClaudeDriver(DriverConfig{}).NewOutputParser()
	lines := []string{
		`{"type":"system","subtype":"init","cwd":"/repo","session_id":"sess-1","tools":["Bash","Read","mcp__flywheel__list_tickets"],"mcp_servers":[{"name":"flywheel","status":"connected"}],"model":"claude-opus-5"}`,
		`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"},"session_id":"sess-1"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"x"},{"type":"tool_use","id":"toolu_1","name":"mcp__flywheel__list_tickets","input":{"project_id":"proj-1"}}]},"session_id":"sess-1"}`,
		`{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_1","type":"tool_result","content":[{"type":"text","text":"{\"tickets\":[]}"}],"is_error":false}]},"session_id":"sess-1"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_2","name":"Read","input":{"file_path":"internal/foo.go"}}]},"session_id":"sess-1"}`,
		`{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_2","type":"tool_result","content":"File does not exist.","is_error":true}]},"session_id":"sess-1"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_3","name":"mcp__flywheel__get_ticket","input":{"ticket_id":"FLY-12"}}]},"session_id":"sess-1"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Milestone 1 is close; two tickets remain. Use create_ticket for the rest."}]},"session_id":"sess-1"}`,
		`{"type":"result","subtype":"success","is_error":false,"duration_ms":5931,"num_turns":2,"result":"Milestone 1 is close; two tickets remain.","session_id":"sess-1","total_cost_usd":0.12}`,
	}
	var got []ParsedEvent
	for _, line := range lines {
		events, ok := p.ParseLine(line)
		if !ok {
			t.Fatalf("line not recognised as structured output: %s", line)
		}
		got = append(got, events...)
	}
	want := []ParsedEvent{
		{Stream: "system", Text: "Claude Code session started · model claude-opus-5 · MCP flywheel connected · 3 tools"},
		{Stream: "tool_call", Text: "list_tickets"},
		{Stream: "tool_result", Text: `list_tickets: {"tickets":[]}`},
		{Stream: "tool_call", Text: "Read: internal/foo.go"},
		{Stream: "tool_result", Text: "Read failed: File does not exist."},
		{Stream: "tool_call", Text: "get_ticket: FLY-12"},
		{Stream: "assistant", Text: "Milestone 1 is close; two tickets remain. Use create_ticket for the rest."},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events mismatch\n got: %#v\nwant: %#v", got, want)
	}

	res := p.Result()
	if res == nil {
		t.Fatal("expected a final result")
	}
	if res.IsError || res.Output != "Milestone 1 is close; two tickets remain." || res.SessionID != "sess-1" || res.NumTurns != 2 || res.CostUSD != 0.12 {
		t.Fatalf("unexpected result: %#v", res)
	}
	if p.Transcript() != "Milestone 1 is close; two tickets remain. Use create_ticket for the rest." {
		t.Fatalf("unexpected transcript: %q", p.Transcript())
	}
}

func TestClaudeStreamParserPassesThroughPlainText(t *testing.T) {
	p := NewClaudeDriver(DriverConfig{}).NewOutputParser()
	for _, line := range []string{
		"Failed to authenticate. API Error: 401 API key is invalid.",
		"",
		"{not json",
		`{"no_type_field": true}`,
	} {
		if events, ok := p.ParseLine(line); ok || len(events) != 0 {
			t.Fatalf("expected %q to pass through, got ok=%v events=%v", line, ok, events)
		}
	}
	if p.Result() == nil || !p.Result().IsError {
		t.Fatal("expected an incomplete-run error without a result record")
	}
}

func TestClaudeStreamParserErrorResult(t *testing.T) {
	p := NewClaudeDriver(DriverConfig{}).NewOutputParser()
	if _, ok := p.ParseLine(`{"type":"result","subtype":"error_during_execution","is_error":true,"errors":["boom"],"result":"","session_id":"s"}`); !ok {
		t.Fatal("expected result record to be recognised")
	}
	res := p.Result()
	if res == nil || !res.IsError || res.Error != "boom" {
		t.Fatalf("unexpected error result: %#v", res)
	}

	p = NewClaudeDriver(DriverConfig{}).NewOutputParser()
	p.ParseLine(`{"type":"result","subtype":"error_max_turns","is_error":true,"result":"","session_id":"s"}`)
	if res := p.Result(); res == nil || res.Error != "claude reported error_max_turns" {
		t.Fatalf("expected subtype fallback, got %#v", res)
	}
}

func TestSummarizeToolInputPrefersDescriptiveFields(t *testing.T) {
	cases := map[string]string{
		`{"command":"go test ./...","description":"Run the Go tests"}`: "Run the Go tests",
		`{"pattern":"TODO","path":"internal"}`:                         "internal",
		`{"project_id":"proj-1"}`:                                      "",
		`{"file_path":"  a/very   spaced/path.go  "}`:                  "a/very spaced/path.go",
		`{"id":42}`: "42",
		`not json`:  "",
	}
	for input, want := range cases {
		if got := summarizeToolInput([]byte(input)); got != want {
			t.Errorf("summarizeToolInput(%s) = %q, want %q", input, got, want)
		}
	}
}
