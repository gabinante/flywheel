package delivery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/catalog"
	"github.com/gabinante/flywheel/internal/environment"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/stateindex"
)

const defaultFlyAPIBaseURL = "https://api.machines.dev"

type FlyIOPipelineProvider struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewFlyIOPipelineProvider(token, baseURL string) *FlyIOPipelineProvider {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultFlyAPIBaseURL
	}
	return &FlyIOPipelineProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *FlyIOPipelineProvider) Name() string { return "flyio" }

func (p *FlyIOPipelineProvider) Sync(ctx context.Context, proj *project.Project, envs []*environment.Environment, cfg Config) (*PipelineSyncResult, error) {
	if p.token == "" {
		return nil, fmt.Errorf("fly.io integration requires FLY_API_TOKEN")
	}
	if cfg.Infrastructure.FlyIO == nil || len(cfg.Infrastructure.FlyIO.Apps) == 0 {
		return nil, fmt.Errorf("delivery_config.infrastructure.flyio.apps must map Fly apps to environment slugs")
	}

	result := &PipelineSyncResult{
		Deployments: []*catalog.DeploymentEntry{},
		Resources:   []*stateindex.ObservedResource{},
		RefreshedAt: time.Now().UTC(),
		Warnings:    []string{},
	}

	envBySlug := make(map[string]*environment.Environment, len(envs))
	for _, env := range envs {
		if env != nil && env.Slug != "" {
			envBySlug[env.Slug] = env
		}
	}

	for _, appCfg := range cfg.Infrastructure.FlyIO.Apps {
		appName := strings.TrimSpace(appCfg.AppName)
		envSlug := strings.TrimSpace(appCfg.EnvironmentSlug)
		if appName == "" || envSlug == "" {
			result.Warnings = append(result.Warnings, "fly.io app mappings require app_name and environment_slug.")
			continue
		}
		env := envBySlug[envSlug]
		machines, err := p.listMachines(ctx, appName)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("fly.io app %s: %v", appName, err))
			continue
		}

		serviceName := strings.TrimSpace(appCfg.Service)
		if serviceName == "" {
			serviceName = appName
		}
		version := summarizeFlyVersion(machines)
		envID := envSlug
		envName := envSlug
		if env != nil {
			envID = env.ID
			envName = env.Name
		}
		result.Deployments = append(result.Deployments, &catalog.DeploymentEntry{
			ServiceID:     serviceName,
			ServiceName:   serviceName,
			EnvironmentID: envID,
			EnvName:       envName,
			Version:       version,
			Source:        catalog.SourceObserved,
			ObservedAt:    result.RefreshedAt,
		})
		for _, machine := range machines {
			result.Resources = append(result.Resources, flyMachineToResource(proj.ID, envSlug, appName, machine, result.RefreshedAt))
		}
	}
	return result, nil
}

type flyMachine struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	State      string `json:"state"`
	Region     string `json:"region"`
	InstanceID string `json:"instance_id"`
	PrivateIP  string `json:"private_ip"`
	Config     struct {
		Image string `json:"image"`
	} `json:"config"`
	ImageRef struct {
		Registry   string `json:"registry"`
		Repository string `json:"repository"`
		Tag        string `json:"tag"`
		Digest     string `json:"digest"`
	} `json:"image_ref"`
}

func (p *FlyIOPipelineProvider) listMachines(ctx context.Context, appName string) ([]flyMachine, error) {
	endpoint := fmt.Sprintf("%s/v1/apps/%s/machines", p.baseURL, url.PathEscape(appName))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("%s", message)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var machines []flyMachine
	if err := json.Unmarshal(body, &machines); err == nil {
		return machines, nil
	}
	var wrapped struct {
		Machines []flyMachine `json:"machines"`
		Data     []flyMachine `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, err
	}
	if len(wrapped.Machines) > 0 {
		return wrapped.Machines, nil
	}
	return wrapped.Data, nil
}

func summarizeFlyVersion(machines []flyMachine) string {
	if len(machines) == 0 {
		return "no-machines"
	}
	versions := make(map[string]struct{})
	for _, machine := range machines {
		version := strings.TrimSpace(machine.Config.Image)
		if version == "" {
			version = imageRefVersion(machine)
		}
		if version == "" {
			version = strings.TrimSpace(machine.InstanceID)
		}
		if version == "" {
			version = strings.TrimSpace(machine.State)
		}
		if version == "" {
			continue
		}
		versions[version] = struct{}{}
	}
	switch len(versions) {
	case 0:
		return "unknown"
	case 1:
		for version := range versions {
			return version
		}
	}
	return fmt.Sprintf("mixed (%d versions)", len(versions))
}

func imageRefVersion(machine flyMachine) string {
	if strings.TrimSpace(machine.ImageRef.Repository) == "" {
		return ""
	}
	switch {
	case strings.TrimSpace(machine.ImageRef.Tag) != "":
		return machine.ImageRef.Repository + ":" + machine.ImageRef.Tag
	case strings.TrimSpace(machine.ImageRef.Digest) != "":
		return machine.ImageRef.Repository + "@" + machine.ImageRef.Digest
	default:
		return machine.ImageRef.Repository
	}
}

func flyMachineToResource(projectID, envSlug, appName string, machine flyMachine, observedAt time.Time) *stateindex.ObservedResource {
	version := strings.TrimSpace(machine.Config.Image)
	if version == "" {
		version = imageRefVersion(machine)
	}
	props := map[string]any{
		"app_name":    appName,
		"state":       machine.State,
		"image":       version,
		"instance_id": machine.InstanceID,
	}
	if machine.PrivateIP != "" {
		props["private_ip"] = machine.PrivateIP
	}
	return &stateindex.ObservedResource{
		ID:           fmt.Sprintf("flyio:%s:%s", appName, machine.ID),
		ProjectID:    projectID,
		ResourceType: stateindex.ResourceVM,
		Environment:  envSlug,
		Name:         firstNonEmpty(machine.Name, machine.ID, appName),
		ExternalID:   machine.ID,
		Provider:     "flyio",
		Region:       machine.Region,
		Properties:   props,
		ObservedAt:   observedAt,
		Source:       "flyio",
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
