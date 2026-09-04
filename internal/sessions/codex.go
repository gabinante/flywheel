package sessions

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // read-only access to Codex's state database
)

// codexCollector ingests Codex sessions from CODEX_HOME (~/.codex).
//
// The desktop app and CLI index every thread in state_N.sqlite (title, cwd, git info, model, token totals,
// spawn edges) and write the full transcript to sessions/YYYY/MM/DD/rollout-*.jsonl. The index is the source
// of session metadata; rollouts are tailed for prompts, tool calls, and token splits.
type codexCollector struct {
	home  string
	store *Store
	repos *repoResolver

	importsLoaded bool
	imported      map[string]bool // Codex threads that are imports of Claude sessions; skipped to avoid duplicates
}

type codexThread struct {
	ID               string
	RolloutPath      string
	CreatedAt        int64
	UpdatedAt        int64
	Source           string
	CWD              string
	Title            string
	GitBranch        string
	GitOriginURL     string
	Model            string
	ReasoningEffort  string
	TokensUsed       int64
	Archived         int64
	ThreadSource     string
	FirstUserMessage string
	AgentNickname    string
}

type codexRolloutLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type codexEventMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Info    *struct {
		Total *struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"total_token_usage"`
	} `json:"info"`
}

type codexResponseItem struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Name    string `json:"name"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type codexSessionMeta struct {
	CWD    string `json:"cwd"`
	Source any    `json:"source"`
	Git    *struct {
		Branch        string `json:"branch"`
		RepositoryURL string `json:"repository_url"`
	} `json:"git"`
}

func newCodexCollector(home string, store *Store, repos *repoResolver) *codexCollector {
	return &codexCollector{home: home, store: store, repos: repos, imported: map[string]bool{}}
}

// statePath returns the newest state_N.sqlite in CODEX_HOME.
func (c *codexCollector) statePath() (string, error) {
	matches, err := filepath.Glob(filepath.Join(c.home, "state_*.sqlite"))
	if err != nil || len(matches) == 0 {
		return "", os.ErrNotExist
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

func (c *codexCollector) loadImports() {
	if c.importsLoaded {
		return
	}
	c.importsLoaded = true
	b, err := os.ReadFile(filepath.Join(c.home, "external_agent_session_imports.json"))
	if err != nil {
		return
	}
	var doc struct {
		Records []struct {
			ImportedThreadID string `json:"imported_thread_id"`
		} `json:"records"`
	}
	if json.Unmarshal(b, &doc) != nil {
		return
	}
	for _, r := range doc.Records {
		if r.ImportedThreadID != "" {
			c.imported[r.ImportedThreadID] = true
		}
	}
}

func (c *codexCollector) run(ctx context.Context) (int, error) {
	dbPath, err := c.statePath()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	c.loadImports()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(3000)&_pragma=query_only(1)")
	if err != nil {
		return 0, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT id, rollout_path, created_at, updated_at, source, cwd, title,
		       COALESCE(git_branch,''), COALESCE(git_origin_url,''), COALESCE(model,''), COALESCE(reasoning_effort,''),
		       tokens_used, archived, COALESCE(thread_source,''), COALESCE(first_user_message,''), COALESCE(agent_nickname,'')
		FROM threads
		WHERE source NOT LIKE '%guardian%' AND COALESCE(model,'') <> 'codex-auto-review'`)
	if err != nil {
		return 0, err
	}
	var threads []codexThread
	for rows.Next() {
		var t codexThread
		if err := rows.Scan(&t.ID, &t.RolloutPath, &t.CreatedAt, &t.UpdatedAt, &t.Source, &t.CWD, &t.Title,
			&t.GitBranch, &t.GitOriginURL, &t.Model, &t.ReasoningEffort, &t.TokensUsed, &t.Archived, &t.ThreadSource,
			&t.FirstUserMessage, &t.AgentNickname); err != nil {
			rows.Close()
			return 0, err
		}
		threads = append(threads, t)
	}
	rows.Close()

	offsets, err := c.store.IngestState(ctx, HarnessCodex)
	if err != nil {
		return 0, err
	}
	touched := 0
	for _, t := range threads {
		if ctx.Err() != nil {
			return touched, ctx.Err()
		}
		if c.imported[t.ID] {
			continue
		}
		ok, err := c.ingestThread(ctx, t, offsets[t.RolloutPath])
		if err != nil {
			slog.Warn("sessions: codex ingest failed", "thread", t.ID, "error", err)
			continue
		}
		if ok {
			touched++
		}
	}
	return touched, nil
}

// codexOrigin classifies a thread from its source JSON/string and thread_source.
func codexOrigin(source, threadSource string) (Origin, string) {
	if threadSource == "subagent" || strings.Contains(source, "thread_spawn") {
		var doc struct {
			Subagent struct {
				ThreadSpawn struct {
					ParentThreadID string `json:"parent_thread_id"`
				} `json:"thread_spawn"`
			} `json:"subagent"`
		}
		_ = json.Unmarshal([]byte(source), &doc)
		return OriginSubagent, doc.Subagent.ThreadSpawn.ParentThreadID
	}
	s := strings.ToLower(source)
	switch {
	case strings.Contains(s, "exec"), strings.Contains(s, "flywheel"):
		return OriginDispatched, ""
	default:
		return OriginInteractive, ""
	}
}

func (c *codexCollector) ingestThread(ctx context.Context, t codexThread, offset int64) (bool, error) {
	existing, err := c.store.GetByExternal(ctx, HarnessCodex, t.ID)
	if err != nil {
		return false, err
	}
	updated := time.Unix(t.UpdatedAt, 0).UTC()
	rolloutGrew := false
	if t.RolloutPath != "" {
		if st, err := os.Stat(t.RolloutPath); err == nil && st.Size() > offset {
			rolloutGrew = true
		}
	}
	if existing != nil && !rolloutGrew && !updated.After(existing.LastActivityAt) && (t.Archived == 0) == (existing.EndedAt == nil) {
		return false, nil
	}
	sess := existing
	if sess == nil {
		sess = &Session{Harness: HarnessCodex, ExternalID: t.ID, Metadata: map[string]any{}}
	}
	if sess.Metadata == nil {
		sess.Metadata = map[string]any{}
	}
	origin, parent := codexOrigin(t.Source, t.ThreadSource)
	sess.Origin = origin
	if parent != "" {
		sess.ParentExternalID = parent
	}
	sess.CWD = t.CWD
	if t.GitBranch != "HEAD" {
		sess.Branch = t.GitBranch
	}
	sess.Model = t.Model
	sess.ReasoningEffort = t.ReasoningEffort
	sess.TranscriptPath = t.RolloutPath
	if t.Title != "" {
		sess.Title = truncate(strings.TrimSpace(t.Title), 200)
	}
	if t.AgentNickname != "" {
		sess.Metadata["agent_nickname"] = t.AgentNickname
		if sess.Title == "" {
			sess.Title = t.AgentNickname
		}
	}
	sess.Metadata["source"] = t.Source
	if fum := strings.TrimSpace(t.FirstUserMessage); fum != "" && sess.FirstPrompt == "" {
		sess.FirstPrompt = truncate(fum, 500)
	}
	created := time.Unix(t.CreatedAt, 0).UTC()
	if sess.StartedAt.IsZero() || created.Before(sess.StartedAt) {
		sess.StartedAt = created
	}
	if updated.After(sess.LastActivityAt) {
		sess.LastActivityAt = updated
	}
	if t.Archived != 0 {
		end := updated
		sess.EndedAt = &end
	} else {
		sess.EndedAt = nil
	}
	sess.Repo = c.repos.Resolve(ctx, t.CWD, t.GitOriginURL)

	var prompts []Prompt
	var links []Link
	if rolloutGrew {
		p, l, consumed, err := c.tailRollout(ctx, sess, t.RolloutPath, offset)
		if err != nil {
			slog.Warn("sessions: codex rollout tail failed", "thread", t.ID, "error", err)
		} else {
			prompts, links = p, l
			sess.IngestOffset = consumed
		}
	}
	if sess.TokensIn == 0 && sess.TokensOut == 0 && t.TokensUsed > 0 {
		sess.TokensIn = t.TokensUsed // index only has a total; treat it as input until the rollout says otherwise
	}
	links = append(links, ExtractRefs(t.Title)...)
	links = append(links, ExtractRefs(t.FirstUserMessage)...)
	links = append(links, RefsFromBranch(t.GitBranch)...)

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

// tailRollout reads new rollout lines from offset, updating sess counters in place.
func (c *codexCollector) tailRollout(ctx context.Context, sess *Session, path string, offset int64) ([]Prompt, []Link, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, offset, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, nil, offset, err
	}
	r := bufio.NewReaderSize(f, 1<<20)
	consumed := offset
	var prompts []Prompt
	var links []Link
	for {
		if ctx.Err() != nil {
			return prompts, links, consumed, ctx.Err()
		}
		line, rerr := r.ReadBytes('\n')
		if rerr != nil && !errors.Is(rerr, io.EOF) {
			return prompts, links, consumed, rerr
		}
		if errors.Is(rerr, io.EOF) && (len(line) == 0 || line[len(line)-1] != '\n') {
			break
		}
		consumed += int64(len(line))
		var l codexRolloutLine
		if json.Unmarshal(line, &l) != nil {
			continue
		}
		ts, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
		if ts.IsZero() {
			ts = sess.LastActivityAt
		}
		switch l.Type {
		case "session_meta":
			var m codexSessionMeta
			if json.Unmarshal(l.Payload, &m) == nil {
				if m.CWD != "" && sess.CWD == "" {
					sess.CWD = m.CWD
				}
				if m.Git != nil {
					if m.Git.Branch != "" && sess.Branch == "" {
						sess.Branch = m.Git.Branch
					}
					if m.Git.RepositoryURL != "" && sess.Repo == "" {
						sess.Repo = RepoFromOriginURL(m.Git.RepositoryURL)
					}
				}
			}
		case "event_msg":
			var e codexEventMsg
			if json.Unmarshal(l.Payload, &e) != nil {
				continue
			}
			switch e.Type {
			case "user_message":
				if text := codexPromptText(e.Message); text != "" {
					sess.PromptCount++
					prompts = append(prompts, Prompt{Seq: sess.PromptCount, Role: "user", Text: truncate(text, 20000), TS: ts})
					if sess.FirstPrompt == "" {
						sess.FirstPrompt = truncate(text, 500)
					}
					links = append(links, ExtractRefs(text)...)
				}
			case "token_count":
				if e.Info != nil && e.Info.Total != nil {
					sess.TokensIn = e.Info.Total.InputTokens
					sess.TokensOut = e.Info.Total.OutputTokens
				}
			}
		case "response_item":
			var it codexResponseItem
			if json.Unmarshal(l.Payload, &it) != nil {
				continue
			}
			switch it.Type {
			case "function_call", "custom_tool_call", "local_shell_call":
				sess.ToolCallCount++
			case "message":
				if it.Role != "user" {
					continue
				}
				var parts []string
				for _, cpart := range it.Content {
					if cpart.Type == "input_text" || cpart.Type == "text" {
						if t := codexPromptText(cpart.Text); t != "" {
							parts = append(parts, t)
						}
					}
				}
				if len(parts) == 0 {
					continue
				}
				text := strings.Join(parts, "\n")
				// Desktop rollouts also emit event_msg/user_message for the same prompt; prefer that path.
				if len(prompts) > 0 && prompts[len(prompts)-1].Text == truncate(text, 20000) {
					continue
				}
				sess.PromptCount++
				prompts = append(prompts, Prompt{Seq: sess.PromptCount, Role: "user", Text: truncate(text, 20000), TS: ts})
				if sess.FirstPrompt == "" {
					sess.FirstPrompt = truncate(text, 500)
				}
				links = append(links, ExtractRefs(text)...)
			}
		}
	}
	return prompts, links, consumed, nil
}

// codexPromptText filters out the instruction blobs Codex injects as user-role
// messages (AGENTS.md contents, permissions, environment context).
func codexPromptText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "<") {
		return ""
	}
	head := s
	if len(head) > 200 {
		head = head[:200]
	}
	if strings.HasPrefix(head, "# AGENTS.md") || strings.Contains(head, "AGENTS.md instructions for") ||
		strings.HasPrefix(head, "--- project-doc") || strings.HasPrefix(head, "# Writing PRs and Linear tickets") {
		return ""
	}
	return s
}
