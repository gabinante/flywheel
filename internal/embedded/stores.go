package embedded

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/environment"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/user"
	"github.com/gabinante/flywheel/internal/workstream"
)

func mustUUID() string { return uuid.Must(uuid.NewV7()).String() }

// ────────────────────────────────────────────────────────────────────────────
// OrgStore (implements org.OrgStore)
// ────────────────────────────────────────────────────────────────────────────

type OrgStore struct{ db *sql.DB }

func NewOrgStore(db *sql.DB) *OrgStore { return &OrgStore{db: db} }

func (s *OrgStore) Create(_ context.Context, o *org.Org) error {
	_, err := s.db.Exec(`INSERT INTO orgs (id, name, slug, created_at) VALUES (?, ?, ?, ?)`,
		o.ID, o.Name, o.Slug, o.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *OrgStore) GetByID(_ context.Context, id string) (*org.Org, error) {
	var o org.Org
	var ts string
	err := s.db.QueryRow(`SELECT id, name, slug, created_at FROM orgs WHERE id = ?`, id).
		Scan(&o.ID, &o.Name, &o.Slug, &ts)
	if err != nil {
		return nil, err
	}
	o.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	return &o, nil
}

func (s *OrgStore) GetBySlug(_ context.Context, slug string) (*org.Org, error) {
	var o org.Org
	var ts string
	err := s.db.QueryRow(`SELECT id, name, slug, created_at FROM orgs WHERE slug = ?`, slug).
		Scan(&o.ID, &o.Name, &o.Slug, &ts)
	if err != nil {
		return nil, err
	}
	o.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	return &o, nil
}

func (s *OrgStore) AddMember(_ context.Context, m *org.Member) error {
	_, err := s.db.Exec(`INSERT INTO org_members (org_id, user_id, role) VALUES (?, ?, ?)
		ON CONFLICT (org_id, user_id) DO UPDATE SET role = excluded.role`,
		m.OrgID, m.UserID, string(m.Role))
	return err
}

func (s *OrgStore) ListMembers(_ context.Context, orgID string) ([]org.Member, error) {
	rows, err := s.db.Query(`SELECT org_id, user_id, role FROM org_members WHERE org_id = ?`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []org.Member
	for rows.Next() {
		var m org.Member
		if err := rows.Scan(&m.OrgID, &m.UserID, &m.Role); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (s *OrgStore) ListOrgIDsByUserID(_ context.Context, userID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT org_id FROM org_members WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *OrgStore) ListOrgsByUserID(_ context.Context, userID string) ([]*org.Org, error) {
	rows, err := s.db.Query(
		`SELECT o.id, o.name, o.slug, o.created_at
		 FROM orgs o INNER JOIN org_members m ON m.org_id = o.id
		 WHERE m.user_id = ? ORDER BY o.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*org.Org
	for rows.Next() {
		var o org.Org
		var ts string
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &ts); err != nil {
			return nil, err
		}
		o.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		list = append(list, &o)
	}
	return list, rows.Err()
}

// ────────────────────────────────────────────────────────────────────────────
// AgentStore (implements agent.AgentStore)
// ────────────────────────────────────────────────────────────────────────────

type AgentStore struct{ db *sql.DB }

func NewAgentStore(db *sql.DB) *AgentStore { return &AgentStore{db: db} }

func (s *AgentStore) Create(_ context.Context, a *agent.Agent) error {
	_, err := s.db.Exec(
		`INSERT INTO agents (id, user_id, name, type, api_key, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, nilIfEmpty(a.UserID), a.Name, string(a.Type), nilIfEmpty(a.APIKey),
		a.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *AgentStore) GetByID(_ context.Context, id string) (*agent.Agent, error) {
	var a agent.Agent
	var apiKey, userID sql.NullString
	var ts string
	err := s.db.QueryRow(`SELECT id, user_id, name, type, api_key, created_at FROM agents WHERE id = ?`, id).
		Scan(&a.ID, &userID, &a.Name, &a.Type, &apiKey, &ts)
	if err != nil {
		return nil, err
	}
	if apiKey.Valid {
		a.APIKey = apiKey.String
	}
	if userID.Valid {
		a.UserID = userID.String
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	return &a, nil
}

func (s *AgentStore) GetByAPIKey(_ context.Context, apiKey string) (*agent.Agent, error) {
	if apiKey == "" {
		return nil, nil
	}
	var a agent.Agent
	var ak, uid sql.NullString
	var ts string
	err := s.db.QueryRow(`SELECT id, user_id, name, type, api_key, created_at FROM agents WHERE api_key = ?`, apiKey).
		Scan(&a.ID, &uid, &a.Name, &a.Type, &ak, &ts)
	if err != nil {
		return nil, err
	}
	if ak.Valid {
		a.APIKey = ak.String
	}
	if uid.Valid {
		a.UserID = uid.String
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	return &a, nil
}

func (s *AgentStore) GetByUserID(_ context.Context, userID string) (*agent.Agent, error) {
	var a agent.Agent
	var ak, uid sql.NullString
	var ts string
	err := s.db.QueryRow(`SELECT id, user_id, name, type, api_key, created_at FROM agents WHERE user_id = ?`, userID).
		Scan(&a.ID, &uid, &a.Name, &a.Type, &ak, &ts)
	if err != nil {
		return nil, err
	}
	if ak.Valid {
		a.APIKey = ak.String
	}
	if uid.Valid {
		a.UserID = uid.String
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	return &a, nil
}

// ────────────────────────────────────────────────────────────────────────────
// UserStore (used by auth.Provisioner; implements user store methods)
// ────────────────────────────────────────────────────────────────────────────

type UserStore struct{ db *sql.DB }

func NewUserStore(db *sql.DB) *UserStore { return &UserStore{db: db} }

func (s *UserStore) Create(_ context.Context, u *user.User) error {
	if u.ID == "" {
		u.ID = mustUUID()
	}
	_, err := s.db.Exec(
		`INSERT INTO users (id, github_id, login, name, email, avatar_url, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.GitHubID, u.Login, u.Name, u.Email, u.AvatarURL,
		u.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *UserStore) GetByGitHubID(_ context.Context, githubID int64) (*user.User, error) {
	var u user.User
	var ts string
	err := s.db.QueryRow(
		`SELECT id, github_id, login, name, email, avatar_url, created_at FROM users WHERE github_id = ?`, githubID).
		Scan(&u.ID, &u.GitHubID, &u.Login, &u.Name, &u.Email, &u.AvatarURL, &ts)
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	return &u, nil
}

func (s *UserStore) GetByID(_ context.Context, id string) (*user.User, error) {
	var u user.User
	var ts string
	err := s.db.QueryRow(
		`SELECT id, github_id, login, name, email, avatar_url, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.GitHubID, &u.Login, &u.Name, &u.Email, &u.AvatarURL, &ts)
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	return &u, nil
}

// ────────────────────────────────────────────────────────────────────────────
// ProjectStore (implements project.ProjectStore — already an interface)
// ────────────────────────────────────────────────────────────────────────────

type ProjectStore struct{ db *sql.DB }

func NewProjectStore(db *sql.DB) *ProjectStore { return &ProjectStore{db: db} }

func (s *ProjectStore) Create(_ context.Context, p *project.Project) error {
	techJSON, _ := json.Marshal(p.TechStack)
	packJSON, _ := json.Marshal(p.ContextPack)
	dispatchConfigJSON, _ := json.Marshal(p.DispatchConfig.Normalized())
	status := p.Status
	if status == "" {
		status = "active"
	}
	branch := p.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	dispatchEnabled := 1
	if !p.DispatchEnabled {
		dispatchEnabled = 0
	}
	_, err := s.db.Exec(
		`INSERT INTO projects (id, org_id, name, slug, repo_url, default_branch, tech_stack, context_pack, status, dispatch_enabled, dispatch_config, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.OrgID, p.Name, p.Slug, nilIfEmpty(p.RepoURL), branch, string(techJSON), string(packJSON), status, dispatchEnabled, string(dispatchConfigJSON),
		p.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO ticket_sequences (project_id, next_val) VALUES (?, 1)`, p.ID)
	return err
}

func (s *ProjectStore) GetByID(_ context.Context, id string) (*project.Project, error) {
	var p project.Project
	var techJSON, packJSON, dispatchConfigJSON string
	var repoURL, defaultBranch sql.NullString
	var dispatchEnabled int
	var ts string
	err := s.db.QueryRow(
		`SELECT id, org_id, name, slug, repo_url, default_branch, tech_stack, context_pack, status, dispatch_enabled, dispatch_config, created_at
		 FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &repoURL, &defaultBranch, &techJSON, &packJSON, &p.Status, &dispatchEnabled, &dispatchConfigJSON, &ts)
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
	p.DispatchEnabled = dispatchEnabled != 0
	_ = json.Unmarshal([]byte(techJSON), &p.TechStack)
	_ = json.Unmarshal([]byte(packJSON), &p.ContextPack)
	_ = json.Unmarshal([]byte(dispatchConfigJSON), &p.DispatchConfig)
	p.DispatchConfig = p.DispatchConfig.Normalized()
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	return &p, nil
}

func (s *ProjectStore) ListByOrgID(_ context.Context, orgID string, statusFilter string) ([]project.Project, error) {
	q := `SELECT id, org_id, name, slug, repo_url, default_branch, tech_stack, context_pack, status, dispatch_enabled, dispatch_config, created_at
		  FROM projects WHERE org_id = ?`
	if statusFilter == "" || statusFilter == "active" {
		q += ` AND status = 'active'`
	} else if statusFilter == "closed" {
		q += ` AND status = 'closed'`
	}
	q += ` ORDER BY name`
	rows, err := s.db.Query(q, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []project.Project
	for rows.Next() {
		var p project.Project
		var techJSON, packJSON, dispatchConfigJSON string
		var repoURL, defaultBranch sql.NullString
		var dispatchEnabled int
		var ts string
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &repoURL, &defaultBranch, &techJSON, &packJSON, &p.Status, &dispatchEnabled, &dispatchConfigJSON, &ts); err != nil {
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
		p.DispatchEnabled = dispatchEnabled != 0
		_ = json.Unmarshal([]byte(techJSON), &p.TechStack)
		_ = json.Unmarshal([]byte(packJSON), &p.ContextPack)
		_ = json.Unmarshal([]byte(dispatchConfigJSON), &p.DispatchConfig)
		p.DispatchConfig = p.DispatchConfig.Normalized()
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *ProjectStore) UpdateContextPack(_ context.Context, projectID string, pack project.ContextPack) error {
	packJSON, _ := json.Marshal(pack)
	_, err := s.db.Exec(`UPDATE projects SET context_pack = ? WHERE id = ?`, string(packJSON), projectID)
	return err
}

func (s *ProjectStore) UpdateStatus(_ context.Context, projectID, status string) error {
	if status != "active" && status != "closed" {
		return project.ErrInvalidStatus
	}
	res, err := s.db.Exec(`UPDATE projects SET status = ? WHERE id = ?`, status, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return project.ErrProjectNotFound
	}
	return nil
}

func (s *ProjectStore) UpdateRepoURL(_ context.Context, projectID, repoURL string) error {
	res, err := s.db.Exec(`UPDATE projects SET repo_url = ? WHERE id = ?`, nilIfEmpty(repoURL), projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return project.ErrProjectNotFound
	}
	return nil
}

func (s *ProjectStore) UpdateName(_ context.Context, projectID, name string) error {
	res, err := s.db.Exec(`UPDATE projects SET name = ? WHERE id = ?`, name, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return project.ErrProjectNotFound
	}
	return nil
}

func (s *ProjectStore) UpdateSlug(_ context.Context, projectID, slug string) error {
	res, err := s.db.Exec(`UPDATE projects SET slug = ? WHERE id = ?`, slug, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return project.ErrProjectNotFound
	}
	return nil
}

func (s *ProjectStore) UpdateDefaultBranch(_ context.Context, projectID, branch string) error {
	if branch == "" {
		branch = "main"
	}
	res, err := s.db.Exec(`UPDATE projects SET default_branch = ? WHERE id = ?`, branch, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return project.ErrProjectNotFound
	}
	return nil
}

func (s *ProjectStore) UpdateDispatchEnabled(_ context.Context, projectID string, enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	res, err := s.db.Exec(`UPDATE projects SET dispatch_enabled = ? WHERE id = ?`, val, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return project.ErrProjectNotFound
	}
	return nil
}

func (s *ProjectStore) UpdateDispatchConfig(_ context.Context, projectID string, cfg project.DispatchConfig) error {
	dispatchConfigJSON, _ := json.Marshal(cfg.Normalized())
	res, err := s.db.Exec(`UPDATE projects SET dispatch_config = ? WHERE id = ?`, string(dispatchConfigJSON), projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return project.ErrProjectNotFound
	}
	return nil
}

// ────────────────────────────────────────────────────────────────────────────
// WorkStreamStore (implements workstream.WorkStreamStore — already an interface)
// ────────────────────────────────────────────────────────────────────────────

type WorkStreamStore struct{ db *sql.DB }

func NewWorkStreamStore(db *sql.DB) *WorkStreamStore { return &WorkStreamStore{db: db} }

func (s *WorkStreamStore) Create(_ context.Context, w *workstream.WorkStream) error {
	_, err := s.db.Exec(
		`INSERT INTO work_streams (id, project_id, name, slug, plan, branch, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.ProjectID, w.Name, w.Slug, nilIfEmpty(w.Plan), nilIfEmpty(w.Branch), w.Status,
		w.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *WorkStreamStore) GetByID(_ context.Context, id string) (*workstream.WorkStream, error) {
	var w workstream.WorkStream
	var plan, branch sql.NullString
	var ts string
	err := s.db.QueryRow(
		`SELECT id, project_id, name, slug, plan, branch, status, created_at FROM work_streams WHERE id = ?`, id).
		Scan(&w.ID, &w.ProjectID, &w.Name, &w.Slug, &plan, &branch, &w.Status, &ts)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, workstream.ErrWorkStreamNotFound
		}
		return nil, err
	}
	if plan.Valid {
		w.Plan = plan.String
	}
	if branch.Valid {
		w.Branch = branch.String
	}
	w.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	return &w, nil
}

func (s *WorkStreamStore) ListByProjectID(_ context.Context, projectID string, statusFilter string) ([]workstream.WorkStream, error) {
	q := `SELECT id, project_id, name, slug, COALESCE(plan,''), COALESCE(branch,''), status, created_at
		  FROM work_streams WHERE project_id = ?`
	if statusFilter == "" || statusFilter == "active" {
		q += ` AND status = 'active'`
	} else if statusFilter == "closed" {
		q += ` AND status = 'closed'`
	}
	q += ` ORDER BY name`
	rows, err := s.db.Query(q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []workstream.WorkStream
	for rows.Next() {
		var w workstream.WorkStream
		var ts string
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.Name, &w.Slug, &w.Plan, &w.Branch, &w.Status, &ts); err != nil {
			return nil, err
		}
		w.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		list = append(list, w)
	}
	return list, rows.Err()
}

func (s *WorkStreamStore) Update(_ context.Context, id string, name, plan, branch, status string) error {
	res, err := s.db.Exec(`UPDATE work_streams SET name = ?, plan = ?, branch = ?, status = ? WHERE id = ?`,
		name, nilIfEmpty(plan), nilIfEmpty(branch), status, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return workstream.ErrWorkStreamNotFound
	}
	return nil
}

// ────────────────────────────────────────────────────────────────────────────
// TicketStore (implements ticket.TicketStore)
// ────────────────────────────────────────────────────────────────────────────

type TicketStore struct{ db *sql.DB }

func NewTicketStore(db *sql.DB) *TicketStore { return &TicketStore{db: db} }

func (s *TicketStore) NextSequence(_ context.Context, projectID string) (int64, error) {
	var next int64
	err := s.db.QueryRow(
		`UPDATE ticket_sequences SET next_val = next_val + 1 WHERE project_id = ? RETURNING next_val`, projectID).
		Scan(&next)
	return next, err
}

func (s *TicketStore) Create(_ context.Context, t *ticket.Ticket) error {
	objJSON, _ := json.Marshal(t.Objective)
	ctxJSON, _ := json.Marshal(t.Context)
	inJSON, _ := json.Marshal(t.Inputs)
	outJSON, _ := json.Marshal(t.Outputs)
	depsJSON, _ := json.Marshal(t.DependsOn)
	_, err := s.db.Exec(
		`INSERT INTO tickets (id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, environment_id, target_repo, assigned_to, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.ProjectID, t.Title, string(t.Type), int(t.Priority), string(t.State), t.Version,
		string(objJSON), string(ctxJSON), string(inJSON), string(outJSON), string(depsJSON),
		nilIfEmpty(t.WorkStreamID), nilIfEmpty(t.EnvironmentID), nilIfEmpty(t.TargetRepo), nilIfEmpty(t.AssignedTo), t.CreatedBy,
		t.CreatedAt.UTC().Format(time.RFC3339Nano), t.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *TicketStore) GetByID(_ context.Context, id string) (*ticket.Ticket, error) {
	return s.scanTicket(`SELECT id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, environment_id, target_repo, assigned_to, created_by, created_at, updated_at
		 FROM tickets WHERE id = ?`, id)
}

func (s *TicketStore) GetByIDs(_ context.Context, ids []string) ([]*ticket.Ticket, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.Query(fmt.Sprintf(
		`SELECT id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, environment_id, target_repo, assigned_to, created_by, created_at, updated_at
		 FROM tickets WHERE id IN (%s)`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanTickets(rows)
}

func (s *TicketStore) GetByProject(_ context.Context, projectID string, workStreamID string, state ticket.State) ([]*ticket.Ticket, error) {
	q := `SELECT id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, environment_id, target_repo, assigned_to, created_by, created_at, updated_at
		 FROM tickets WHERE project_id = ?`
	args := []any{projectID}
	if workStreamID != "" {
		q += ` AND work_stream_id = ?`
		args = append(args, workStreamID)
	}
	if state != "" {
		q += ` AND state = ?`
		args = append(args, string(state))
	}
	q += ` ORDER BY priority, created_at`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanTickets(rows)
}

func (s *TicketStore) ListByState(_ context.Context, projectID string, state ticket.State) ([]*ticket.Ticket, error) {
	var q string
	var args []any
	if projectID != "" {
		q = `SELECT id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, environment_id, target_repo, assigned_to, created_by, created_at, updated_at
		     FROM tickets WHERE project_id = ? AND state = ? ORDER BY priority, created_at`
		args = []any{projectID, string(state)}
	} else {
		q = `SELECT id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, environment_id, target_repo, assigned_to, created_by, created_at, updated_at
		     FROM tickets WHERE state = ? ORDER BY priority, created_at`
		args = []any{string(state)}
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanTickets(rows)
}

func (s *TicketStore) UpdateState(_ context.Context, id string, version int, newState ticket.State, assignedTo string) error {
	res, err := s.db.Exec(
		`UPDATE tickets SET state = ?, version = version + 1, updated_at = datetime('now'), assigned_to = ? WHERE id = ? AND version = ?`,
		string(newState), nilIfEmpty(assignedTo), id, version)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ticket.ErrVersionConflict
	}
	return nil
}

func (s *TicketStore) UpdateOutputs(_ context.Context, id string, version int, outputs map[string]any) error {
	outJSON, _ := json.Marshal(outputs)
	res, err := s.db.Exec(
		`UPDATE tickets SET outputs = ?, version = version + 1, updated_at = datetime('now') WHERE id = ? AND version = ?`,
		string(outJSON), id, version)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ticket.ErrVersionConflict
	}
	return nil
}

func (s *TicketStore) UpdateContext(_ context.Context, id string, ctxVal ticket.TicketContext) error {
	ctxJSON, _ := json.Marshal(ctxVal)
	_, err := s.db.Exec(`UPDATE tickets SET ticket_context = ?, updated_at = datetime('now') WHERE id = ?`, string(ctxJSON), id)
	return err
}

func (s *TicketStore) UpdateDependsOn(_ context.Context, id string, dependsOn []string) error {
	depsJSON, _ := json.Marshal(dependsOn)
	_, err := s.db.Exec(`UPDATE tickets SET depends_on = ?, updated_at = datetime('now') WHERE id = ?`, string(depsJSON), id)
	return err
}

func (s *TicketStore) UpdateWorkStreamID(_ context.Context, id string, workStreamID string) error {
	_, err := s.db.Exec(`UPDATE tickets SET work_stream_id = ?, updated_at = datetime('now') WHERE id = ?`, nilIfEmpty(workStreamID), id)
	return err
}

func (s *TicketStore) UpdateEnvironmentID(_ context.Context, id string, environmentID string) error {
	_, err := s.db.Exec(`UPDATE tickets SET environment_id = ?, updated_at = datetime('now') WHERE id = ?`, nilIfEmpty(environmentID), id)
	return err
}

func (s *TicketStore) UpdateTargetRepo(_ context.Context, id string, targetRepo string) error {
	_, err := s.db.Exec(`UPDATE tickets SET target_repo = ?, updated_at = datetime('now') WHERE id = ?`, nilIfEmpty(targetRepo), id)
	return err
}

func (s *TicketStore) UpdateTitleAndObjective(_ context.Context, id string, title string, obj ticket.Objective) error {
	objJSON, _ := json.Marshal(obj)
	_, err := s.db.Exec(`UPDATE tickets SET title = ?, objective = ?, updated_at = datetime('now') WHERE id = ?`, title, string(objJSON), id)
	return err
}

func (s *TicketStore) CountByCreatedBy(_ context.Context, createdBy string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM tickets WHERE created_by = ?`, createdBy).Scan(&n)
	return n, err
}

func (s *TicketStore) CountByCreatedByPerDay(_ context.Context, createdBy string, days int) ([]int, error) {
	if days <= 0 {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT date(created_at) AS d, COUNT(*) AS c
		 FROM tickets WHERE created_by = ? AND created_at >= date('now', ?)
		 GROUP BY 1 ORDER BY 1`, createdBy, fmt.Sprintf("-%d days", days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	countByDate := make(map[string]int)
	for rows.Next() {
		var d string
		var c int
		if err := rows.Scan(&d, &c); err != nil {
			return nil, err
		}
		countByDate[d] = c
	}
	out := make([]int, days)
	now := time.Now().UTC()
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -days+1+i).Format("2006-01-02")
		out[i] = countByDate[date]
	}
	return out, rows.Err()
}

func (s *TicketStore) GetTicketIDByCreateIdempotency(_ context.Context, projectID, idempotencyKey string) (string, error) {
	var ticketID string
	err := s.db.QueryRow(
		`SELECT ticket_id FROM idempotency_creates WHERE project_id = ? AND idempotency_key = ?`,
		projectID, idempotencyKey).Scan(&ticketID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return ticketID, err
}

func (s *TicketStore) SetCreateIdempotency(_ context.Context, projectID, idempotencyKey, ticketID string) error {
	_, err := s.db.Exec(
		`INSERT INTO idempotency_creates (project_id, idempotency_key, ticket_id) VALUES (?, ?, ?)
		 ON CONFLICT (project_id, idempotency_key) DO NOTHING`,
		projectID, idempotencyKey, ticketID)
	return err
}

func (s *TicketStore) ListStaleTickets(_ context.Context, states []ticket.State, threshold time.Duration) ([]*ticket.Ticket, error) {
	if len(states) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(states))
	placeholders = placeholders[:len(placeholders)-1]
	cutoff := time.Now().UTC().Add(-threshold).Format(time.RFC3339Nano)
	args := make([]any, len(states)+1)
	for i, st := range states {
		args[i] = string(st)
	}
	args[len(states)] = cutoff
	rows, err := s.db.Query(fmt.Sprintf(
		`SELECT id, project_id, title, type, priority, state, version, objective, ticket_context, inputs, outputs, depends_on, work_stream_id, environment_id, target_repo, assigned_to, created_by, created_at, updated_at
		 FROM tickets WHERE state IN (%s) AND updated_at < ? ORDER BY updated_at`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanTickets(rows)
}

// scanTicket scans a single ticket row.
func (s *TicketStore) scanTicket(query string, args ...any) (*ticket.Ticket, error) {
	var t ticket.Ticket
	var objJSON, ctxJSON, inJSON, outJSON, depsJSON string
	var workStreamID, environmentID, targetRepo, assignedTo sql.NullString
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, args...).
		Scan(&t.ID, &t.ProjectID, &t.Title, &t.Type, &t.Priority, &t.State, &t.Version,
			&objJSON, &ctxJSON, &inJSON, &outJSON, &depsJSON, &workStreamID, &environmentID, &targetRepo, &assignedTo, &t.CreatedBy, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if assignedTo.Valid {
		t.AssignedTo = assignedTo.String
	}
	if workStreamID.Valid {
		t.WorkStreamID = workStreamID.String
	}
	if environmentID.Valid {
		t.EnvironmentID = environmentID.String
	}
	if targetRepo.Valid {
		t.TargetRepo = targetRepo.String
	}
	_ = json.Unmarshal([]byte(objJSON), &t.Objective)
	_ = json.Unmarshal([]byte(ctxJSON), &t.Context)
	t.Inputs = make(map[string]any)
	_ = json.Unmarshal([]byte(inJSON), &t.Inputs)
	t.Outputs = make(map[string]any)
	_ = json.Unmarshal([]byte(outJSON), &t.Outputs)
	_ = json.Unmarshal([]byte(depsJSON), &t.DependsOn)
	if t.DependsOn == nil {
		t.DependsOn = []string{}
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &t, nil
}

// scanTickets scans multiple ticket rows.
func (s *TicketStore) scanTickets(rows *sql.Rows) ([]*ticket.Ticket, error) {
	var list []*ticket.Ticket
	for rows.Next() {
		var t ticket.Ticket
		var objJSON, ctxJSON, inJSON, outJSON, depsJSON string
		var workStreamID, environmentID, targetRepo, assignedTo sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Type, &t.Priority, &t.State, &t.Version,
			&objJSON, &ctxJSON, &inJSON, &outJSON, &depsJSON, &workStreamID, &environmentID, &targetRepo, &assignedTo, &t.CreatedBy, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if assignedTo.Valid {
			t.AssignedTo = assignedTo.String
		}
		if workStreamID.Valid {
			t.WorkStreamID = workStreamID.String
		}
		if environmentID.Valid {
			t.EnvironmentID = environmentID.String
		}
		if targetRepo.Valid {
			t.TargetRepo = targetRepo.String
		}
		_ = json.Unmarshal([]byte(objJSON), &t.Objective)
		_ = json.Unmarshal([]byte(ctxJSON), &t.Context)
		t.Inputs = make(map[string]any)
		_ = json.Unmarshal([]byte(inJSON), &t.Inputs)
		t.Outputs = make(map[string]any)
		_ = json.Unmarshal([]byte(outJSON), &t.Outputs)
		_ = json.Unmarshal([]byte(depsJSON), &t.DependsOn)
		if t.DependsOn == nil {
			t.DependsOn = []string{}
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		list = append(list, &t)
	}
	return list, rows.Err()
}

// ────────────────────────────────────────────────────────────────────────────
// ExecutionStepStore (implements execution.StepStore — already an interface)
// ────────────────────────────────────────────────────────────────────────────

type ExecutionStepStore struct{ db *sql.DB }

func NewExecutionStepStore(db *sql.DB) *ExecutionStepStore { return &ExecutionStepStore{db: db} }

func (s *ExecutionStepStore) AppendStep(_ context.Context, ticketID, agentID string, step execution.Step) error {
	payloadJSON, _ := json.Marshal(step.Payload)
	if step.ID == "" {
		step.ID = mustUUID()
	}
	// Include worker_type column (nullable) for worker type tracking.
	var workerType *string
	if step.WorkerType != "" {
		workerType = &step.WorkerType
	}
	_, err := s.db.Exec(
		`INSERT INTO execution_steps (id, ticket_id, agent_id, type, payload, worker_type, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		step.ID, ticketID, agentID, string(step.Type), string(payloadJSON), workerType,
		step.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *ExecutionStepStore) GetStepsByTicketID(_ context.Context, ticketID string) ([]execution.Step, error) {
	rows, err := s.db.Query(
		`SELECT id, type, payload, worker_type, created_at FROM execution_steps WHERE ticket_id = ? ORDER BY created_at`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []execution.Step
	for rows.Next() {
		var st execution.Step
		var payloadJSON, ts string
		var workerType *string
		if err := rows.Scan(&st.ID, &st.Type, &payloadJSON, &workerType, &ts); err != nil {
			return nil, err
		}
		st.Payload = make(map[string]any)
		_ = json.Unmarshal([]byte(payloadJSON), &st.Payload)
		if workerType != nil {
			st.WorkerType = *workerType
		}
		st.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

func (s *ExecutionStepStore) GetAgentIDByTicketID(_ context.Context, ticketID string) (string, error) {
	var agentID string
	err := s.db.QueryRow(
		`SELECT agent_id FROM execution_steps WHERE ticket_id = ? AND agent_id <> '' ORDER BY created_at DESC LIMIT 1`, ticketID).
		Scan(&agentID)
	return agentID, err
}

// ────────────────────────────────────────────────────────────────────────────
// ReviewStore (implements review.ReviewStore)
// ────────────────────────────────────────────────────────────────────────────

type ReviewStore struct{ db *sql.DB }

func NewReviewStore(db *sql.DB) *ReviewStore { return &ReviewStore{db: db} }

func (s *ReviewStore) CreateReview(_ context.Context, r *review.Review) error {
	r.ID = mustUUID()
	_, err := s.db.Exec(
		`INSERT INTO reviews (id, ticket_id, reviewer_id, decision, notes, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		r.ID, r.TicketID, r.ReviewerID, r.Decision, r.Notes,
		r.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *ReviewStore) CreateEscalation(_ context.Context, e *review.Escalation) error {
	e.ID = mustUUID()
	_, err := s.db.Exec(
		`INSERT INTO escalations (id, ticket_id, agent_id, reason, question, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		e.ID, e.TicketID, e.AgentID, e.Reason, e.Question,
		e.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *ReviewStore) UpdateEscalationResolved(_ context.Context, id, answer, resolvedBy string) error {
	_, err := s.db.Exec(
		`UPDATE escalations SET answer = ?, resolved_by = ?, resolved_at = datetime('now') WHERE id = ?`,
		answer, resolvedBy, id)
	return err
}

func (s *ReviewStore) ListReviewsByTicket(_ context.Context, ticketID string) ([]review.Review, error) {
	rows, err := s.db.Query(
		`SELECT id, ticket_id, reviewer_id, decision, notes, created_at FROM reviews WHERE ticket_id = ? ORDER BY created_at`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []review.Review
	for rows.Next() {
		var r review.Review
		var ts string
		if err := rows.Scan(&r.ID, &r.TicketID, &r.ReviewerID, &r.Decision, &r.Notes, &ts); err != nil {
			return nil, err
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *ReviewStore) CountByReviewer(_ context.Context, reviewerID string) (approved, rejected int, err error) {
	err = s.db.QueryRow(
		`SELECT COALESCE(SUM(CASE WHEN decision = 'approved' THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN decision = 'rejected' THEN 1 ELSE 0 END), 0)
		 FROM reviews WHERE reviewer_id = ?`, reviewerID).Scan(&approved, &rejected)
	return
}

func (s *ReviewStore) CountByReviewerPerDay(_ context.Context, reviewerID string, days int) (approved, rejected []int, err error) {
	if days <= 0 {
		return nil, nil, nil
	}
	rows, err := s.db.Query(
		`SELECT date(created_at) AS d,
		 COALESCE(SUM(CASE WHEN decision = 'approved' THEN 1 ELSE 0 END), 0),
		 COALESCE(SUM(CASE WHEN decision = 'rejected' THEN 1 ELSE 0 END), 0)
		 FROM reviews WHERE reviewer_id = ? AND created_at >= date('now', ?)
		 GROUP BY 1 ORDER BY 1`, reviewerID, fmt.Sprintf("-%d days", days))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	type dayCount struct {
		date string
		a, r int
	}
	byDate := make(map[string]dayCount)
	for rows.Next() {
		var d string
		var a, r int
		if err := rows.Scan(&d, &a, &r); err != nil {
			return nil, nil, err
		}
		byDate[d] = dayCount{d, a, r}
	}
	approved = make([]int, days)
	rejected = make([]int, days)
	now := time.Now().UTC()
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -days+1+i).Format("2006-01-02")
		if v, ok := byDate[date]; ok {
			approved[i] = v.a
			rejected[i] = v.r
		}
	}
	return approved, rejected, rows.Err()
}

func (s *ReviewStore) ListPendingReviewTicketIDs(_ context.Context, projectID string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT id FROM tickets WHERE project_id = ? AND state = 'awaiting_review' ORDER BY updated_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *ReviewStore) ListEscalationsByProject(_ context.Context, projectID string) ([]review.Escalation, error) {
	rows, err := s.db.Query(
		`SELECT e.id, e.ticket_id, e.agent_id, e.reason, e.question, e.answer, e.resolved_by, e.resolved_at, e.created_at
		 FROM escalations e JOIN tickets t ON t.id = e.ticket_id WHERE t.project_id = ? AND e.resolved_at IS NULL ORDER BY e.created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []review.Escalation
	for rows.Next() {
		var e review.Escalation
		var answer, resolvedBy, resolvedAt sql.NullString
		var ts string
		if err := rows.Scan(&e.ID, &e.TicketID, &e.AgentID, &e.Reason, &e.Question, &answer, &resolvedBy, &resolvedAt, &ts); err != nil {
			return nil, err
		}
		if answer.Valid {
			e.Answer = answer.String
		}
		if resolvedBy.Valid {
			e.ResolvedBy = resolvedBy.String
		}
		if resolvedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, resolvedAt.String)
			e.ResolvedAt = &t
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		list = append(list, e)
	}
	return list, rows.Err()
}

func (s *ReviewStore) GetEscalationByID(_ context.Context, id string) (*review.Escalation, error) {
	var e review.Escalation
	var answer, resolvedBy, resolvedAt sql.NullString
	var ts string
	err := s.db.QueryRow(
		`SELECT id, ticket_id, agent_id, reason, question, answer, resolved_by, resolved_at, created_at FROM escalations WHERE id = ?`, id).
		Scan(&e.ID, &e.TicketID, &e.AgentID, &e.Reason, &e.Question, &answer, &resolvedBy, &resolvedAt, &ts)
	if err != nil {
		return nil, err
	}
	if answer.Valid {
		e.Answer = answer.String
	}
	if resolvedBy.Valid {
		e.ResolvedBy = resolvedBy.String
	}
	if resolvedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, resolvedAt.String)
		e.ResolvedAt = &t
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
	return &e, nil
}

// ────────────────────────────────────────────────────────────────────────────
// EnvironmentStore (implements environment.EnvironmentStore)
// ────────────────────────────────────────────────────────────────────────────

type EnvironmentStore struct{ db *sql.DB }

func NewEnvironmentStore(db *sql.DB) *EnvironmentStore { return &EnvironmentStore{db: db} }

func (s *EnvironmentStore) Create(_ context.Context, env *environment.Environment) error {
	_, err := s.db.Exec(
		`INSERT INTO environments (id, project_id, name, slug, infrastructure, data_tenancy, integration_mode, is_default, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		env.ID, env.ProjectID, env.Name, env.Slug,
		string(env.Infrastructure), string(env.DataTenancy), string(env.IntegrationMode),
		boolToInt(env.IsDefault), env.CreatedAt.UTC().Format(time.RFC3339), env.UpdatedAt.UTC().Format(time.RFC3339))
	return err
}

func (s *EnvironmentStore) GetByID(_ context.Context, id string) (*environment.Environment, error) {
	return s.scanEnvironment(`SELECT id, project_id, name, slug, infrastructure, data_tenancy, integration_mode, is_default, created_at, updated_at
		 FROM environments WHERE id = ?`, id)
}

func (s *EnvironmentStore) GetBySlug(_ context.Context, projectID, slug string) (*environment.Environment, error) {
	return s.scanEnvironment(`SELECT id, project_id, name, slug, infrastructure, data_tenancy, integration_mode, is_default, created_at, updated_at
		 FROM environments WHERE project_id = ? AND slug = ?`, projectID, slug)
}

func (s *EnvironmentStore) ListByProject(_ context.Context, projectID string) ([]*environment.Environment, error) {
	rows, err := s.db.Query(
		`SELECT id, project_id, name, slug, infrastructure, data_tenancy, integration_mode, is_default, created_at, updated_at
		 FROM environments WHERE project_id = ? ORDER BY is_default DESC, name ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanEnvironmentRows(rows)
}

func (s *EnvironmentStore) CountByProject(_ context.Context, projectID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM environments WHERE project_id = ?`, projectID).Scan(&count)
	return count, err
}

func (s *EnvironmentStore) Update(_ context.Context, env *environment.Environment) error {
	res, err := s.db.Exec(
		`UPDATE environments SET name = ?, slug = ?, infrastructure = ?, data_tenancy = ?, integration_mode = ?, is_default = ?, updated_at = datetime('now')
		 WHERE id = ?`,
		env.Name, env.Slug, string(env.Infrastructure), string(env.DataTenancy), string(env.IntegrationMode),
		boolToInt(env.IsDefault), env.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return environment.ErrNotFound
	}
	return nil
}

func (s *EnvironmentStore) Delete(_ context.Context, id string) error {
	res, err := s.db.Exec(`DELETE FROM environments WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return environment.ErrNotFound
	}
	return nil
}

func (s *EnvironmentStore) ClearDefault(_ context.Context, projectID string) error {
	_, err := s.db.Exec(`UPDATE environments SET is_default = 0 WHERE project_id = ?`, projectID)
	return err
}

func (s *EnvironmentStore) scanEnvironment(query string, args ...any) (*environment.Environment, error) {
	var env environment.Environment
	var isDefault int
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, args...).
		Scan(&env.ID, &env.ProjectID, &env.Name, &env.Slug,
			&env.Infrastructure, &env.DataTenancy, &env.IntegrationMode,
			&isDefault, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, environment.ErrNotFound
		}
		return nil, err
	}
	env.IsDefault = isDefault != 0
	env.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	env.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &env, nil
}

func (s *EnvironmentStore) scanEnvironmentRows(rows *sql.Rows) ([]*environment.Environment, error) {
	var list []*environment.Environment
	for rows.Next() {
		var env environment.Environment
		var isDefault int
		var createdAt, updatedAt string
		if err := rows.Scan(&env.ID, &env.ProjectID, &env.Name, &env.Slug,
			&env.Infrastructure, &env.DataTenancy, &env.IntegrationMode,
			&isDefault, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		env.IsDefault = isDefault != 0
		env.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		env.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		list = append(list, &env)
	}
	return list, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
