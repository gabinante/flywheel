package org

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists orgs and members.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create inserts an org.
func (s *Store) Create(ctx context.Context, o *Org) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO orgs (id, name, slug, created_at) VALUES ($1, $2, $3, $4)`,
		o.ID, o.Name, o.Slug, o.CreatedAt)
	return err
}

// GetByID returns an org by ID.
func (s *Store) GetByID(ctx context.Context, id string) (*Org, error) {
	var o Org
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, created_at FROM orgs WHERE id = $1`, id).
		Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetBySlug returns an org by slug.
func (s *Store) GetBySlug(ctx context.Context, slug string) (*Org, error) {
	var o Org
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, created_at FROM orgs WHERE slug = $1`, slug).
		Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// AddMember inserts an org member.
func (s *Store) AddMember(ctx context.Context, m *Member) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, $3)
		 ON CONFLICT (org_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		m.OrgID, m.UserID, string(m.Role))
	return err
}

// ListMembers returns all members of an org.
func (s *Store) ListMembers(ctx context.Context, orgID string) ([]Member, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT org_id, user_id, role FROM org_members WHERE org_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.OrgID, &m.UserID, &m.Role); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// ListOrgIDsByUserID returns org IDs the user is a member of.
func (s *Store) ListOrgIDsByUserID(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT org_id FROM org_members WHERE user_id = $1`, userID)
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

// ListOrgsByUserID returns orgs the user is a member of (id, name, slug, created_at).
func (s *Store) ListOrgsByUserID(ctx context.Context, userID string) ([]*Org, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT o.id, o.name, o.slug, o.created_at
		 FROM orgs o
		 INNER JOIN org_members m ON m.org_id = o.id
		 WHERE m.user_id = $1
		 ORDER BY o.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Org
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &o)
	}
	return list, rows.Err()
}

// CreateInvite inserts an org invite.
func (s *Store) CreateInvite(ctx context.Context, inv *Invite) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO org_invites (id, org_id, code, role, created_by, expires_at, max_uses, use_count, revoked, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		inv.ID, inv.OrgID, inv.Code, string(inv.Role), inv.CreatedBy,
		inv.ExpiresAt, inv.MaxUses, inv.UseCount, inv.Revoked, inv.CreatedAt)
	return err
}

// GetInviteByCode returns an invite by its unique code.
func (s *Store) GetInviteByCode(ctx context.Context, code string) (*Invite, error) {
	var inv Invite
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, code, role, created_by, expires_at, max_uses, use_count, revoked, created_at
		 FROM org_invites WHERE code = $1`, code).
		Scan(&inv.ID, &inv.OrgID, &inv.Code, &inv.Role, &inv.CreatedBy,
			&inv.ExpiresAt, &inv.MaxUses, &inv.UseCount, &inv.Revoked, &inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// ListInvitesByOrg returns non-revoked invites for an org.
func (s *Store) ListInvitesByOrg(ctx context.Context, orgID string) ([]Invite, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, code, role, created_by, expires_at, max_uses, use_count, revoked, created_at
		 FROM org_invites WHERE org_id = $1 AND revoked = FALSE
		 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Invite
	for rows.Next() {
		var inv Invite
		if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.Code, &inv.Role, &inv.CreatedBy,
			&inv.ExpiresAt, &inv.MaxUses, &inv.UseCount, &inv.Revoked, &inv.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, inv)
	}
	return list, rows.Err()
}

// IncrementInviteUseCount atomically increments use_count.
func (s *Store) IncrementInviteUseCount(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE org_invites SET use_count = use_count + 1 WHERE id = $1`, id)
	return err
}

// RevokeInvite sets revoked=true on an invite.
func (s *Store) RevokeInvite(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE org_invites SET revoked = TRUE WHERE id = $1`, id)
	return err
}
