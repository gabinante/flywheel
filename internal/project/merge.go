package project

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Merge errors.
var (
	ErrMergeSameProject  = errors.New("cannot merge a project into itself")
	ErrMergeDifferentOrg = errors.New("projects belong to different organizations")
	ErrMergeBothLinked   = errors.New("both projects are linked to different Linear projects; unlink one first (a Flywheel project maps to a single Linear project)")
)

// MergeResult summarizes what a merge moved.
type MergeResult struct {
	SourceID         string           `json:"source_id"`
	SourceName       string           `json:"source_name"`
	TargetID         string           `json:"target_id"`
	TicketsMoved     int64            `json:"tickets_moved"`
	WorkStreamsMoved int64            `json:"work_streams_moved"`
	ReposMoved       int64            `json:"repos_moved"`
	LinearLinkMoved  bool             `json:"linear_link_moved"`
	TablesTouched    map[string]int64 `json:"tables_touched"`
}

// specially handled tables (unique constraints or semantics beyond a plain repoint).
var mergeSpecialTables = map[string]bool{
	"tickets": true, "work_streams": true, "project_repositories": true,
	"project_linear_links": true, "ticket_sequences": true,
}

// Merge moves everything owned by source into target and deletes source, in one
// transaction. Target keeps its own identity and settings; empty target fields
// (repo URL, description, tech stack, context pack) are filled from source.
func (s *Store) Merge(ctx context.Context, sourceID, targetID string) (*MergeResult, error) {
	if sourceID == targetID {
		return nil, ErrMergeSameProject
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var srcOrg, tgtOrg, srcName string
	if err := tx.QueryRow(ctx, `SELECT org_id, name FROM projects WHERE id = $1 FOR UPDATE`, sourceID).Scan(&srcOrg, &srcName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	if err := tx.QueryRow(ctx, `SELECT org_id FROM projects WHERE id = $1 FOR UPDATE`, targetID).Scan(&tgtOrg); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	if srcOrg != tgtOrg {
		return nil, ErrMergeDifferentOrg
	}
	res := &MergeResult{SourceID: sourceID, SourceName: srcName, TargetID: targetID, TablesTouched: map[string]int64{}}

	// Linear link: a Flywheel project maps to one Linear project.
	var srcLinear, tgtLinear string
	_ = tx.QueryRow(ctx, `SELECT linear_project_id FROM project_linear_links WHERE project_id = $1`, sourceID).Scan(&srcLinear)
	_ = tx.QueryRow(ctx, `SELECT linear_project_id FROM project_linear_links WHERE project_id = $1`, targetID).Scan(&tgtLinear)
	switch {
	case srcLinear != "" && tgtLinear != "" && srcLinear != tgtLinear:
		return nil, ErrMergeBothLinked
	case srcLinear != "" && tgtLinear == "":
		if _, err := tx.Exec(ctx, `UPDATE project_linear_links SET project_id = $1, updated_at = now() WHERE project_id = $2`, targetID, sourceID); err != nil {
			return nil, fmt.Errorf("move linear link: %w", err)
		}
		res.LinearLinkMoved = true
	case srcLinear != "":
		if _, err := tx.Exec(ctx, `DELETE FROM project_linear_links WHERE project_id = $1`, sourceID); err != nil {
			return nil, err
		}
	}

	// Tickets: no per-project uniqueness, plain repoint.
	ct, err := tx.Exec(ctx, `UPDATE tickets SET project_id = $1 WHERE project_id = $2`, targetID, sourceID)
	if err != nil {
		return nil, fmt.Errorf("move tickets: %w", err)
	}
	res.TicketsMoved = ct.RowsAffected()

	// Ticket numbering: keep the higher counter so identifiers never repeat.
	if _, err := tx.Exec(ctx, `INSERT INTO ticket_sequences (project_id, next_val)
		SELECT $1, next_val FROM ticket_sequences WHERE project_id = $2
		ON CONFLICT (project_id) DO UPDATE SET next_val = GREATEST(ticket_sequences.next_val, EXCLUDED.next_val)`, targetID, sourceID); err != nil {
		return nil, fmt.Errorf("merge ticket sequence: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ticket_sequences WHERE project_id = $1`, sourceID); err != nil {
		return nil, err
	}

	// Work streams: unique (project_id, slug); suffix colliding slugs.
	ct, err = tx.Exec(ctx, `UPDATE work_streams w SET project_id = $1,
		slug = CASE WHEN EXISTS (SELECT 1 FROM work_streams x WHERE x.project_id = $1 AND x.slug = w.slug)
			THEN w.slug || '-' || left(w.id, 6) ELSE w.slug END
		WHERE w.project_id = $2`, targetID, sourceID)
	if err != nil {
		return nil, fmt.Errorf("move work streams: %w", err)
	}
	res.WorkStreamsMoved = ct.RowsAffected()

	// Repositories: drop duplicates of repos the target already has, then move the rest,
	// renaming colliding aliases and demoting a second primary.
	if _, err := tx.Exec(ctx, `DELETE FROM project_repositories s USING project_repositories t
		WHERE s.project_id = $2 AND t.project_id = $1 AND s.repo_url = t.repo_url`, targetID, sourceID); err != nil {
		return nil, err
	}
	ct, err = tx.Exec(ctx, `UPDATE project_repositories r SET project_id = $1,
		alias = CASE WHEN EXISTS (SELECT 1 FROM project_repositories x WHERE x.project_id = $1 AND x.alias = r.alias)
			THEN r.alias || '-' || left(r.id, 6) ELSE r.alias END,
		is_primary = CASE WHEN EXISTS (SELECT 1 FROM project_repositories x WHERE x.project_id = $1 AND x.is_primary)
			THEN false ELSE r.is_primary END
		WHERE r.project_id = $2`, targetID, sourceID)
	if err != nil {
		return nil, fmt.Errorf("move repositories: %w", err)
	}
	res.ReposMoved = ct.RowsAffected()

	// Everything else that hangs off a project (reports, orchestrator threads, legacy tables).
	rows, err := tx.Query(ctx, `SELECT DISTINCT table_name FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name = 'project_id' ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return nil, err
		}
		if !mergeSpecialTables[t] {
			tables = append(tables, t)
		}
	}
	rows.Close()
	for _, t := range tables {
		ct, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET project_id = $1 WHERE project_id = $2`, pgx.Identifier{t}.Sanitize()), targetID, sourceID)
		if err != nil {
			return nil, fmt.Errorf("move %s: %w", t, err)
		}
		if n := ct.RowsAffected(); n > 0 {
			res.TablesTouched[t] = n
		}
	}

	// Fill empty target fields from the source, then remove the source.
	if _, err := tx.Exec(ctx, `UPDATE projects t SET
		repo_url = COALESCE(NULLIF(t.repo_url, ''), s.repo_url),
		description = CASE WHEN t.description = '' THEN s.description ELSE t.description END,
		tech_stack = CASE WHEN COALESCE(array_length(t.tech_stack, 1), 0) = 0 THEN s.tech_stack ELSE t.tech_stack END,
		context_pack = CASE WHEN t.context_pack = '{}'::jsonb THEN s.context_pack ELSE t.context_pack END
		FROM projects s WHERE t.id = $1 AND s.id = $2`, targetID, sourceID); err != nil {
		return nil, fmt.Errorf("fill target fields: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM projects WHERE id = $1`, sourceID); err != nil {
		return nil, fmt.Errorf("delete source project: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return res, nil
}

// Merger is implemented by stores that can merge projects.
type Merger interface {
	Merge(ctx context.Context, sourceID, targetID string) (*MergeResult, error)
}

// MergeProjects folds source into target (see Store.Merge). Both projects must exist.
func (s *Service) MergeProjects(ctx context.Context, sourceID, targetID string) (*MergeResult, error) {
	m, ok := s.store.(Merger)
	if !ok {
		return nil, errors.New("project store does not support merging")
	}
	if _, err := s.store.GetByID(ctx, sourceID); err != nil {
		return nil, err
	}
	if _, err := s.store.GetByID(ctx, targetID); err != nil {
		return nil, err
	}
	return m.Merge(ctx, sourceID, targetID)
}
