package dispatch

import (
	"encoding/json"
	"strings"
)

// codexStreamParser retains the final answer and identity, with semantic trace events.
type codexStreamParser struct {
	result     ParsedResult
	transcript []string
	completed  bool
}

func (d *CodexDriver) NewOutputParser() OutputParser { return &codexStreamParser{} }
func (p *codexStreamParser) ParseLine(line string) ([]ParsedEvent, bool) {
	var ev struct {
		Type     string `json:"type"`
		ThreadID string `json:"thread_id"`
		Message  string `json:"message"`
		Error    struct {
			Message string `json:"message"`
		} `json:"error"`
		Item struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Command string `json:"command"`
			Output  string `json:"aggregated_output"`
		} `json:"item"`
	}
	if json.Unmarshal([]byte(line), &ev) != nil || ev.Type == "" {
		return nil, false
	}
	switch ev.Type {
	case "thread.started":
		p.result.SessionID = ev.ThreadID
	case "turn.completed":
		p.completed = true
	case "turn.failed", "error":
		p.completed = true
		p.result.IsError = true
		p.result.Error = joinNonEmpty(ev.Error.Message, ev.Message)
	case "item.completed":
		switch ev.Item.Type {
		case "agent_message":
			p.result.Output = ev.Item.Text
			p.transcript = append(p.transcript, ev.Item.Text)
			return []ParsedEvent{{Stream: "assistant", Text: ev.Item.Text}}, true
		case "command_execution":
			return []ParsedEvent{{Stream: "tool_result", Text: joinNonEmpty(ev.Item.Command, ev.Item.Output)}}, true
		}
	}
	return nil, true
}
func (p *codexStreamParser) Result() *ParsedResult {
	if !p.completed {
		p.result.IsError = true
		p.result.Error = "codex exited without a completed turn"
	}
	return &p.result
}
func (p *codexStreamParser) Transcript() string { return strings.Join(p.transcript, "\n\n") }
