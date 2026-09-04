package dispatch

import (
	"encoding/json"
	"strconv"
	"strings"
)

// OutputParsingDriver is implemented by drivers whose CLI writes structured
// events (one JSON object per line) to stdout instead of plain text. The worker
// creates one parser per run and routes every stdout line through it: the
// semantic events the parser yields replace the raw line in the output stream,
// and the parser's final result replaces the raw transcript as the run output.
type OutputParsingDriver interface {
	NewOutputParser() OutputParser
}

// OutputParser turns one harness stdout line into semantic worker output.
//
// Semantic streams share one text protocol so every consumer (the orchestrator
// run trace, the ticket execution trace) can classify them without knowing
// which harness produced them:
//
//	tool_call    "<tool>" or "<tool>: <what it was asked to do>"
//	tool_result  "<tool>: <summary>" or "<tool> failed: <error>"
//	assistant    a text block the agent emitted mid-run
//	system       harness lifecycle (session started, MCP servers connected)
type OutputParser interface {
	// ParseLine parses one stdout line. ok is false when the line is not a
	// structured event; the caller then forwards it as plain stdout.
	ParseLine(line string) (events []ParsedEvent, ok bool)
	// Result returns the harness's final result, or nil when the process ended
	// without reporting one (killed, crashed, or never structured).
	Result() *ParsedResult
	// Transcript returns the assistant text seen so far. It stands in for the
	// output when the run ends without a final result.
	Transcript() string
}

// ParsedEvent is one semantic output event.
type ParsedEvent struct {
	Stream string
	Text   string
}

// ParsedResult is the harness's own account of how the run ended.
type ParsedResult struct {
	Output     string
	IsError    bool
	Error      string
	SessionID  string
	NumTurns   int
	DurationMS int64
	CostUSD    float64
}

// normalizeMCPToolName maps an MCP-qualified tool name such as
// mcp__flywheel__list_tickets to the bare name consumers key on.
func normalizeMCPToolName(name string) string {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "mcp__") {
		return name
	}
	if idx := strings.LastIndex(name, "__"); idx >= len("mcp__") && idx+2 < len(name) {
		return name[idx+2:]
	}
	return name
}

// toolInputDetailKeys lists, in priority order, the input fields that best say
// what a tool call is about. The first one present becomes the call's detail.
var toolInputDetailKeys = []string{
	"file_path", "notebook_path", "path", "pattern", "query", "url",
	"description", "command", "title", "identifier", "ticket_id",
	"work_stream_id", "id", "name", "prompt",
}

// summarizeToolInput picks the one input field that best describes a tool call.
func summarizeToolInput(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var fields map[string]any
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	for _, key := range toolInputDetailKeys {
		switch v := fields[key].(type) {
		case string:
			if s := compactText(v, 120); s != "" {
				return s
			}
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
	}
	return ""
}

// flattenContent renders a message content field — a plain string or a list of
// content blocks — as text.
func flattenContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, strings.TrimSpace(b.Text))
		}
	}
	return strings.Join(parts, " ")
}

// compactText collapses whitespace and truncates to max runes.
func compactText(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max > 0 && len([]rune(s)) > max {
		return string([]rune(s)[:max]) + "..."
	}
	return s
}

func joinNonEmpty(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\n")
}
