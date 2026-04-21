-- Code knowledge layer: structural graph for symbols and relationships (Layer 3).
-- Stores the output of AST analysis (Tree-sitter or Go AST) so agents can query
-- callers, callees, importers, blast radius, and symbol metadata.

CREATE TABLE code_symbols (
    id          TEXT PRIMARY KEY,           -- file:SymbolName (stable within a project)
    project_id  TEXT NOT NULL REFERENCES projects(id),
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL,              -- function, method, type, interface, variable, constant
    file        TEXT NOT NULL,
    line        INT  NOT NULL DEFAULT 0,
    language    TEXT NOT NULL,              -- go, python, typescript, rust
    package     TEXT NOT NULL DEFAULT '',
    visibility  TEXT NOT NULL DEFAULT 'public', -- public, private, internal
    signature   TEXT NOT NULL DEFAULT '',
    doc_comment TEXT NOT NULL DEFAULT '',
    commit_sha  TEXT NOT NULL DEFAULT '',   -- commit at which this symbol was indexed
    indexed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX code_symbols_project   ON code_symbols(project_id);
CREATE INDEX code_symbols_name      ON code_symbols(project_id, name);
CREATE INDEX code_symbols_file      ON code_symbols(project_id, file);
CREATE INDEX code_symbols_language  ON code_symbols(project_id, language);
CREATE INDEX code_symbols_kind      ON code_symbols(project_id, kind);

-- Call edges: caller -> callee relationships extracted from AST.
CREATE TABLE code_edges (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    caller_id   TEXT NOT NULL,              -- references code_symbols(id)
    callee_id   TEXT NOT NULL,              -- references code_symbols(id) or external qualified name
    file        TEXT NOT NULL,              -- file where the call occurs
    line        INT  NOT NULL DEFAULT 0,
    commit_sha  TEXT NOT NULL DEFAULT '',
    indexed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX code_edges_project     ON code_edges(project_id);
CREATE INDEX code_edges_caller      ON code_edges(project_id, caller_id);
CREATE INDEX code_edges_callee      ON code_edges(project_id, callee_id);

-- Import edges: file -> package relationships.
CREATE TABLE code_imports (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id),
    file        TEXT NOT NULL,
    package     TEXT NOT NULL,
    commit_sha  TEXT NOT NULL DEFAULT '',
    indexed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX code_imports_project   ON code_imports(project_id);
CREATE INDEX code_imports_package   ON code_imports(project_id, package);

-- Index status: tracks when each project was last indexed.
CREATE TABLE code_index_status (
    project_id     TEXT PRIMARY KEY REFERENCES projects(id),
    last_commit_sha TEXT NOT NULL DEFAULT '',
    last_indexed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    total_symbols   INT NOT NULL DEFAULT 0,
    total_files     INT NOT NULL DEFAULT 0,
    languages       TEXT[] NOT NULL DEFAULT '{}',
    stale           BOOLEAN NOT NULL DEFAULT true
);
