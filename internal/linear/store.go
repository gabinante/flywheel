package linear

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gabinante/flywheel/internal/ticket"
)

// ProjectLink maps a Flywheel project to a Linear project.
type ProjectLink struct {
	ProjectID         string     `json:"project_id"`
	LinearProjectID   string     `json:"linear_project_id"`
	LinearProjectName string     `json:"linear_project_name"`
	LinearProjectURL  string     `json:"linear_project_url"`
	TeamIDs           []string   `json:"team_ids"`
	TeamKeys          []string   `json:"team_keys"`
	SyncedAt          *time.Time `json:"synced_at,omitempty"`
	LastError         string     `json:"last_error,omitempty"`
}

// Store persists project links and ticket projections. It also implements
// ticket.ExternalRefLookup so ticket reads carry their Linear projection.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const linkCols = `project_id, linear_project_id, linear_project_name, linear_project_url, team_ids, team_keys, synced_at, last_error`

func scanLink(row pgx.Row) (*ProjectLink, error) {
	var l ProjectLink
	if err := row.Scan(&l.ProjectID, &l.LinearProjectID, &l.LinearProjectName, &l.LinearProjectURL, &l.TeamIDs, &l.TeamKeys, &l.SyncedAt, &l.LastError); err != nil {
		return nil, err
	}
	return &l, nil
}

// UpsertLink creates or refreshes a project link.
func (s *Store) UpsertLink(ctx context.Context, l *ProjectLink) error {
	if l.TeamIDs == nil {
		l.TeamIDs = []string{}
	}
	if l.TeamKeys == nil {
		l.TeamKeys = []string{}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO project_linear_links (project_id, linear_project_id, linear_project_name, linear_project_url, team_ids, team_keys)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (project_id) DO UPDATE SET
			linear_project_id = EXCLUDED.linear_project_id,
			linear_project_name = EXCLUDED.linear_project_name,
			linear_project_url = EXCLUDED.linear_project_url,
			team_ids = EXCLUDED.team_ids,
			team_keys = EXCLUDED.team_keys,
			updated_at = now()`,
		l.ProjectID, l.LinearProjectID, l.LinearProjectName, l.LinearProjectURL, l.TeamIDs, l.TeamKeys)
	return err
}

// MarkSynced records a successful (or failed) sync pass.
func (s *Store) MarkSynced(ctx context.Context, projectID string, at time.Time, syncErr string) error {
	if syncErr != "" {
		_, err := s.pool.Exec(ctx, `UPDATE project_linear_links SET last_error = $2, updated_at = now() WHERE project_id = $1`, projectID, syncErr)
		return err
	}
	_, err := s.pool.Exec(ctx, `UPDATE project_linear_links SET synced_at = $2, last_error = '', updated_at = now() WHERE project_id = $1`, projectID, at)
	return err
}

// ListLinks returns all project links.
func (s *Store) ListLinks(ctx context.Context) ([]*ProjectLink, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+linkCols+` FROM project_linear_links ORDER BY linear_project_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ProjectLink
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LinkByProject returns the link for a Flywheel project, or nil.
func (s *Store) LinkByProject(ctx context.Context, projectID string) (*ProjectLink, error) {
	l, err := scanLink(s.pool.QueryRow(ctx, `SELECT `+linkCols+` FROM project_linear_links WHERE project_id = $1`, projectID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return l, err
}

// LinkByLinearProject returns the link for a Linear project id, or nil.
func (s *Store) LinkByLinearProject(ctx context.Context, linearProjectID string) (*ProjectLink, error) {
	l, err := scanLink(s.pool.QueryRow(ctx, `SELECT `+linkCols+` FROM project_linear_links WHERE linear_project_id = $1`, linearProjectID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return l, err
}

const refCols = `ticket_id, provider, external_id, identifier, url, state_name, state_type, assignee, team_key, priority, labels, branch_name, updated_at, synced_at`

type refRow struct {
	TicketID string
	Ref      ticket.ExternalRef
}

func scanRef(row pgx.Row) (*refRow, error) {
	var r refRow
	if err := row.Scan(&r.TicketID, &r.Ref.Provider, &r.Ref.ExternalID, &r.Ref.Identifier, &r.Ref.URL, &r.Ref.StateName,
		&r.Ref.StateType, &r.Ref.Assignee, &r.Ref.TeamKey, &r.Ref.Priority, &r.Ref.Labels, &r.Ref.BranchName, &r.Ref.UpdatedAt, &r.Ref.SyncedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

// UpsertRef stores the projection for a ticket.
func (s *Store) UpsertRef(ctx context.Context, ticketID string, ref ticket.ExternalRef) error {
	if ref.Labels == nil {
		ref.Labels = []string{}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ticket_external_refs (ticket_id, provider, external_id, identifier, url, state_name, state_type, assignee, team_key, priority, labels, branch_name, updated_at, synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now())
		ON CONFLICT (ticket_id) DO UPDATE SET
			external_id = EXCLUDED.external_id, identifier = EXCLUDED.identifier, url = EXCLUDED.url,
			state_name = EXCLUDED.state_name, state_type = EXCLUDED.state_type, assignee = EXCLUDED.assignee,
			team_key = EXCLUDED.team_key, priority = EXCLUDED.priority, labels = EXCLUDED.labels,
			branch_name = EXCLUDED.branch_name, updated_at = EXCLUDED.updated_at, synced_at = now()`,
		ticketID, ref.Provider, ref.ExternalID, ref.Identifier, ref.URL, ref.StateName, ref.StateType, ref.Assignee,
		ref.TeamKey, ref.Priority, ref.Labels, ref.BranchName, ref.UpdatedAt)
	return err
}

// RefByExternalID returns the ticket id and projection for a Linear issue id, or nil.
func (s *Store) RefByExternalID(ctx context.Context, provider, externalID string) (string, *ticket.ExternalRef, error) {
	r, err := scanRef(s.pool.QueryRow(ctx, `SELECT `+refCols+` FROM ticket_external_refs WHERE provider = $1 AND external_id = $2`, provider, externalID))
	if err == pgx.ErrNoRows {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	return r.TicketID, &r.Ref, nil
}

// RefByTicketID returns the projection for a ticket, or nil.
func (s *Store) RefByTicketID(ctx context.Context, ticketID string) (*ticket.ExternalRef, error) {
	r, err := scanRef(s.pool.QueryRow(ctx, `SELECT `+refCols+` FROM ticket_external_refs WHERE ticket_id = $1`, ticketID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r.Ref, nil
}

// RefsByTicketIDs implements ticket.ExternalRefLookup.
func (s *Store) RefsByTicketIDs(ctx context.Context, ticketIDs []string) (map[string]*ticket.ExternalRef, error) {
	out := map[string]*ticket.ExternalRef{}
	if len(ticketIDs) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT `+refCols+` FROM ticket_external_refs WHERE ticket_id = ANY($1)`, ticketIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanRef(rows)
		if err != nil {
			return nil, err
		}
		ref := r.Ref
		out[r.TicketID] = &ref
	}
	return out, rows.Err()
}

// TicketIDByIdentifier implements ticket.ExternalRefLookup.
func (s *Store) TicketIDByIdentifier(ctx context.Context, identifier string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT ticket_id FROM ticket_external_refs WHERE identifier = $1`, identifier).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}

// CountRefsByProject returns how many tickets in a project carry a projection.
func (s *Store) CountRefsByProject(ctx context.Context, projectID string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM ticket_external_refs r JOIN tickets t ON t.id = r.ticket_id WHERE t.project_id = $1`, projectID).Scan(&n)
	return n, err
}

// CountRefs returns the total number of ticket projections.
func (s *Store) CountRefs(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM ticket_external_refs`).Scan(&n)
	return n, err
}

// DefaultOrgID returns the operator's organization (the oldest org), or "".
func (s *Store) DefaultOrgID(ctx context.Context) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id FROM orgs ORDER BY created_at LIMIT 1`).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}
