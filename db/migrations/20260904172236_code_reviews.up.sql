-- PR-keyed code review: review requests, their findings, and reviews that landed on the operator's own PRs.
CREATE TABLE IF NOT EXISTS code_review_requests (
    id                     TEXT PRIMARY KEY,
    repo                   TEXT NOT NULL,                 -- owner/name
    number                 INTEGER NOT NULL,
    url                    TEXT NOT NULL DEFAULT '',
    title                  TEXT NOT NULL DEFAULT '',
    author                 TEXT NOT NULL DEFAULT '',
    base_ref               TEXT NOT NULL DEFAULT '',
    head_ref               TEXT NOT NULL DEFAULT '',
    head_sha               TEXT NOT NULL DEFAULT '',
    origin                 TEXT NOT NULL DEFAULT 'paste',  -- paste | review_requested | re_review | mcp
    recipe                 TEXT NOT NULL DEFAULT 'inline_conversational_p1_gate',
    harness                TEXT NOT NULL DEFAULT 'codex',
    model                  TEXT NOT NULL DEFAULT '',
    reasoning_effort       TEXT NOT NULL DEFAULT '',
    state                  TEXT NOT NULL DEFAULT 'queued',
    attempt                INTEGER NOT NULL DEFAULT 1,
    watch                  BOOLEAN NOT NULL DEFAULT TRUE,
    dry_run                BOOLEAN NOT NULL DEFAULT FALSE, -- review but do not post to GitHub
    verdict                TEXT NOT NULL DEFAULT '',       -- approve | request_changes | comment
    summary                TEXT NOT NULL DEFAULT '',
    review_url             TEXT NOT NULL DEFAULT '',
    my_review_state        TEXT NOT NULL DEFAULT '',       -- GitHub: APPROVED | CHANGES_REQUESTED | COMMENTED | DISMISSED
    my_review_id           BIGINT NOT NULL DEFAULT 0,
    last_reviewed_head_sha TEXT NOT NULL DEFAULT '',
    session_id             TEXT NOT NULL DEFAULT '',       -- agent_sessions.id of the reviewing run
    session_external_id    TEXT NOT NULL DEFAULT '',
    worktree_path          TEXT NOT NULL DEFAULT '',
    ticket_id              TEXT NOT NULL DEFAULT '',
    error                  TEXT NOT NULL DEFAULT '',
    last_checked_at        TIMESTAMPTZ,
    reviewed_at            TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repo, number)
);

CREATE INDEX IF NOT EXISTS idx_code_review_requests_state ON code_review_requests (state, updated_at DESC);

CREATE TABLE IF NOT EXISTS code_review_findings (
    id                TEXT PRIMARY KEY,
    request_id        TEXT NOT NULL REFERENCES code_review_requests(id) ON DELETE CASCADE,
    attempt           INTEGER NOT NULL DEFAULT 1,
    severity          TEXT NOT NULL,                      -- P0 | P1 | P2 | P3
    path              TEXT NOT NULL DEFAULT '',
    line              INTEGER NOT NULL DEFAULT 0,
    side              TEXT NOT NULL DEFAULT 'RIGHT',
    title             TEXT NOT NULL DEFAULT '',
    body              TEXT NOT NULL,
    github_comment_id BIGINT NOT NULL DEFAULT 0,
    status            TEXT NOT NULL DEFAULT 'pending',    -- pending | posted | in_body | withheld | resolved | outdated
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_code_review_findings_request ON code_review_findings (request_id, attempt);

-- Reviews other people (or bots) submitted on PRs the operator authored; each row can trigger an address-feedback run.
CREATE TABLE IF NOT EXISTS pr_feedback_rounds (
    id            TEXT PRIMARY KEY,
    repo          TEXT NOT NULL,
    number        INTEGER NOT NULL,
    url           TEXT NOT NULL DEFAULT '',
    title         TEXT NOT NULL DEFAULT '',
    head_sha      TEXT NOT NULL DEFAULT '',
    reviewer      TEXT NOT NULL DEFAULT '',
    review_state  TEXT NOT NULL DEFAULT '',                -- APPROVED | CHANGES_REQUESTED | COMMENTED | DISMISSED
    review_id     BIGINT NOT NULL,
    comment_count INTEGER NOT NULL DEFAULT 0,
    body          TEXT NOT NULL DEFAULT '',
    state         TEXT NOT NULL DEFAULT 'new',             -- new | dispatched | addressed | ignored
    ticket_id     TEXT NOT NULL DEFAULT '',
    session_id    TEXT NOT NULL DEFAULT '',
    observed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    submitted_at  TIMESTAMPTZ,
    UNIQUE (repo, number, review_id)
);

CREATE INDEX IF NOT EXISTS idx_pr_feedback_rounds_state ON pr_feedback_rounds (state, observed_at DESC);
