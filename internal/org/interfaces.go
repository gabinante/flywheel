package org

import "context"

// OrgStore is the persistence interface for orgs.
// *Store (Postgres) and embedded.OrgStore (SQLite) implement this.
type OrgStore interface {
	Create(ctx context.Context, o *Org) error
	GetByID(ctx context.Context, id string) (*Org, error)
	GetBySlug(ctx context.Context, slug string) (*Org, error)
	AddMember(ctx context.Context, m *Member) error
	ListMembers(ctx context.Context, orgID string) ([]Member, error)
	ListOrgIDsByUserID(ctx context.Context, userID string) ([]string, error)
	ListOrgsByUserID(ctx context.Context, userID string) ([]*Org, error)
}
