package project

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gabinante/flywheel/events/hooks"
	"github.com/google/uuid"
)

// ProjectStore is the persistence interface used by Service. *Store implements it.
type ProjectStore interface {
	Create(ctx context.Context, p *Project) error
	GetByID(ctx context.Context, id string) (*Project, error)
	ListByOrgID(ctx context.Context, orgID string, statusFilter string) ([]Project, error)
	UpdateStatus(ctx context.Context, projectID, status string) error
	UpdateRepoURL(ctx context.Context, projectID, repoURL string) error
	UpdateName(ctx context.Context, projectID, name string) error
	UpdateSlug(ctx context.Context, projectID, slug string) error
	UpdateDefaultBranch(ctx context.Context, projectID, branch string) error
	UpdateContextPack(ctx context.Context, projectID string, pack ContextPack) error
	UpdateDispatchEnabled(ctx context.Context, projectID string, enabled bool) error
	UpdateDispatchConfig(ctx context.Context, projectID string, cfg DispatchConfig) error
	UpdateWebhookSecret(ctx context.Context, projectID, secret string) error
}

// Service provides project operations.
type Service struct {
	store ProjectStore
}

// NewService returns a new Service.
func NewService(store ProjectStore) *Service {
	return &Service{store: store}
}

// CreateProject creates a project under an org.
// A webhook signing secret is auto-generated for every new project.
func (s *Service) CreateProject(ctx context.Context, orgID, name, slug, repoURL string, techStack []string) (*Project, error) {
	if slug == "" {
		slug = slugify(name)
	}
	id := uuid.Must(uuid.NewV7()).String()
	webhookSecret, err := hooks.GenerateWebhookSecret()
	if err != nil {
		return nil, fmt.Errorf("create project: generate webhook secret: %w", err)
	}
	p := &Project{
		ID:              id,
		OrgID:           orgID,
		Name:            name,
		Slug:            slug,
		RepoURL:         repoURL,
		TechStack:       techStack,
		ContextPack:     ContextPack{},
		Status:          "active",
		DispatchEnabled: true,
		WebhookSecret:   webhookSecret,
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.store.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// UpdateContextPack updates the project's context pack.
func (s *Service) UpdateContextPack(ctx context.Context, projectID string, pack ContextPack) error {
	return s.store.UpdateContextPack(ctx, projectID, pack)
}

// GetProject returns a project by ID.
func (s *Service) GetProject(ctx context.Context, id string) (*Project, error) {
	return s.store.GetByID(ctx, id)
}

// ListByOrgID returns projects for an org. statusFilter: "" or "active" = active only, "closed" = closed only, "all" = all.
func (s *Service) ListByOrgID(ctx context.Context, orgID string, statusFilter string) ([]Project, error) {
	return s.store.ListByOrgID(ctx, orgID, statusFilter)
}

// UpdateStatus sets project status to "active" or "closed". Returns errProjectNotFound if project does not exist.
func (s *Service) UpdateStatus(ctx context.Context, projectID, status string) error {
	if status != "active" && status != "closed" {
		return ErrInvalidStatus
	}
	return s.store.UpdateStatus(ctx, projectID, status)
}

// UpdateRepoURL sets project repo_url. Empty string disables work streams + git integration.
func (s *Service) UpdateRepoURL(ctx context.Context, projectID, repoURL string) error {
	return s.store.UpdateRepoURL(ctx, projectID, repoURL)
}

// UpdateName sets project name.
func (s *Service) UpdateName(ctx context.Context, projectID, name string) error {
	return s.store.UpdateName(ctx, projectID, name)
}

// UpdateSlug sets project slug. If empty, slugifies the current name.
func (s *Service) UpdateSlug(ctx context.Context, projectID, slug string) error {
	if slug == "" {
		p, err := s.store.GetByID(ctx, projectID)
		if err != nil || p == nil {
			return ErrProjectNotFound
		}
		slug = slugify(p.Name)
	}
	return s.store.UpdateSlug(ctx, projectID, slug)
}

// UpdateDefaultBranch sets the branch to checkout when closing a work stream. Default "main".
func (s *Service) UpdateDefaultBranch(ctx context.Context, projectID, branch string) error {
	return s.store.UpdateDefaultBranch(ctx, projectID, branch)
}

// UpdateDispatchEnabled sets whether the dispatcher picks up tickets for this project.
func (s *Service) UpdateDispatchEnabled(ctx context.Context, projectID string, enabled bool) error {
	return s.store.UpdateDispatchEnabled(ctx, projectID, enabled)
}

// UpdateDispatchConfig stores project-specific dispatch worker and policy configuration.
func (s *Service) UpdateDispatchConfig(ctx context.Context, projectID string, cfg DispatchConfig) error {
	return s.store.UpdateDispatchConfig(ctx, projectID, cfg.Normalized())
}

// GetWebhookConfig returns the project's webhook secret (masked) for display.
// The full secret is never returned through this method; use RotateWebhookSecret
// to obtain the full secret (returned exactly once on rotation).
func (s *Service) GetWebhookConfig(ctx context.Context, projectID string) (maskedSecret string, err error) {
	p, err := s.store.GetByID(ctx, projectID)
	if err != nil {
		return "", err
	}
	return hooks.MaskSecret(p.WebhookSecret), nil
}

// RotateWebhookSecret generates a new webhook signing secret for the project.
// Returns the full new secret (caller should display it once, then discard).
func (s *Service) RotateWebhookSecret(ctx context.Context, projectID string) (newSecret string, err error) {
	secret, err := hooks.GenerateWebhookSecret()
	if err != nil {
		return "", fmt.Errorf("rotate webhook secret: %w", err)
	}
	if err := s.store.UpdateWebhookSecret(ctx, projectID, secret); err != nil {
		return "", err
	}
	return secret, nil
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "project"
	}
	return s
}
