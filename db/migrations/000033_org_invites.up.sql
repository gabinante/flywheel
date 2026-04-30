CREATE TABLE org_invites (
    id          TEXT PRIMARY KEY,
    org_id      TEXT NOT NULL REFERENCES orgs(id),
    code        TEXT NOT NULL UNIQUE,
    role        TEXT NOT NULL DEFAULT 'member',
    created_by  TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    max_uses    INTEGER NOT NULL DEFAULT 1,
    use_count   INTEGER NOT NULL DEFAULT 0,
    revoked     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_org_invites_code ON org_invites(code);
CREATE INDEX idx_org_invites_org ON org_invites(org_id);
