package delivery

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/catalog"
	"github.com/gabinante/flywheel/internal/environment"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/stateindex"
)

type ProjectStore interface {
	GetProject(ctx context.Context, projectID string) (*project.Project, error)
	UpdateContextPack(ctx context.Context, projectID string, pack project.ContextPack) error
}

type EnvironmentStore interface {
	ListEnvironments(ctx context.Context, projectID string) ([]*environment.Environment, error)
}

type CatalogStore interface {
	DeploymentMatrix(ctx context.Context, projectID, serviceID string) ([]*catalog.DeploymentEntry, error)
	UpsertDeployment(ctx context.Context, d *catalog.DeploymentEntry) error
}

type StateStore interface {
	ObserveResource(ctx context.Context, r *stateindex.ObservedResource) (*stateindex.StateChange, error)
	QueryResources(ctx context.Context, q stateindex.ResourceQuery) ([]*stateindex.ObservedResource, error)
}

type PullRequestProvider interface {
	Name() string
	Supports(repoURL string) bool
	ListOpenPullRequests(ctx context.Context, repoURL string) (string, []PullRequest, error)
}

type PipelineProvider interface {
	Name() string
	Sync(ctx context.Context, proj *project.Project, envs []*environment.Environment, cfg Config) (*PipelineSyncResult, error)
}

type PipelineSyncResult struct {
	Deployments []*catalog.DeploymentEntry
	Resources   []*stateindex.ObservedResource
	RefreshedAt time.Time
	Warnings    []string
}

type Service struct {
	projects             ProjectStore
	environments         EnvironmentStore
	catalog              CatalogStore
	state                StateStore
	pullRequestProviders []PullRequestProvider
	pipelineProviders    map[string]PipelineProvider
	flyIOFallbackToken   string // server-wide FLY_API_TOKEN env var
	flyIOBaseURL         string // optional override for Fly.io API base URL
}

func NewService(projects ProjectStore, environments EnvironmentStore, catalog CatalogStore, state StateStore) *Service {
	return &Service{
		projects:             projects,
		environments:         environments,
		catalog:              catalog,
		state:                state,
		pipelineProviders:    make(map[string]PipelineProvider),
		pullRequestProviders: []PullRequestProvider{},
	}
}

// SetFlyIOFallback configures the server-wide Fly.io API token and base URL.
// Per-project tokens take precedence; this is the fallback.
func (s *Service) SetFlyIOFallback(token, baseURL string) {
	s.flyIOFallbackToken = strings.TrimSpace(token)
	s.flyIOBaseURL = strings.TrimSpace(baseURL)
}

func (s *Service) RegisterPullRequestProvider(provider PullRequestProvider) {
	if provider == nil {
		return
	}
	s.pullRequestProviders = append(s.pullRequestProviders, provider)
}

func (s *Service) RegisterPipelineProvider(provider PipelineProvider) {
	if provider == nil {
		return
	}
	s.pipelineProviders[strings.ToLower(provider.Name())] = provider
}

func (s *Service) GetConfig(ctx context.Context, projectID string) (Config, error) {
	proj, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		return Config{}, err
	}
	if proj == nil {
		return Config{}, project.ErrProjectNotFound
	}
	return ParseConfig(proj.ContextPack.Extra)
}

// GetMaskedConfig returns a ConfigResponse with credential status indicators
// but never raw credential values.
func (s *Service) GetMaskedConfig(ctx context.Context, projectID string) (ConfigResponse, error) {
	cfg, err := s.GetConfig(ctx, projectID)
	if err != nil {
		return ConfigResponse{}, err
	}
	return MaskedConfigResponse(cfg), nil
}

func (s *Service) UpdateConfig(ctx context.Context, projectID string, cfg Config) error {
	proj, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	if proj == nil {
		return project.ErrProjectNotFound
	}

	// If credentials are provided but the token looks masked (contains "***"),
	// preserve the existing credentials to avoid overwriting real values with masks.
	if cfg.Credentials != nil && strings.Contains(cfg.Credentials.FlyIOAPIToken, "***") {
		existing, _ := ParseConfig(proj.ContextPack.Extra)
		if existing.Credentials != nil {
			cfg.Credentials.FlyIOAPIToken = existing.Credentials.FlyIOAPIToken
		}
	}

	pack, err := StoreConfig(proj.ContextPack, cfg)
	if err != nil {
		return err
	}
	return s.projects.UpdateContextPack(ctx, projectID, pack)
}

// resolveProvider returns a PipelineProvider for the given config.
// For Fly.io, it creates a per-project provider with the resolved token
// (per-project first, then fallback to server-wide env var).
func (s *Service) resolveProvider(cfg Config) PipelineProvider {
	providerName := strings.ToLower(strings.TrimSpace(cfg.Infrastructure.Provider))
	if providerName == "" {
		return nil
	}
	// For flyio, create a per-request provider with the resolved token.
	if providerName == "flyio" {
		token := ResolveFlyIOToken(cfg, s.flyIOFallbackToken)
		if token == "" {
			return nil
		}
		return NewFlyIOPipelineProvider(token, s.flyIOBaseURL)
	}
	return s.pipelineProviders[providerName]
}

func (s *Service) ListOpenPullRequests(ctx context.Context, projectID string) (*PullRequestOverview, error) {
	proj, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	overview := &PullRequestOverview{
		ProjectID:    projectID,
		Status:       StatusUnconfigured,
		PullRequests: []PullRequest{},
	}
	if proj == nil {
		return overview, project.ErrProjectNotFound
	}
	if strings.TrimSpace(proj.RepoURL) == "" {
		overview.Message = "Repository URL is not configured."
		return overview, nil
	}
	for _, provider := range s.pullRequestProviders {
		if !provider.Supports(proj.RepoURL) {
			continue
		}
		repo, prs, err := provider.ListOpenPullRequests(ctx, proj.RepoURL)
		overview.Provider = provider.Name()
		overview.Repository = repo
		if err != nil {
			overview.Status = StatusDegraded
			overview.Message = err.Error()
			return overview, nil
		}
		slices.SortFunc(prs, func(a, b PullRequest) int {
			switch {
			case a.UpdatedAt.After(b.UpdatedAt):
				return -1
			case a.UpdatedAt.Before(b.UpdatedAt):
				return 1
			default:
				return a.Number - b.Number
			}
		})
		overview.Status = StatusOK
		overview.PullRequests = prs
		return overview, nil
	}
	overview.Status = StatusUnsupported
	overview.Message = "No pull request provider is registered for this repository."
	return overview, nil
}

func (s *Service) Pipeline(ctx context.Context, projectID string, refresh bool) (*PipelineOverview, error) {
	proj, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if proj == nil {
		return nil, project.ErrProjectNotFound
	}
	envs, err := s.environments.ListEnvironments(ctx, projectID)
	if err != nil {
		return nil, err
	}
	cfg, cfgErr := ParseConfig(proj.ContextPack.Extra)
	overview := &PipelineOverview{
		ProjectID:    projectID,
		Status:       StatusUnconfigured,
		Environments: makePipelineEnvironments(envs),
	}
	if cfgErr != nil {
		overview.Status = StatusDegraded
		overview.Message = fmt.Sprintf("delivery_config is invalid: %v", cfgErr)
	}

	var syncedResources []*stateindex.ObservedResource
	if refresh && strings.TrimSpace(cfg.Infrastructure.Provider) != "" {
		provider := s.resolveProvider(cfg)
		if provider == nil {
			overview.Status = StatusUnsupported
			overview.Provider = cfg.Infrastructure.Provider
			overview.Message = "The configured infrastructure provider is not available on this server."
		} else {
			overview.Provider = provider.Name()
			result, err := provider.Sync(ctx, proj, envs, cfg)
			if err != nil {
				overview.Status = StatusDegraded
				overview.Message = err.Error()
			} else {
				if !result.RefreshedAt.IsZero() {
					refreshedAt := result.RefreshedAt
					overview.RefreshedAt = &refreshedAt
				}
				for _, dep := range result.Deployments {
					if s.catalog != nil && dep != nil {
						_ = s.catalog.UpsertDeployment(ctx, dep)
					}
				}
				for _, resource := range result.Resources {
					if s.state != nil && resource != nil {
						_, _ = s.state.ObserveResource(ctx, resource)
					}
				}
				syncedResources = result.Resources
				switch {
				case len(result.Warnings) > 0:
					overview.Status = StatusDegraded
					overview.Message = strings.Join(result.Warnings, " ")
				default:
					overview.Status = StatusOK
				}
			}
		}
	}

	if strings.TrimSpace(cfg.Infrastructure.Provider) == "" && overview.Message == "" {
		overview.Message = "No infrastructure integration is configured."
	}

	deployments := []*catalog.DeploymentEntry{}
	if s.catalog != nil {
		deployments, err = s.catalog.DeploymentMatrix(ctx, projectID, "")
		if err != nil {
			return nil, err
		}
	}
	resources := syncedResources
	if s.state != nil {
		observed, err := s.state.QueryResources(ctx, stateindex.ResourceQuery{
			ProjectID: projectID,
			Limit:     500,
		})
		if err != nil {
			return nil, err
		}
		if len(observed) > 0 {
			resources = observed
		}
	}
	attachDeployments(overview.Environments, deployments)
	attachResources(overview.Environments, resources)

	if overview.Status == StatusUnconfigured && hasPipelineData(overview.Environments) {
		overview.Message = "Showing stored deployment and infrastructure data."
	}
	return overview, nil
}

func makePipelineEnvironments(envs []*environment.Environment) []PipelineEnvironment {
	out := make([]PipelineEnvironment, 0, len(envs))
	for _, env := range envs {
		if env == nil {
			continue
		}
		out = append(out, PipelineEnvironment{
			ID:              env.ID,
			Name:            env.Name,
			Slug:            env.Slug,
			Infrastructure:  string(env.Infrastructure),
			DataTenancy:     string(env.DataTenancy),
			IntegrationMode: string(env.IntegrationMode),
			IsDefault:       env.IsDefault,
			Deployments:     []PipelineDeployment{},
			Resources:       []PipelineResource{},
		})
	}
	slices.SortFunc(out, func(a, b PipelineEnvironment) int {
		if a.IsDefault != b.IsDefault {
			if a.IsDefault {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

func attachDeployments(envs []PipelineEnvironment, deployments []*catalog.DeploymentEntry) {
	index := make(map[string]int, len(envs))
	for i := range envs {
		index[envs[i].ID] = i
		if envs[i].Slug != "" {
			index[envs[i].Slug] = i
		}
		if envs[i].Name != "" {
			index[envs[i].Name] = i
		}
	}
	for _, dep := range deployments {
		if dep == nil {
			continue
		}
		idx, ok := index[dep.EnvironmentID]
		if !ok {
			idx, ok = index[dep.EnvName]
		}
		if !ok {
			continue
		}
		envs[idx].Deployments = append(envs[idx].Deployments, PipelineDeployment{
			Service:    dep.ServiceName,
			Version:    dep.Version,
			Source:     string(dep.Source),
			ObservedAt: dep.ObservedAt,
		})
	}
	for i := range envs {
		slices.SortFunc(envs[i].Deployments, func(a, b PipelineDeployment) int {
			if a.Service == b.Service {
				switch {
				case a.ObservedAt.After(b.ObservedAt):
					return -1
				case a.ObservedAt.Before(b.ObservedAt):
					return 1
				default:
					return strings.Compare(a.Version, b.Version)
				}
			}
			return strings.Compare(a.Service, b.Service)
		})
	}
}

func attachResources(envs []PipelineEnvironment, resources []*stateindex.ObservedResource) {
	index := make(map[string]int, len(envs))
	for i := range envs {
		if envs[i].Slug != "" {
			index[envs[i].Slug] = i
		}
		if envs[i].Name != "" {
			index[envs[i].Name] = i
		}
	}
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		idx, ok := index[resource.Environment]
		if !ok {
			continue
		}
		envs[idx].Resources = append(envs[idx].Resources, PipelineResource{
			ID:         resource.ID,
			Name:       resource.Name,
			Type:       string(resource.ResourceType),
			Provider:   resource.Provider,
			Region:     resource.Region,
			Status:     stringProperty(resource.Properties, "state", "status"),
			Version:    stringProperty(resource.Properties, "image", "release_version"),
			ObservedAt: resource.ObservedAt,
		})
	}
	for i := range envs {
		slices.SortFunc(envs[i].Resources, func(a, b PipelineResource) int {
			if a.Type == b.Type {
				return strings.Compare(a.Name, b.Name)
			}
			return strings.Compare(a.Type, b.Type)
		})
	}
}

func stringProperty(props map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := props[key]; ok {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				return text
			}
		}
	}
	return ""
}

func hasPipelineData(envs []PipelineEnvironment) bool {
	for _, env := range envs {
		if len(env.Deployments) > 0 || len(env.Resources) > 0 {
			return true
		}
	}
	return false
}
