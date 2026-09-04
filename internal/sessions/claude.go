package sessions

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// claudeCollector ingests Claude Code transcripts from ~/.claude/projects.
//
// Layout: <root>/<cwd-slug>/<sessionId>.jsonl is a session; <root>/<cwd-slug>/<sessionId>/subagents/agent-<id>.jsonl
// (with a sibling .meta.json) is a subagent spawned by that session. Files are append-only, so each is
// tailed from the last consumed byte offset.
type claudeCollector struct {
	root  string
	store *Store
	repos *repoResolver
}

// claudeLine is the subset of a transcript line the collector cares about.
type claudeLine struct {
	Type        string `json:"type"`
	SessionID   string `json:"sessionId"`
	Timestamp   string `json:"timestamp"`
	CWD         string `json:"cwd"`
	GitBranch   string `json:"gitBranch"`
	Version     string `json:"version"`
	Entrypoint  string `json:"entrypoint"`
	IsMeta      bool   `json:"isMeta"`
	IsSidechain bool   `json:"isSidechain"`
	AgentID     string `json:"agentId"`
	Message     *struct {
		Role    string          `json:"role"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			InputTokens   int64 `json:"input_tokens"`
			OutputTokens  int64 `json:"output_tokens"`
			CacheCreation int64 `json:"cache_creation_input_tokens"`
			CacheRead     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	AITitle      string `json:"aiTitle"`
	PRURL        string `json:"prUrl"`
	PRRepository string `json:"prRepository"`
	PRNumber     int    `json:"prNumber"`
}

type claudeBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Name string `json:"name"`
}

type claudeSubagentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
	SpawnDepth  int    `json:"spawnDepth"`
}

func newClaudeCollector(root string, store *Store, repos *repoResolver) *claudeCollector {
	return &claudeCollector{root: root, store: store, repos: repos}
}

// run ingests all new transcript bytes. It returns the number of sessions touched.
func (c *claudeCollector) run(ctx context.Context) (int, error) {
	projDirs, err := os.ReadDir(c.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	offsets, err := c.store.IngestState(ctx, HarnessClaudeCode)
	if err != nil {
		return 0, err
	}
	touched := 0
	for _, pd := range projDirs {
		if !pd.IsDir() || pd.Name() == "memory" {
			continue
		}
		dir := filepath.Join(c.root, pd.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if ctx.Err() != nil {
				return touched, ctx.Err()
			}
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				path := filepath.Join(dir, e.Name())
				if ok, err := c.ingestFile(ctx, path, offsets[path], "", nil); err != nil {
					slog.Warn("sessions: claude ingest failed", "path", path, "error", err)
				} else if ok {
					touched++
				}
				continue
			}
			if e.IsDir() {
				subDir := filepath.Join(dir, e.Name(), "subagents")
				subs, err := os.ReadDir(subDir)
				if err != nil {
					continue
				}
				for _, se := range subs {
					if !strings.HasSuffix(se.Name(), ".jsonl") {
						continue
					}
					path := filepath.Join(subDir, se.Name())
					var meta *claudeSubagentMeta
					if b, err := os.ReadFile(strings.TrimSuffix(path, ".jsonl") + ".meta.json"); err == nil {
						var m claudeSubagentMeta
						if json.Unmarshal(b, &m) == nil {
							meta = &m
						}
					}
					if ok, err := c.ingestFile(ctx, path, offsets[path], e.Name(), meta); err != nil {
						slog.Warn("sessions: claude subagent ingest failed", "path", path, "error", err)
					} else if ok {
						touched++
					}
				}
			}
		}
	}
	return touched, nil
}

// ingestFile tails one transcript from offset. parentSID is set for subagent transcripts.
// It returns true when the session row was written.
func (c *claudeCollector) ingestFile(ctx context.Context, path string, offset int64, parentSID string, meta *claudeSubagentMeta) (bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if st.Size() <= offset {
		return false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return false, err
	}

	var (
		sess      *Session
		prompts   []Prompt
		links     []Link
		consumed  = offset
		r         = bufio.NewReaderSize(f, 1<<20)
		lastTS    time.Time
		firstTS   time.Time
		agentID   string
		sessionID string
	)
	for {
		line, rerr := r.ReadBytes('\n')
		if rerr != nil && !errors.Is(rerr, io.EOF) {
			return false, rerr
		}
		if errors.Is(rerr, io.EOF) && (len(line) == 0 || line[len(line)-1] != '\n') {
			break // partial trailing line: leave for the next pass
		}
		consumed += int64(len(line))
		var l claudeLine
		if json.Unmarshal(line, &l) != nil {
			continue
		}
		if sess == nil {
			sid := l.SessionID
			if sid == "" {
				sid = parentSID
			}
			if sid == "" {
				sid = strings.TrimSuffix(filepath.Base(path), ".jsonl")
			}
			sessionID = sid
			external := sid
			if parentSID != "" {
				agentID = strings.TrimPrefix(strings.TrimSuffix(filepath.Base(path), ".jsonl"), "agent-")
				external = parentSID + "/" + agentID
			}
			existing, err := c.store.GetByExternal(ctx, HarnessClaudeCode, external)
			if err != nil {
				return false, err
			}
			if existing != nil {
				sess = existing
			} else {
				sess = &Session{Harness: HarnessClaudeCode, ExternalID: external, Origin: OriginInteractive, Metadata: map[string]any{}}
			}
			sess.TranscriptPath = path
			if parentSID != "" {
				sess.Origin = OriginSubagent
				sess.ParentExternalID = parentSID
				if meta != nil {
					if sess.Title == "" {
						sess.Title = strings.TrimSpace(meta.AgentType + ": " + meta.Description)
					}
					sess.Metadata["agent_type"] = meta.AgentType
					sess.Metadata["spawn_depth"] = meta.SpawnDepth
				}
			}
		}
		if l.AgentID != "" && agentID == "" && l.IsSidechain {
			agentID = l.AgentID
		}
		if ts, err := time.Parse(time.RFC3339Nano, l.Timestamp); err == nil {
			if firstTS.IsZero() || ts.Before(firstTS) {
				firstTS = ts
			}
			if ts.After(lastTS) {
				lastTS = ts
			}
		}
		if l.CWD != "" {
			sess.CWD = l.CWD
		}
		if l.GitBranch != "" && l.GitBranch != "HEAD" { // "HEAD" = detached checkout, not a branch
			sess.Branch = l.GitBranch
		}
		if l.Version != "" {
			sess.Metadata["harness_version"] = l.Version
		}
		if l.Entrypoint != "" && parentSID == "" {
			sess.Metadata["entrypoint"] = l.Entrypoint
			if o := claudeOrigin(l.Entrypoint); o != OriginInteractive {
				sess.Origin = o
			}
		}
		switch l.Type {
		case "user":
			if l.IsMeta || l.Message == nil {
				continue
			}
			text := claudeText(l.Message.Content)
			if text == "" || strings.HasPrefix(text, "<") {
				continue
			}
			ts := lastTS
			if ts.IsZero() {
				ts = time.Now()
			}
			sess.PromptCount++
			prompts = append(prompts, Prompt{Seq: sess.PromptCount, Role: "user", Text: truncate(text, 20000), TS: ts})
			if sess.FirstPrompt == "" {
				sess.FirstPrompt = truncate(text, 500)
			}
			links = append(links, ExtractRefs(text)...)
		case "assistant":
			if l.Message == nil {
				continue
			}
			if l.Message.Model != "" {
				sess.Model = l.Message.Model
			}
			if u := l.Message.Usage; u != nil {
				sess.TokensIn += u.InputTokens + u.CacheCreation + u.CacheRead
				sess.TokensOut += u.OutputTokens
			}
			sess.ToolCallCount += claudeToolUses(l.Message.Content)
		case "ai-title":
			if l.AITitle != "" {
				sess.Title = l.AITitle
			}
		case "pr-link":
			if l.PRRepository != "" && l.PRNumber > 0 {
				links = append(links, Link{Kind: LinkPR, Ref: strings.TrimSuffix(l.PRRepository, ".git") + "#" + itoa(l.PRNumber), Source: LinkSourceInferred})
			} else if l.PRURL != "" {
				links = append(links, ExtractRefs(l.PRURL)...)
			}
		}
	}
	if sess == nil {
		return false, nil
	}
	_ = sessionID
	if sess.StartedAt.IsZero() {
		if !firstTS.IsZero() {
			sess.StartedAt = firstTS
		} else {
			sess.StartedAt = st.ModTime()
		}
	} else if !firstTS.IsZero() && firstTS.Before(sess.StartedAt) {
		sess.StartedAt = firstTS
	}
	if lastTS.After(sess.LastActivityAt) {
		sess.LastActivityAt = lastTS
	}
	if sess.LastActivityAt.IsZero() {
		sess.LastActivityAt = st.ModTime()
	}
	if sess.Repo == "" {
		sess.Repo = c.repos.Resolve(ctx, sess.CWD, "")
	}
	if sess.Branch != "" {
		links = append(links, RefsFromBranch(sess.Branch)...)
	}
	if sess.Title == "" && sess.FirstPrompt != "" {
		sess.Title = truncate(sess.FirstPrompt, 80)
	}
	sess.IngestOffset = consumed
	if err := c.store.Upsert(ctx, sess); err != nil {
		return false, err
	}
	if err := c.store.AppendPrompts(ctx, sess.ID, prompts); err != nil {
		return false, err
	}
	if err := c.store.AddLinks(ctx, sess.ID, links); err != nil {
		return false, err
	}
	return true, nil
}

// claudeOrigin maps Claude Code's entrypoint marker to an Origin. Dispatched
// workers set CLAUDE_CODE_ENTRYPOINT to a Flywheel-specific value.
func claudeOrigin(entrypoint string) Origin {
	e := strings.ToLower(entrypoint)
	switch {
	case strings.Contains(e, "flywheel"), strings.Contains(e, "warrant"), strings.Contains(e, "dispatch"):
		return OriginDispatched
	case strings.HasPrefix(e, "sdk"):
		return OriginDispatched
	default:
		return OriginInteractive
	}
}

// claudeText extracts operator-visible text from a user message's content.
func claudeText(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return strings.TrimSpace(s)
		}
		return ""
	}
	var blocks []claudeBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, strings.TrimSpace(b.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func claudeToolUses(raw json.RawMessage) int {
	var blocks []claudeBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return 0
	}
	n := 0
	for _, b := range blocks {
		if b.Type == "tool_use" {
			n++
		}
	}
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func itoa(n int) string {
	return json.Number(strconvItoa(n)).String()
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
