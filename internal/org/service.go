package org

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Default org slug prefix for per-user orgs (named after email/login). Collaboration orgs use a different flow.
const defaultOrgSlugPrefix = "u-"

// Service provides org operations. Other packages call this, not the store.
type Service struct {
	store OrgStore
}

// NewService returns a new Service. The store parameter accepts any OrgStore
// implementation (Postgres *Store, embedded SQLite, etc.).
func NewService(store OrgStore) *Service {
	return &Service{store: store}
}

// CreateOrg creates an organization. Slug is derived from name if not provided.
func (s *Service) CreateOrg(ctx context.Context, name, slug string) (*Org, error) {
	if slug == "" {
		slug = slugify(name)
	}
	id := uuid.Must(uuid.NewV7()).String()
	o := &Org{ID: id, Name: name, Slug: slug, CreatedAt: time.Now().UTC()}
	if err := s.store.Create(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// CreateOrgWithOwner creates an org and adds the given user as owner. Use after OAuth or authenticated API.
func (s *Service) CreateOrgWithOwner(ctx context.Context, name, slug, ownerUserID string) (*Org, error) {
	o, err := s.CreateOrg(ctx, name, slug)
	if err != nil {
		return nil, err
	}
	if err := s.AddMember(ctx, o.ID, ownerUserID, RoleOwner); err != nil {
		return nil, err
	}
	return o, nil
}

// ListOrgIDsForUser returns org IDs the user is a member of (for scoping MCP/REST).
func (s *Service) ListOrgIDsForUser(ctx context.Context, userID string) ([]string, error) {
	return s.store.ListOrgIDsByUserID(ctx, userID)
}

// ListOrgsForUser returns orgs the user is a member of (for list_orgs MCP).
func (s *Service) ListOrgsForUser(ctx context.Context, userID string) ([]*Org, error) {
	return s.store.ListOrgsByUserID(ctx, userID)
}

// EnsureDefaultOrgForUser creates a personal org for the user (named after email or login) and adds them as owner, only if they have no orgs yet. Call after OAuth sign-up or on first MCP use (e.g. list_orgs) so existing users get a default org without re-signing in. Collaboration orgs are created separately when the user wants to work with others.
func (s *Service) EnsureDefaultOrgForUser(ctx context.Context, userID, displayName string) error {
	orgIDs, err := s.store.ListOrgIDsByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if len(orgIDs) > 0 {
		return nil
	}
	// Unique slug from user ID so we never collide (e.g. u-a1b2c3d4e5f6)
	slug := defaultOrgSlugPrefix + strings.ReplaceAll(userID, "-", "")[:12]
	name := displayName
	if name == "" {
		name = "Personal"
	}
	_, err = s.CreateOrgWithOwner(ctx, name, slug, userID)
	return err
}

// AddMember adds or updates a member's role.
func (s *Service) AddMember(ctx context.Context, orgID, userID string, role Role) error {
	return s.store.AddMember(ctx, &Member{OrgID: orgID, UserID: userID, Role: role})
}

// GetOrg returns an org by ID.
func (s *Service) GetOrg(ctx context.Context, id string) (*Org, error) {
	return s.store.GetByID(ctx, id)
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "org"
	}
	return s
}
