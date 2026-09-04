package project

import "time"

// Repository represents a git repository associated with a project.
// A project can have multiple repositories; one is marked as primary.
// The primary repo corresponds to the legacy project.repo_url field.
type Repository struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	Alias         string    `json:"alias"` // unique per project, e.g. "backend", "frontend"
	RepoURL       string    `json:"repo_url"`
	DefaultBranch string    `json:"default_branch"`
	IsPrimary     bool      `json:"is_primary"`
	CreatedAt     time.Time `json:"created_at"`
}
