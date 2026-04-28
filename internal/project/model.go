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
	RepoURL         string         `json:"repo_url,omitempty"`
	DefaultBranch   string         `json:"default_branch,omitempty"` // branch to checkout when closing work stream; default "main"
	TechStack       []string       `json:"tech_stack,omitempty"`
	ContextPack     ContextPack    `json:"context_pack"`
	Status          string         `json:"status"`           // "active" or "closed"
	DispatchEnabled bool           `json:"dispatch_enabled"` // whether the dispatcher picks up tickets for this project (default true)
	DispatchConfig  DispatchConfig `json:"dispatch_config,omitempty"`
	WebhookSecret   string         `json:"webhook_secret,omitempty"` // 32-byte hex HMAC key for outbound webhook signing
	CreatedAt       time.Time      `json:"created_at"`
}

type DispatchConfig struct {
	Workers  []DispatchWorkerProfile       `json:"workers,omitempty"`
	Policies map[string]DispatchRolePolicy `json:"policies,omitempty"`
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
}

type DispatchRolePolicy struct {
	SelectionMode string   `json:"selection_mode,omitempty"`
	WorkerIDs     []string `json:"worker_ids,omitempty"`
}

func (c DispatchConfig) Normalized() DispatchConfig {
	out := c
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
