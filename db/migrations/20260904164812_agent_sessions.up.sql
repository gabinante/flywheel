-- Agent sessions: every Claude Code / Codex session (interactive, dispatched, automation, subagent)
-- ingested from the harnesses' local stores and linked to PRs and Linear issues.
CREATE TABLE IF NOT EXISTS agent_sessions (
    id                 TEXT PRIMARY KEY,
    harness            TEXT NOT NULL CHECK (harness IN ('claude_code', 'codex')),
    external_id        TEXT NOT NULL,                    -- Claude sessionId (or sessionId/agentId for subagents), Codex thread id
    origin             TEXT NOT NULL DEFAULT 'interactive' CHECK (origin IN ('interactive', 'dispatched', 'automation', 'subagent')),
    parent_session_id  TEXT REFERENCES agent_sessions(id) ON DELETE SET NULL,
    parent_external_id TEXT NOT NULL DEFAULT '',         -- resolved into parent_session_id once the parent is ingested
    cwd                TEXT NOT NULL DEFAULT '',
    repo               TEXT NOT NULL DEFAULT '',         -- owner/name when known, else bare directory name
    branch             TEXT NOT NULL DEFAULT '',
    model              TEXT NOT NULL DEFAULT '',
    reasoning_effort   TEXT NOT NULL DEFAULT '',
    title              TEXT NOT NULL DEFAULT '',
    first_prompt       TEXT NOT NULL DEFAULT '',
    transcript_path    TEXT NOT NULL DEFAULT '',
    tokens_in          BIGINT NOT NULL DEFAULT 0,
    tokens_out         BIGINT NOT NULL DEFAULT 0,
    prompt_count       INTEGER NOT NULL DEFAULT 0,
    tool_call_count    INTEGER NOT NULL DEFAULT 0,
    started_at         TIMESTAMPTZ NOT NULL,
    last_activity_at   TIMESTAMPTZ NOT NULL,
    ended_at           TIMESTAMPTZ,
    ingest_offset      BIGINT NOT NULL DEFAULT 0,        -- bytes of the transcript consumed so far
    metadata           JSONB NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (harness, external_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_sessions_last_activity ON agent_sessions (last_activity_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_repo_branch  ON agent_sessions (repo, branch);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_parent       ON agent_sessions (parent_session_id);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_transcript   ON agent_sessions (transcript_path);

-- What a session touched: pull requests, Linear issues, Flywheel tickets, review requests.
CREATE TABLE IF NOT EXISTS session_links (
    session_id TEXT NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('pr', 'linear_issue', 'ticket', 'review')),
    ref        TEXT NOT NULL,                            -- owner/repo#123 | RLETD-465 | ticket id | review request id
    source     TEXT NOT NULL DEFAULT 'inferred' CHECK (source IN ('inferred', 'explicit', 'dispatch')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (session_id, kind, ref)
);

CREATE INDEX IF NOT EXISTS idx_session_links_ref ON session_links (kind, ref);

-- Operator prompts (and select agent messages) for search and timeline display.
CREATE TABLE IF NOT EXISTS session_prompts (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
    seq        INTEGER NOT NULL,
    role       TEXT NOT NULL,                            -- user | assistant
    text       TEXT NOT NULL,
    ts         TIMESTAMPTZ NOT NULL,
    UNIQUE (session_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_session_prompts_fts ON session_prompts USING GIN (to_tsvector('english', text));
