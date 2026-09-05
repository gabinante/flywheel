package dispatch

import (
	"encoding/json"
	"fmt"
	"strings"
)

// claudeStreamParser reads Claude Code's `--output-format stream-json` records.
// Each stdout line is one JSON object: a system/init record once the session is
// up, assistant messages whose content blocks carry text and tool_use, user
// messages carrying tool_result blocks, and one final result record.
type claudeStreamParser struct {
	toolNames  map[string]string // tool_use id → tool name, to pair results with calls
	transcript []string
	result     *ParsedResult
}

// NewOutputParser returns a parser for one stream-json run.
func (d *ClaudeDriver) NewOutputParser() OutputParser {
	return &claudeStreamParser{toolNames: map[string]string{}}
}

type claudeStreamEvent struct {
	Type       string            `json:"type"`
	Subtype    string            `json:"subtype"`
	SessionID  string            `json:"session_id"`
	Model      string            `json:"model"`
	Tools      []json.RawMessage `json:"tools"`
	MCPServers []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"mcp_servers"`
	Message *struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	IsError      bool     `json:"is_error"`
	Result       string   `json:"result"`
	Errors       []string `json:"errors"`
	NumTurns     int      `json:"num_turns"`
	DurationMS   int64    `json:"duration_ms"`
	TotalCostUSD float64  `json:"total_cost_usd"`
}

type claudeContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// ParseLine implements OutputParser.
func (p *claudeStreamParser) ParseLine(line string) ([]ParsedEvent, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "{") {
		return nil, false
	}
	var ev claudeStreamEvent
	if err := json.Unmarshal([]byte(trimmed), &ev); err != nil || ev.Type == "" {
		return nil, false
	}
	switch ev.Type {
	case "system":
		if ev.Subtype != "init" {
			return nil, true
		}
		return []ParsedEvent{{Stream: "system", Text: describeClaudeInit(ev)}}, true
	case "assistant":
		var events []ParsedEvent
		for _, block := range decodeClaudeBlocks(ev.Message) {
			switch block.Type {
			case "text":
				text := strings.TrimSpace(block.Text)
				if text == "" {
					continue
				}
				p.transcript = append(p.transcript, text)
				events = append(events, ParsedEvent{Stream: "assistant", Text: text})
			case "tool_use":
				name := normalizeMCPToolName(block.Name)
				if name == "" {
					name = "tool"
				}
				if block.ID != "" {
					p.toolNames[block.ID] = name
				}
				text := name
				if detail := summarizeToolInput(block.Input); detail != "" {
					text += ": " + detail
				}
				events = append(events, ParsedEvent{Stream: "tool_call", Text: text})
			}
		}
		return events, true
	case "user":
		var events []ParsedEvent
		for _, block := range decodeClaudeBlocks(ev.Message) {
			if block.Type != "tool_result" {
				continue
			}
			name := p.toolNames[block.ToolUseID]
			if name == "" {
				name = "tool"
			}
			summary := compactText(flattenContent(block.Content), 300)
			if block.IsError {
				if summary == "" {
					summary = "error"
				}
				events = append(events, ParsedEvent{Stream: "tool_result", Text: name + " failed: " + summary})
				continue
			}
			if summary == "" {
				summary = "ok"
			}
			events = append(events, ParsedEvent{Stream: "tool_result", Text: name + ": " + summary})
		}
		return events, true
	case "result":
		res := &ParsedResult{
			Output:     strings.TrimSpace(ev.Result),
			IsError:    ev.IsError || (ev.Subtype != "" && ev.Subtype != "success"),
			SessionID:  ev.SessionID,
			NumTurns:   ev.NumTurns,
			DurationMS: ev.DurationMS,
			CostUSD:    ev.TotalCostUSD,
		}
		if res.IsError {
			res.Error = joinNonEmpty(ev.Errors...)
			if res.Error == "" {
				res.Error = res.Output
			}
			if res.Error == "" {
				res.Error = "claude reported " + firstNonEmpty(ev.Subtype, "an error")
			}
		}
		p.result = res
		return nil, true
	default:
		// rate_limit_event, stream_event and future record types carry nothing
		// the trace needs.
		return nil, true
	}
}

// Result implements OutputParser.
func (p *claudeStreamParser) Result() *ParsedResult {
	if p.result == nil {
		return &ParsedResult{IsError: true, Error: "claude exited without a final result"}
	}
	return p.result
}

// Transcript implements OutputParser.
func (p *claudeStreamParser) Transcript() string { return strings.Join(p.transcript, "\n\n") }

func describeClaudeInit(ev claudeStreamEvent) string {
	parts := []string{"Claude Code session started"}
	if ev.Model != "" {
		parts = append(parts, "model "+ev.Model)
	}
	for _, s := range ev.MCPServers {
		parts = append(parts, fmt.Sprintf("MCP %s %s", s.Name, s.Status))
	}
	switch n := len(ev.Tools); {
	case n == 1:
		parts = append(parts, "1 tool")
	case n > 1:
		parts = append(parts, fmt.Sprintf("%d tools", n))
	}
	return strings.Join(parts, " · ")
}

func decodeClaudeBlocks(msg *struct {
	Content json.RawMessage `json:"content"`
}) []claudeContentBlock {
	if msg == nil || len(msg.Content) == 0 {
		return nil
	}
	var blocks []claudeContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil // a plain-string content field is a user turn, not something to trace
	}
	return blocks
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
