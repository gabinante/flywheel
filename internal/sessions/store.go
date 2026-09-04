package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mustUUID() string { return uuid.Must(uuid.NewV7()).String() }

// Store persists sessions, links, and prompts.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const sessionCols = `s.id, s.harness, s.external_id, s.origin, COALESCE(s.parent_session_id, ''), s.parent_external_id,
	s.cwd, s.repo, s.branch, s.model, s.reasoning_effort, s.title, s.first_prompt, s.transcript_path,
	s.tokens_in, s.tokens_out, s.prompt_count, s.tool_call_count, s.started_at, s.last_activity_at, s.ended_at,
	s.ingest_offset, s.metadata`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSession(r rowScanner) (*Session, error) {
	var s Session
	var meta []byte
	if err := r.Scan(&s.ID, &s.Harness, &s.ExternalID, &s.Origin, &s.ParentSessionID, &s.ParentExternalID,
		&s.CWD, &s.Repo, &s.Branch, &s.Model, &s.ReasoningEffort, &s.Title, &s.FirstPrompt, &s.TranscriptPath,
		&s.TokensIn, &s.TokensOut, &s.PromptCount, &s.ToolCallCount, &s.StartedAt, &s.LastActivityAt, &s.EndedAt,
		&s.IngestOffset, &meta); err != nil {
		return nil, err
	}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &s.Metadata)
	}
	return &s, nil
}

// Upsert inserts or updates a session keyed by (harness, external_id). Empty
// descriptive fields never overwrite existing values; counters are absolute.
// The stored ID is written back into sess.
func (st *Store) Upsert(ctx context.Context, sess *Session) error {
	if sess.ID == "" {
		sess.ID = mustUUID()
	}
	if sess.Metadata == nil {
		sess.Metadata = map[string]any{}
	}
	meta, _ := json.Marshal(sess.Metadata)
	err := st.pool.QueryRow(ctx, `
		INSERT INTO agent_sessions (id, harness, external_id, origin, parent_external_id, cwd, repo, branch, model,
			reasoning_effort, title, first_prompt, transcript_path, tokens_in, tokens_out, prompt_count, tool_call_count,
			started_at, last_activity_at, ended_at, ingest_offset, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
		ON CONFLICT (harness, external_id) DO UPDATE SET
			origin             = CASE WHEN agent_sessions.origin IN ('dispatched','automation') AND EXCLUDED.origin = 'interactive' THEN agent_sessions.origin ELSE EXCLUDED.origin END,
			parent_external_id = CASE WHEN EXCLUDED.parent_external_id <> '' THEN EXCLUDED.parent_external_id ELSE agent_sessions.parent_external_id END,
			cwd                = CASE WHEN EXCLUDED.cwd <> '' THEN EXCLUDED.cwd ELSE agent_sessions.cwd END,
			repo               = CASE WHEN EXCLUDED.repo <> '' THEN EXCLUDED.repo ELSE agent_sessions.repo END,
			branch             = CASE WHEN EXCLUDED.branch <> '' THEN EXCLUDED.branch ELSE agent_sessions.branch END,
			model              = CASE WHEN EXCLUDED.model <> '' THEN EXCLUDED.model ELSE agent_sessions.model END,
			reasoning_effort   = CASE WHEN EXCLUDED.reasoning_effort <> '' THEN EXCLUDED.reasoning_effort ELSE agent_sessions.reasoning_effort END,
			title              = CASE WHEN EXCLUDED.title <> '' THEN EXCLUDED.title ELSE agent_sessions.title END,
			first_prompt       = CASE WHEN EXCLUDED.first_prompt <> '' THEN EXCLUDED.first_prompt ELSE agent_sessions.first_prompt END,
			transcript_path    = CASE WHEN EXCLUDED.transcript_path <> '' THEN EXCLUDED.transcript_path ELSE agent_sessions.transcript_path END,
			tokens_in          = EXCLUDED.tokens_in,
			tokens_out         = EXCLUDED.tokens_out,
			prompt_count       = EXCLUDED.prompt_count,
			tool_call_count    = EXCLUDED.tool_call_count,
			started_at         = LEAST(agent_sessions.started_at, EXCLUDED.started_at),
			last_activity_at   = GREATEST(agent_sessions.last_activity_at, EXCLUDED.last_activity_at),
			ended_at           = EXCLUDED.ended_at,
			ingest_offset      = EXCLUDED.ingest_offset,
			metadata           = agent_sessions.metadata || EXCLUDED.metadata,
			updated_at         = now()
		RETURNING id`,
		sess.ID, string(sess.Harness), sess.ExternalID, string(sess.Origin), sess.ParentExternalID, sess.CWD, sess.Repo,
		sess.Branch, sess.Model, sess.ReasoningEffort, sess.Title, sess.FirstPrompt, sess.TranscriptPath, sess.TokensIn,
		sess.TokensOut, sess.PromptCount, sess.ToolCallCount, sess.StartedAt, sess.LastActivityAt, sess.EndedAt,
		sess.IngestOffset, meta).Scan(&sess.ID)
	return err
}

// GetByExternal returns the session for a harness-native id, or nil when unknown.
func (st *Store) GetByExternal(ctx context.Context, harness Harness, externalID string) (*Session, error) {
	row := st.pool.QueryRow(ctx, `SELECT `+sessionCols+` FROM agent_sessions s WHERE s.harness = $1 AND s.external_id = $2`, string(harness), externalID)
	s, err := scanSession(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// Get returns a session with its links, or nil when unknown.
func (st *Store) Get(ctx context.Context, id string) (*Session, error) {
	row := st.pool.QueryRow(ctx, `SELECT `+sessionCols+` FROM agent_sessions s WHERE s.id = $1`, id)
	s, err := scanSession(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	links, err := st.linksFor(ctx, []string{s.ID})
	if err != nil {
		return nil, err
	}
	s.Links = links[s.ID]
	return s, nil
}

// IngestState returns transcript path → consumed byte offset for a harness.
func (st *Store) IngestState(ctx context.Context, harness Harness) (map[string]int64, error) {
	rows, err := st.pool.Query(ctx, `SELECT transcript_path, ingest_offset FROM agent_sessions WHERE harness = $1 AND transcript_path <> ''`, string(harness))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var p string
		var off int64
		if err := rows.Scan(&p, &off); err != nil {
			return nil, err
		}
		out[p] = off
	}
	return out, rows.Err()
}

// AppendPrompts stores prompts, ignoring sequence numbers already present.
func (st *Store) AppendPrompts(ctx context.Context, sessionID string, prompts []Prompt) error {
	if len(prompts) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, p := range prompts {
		id := p.ID
		if id == "" {
			id = mustUUID()
		}
		batch.Queue(`INSERT INTO session_prompts (id, session_id, seq, role, text, ts) VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (session_id, seq) DO NOTHING`, id, sessionID, p.Seq, p.Role, p.Text, p.TS)
	}
	res := st.pool.SendBatch(ctx, batch)
	defer res.Close()
	for range prompts {
		if _, err := res.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// AddLinks records links, ignoring duplicates.
func (st *Store) AddLinks(ctx context.Context, sessionID string, links []Link) error {
	if len(links) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, l := range links {
		src := l.Source
		if src == "" {
			src = LinkSourceInferred
		}
		batch.Queue(`INSERT INTO session_links (session_id, kind, ref, source) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, sessionID, l.Kind, l.Ref, src)
	}
	res := st.pool.SendBatch(ctx, batch)
	defer res.Close()
	for range links {
		if _, err := res.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// ResolveParents fills parent_session_id for sessions whose parent has since been ingested.
func (st *Store) ResolveParents(ctx context.Context) (int64, error) {
	tag, err := st.pool.Exec(ctx, `UPDATE agent_sessions c SET parent_session_id = p.id
		FROM agent_sessions p
		WHERE c.parent_session_id IS NULL AND c.parent_external_id <> ''
		  AND p.harness = c.harness AND p.external_id = c.parent_external_id`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// List returns sessions matching f ordered by recency, plus the total match count.
func (st *Store) List(ctx context.Context, f Filter) ([]*Session, int, error) {
	var where []string
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if f.Harness != "" {
		where = append(where, "s.harness = "+arg(string(f.Harness)))
	}
	if f.Origin != "" {
		where = append(where, "s.origin = "+arg(string(f.Origin)))
	} else if !f.IncludeSubagents {
		where = append(where, "s.origin <> 'subagent'")
	}
	if f.Repo != "" {
		where = append(where, "(s.repo = "+arg(f.Repo)+" OR s.repo LIKE "+arg("%/"+f.Repo)+")")
	}
	if f.Branch != "" {
		where = append(where, "s.branch = "+arg(f.Branch))
	}
	if f.Since != nil {
		where = append(where, "s.last_activity_at >= "+arg(*f.Since))
	}
	switch f.Status {
	case StatusActive:
		where = append(where, "s.ended_at IS NULL AND s.last_activity_at > now() - interval '5 minutes'")
	case StatusIdle:
		where = append(where, "s.ended_at IS NULL AND s.last_activity_at <= now() - interval '5 minutes' AND s.last_activity_at > now() - interval '1 hour'")
	case StatusEnded:
		where = append(where, "(s.ended_at IS NOT NULL OR s.last_activity_at <= now() - interval '1 hour')")
	}
	if f.Ref != "" {
		where = append(where, "EXISTS (SELECT 1 FROM session_links sl WHERE sl.session_id = s.id AND sl.ref = "+arg(f.Ref)+")")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := arg("%" + q + "%")
		ts := arg(q)
		where = append(where, "(s.title ILIKE "+like+" OR s.first_prompt ILIKE "+like+" OR s.repo ILIKE "+like+" OR s.branch ILIKE "+like+
			" OR EXISTS (SELECT 1 FROM session_prompts sp WHERE sp.session_id = s.id AND (to_tsvector('english', sp.text) @@ plainto_tsquery('english', "+ts+") OR sp.text ILIKE "+like+"))"+
			" OR EXISTS (SELECT 1 FROM session_links sl WHERE sl.session_id = s.id AND sl.ref ILIKE "+like+"))")
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM agent_sessions s`+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := st.pool.Query(ctx, `SELECT `+sessionCols+` FROM agent_sessions s`+clause+
		` ORDER BY s.last_activity_at DESC LIMIT `+arg(limit)+` OFFSET `+arg(f.Offset), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*Session
	var ids []string
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, s)
		ids = append(ids, s.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	links, err := st.linksFor(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	for _, s := range out {
		s.Links = links[s.ID]
	}
	return out, total, nil
}

func (st *Store) linksFor(ctx context.Context, ids []string) (map[string][]Link, error) {
	out := map[string][]Link{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := st.pool.Query(ctx, `SELECT session_id, kind, ref, source, created_at FROM session_links WHERE session_id = ANY($1) ORDER BY created_at`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.SessionID, &l.Kind, &l.Ref, &l.Source, &l.CreatedAt); err != nil {
			return nil, err
		}
		out[l.SessionID] = append(out[l.SessionID], l)
	}
	return out, rows.Err()
}

// ListPrompts returns a session's prompts in order.
func (st *Store) ListPrompts(ctx context.Context, sessionID string, limit int) ([]Prompt, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := st.pool.Query(ctx, `SELECT id, session_id, seq, role, text, ts FROM session_prompts WHERE session_id = $1 ORDER BY seq LIMIT $2`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Prompt
	for rows.Next() {
		var p Prompt
		if err := rows.Scan(&p.ID, &p.SessionID, &p.Seq, &p.Role, &p.Text, &p.TS); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListChildren returns subagent sessions spawned by parentID.
func (st *Store) ListChildren(ctx context.Context, parentID string) ([]*Session, error) {
	rows, err := st.pool.Query(ctx, `SELECT `+sessionCols+` FROM agent_sessions s WHERE s.parent_session_id = $1 ORDER BY s.started_at`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListByLink returns sessions linked to a given ref, most recent first.
func (st *Store) ListByLink(ctx context.Context, kind, ref string) ([]*Session, error) {
	rows, err := st.pool.Query(ctx, `SELECT `+sessionCols+` FROM agent_sessions s
		JOIN session_links l ON l.session_id = s.id WHERE l.kind = $1 AND l.ref = $2 ORDER BY s.last_activity_at DESC`, kind, ref)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Counts returns session counts by harness.
func (st *Store) Counts(ctx context.Context) (map[string]int, int, error) {
	rows, err := st.pool.Query(ctx, `SELECT harness, count(*) FROM agent_sessions GROUP BY harness`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := map[string]int{}
	total := 0
	for rows.Next() {
		var h string
		var n int
		if err := rows.Scan(&h, &n); err != nil {
			return nil, 0, err
		}
		out[h] = n
		total += n
	}
	return out, total, rows.Err()
}

// touch is a helper for callers that only need to bump activity (used by hooks later).
func (st *Store) touch(ctx context.Context, id string, at time.Time) error {
	_, err := st.pool.Exec(ctx, `UPDATE agent_sessions SET last_activity_at = GREATEST(last_activity_at, $2), updated_at = now() WHERE id = $1`, id, at)
	return err
}
