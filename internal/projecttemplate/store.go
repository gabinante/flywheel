package projecttemplate

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) ListWorkstreamTemplates(ctx context.Context, orgID string) ([]WorkstreamTemplate, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, name, slug, description, plan, tickets, created_at, updated_at
		 FROM workstream_templates
		 WHERE org_id IS NULL OR org_id = $1
		 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []WorkstreamTemplate
	for rows.Next() {
		wt, err := scanWorkstreamTemplate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *wt)
	}
	return result, rows.Err()
}

func (s *Store) GetWorkstreamTemplate(ctx context.Context, id string) (*WorkstreamTemplate, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, org_id, name, slug, description, plan, tickets, created_at, updated_at
		 FROM workstream_templates WHERE id = $1`, id)
	return scanWorkstreamTemplateRow(row)
}

func (s *Store) GetWorkstreamTemplatesByIDs(ctx context.Context, ids []string) ([]WorkstreamTemplate, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, name, slug, description, plan, tickets, created_at, updated_at
		 FROM workstream_templates WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []WorkstreamTemplate
	for rows.Next() {
		wt, err := scanWorkstreamTemplate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *wt)
	}
	return result, rows.Err()
}

func (s *Store) ListProjectTemplates(ctx context.Context, orgID string) ([]ProjectTemplate, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, name, slug, description, workstream_template_ids, created_at, updated_at
		 FROM project_templates
		 WHERE org_id IS NULL OR org_id = $1
		 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ProjectTemplate
	for rows.Next() {
		pt, err := scanProjectTemplate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *pt)
	}
	return result, rows.Err()
}

func (s *Store) GetProjectTemplate(ctx context.Context, id string) (*ProjectTemplate, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, org_id, name, slug, description, workstream_template_ids, created_at, updated_at
		 FROM project_templates WHERE id = $1`, id)
	return scanProjectTemplateRow(row)
}

// scanner interface shared by pgx rows and row
type scanner interface {
	Scan(dest ...any) error
}

func scanWorkstreamTemplate(s scanner) (*WorkstreamTemplate, error) {
	var wt WorkstreamTemplate
	var orgID sql.NullString
	var ticketsJSON []byte
	if err := s.Scan(&wt.ID, &orgID, &wt.Name, &wt.Slug, &wt.Description, &wt.Plan, &ticketsJSON, &wt.CreatedAt, &wt.UpdatedAt); err != nil {
		return nil, err
	}
	wt.OrgID = orgID.String
	if err := json.Unmarshal(ticketsJSON, &wt.Tickets); err != nil {
		return nil, err
	}
	return &wt, nil
}

func scanWorkstreamTemplateRow(row interface{ Scan(dest ...any) error }) (*WorkstreamTemplate, error) {
	return scanWorkstreamTemplate(row)
}

func scanProjectTemplate(s scanner) (*ProjectTemplate, error) {
	var pt ProjectTemplate
	var orgID sql.NullString
	if err := s.Scan(&pt.ID, &orgID, &pt.Name, &pt.Slug, &pt.Description, &pt.WorkstreamTemplateIDs, &pt.CreatedAt, &pt.UpdatedAt); err != nil {
		return nil, err
	}
	pt.OrgID = orgID.String
	if pt.WorkstreamTemplateIDs == nil {
		pt.WorkstreamTemplateIDs = []string{}
	}
	return &pt, nil
}

func scanProjectTemplateRow(row interface{ Scan(dest ...any) error }) (*ProjectTemplate, error) {
	return scanProjectTemplate(row)
}
