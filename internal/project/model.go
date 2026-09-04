package project

import (
	"fmt"
	"strings"
	"time"
)

// Project is a project under an org (e.g. "hubble-backend").
// Status is "active" (default) or "closed". List endpoints default to active only.
type Project struct {
	ID              string         `json:"id"`
	OrgID           string         `json:"org_id"`
	Name            string         `json:"name"`
	Slug            string         `json:"slug"`
	Description     string         `json:"description,omitempty"`
	RepoURL         string         `json:"repo_url,omitempty"`
	DefaultBranch   string         `json:"default_branch,omitempty"` // branch to checkout when closing work stream; default "main"
	TechStack       []string       `json:"tech_stack,omitempty"`
	ContextPack     ContextPack    `json:"context_pack"`
	Status          string         `json:"status"`           // "active" or "closed"
	DispatchEnabled bool           `json:"dispatch_enabled"` // whether the dispatcher picks up tickets for this project (default true)
	DispatchConfig  DispatchConfig `json:"dispatch_config,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}

type DispatchConfig struct {
	MaxActiveWorkers int                           `json:"max_active_workers,omitempty"`
	Roles            []DispatchWorkerRole          `json:"roles,omitempty"`
	Workers          []DispatchWorkerProfile       `json:"workers,omitempty"`
	Policies         map[string]DispatchRolePolicy `json:"policies,omitempty"`
	GitPolicy        *GitPolicy                    `json:"git_policy,omitempty"`
}

type DispatchWorkerRole struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	BaseType    string `json:"base_type,omitempty"`
}

type DispatchWorkerProfile struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Enabled          bool     `json:"enabled"`
	Runner           string   `json:"runner,omitempty"`
	Driver           string   `json:"driver,omitempty"`
	CLIPath          string   `json:"cli_path,omitempty"`
	Model            string   `json:"model,omitempty"`
	ReasoningEffort  string   `json:"reasoning_effort,omitempty"`
	APIBaseURL       string   `json:"api_base_url,omitempty"`
	CredentialEnvVar string   `json:"credential_env_var,omitempty"`
	Args             []string `json:"args,omitempty"`
	SystemPrompt     string   `json:"system_prompt,omitempty"` // base prompt prepended for this worker
}

type DispatchRolePolicy struct {
	SelectionMode string   `json:"selection_mode,omitempty"`
	WorkerIDs     []string `json:"worker_ids,omitempty"`
}

func (c DispatchConfig) Normalized() DispatchConfig {
	out := c
	if out.MaxActiveWorkers < 0 {
		out.MaxActiveWorkers = 0
	}
	if out.Roles == nil {
		out.Roles = []DispatchWorkerRole{}
	}
	if out.Workers == nil {
		out.Workers = []DispatchWorkerProfile{}
	}
	if out.Policies == nil {
		out.Policies = map[string]DispatchRolePolicy{}
	}
	for key, policy := range out.Policies {
		if policy.SelectionMode != "any" {
			policy.SelectionMode = "ordered"
		}
		if policy.WorkerIDs == nil {
			policy.WorkerIDs = []string{}
		}
		out.Policies[key] = policy
	}
	roles := make([]DispatchWorkerRole, 0, len(out.Roles))
	seenRoles := make(map[string]struct{}, len(out.Roles))
	for i, role := range out.Roles {
		role.ID = normalizeDispatchRoleID(role.ID)
		if role.ID == "" {
			role.ID = normalizeDispatchRoleID(role.Name)
		}
		if role.ID == "" {
			role.ID = fmt.Sprintf("custom_role_%d", i+1)
		}
		if isReservedDispatchRoleID(role.ID) {
			continue
		}
		if _, ok := seenRoles[role.ID]; ok {
			continue
		}
		seenRoles[role.ID] = struct{}{}

		role.Name = strings.TrimSpace(role.Name)
		if role.Name == "" {
			role.Name = role.ID
		}
		role.Description = strings.TrimSpace(role.Description)
		role.BaseType = normalizeDispatchRoleBaseType(role.BaseType)
		roles = append(roles, role)
	}
	out.Roles = roles
	for i := range out.Workers {
		out.Workers[i].ID = strings.TrimSpace(out.Workers[i].ID)
		if out.Workers[i].ID == "" {
			out.Workers[i].ID = fmt.Sprintf("worker-%d", i+1)
		}
		out.Workers[i].Name = strings.TrimSpace(out.Workers[i].Name)
		if out.Workers[i].Name == "" {
			out.Workers[i].Name = out.Workers[i].ID
		}
		out.Workers[i].Runner = strings.TrimSpace(out.Workers[i].Runner)
		out.Workers[i].Driver = strings.TrimSpace(out.Workers[i].Driver)
		out.Workers[i].CLIPath = strings.TrimSpace(out.Workers[i].CLIPath)
		out.Workers[i].Model = strings.TrimSpace(out.Workers[i].Model)
		out.Workers[i].ReasoningEffort = strings.TrimSpace(out.Workers[i].ReasoningEffort)
		out.Workers[i].APIBaseURL = strings.TrimSpace(out.Workers[i].APIBaseURL)
		out.Workers[i].CredentialEnvVar = strings.TrimSpace(out.Workers[i].CredentialEnvVar)
		if out.Workers[i].Args == nil {
			out.Workers[i].Args = []string{}
		}
	}
	return out
}

func normalizeDispatchRoleID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastSeparator := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastSeparator = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSeparator = false
		case r == '_' || r == '-' || r == ' ' || r == '/':
			if b.Len() > 0 && !lastSeparator {
				b.WriteByte('_')
				lastSeparator = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func normalizeDispatchRoleBaseType(value string) string {
	switch normalizeDispatchRoleID(value) {
	case "planner", "planning":
		return "planner"
	case "validator", "review", "validation":
		return "validator"
	case "deployer", "deploy", "deployment":
		return "deployer"
	case "investigator", "investigation":
		return "investigator"
	default:
		return "executor"
	}
}

func isReservedDispatchRoleID(id string) bool {
	switch normalizeDispatchRoleID(id) {
	case "orchestrator", "planner", "executor", "validator", "deployer", "investigator", "conflict_resolver":
		return true
	default:
		return false
	}
}

// ContextPack is injected into every agent ticket claim.
type ContextPack struct {
	Conventions  string            `json:"conventions,omitempty"`
	KeyFiles     []FileRef         `json:"key_files,omitempty"`
	SystemPrompt string            `json:"system_prompt,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
}

// FileRef points to a file (path and optional snippet).
type FileRef struct {
	Path    string `json:"path"`
	Snippet string `json:"snippet,omitempty"`
}

// GitPolicy configures how dispatched work maps onto git: branch naming, PR
// requirements, and merge behavior. Stored in the project's dispatch config.
type GitPolicy struct {
	BranchPrefix     string `json:"branch_prefix"`      // prefix for ticket branches (default "ticket/")
	BaseBranch       string `json:"base_branch"`        // PR target branch (default "main")
	RequirePR        bool   `json:"require_pr"`         // require a PR before merge
	RequireReview    bool   `json:"require_review"`     // require PR review before merge
	RequireCI        bool   `json:"require_ci"`         // require CI to pass before merge
	AutoMerge        bool   `json:"auto_merge"`         // merge automatically when checks pass
	CommitTagPattern string `json:"commit_tag_pattern"` // commit tag pattern (default "ticket/{ticket_id}")
}

// BranchNameForTicket returns the branch name for a ticket using BranchPrefix
// (default "ticket/").
func (g *GitPolicy) BranchNameForTicket(ticketID string) string {
	prefix := "ticket/"
	if g != nil && g.BranchPrefix != "" {
		prefix = g.BranchPrefix
	}
	return prefix + ticketID
}
