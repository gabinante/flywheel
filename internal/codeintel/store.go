// Package codeintel provides a Postgres-backed store for the code knowledge layer.
// It persists symbols, call edges, and import relationships extracted by the
// bundled Tree-sitter/Go-AST default or any CodeIntelligenceProvider implementation.
package codeintel

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Symbol represents a code symbol stored in Postgres.
type Symbol struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	File       string    `json:"file"`
	Line       int       `json:"line"`
	Language   string    `json:"language"`
	Package    string    `json:"package"`
	Visibility string    `json:"visibility"`
	Signature  string    `json:"signature"`
	DocComment string    `json:"doc_comment"`
	CommitSHA  string    `json:"commit_sha"`
	IndexedAt  time.Time `json:"indexed_at"`
}

// Edge represents a caller->callee relationship.
type Edge struct {
	ID        int64     `json:"id"`
	ProjectID string    `json:"project_id"`
	CallerID  string    `json:"caller_id"`
	CalleeID  string    `json:"callee_id"`
	File      string    `json:"file"`
	Line      int       `json:"line"`
	CommitSHA string    `json:"commit_sha"`
	IndexedAt time.Time `json:"indexed_at"`
}

// Import represents a file->package import relationship.
type Import struct {
	ID        int64     `json:"id"`
	ProjectID string    `json:"project_id"`
	File      string    `json:"file"`
	Package   string    `json:"package"`
	CommitSHA string    `json:"commit_sha"`
	IndexedAt time.Time `json:"indexed_at"`
}

// IndexStatus tracks when a project was last indexed.
type IndexStatus struct {
	ProjectID     string    `json:"project_id"`
	LastCommitSHA string    `json:"last_commit_sha"`
	LastIndexedAt time.Time `json:"last_indexed_at"`
	TotalSymbols  int       `json:"total_symbols"`
	TotalFiles    int       `json:"total_files"`
	Languages     []string  `json:"languages"`
	Stale         bool      `json:"stale"`
}

// Store provides Postgres persistence for the code knowledge layer.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store backed by the given Postgres connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// UpsertSymbols inserts or updates symbols for a project. Uses ON CONFLICT for idempotency.
func (s *Store) UpsertSymbols(ctx context.Context, projectID string, symbols []Symbol) error {
	if len(symbols) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, sym := range symbols {
		batch.Queue(`
			INSERT INTO code_symbols (id, project_id, name, kind, file, line, language, package, visibility, signature, doc_comment, commit_sha, indexed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now())
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				kind = EXCLUDED.kind,
				file = EXCLUDED.file,
				line = EXCLUDED.line,
				language = EXCLUDED.language,
				package = EXCLUDED.package,
				visibility = EXCLUDED.visibility,
				signature = EXCLUDED.signature,
				doc_comment = EXCLUDED.doc_comment,
				commit_sha = EXCLUDED.commit_sha,
				indexed_at = now()`,
			sym.ID, projectID, sym.Name, sym.Kind, sym.File, sym.Line,
			sym.Language, sym.Package, sym.Visibility, sym.Signature, sym.DocComment, sym.CommitSHA)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range symbols {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceEdges replaces all call edges for a project (delete + insert).
func (s *Store) ReplaceEdges(ctx context.Context, projectID string, edges []Edge) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM code_edges WHERE project_id = $1`, projectID); err != nil {
		return err
	}

	if len(edges) > 0 {
		batch := &pgx.Batch{}
		for _, e := range edges {
			batch.Queue(`
				INSERT INTO code_edges (project_id, caller_id, callee_id, file, line, commit_sha, indexed_at)
				VALUES ($1, $2, $3, $4, $5, $6, now())`,
				projectID, e.CallerID, e.CalleeID, e.File, e.Line, e.CommitSHA)
		}
		br := tx.SendBatch(ctx, batch)
		for range edges {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return err
			}
		}
		br.Close()
	}

	return tx.Commit(ctx)
}

// ReplaceImports replaces all import edges for a project (delete + insert).
func (s *Store) ReplaceImports(ctx context.Context, projectID string, imports []Import) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM code_imports WHERE project_id = $1`, projectID); err != nil {
		return err
	}

	if len(imports) > 0 {
		batch := &pgx.Batch{}
		for _, imp := range imports {
			batch.Queue(`
				INSERT INTO code_imports (project_id, file, package, commit_sha, indexed_at)
				VALUES ($1, $2, $3, $4, now())`,
				projectID, imp.File, imp.Package, imp.CommitSHA)
		}
		br := tx.SendBatch(ctx, batch)
		for range imports {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return err
			}
		}
		br.Close()
	}

	return tx.Commit(ctx)
}

// UpsertIndexStatus updates the index status for a project.
func (s *Store) UpsertIndexStatus(ctx context.Context, status IndexStatus) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO code_index_status (project_id, last_commit_sha, last_indexed_at, total_symbols, total_files, languages, stale)
		VALUES ($1, $2, now(), $3, $4, $5, $6)
		ON CONFLICT (project_id) DO UPDATE SET
			last_commit_sha = EXCLUDED.last_commit_sha,
			last_indexed_at = now(),
			total_symbols = EXCLUDED.total_symbols,
			total_files = EXCLUDED.total_files,
			languages = EXCLUDED.languages,
			stale = EXCLUDED.stale`,
		status.ProjectID, status.LastCommitSHA, status.TotalSymbols, status.TotalFiles, status.Languages, status.Stale)
	return err
}

// GetIndexStatus returns the index status for a project.
func (s *Store) GetIndexStatus(ctx context.Context, projectID string) (*IndexStatus, error) {
	var st IndexStatus
	err := s.pool.QueryRow(ctx, `
		SELECT project_id, last_commit_sha, last_indexed_at, total_symbols, total_files, languages, stale
		FROM code_index_status WHERE project_id = $1`, projectID).
		Scan(&st.ProjectID, &st.LastCommitSHA, &st.LastIndexedAt, &st.TotalSymbols, &st.TotalFiles, &st.Languages, &st.Stale)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// SearchSymbols searches for symbols matching a query (case-insensitive substring match).
func (s *Store) SearchSymbols(ctx context.Context, projectID, query, language, kind string, limit int) ([]Symbol, error) {
	if limit <= 0 {
		limit = 20
	}

	q := `SELECT id, project_id, name, kind, file, line, language, package, visibility, signature, doc_comment, commit_sha, indexed_at
		FROM code_symbols
		WHERE project_id = $1 AND (LOWER(name) LIKE '%' || LOWER($2) || '%' OR LOWER(file || ':' || name) LIKE '%' || LOWER($2) || '%')`
	args := []any{projectID, query}
	argN := 3

	if language != "" {
		q += " AND language = $" + itoa(argN)
		args = append(args, language)
		argN++
	}
	if kind != "" {
		q += " AND kind = $" + itoa(argN)
		args = append(args, kind)
		argN++
	}

	q += " ORDER BY name LIMIT $" + itoa(argN)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var symbols []Symbol
	for rows.Next() {
		var sym Symbol
		if err := rows.Scan(&sym.ID, &sym.ProjectID, &sym.Name, &sym.Kind, &sym.File, &sym.Line,
			&sym.Language, &sym.Package, &sym.Visibility, &sym.Signature, &sym.DocComment, &sym.CommitSHA, &sym.IndexedAt); err != nil {
			return nil, err
		}
		symbols = append(symbols, sym)
	}
	return symbols, rows.Err()
}

// FindCallers returns edges where callee_id matches the given symbol.
func (s *Store) FindCallers(ctx context.Context, projectID, symbolID string, limit int) ([]Edge, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, caller_id, callee_id, file, line, commit_sha, indexed_at
		FROM code_edges WHERE project_id = $1 AND callee_id = $2 LIMIT $3`,
		projectID, symbolID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// FindCallees returns edges where caller_id matches the given symbol.
func (s *Store) FindCallees(ctx context.Context, projectID, symbolID string, limit int) ([]Edge, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, caller_id, callee_id, file, line, commit_sha, indexed_at
		FROM code_edges WHERE project_id = $1 AND caller_id = $2 LIMIT $3`,
		projectID, symbolID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// FindImporters returns files importing the given package.
func (s *Store) FindImporters(ctx context.Context, projectID, pkg string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT file FROM code_imports
		WHERE project_id = $1 AND package LIKE '%' || $2 || '%' LIMIT $3`,
		projectID, pkg, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// DeleteProjectData removes all code intelligence data for a project.
func (s *Store) DeleteProjectData(ctx context.Context, projectID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tx.Exec(ctx, `DELETE FROM code_edges WHERE project_id = $1`, projectID)
	tx.Exec(ctx, `DELETE FROM code_imports WHERE project_id = $1`, projectID)
	tx.Exec(ctx, `DELETE FROM code_symbols WHERE project_id = $1`, projectID)
	tx.Exec(ctx, `DELETE FROM code_index_status WHERE project_id = $1`, projectID)
	return tx.Commit(ctx)
}

// --- helpers ---

func scanEdges(rows pgx.Rows) ([]Edge, error) {
	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.CallerID, &e.CalleeID, &e.File, &e.Line, &e.CommitSHA, &e.IndexedAt); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

func itoa(n int) string {
	digits := "0123456789"
	if n < 10 {
		return string(digits[n])
	}
	return itoa(n/10) + string(digits[n%10])
}
