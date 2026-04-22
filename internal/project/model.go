package project

import "time"

// Project is a project under an org (e.g. "hubble-backend").
// Status is "active" (default) or "closed". List endpoints default to active only.
type Project struct {
	ID              string      `json:"id"`
	OrgID           string      `json:"org_id"`
	Name            string      `json:"name"`
	Slug            string      `json:"slug"`
	RepoURL         string      `json:"repo_url,omitempty"`
	DefaultBranch   string      `json:"default_branch,omitempty"` // branch to checkout when closing work stream; default "main"
	TechStack       []string    `json:"tech_stack,omitempty"`
	ContextPack     ContextPack `json:"context_pack"`
	Status          string      `json:"status"`           // "active" or "closed"
	DispatchEnabled bool        `json:"dispatch_enabled"` // whether the dispatcher picks up tickets for this project (default true)
	CreatedAt       time.Time   `json:"created_at"`
}

// ContextPack is injected into every agent ticket claim.
type ContextPack struct {
	Conventions     string            `json:"conventions,omitempty"`
	KeyFiles        []FileRef         `json:"key_files,omitempty"`
	SystemPrompt   string            `json:"system_prompt,omitempty"`
	Extra          map[string]string  `json:"extra,omitempty"`
}

// FileRef points to a file (path and optional snippet).
type FileRef struct {
	Path    string `json:"path"`
	Snippet string `json:"snippet,omitempty"`
}
