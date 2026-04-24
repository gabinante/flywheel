package delivery

import "time"

const ConfigKey = "delivery_config"

type IntegrationStatus string

const (
	StatusOK           IntegrationStatus = "ok"
	StatusUnconfigured IntegrationStatus = "unconfigured"
	StatusUnsupported  IntegrationStatus = "unsupported"
	StatusDegraded     IntegrationStatus = "degraded"
)

type Config struct {
	SCM            SCMConfig            `json:"scm,omitempty"`
	Infrastructure InfrastructureConfig `json:"infrastructure,omitempty"`
}

type SCMConfig struct {
	Provider string `json:"provider,omitempty"`
}

type InfrastructureConfig struct {
	Provider string       `json:"provider,omitempty"`
	FlyIO    *FlyIOConfig `json:"flyio,omitempty"`
}

type FlyIOConfig struct {
	OrganizationSlug string           `json:"organization_slug,omitempty"`
	Apps             []FlyIOAppConfig `json:"apps,omitempty"`
}

type FlyIOAppConfig struct {
	AppName         string `json:"app_name"`
	EnvironmentSlug string `json:"environment_slug"`
	Service         string `json:"service,omitempty"`
}

type PullRequest struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	State     string    `json:"state"`
	Draft     bool      `json:"draft"`
	Author    string    `json:"author,omitempty"`
	HeadRef   string    `json:"head_ref,omitempty"`
	BaseRef   string    `json:"base_ref,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PullRequestOverview struct {
	ProjectID    string            `json:"project_id"`
	Status       IntegrationStatus `json:"status"`
	Provider     string            `json:"provider,omitempty"`
	Repository   string            `json:"repository,omitempty"`
	Message      string            `json:"message,omitempty"`
	PullRequests []PullRequest     `json:"pull_requests"`
}

type PipelineOverview struct {
	ProjectID    string                `json:"project_id"`
	Status       IntegrationStatus     `json:"status"`
	Provider     string                `json:"provider,omitempty"`
	Message      string                `json:"message,omitempty"`
	RefreshedAt  *time.Time            `json:"refreshed_at,omitempty"`
	Environments []PipelineEnvironment `json:"environments"`
}

type PipelineEnvironment struct {
	ID              string               `json:"id,omitempty"`
	Name            string               `json:"name"`
	Slug            string               `json:"slug"`
	Infrastructure  string               `json:"infrastructure,omitempty"`
	DataTenancy     string               `json:"data_tenancy,omitempty"`
	IntegrationMode string               `json:"integration_mode,omitempty"`
	IsDefault       bool                 `json:"is_default"`
	Deployments     []PipelineDeployment `json:"deployments"`
	Resources       []PipelineResource   `json:"resources"`
}

type PipelineDeployment struct {
	Service    string    `json:"service"`
	Version    string    `json:"version"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
}

type PipelineResource struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Provider   string    `json:"provider,omitempty"`
	Region     string    `json:"region,omitempty"`
	Status     string    `json:"status,omitempty"`
	Version    string    `json:"version,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}
