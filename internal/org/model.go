package org

import "time"

// Role is a member's role in an organization.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// Org is an organization (tenant).
type Org struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// Member is an org member with a role.
type Member struct {
	OrgID  string
	UserID string
	Role   Role
}

// Invite is a shareable link to join an organization.
type Invite struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Code      string    `json:"code"`
	Role      Role      `json:"role"`
	CreatedBy string    `json:"created_by"`
	ExpiresAt time.Time `json:"expires_at"`
	MaxUses   int       `json:"max_uses"`
	UseCount  int       `json:"use_count"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
}
