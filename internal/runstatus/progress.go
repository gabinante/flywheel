package runstatus

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"
)

const QuietAfter = 2 * time.Minute
const recentLimit = 20

// Progress separates process ownership, output recency, and actual harness work.
// A live process with no output is quiet, not proof of a hung model request.
type Progress struct {
	WorkerState       string     `json:"worker_state"`
	PID               int        `json:"pid,omitempty"`
	ProcessStartedAt  *time.Time `json:"process_started_at,omitempty"`
	DeadlineAt        *time.Time `json:"deadline_at,omitempty"`
	LastOutputAt      *time.Time `json:"last_output_at,omitempty"`
	LastActivityAt    *time.Time `json:"last_activity_at,omitempty"`
	LastUsageAt       *time.Time `json:"last_usage_at,omitempty"`
	Health            string     `json:"health,omitempty"`
	OutputEvents      int        `json:"output_events"`
	ToolCalls         int        `json:"tool_calls"`
	AssistantMessages int        `json:"assistant_messages"`
	ReasoningUpdates  int        `json:"reasoning_updates"`
	CompletedTurns    int        `json:"completed_turns"`
	TokensIn          *int64     `json:"tokens_in,omitempty"`
	TokensOut         *int64     `json:"tokens_out,omitempty"`
	Recent            []Activity `json:"recent,omitempty"`
	messageUsage      map[string]reportedUsage
}

type Activity struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Summary string    `json:"summary"`
}

type reportedUsage struct {
	Input  int64 `json:"input_tokens"`
	Output int64 `json:"output_tokens"`
	Read   int64 `json:"cache_read_input_tokens"`
	Create int64 `json:"cache_creation_input_tokens"`
}

// Claude emits cumulative usage on multiple blocks of the same message.
func messageUsage(ctx context.Context, id string, u reportedUsage) {
	if id == "" {
		return
	}
	mutate(ctx, func(p *Progress) {
		if p.messageUsage == nil {
			p.messageUsage = make(map[string]reportedUsage)
		}
		old := p.messageUsage[id]
		u.Input, u.Read, u.Create, u.Output = max(old.Input, u.Input), max(old.Read, u.Read), max(old.Create, u.Create), max(old.Output, u.Output)
		in, out := u.Input+u.Read+u.Create-old.Input-old.Read-old.Create, u.Output-old.Output
		if p.TokensIn != nil {
			in += *p.TokensIn
		}
		if p.TokensOut != nil {
			out += *p.TokensOut
		}
		now := time.Now().UTC()
		p.TokensIn, p.TokensOut, p.LastUsageAt = &in, &out, &now
		p.messageUsage[id] = u
	})
}

func (p Progress) At(now time.Time) Progress {
	p.Recent = append([]Activity(nil), p.Recent...)
	p.Health = p.WorkerState
	if p.WorkerState == "running" {
		last := p.ProcessStartedAt
		if p.LastOutputAt != nil {
			last = p.LastOutputAt
		}
		p.Health = "active"
		if last != nil && now.Sub(*last) >= QuietAfter {
			p.Health = "quiet"
		}
	}
	return p
}

func mutate(ctx context.Context, f func(*Progress)) {
	h, ok := ctx.Value(contextKey{}).(*handle)
	if !ok {
		return
	}
	h.registry.mu.Lock()
	defer h.registry.changed()
	defer h.registry.mu.Unlock()
	if v, exists := h.registry.runs[h.id]; exists {
		f(&v.Progress)
		h.registry.runs[h.id] = v
	}
}

func Started(ctx context.Context, pid int) {
	Running(ctx)
	mutate(ctx, func(p *Progress) {
		now := time.Now().UTC()
		p.PID, p.WorkerState, p.ProcessStartedAt = pid, "running", &now
		if deadline, ok := ctx.Deadline(); ok {
			p.DeadlineAt = &deadline
		}
	})
}

func Exited(ctx context.Context) {
	mutate(ctx, func(p *Progress) { p.WorkerState = "exited" })
}

func Output(ctx context.Context) {
	mutate(ctx, func(p *Progress) {
		now := time.Now().UTC()
		p.LastOutputAt = &now
		p.OutputEvents++
	})
}

func activity(ctx context.Context, kind, summary string) {
	mutate(ctx, func(p *Progress) {
		now := time.Now().UTC()
		p.LastActivityAt = &now
		switch kind {
		case "tool":
			p.ToolCalls++
		case "assistant":
			p.AssistantMessages++
		case "reasoning":
			p.ReasoningUpdates++
		case "turn":
			p.CompletedTurns++
		}
		// Partial tokens update recency, but coalesce into one row in the timeline.
		if kind == "stream" && len(p.Recent) > 0 && p.Recent[len(p.Recent)-1].Kind == kind {
			p.Recent[len(p.Recent)-1].At = now
			return
		}
		p.Recent = append(p.Recent, Activity{At: now, Kind: kind, Summary: summary})
		if len(p.Recent) > recentLimit {
			p.Recent = append([]Activity(nil), p.Recent[len(p.Recent)-recentLimit:]...)
		}
	})
}

func usage(ctx context.Context, in, out int64, additive bool) {
	mutate(ctx, func(p *Progress) {
		now := time.Now().UTC()
		p.LastUsageAt = &now
		if additive && p.TokensIn != nil {
			in += *p.TokensIn
		}
		if additive && p.TokensOut != nil {
			out += *p.TokensOut
		}
		p.TokensIn, p.TokensOut = &in, &out
	})
}

// ParseOutput records only observable progress. Reasoning contents, tool arguments,
// tool results, and stderr are not copied into the global tray or session metadata.
func ParseOutput(ctx context.Context, line []byte) {
	Output(ctx)
	var e struct {
		Type, Subtype string
		SessionID     string                            `json:"session_id"`
		ThreadID      string                            `json:"thread_id"`
		Item          struct{ Type, Name, Tool string } `json:"item"`
		Message       struct {
			ID      string                        `json:"id"`
			Usage   *reportedUsage                `json:"usage"`
			Content []struct{ Type, Name string } `json:"content"`
		} `json:"message"`
		Event struct{ Type string } `json:"event"`
		Usage *struct {
			Input  int64 `json:"input_tokens"`
			Output int64 `json:"output_tokens"`
			Read   int64 `json:"cache_read_input_tokens"`
			Create int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(line, &e) != nil {
		return
	}
	switch e.Type {
	case "thread.started":
		Session(ctx, e.ThreadID)
		activity(ctx, "session", "Session connected")
	case "system":
		Session(ctx, e.SessionID)
		if e.Subtype == "init" {
			activity(ctx, "session", "Session connected")
		}
	case "turn.started":
		activity(ctx, "status", "Model turn started")
	case "turn.completed":
		activity(ctx, "turn", "Model turn completed")
		if e.Usage != nil {
			usage(ctx, e.Usage.Input, e.Usage.Output, true)
		}
	case "item.started", "item.updated", "item.completed":
		kind, label := "status", ""
		switch e.Item.Type {
		case "command_execution":
			label = "Command"
		case "mcp_tool_call":
			label = "Tool " + safeName(e.Item.Tool)
		case "web_search":
			label = "Web search"
		case "file_change":
			label = "File change"
		case "reasoning":
			if e.Type == "item.completed" {
				kind, label = "reasoning", "Reasoning update received"
			}
		case "agent_message":
			if e.Type == "item.completed" {
				kind, label = "assistant", "Assistant message received"
			}
		case "todo_list":
			label = "Plan updated"
		}
		if label == "" {
			return
		}
		if e.Item.Type == "command_execution" || e.Item.Type == "mcp_tool_call" || e.Item.Type == "web_search" || e.Item.Type == "file_change" {
			if e.Type == "item.started" {
				kind, label = "tool", label+" started"
			} else {
				label += " updated"
			}
			if e.Type == "item.completed" {
				label = strings.TrimSuffix(label, " updated") + " completed"
			}
		}
		activity(ctx, kind, label)
	case "assistant":
		activity(ctx, "assistant", "Assistant message received")
		if e.Message.Usage != nil {
			messageUsage(ctx, e.Message.ID, *e.Message.Usage)
		}
		for _, block := range e.Message.Content {
			switch block.Type {
			case "tool_use":
				activity(ctx, "tool", "Tool "+safeName(block.Name)+" started")
			case "thinking":
				activity(ctx, "reasoning", "Reasoning update received")
			}
		}
	case "user":
		for _, block := range e.Message.Content {
			if block.Type == "tool_result" {
				activity(ctx, "status", "Tool completed")
			}
		}
	case "stream_event":
		if e.Event.Type == "content_block_delta" {
			activity(ctx, "stream", "Model output streaming")
		}
	case "result":
		Session(ctx, e.SessionID)
		activity(ctx, "turn", "Harness result received")
		if e.Usage != nil {
			usage(ctx, e.Usage.Input+e.Usage.Read+e.Usage.Create, e.Usage.Output, false)
		}
	case "error", "turn.failed":
		activity(ctx, "error", "Harness reported an error")
	case "rate_limit_event":
		activity(ctx, "status", "Rate limit status received")
	}
}

func safeName(s string) string {
	if len(s) > 80 {
		s = s[:80]
	}
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_.:-", r) {
			return r
		}
		return -1
	}, s)
}

type outputWriter struct {
	ctx context.Context
	dst io.Writer
}

func (w outputWriter) Write(b []byte) (int, error) {
	if len(b) > 0 {
		Output(w.ctx)
	}
	return w.dst.Write(b)
}

// WatchOutput tracks unstructured output recency without retaining its contents.
func WatchOutput(ctx context.Context, dst io.Writer) io.Writer { return outputWriter{ctx, dst} }
