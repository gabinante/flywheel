package project

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists projects.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create inserts a project and initializes its ticket sequence.
func (s *Store) Create(ctx context.Context, p *Project) error {
	packJSON, err := json.Marshal(p.ContextPack)
	if err != nil {
		return err
	}
	dispatchConfigJSON, err := json.Marshal(p.DispatchConfig.Normalized())
	if err != nil {
		return err
	}
	status := p.Status
	if status == "" {
		status = "active"
	}
	defaultBranch := p.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO projects (id, org_id, name, slug, description, repo_url, default_branch, tech_stack, context_pack, status, dispatch_enabled, dispatch_config, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		p.ID, p.OrgID, p.Name, p.Slug, p.Description, p.RepoURL, defaultBranch, p.TechStack, packJSON, status, p.DispatchEnabled, dispatchConfigJSON, p.CreatedAt)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO ticket_sequences (project_id, next_val) VALUES ($1, 1)`, p.ID)
	return err
}

// GetByID returns a project by ID.
func (s *Store) GetByID(ctx context.Context, id string) (*Project, error) {
	var p Project
	var packJSON, dispatchConfigJSON []byte
	var techStack []string
	var repoURL sql.NullString
	var defaultBranch sql.NullString
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, name, slug, description, repo_url, default_branch, tech_stack, context_pack, status, dispatch_enabled, dispatch_config, created_at
		 FROM projects WHERE id = $1`, id).
		Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Description, &repoURL, &defaultBranch, &techStack, &packJSON, &p.Status, &p.DispatchEnabled, &dispatchConfigJSON, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	if repoURL.Valid {
		p.RepoURL = repoURL.String
	}
	if defaultBranch.Valid && defaultBranch.String != "" {
		p.DefaultBranch = defaultBranch.String
	} else {
		p.DefaultBranch = "main"
	}
	p.TechStack = techStack
	if len(packJSON) > 0 {
		_ = json.Unmarshal(packJSON, &p.ContextPack)
	}
	if len(dispatchConfigJSON) > 0 {
		_ = json.Unmarshal(dispatchConfigJSON, &p.DispatchConfig)
	}
	p.DispatchConfig = p.DispatchConfig.Normalized()
	return &p, nil
}

// GetBySlug returns a project by org ID and slug.
func (s *Store) GetBySlug(ctx context.Context, orgID, slug string) (*Project, error) {
	var p Project
	var packJSON, dispatchConfigJSON []byte
	var techStack []string
	var repoURL sql.NullString
	var defaultBranch sql.NullString
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, name, slug, description, repo_url, default_branch, tech_stack, context_pack, status, dispatch_enabled, dispatch_config, created_at
		 FROM projects WHERE org_id = $1 AND slug = $2`, orgID, slug).
		Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Description, &repoURL, &defaultBranch, &techStack, &packJSON, &p.Status, &p.DispatchEnabled, &dispatchConfigJSON, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	if repoURL.Valid {
		p.RepoURL = repoURL.String
	}
	if defaultBranch.Valid && defaultBranch.String != "" {
		p.DefaultBranch = defaultBranch.String
	} else {
		p.DefaultBranch = "main"
	}
	p.TechStack = techStack
	if len(packJSON) > 0 {
		_ = json.Unmarshal(packJSON, &p.ContextPack)
	}
	if len(dispatchConfigJSON) > 0 {
		_ = json.Unmarshal(dispatchConfigJSON, &p.DispatchConfig)
	}
	p.DispatchConfig = p.DispatchConfig.Normalized()
	return &p, nil
}

// ListByOrgID returns projects for an org. statusFilter: "" or "active" = active only, "closed" = closed only, "all" = no filter.
func (s *Store) ListByOrgID(ctx context.Context, orgID string, statusFilter string) ([]Project, error) {
	q := `SELECT id, org_id, name, slug, description, repo_url, default_branch, tech_stack, context_pack, status, dispatch_enabled, dispatch_config, created_at
		  FROM projects WHERE org_id = $1`
	args := []any{orgID}
	if statusFilter == "" || statusFilter == "active" {
		q += ` AND status = 'active'`
	} else if statusFilter == "closed" {
		q += ` AND status = 'closed'`
	}
	q += ` ORDER BY name`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Project
	for rows.Next() {
		var p Project
		var packJSON, dispatchConfigJSON []byte
		var techStack []string
		var repoURL, defaultBranch sql.NullString
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Description, &repoURL, &defaultBranch, &techStack, &packJSON, &p.Status, &p.DispatchEnabled, &dispatchConfigJSON, &p.CreatedAt); err != nil {
			return nil, err
		}
		if repoURL.Valid {
			p.RepoURL = repoURL.String
		}
		if defaultBranch.Valid && defaultBranch.String != "" {
			p.DefaultBranch = defaultBranch.String
		} else {
			p.DefaultBranch = "main"
		}
		p.TechStack = techStack
		if len(packJSON) > 0 {
			_ = json.Unmarshal(packJSON, &p.ContextPack)
		}
		if len(dispatchConfigJSON) > 0 {
			_ = json.Unmarshal(dispatchConfigJSON, &p.DispatchConfig)
		}
		p.DispatchConfig = p.DispatchConfig.Normalized()
		list = append(list, p)
	}
	return list, rows.Err()
}

// UpdateContextPack updates only the context_pack JSONB.
func (s *Store) UpdateContextPack(ctx context.Context, projectID string, pack ContextPack) error {
	packJSON, err := json.Marshal(pack)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE projects SET context_pack = $1 WHERE id = $2`, packJSON, projectID)
	return err
}

// UpdateStatus sets project status to "active" or "closed".
func (s *Store) UpdateStatus(ctx context.Context, projectID, status string) error {
	if status != "active" && status != "closed" {
		return ErrInvalidStatus
	}
	res, err := s.pool.Exec(ctx, `UPDATE projects SET status = $1 WHERE id = $2`, status, projectID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// UpdateRepoURL sets project repo_url (empty string to disable work streams + git integration).
func (s *Store) UpdateRepoURL(ctx context.Context, projectID, repoURL string) error {
	res, err := s.pool.Exec(ctx, `UPDATE projects SET repo_url = $1 WHERE id = $2`, nullIfEmpty(repoURL), projectID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// UpdateDescription sets project description.
func (s *Store) UpdateDescription(ctx context.Context, projectID, description string) error {
	res, err := s.pool.Exec(ctx, `UPDATE projects SET description = $1 WHERE id = $2`, description, projectID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// UpdateName sets project name.
func (s *Store) UpdateName(ctx context.Context, projectID, name string) error {
	res, err := s.pool.Exec(ctx, `UPDATE projects SET name = $1 WHERE id = $2`, name, projectID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// UpdateSlug sets project slug (must be unique per org).
func (s *Store) UpdateSlug(ctx context.Context, projectID, slug string) error {
	res, err := s.pool.Exec(ctx, `UPDATE projects SET slug = $1 WHERE id = $2`, slug, projectID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// UpdateDefaultBranch sets project default_branch (e.g. main, master).
func (s *Store) UpdateDefaultBranch(ctx context.Context, projectID, branch string) error {
	b := branch
	if b == "" {
		b = "main"
	}
	res, err := s.pool.Exec(ctx, `UPDATE projects SET default_branch = $1 WHERE id = $2`, b, projectID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// UpdateDispatchEnabled sets whether the dispatcher picks up tickets for this project.
func (s *Store) UpdateDispatchEnabled(ctx context.Context, projectID string, enabled bool) error {
	res, err := s.pool.Exec(ctx, `UPDATE projects SET dispatch_enabled = $1 WHERE id = $2`, enabled, projectID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrProjectNotFound
	}
	return nil
}

func (s *Store) UpdateDispatchConfig(ctx context.Context, projectID string, cfg DispatchConfig) error {
	dispatchConfigJSON, err := json.Marshal(cfg.Normalized())
	if err != nil {
		return err
	}
	res, err := s.pool.Exec(ctx, `UPDATE projects SET dispatch_config = $1 WHERE id = $2`, dispatchConfigJSON, projectID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrProjectNotFound
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ErrInvalidStatus is returned when status is not "active" or "closed".
var ErrInvalidStatus = errors.New("invalid status: must be active or closed")

// ErrProjectNotFound is returned when UpdateStatus affects no rows.
var ErrProjectNotFound = errors.New("project not found")
