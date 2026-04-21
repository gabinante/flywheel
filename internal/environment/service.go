package environment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/google/uuid"
)

// Service provides environment CRUD operations with business rules.
type Service struct {
	store *Store
	bus   events.Bus
}

// NewService returns a new environment Service.
func NewService(store *Store, bus events.Bus) *Service {
	return &Service{store: store, bus: bus}
}

// CreateEnvironment creates a new environment for a project.
// The first environment for a project is automatically marked as default.
func (s *Service) CreateEnvironment(ctx context.Context, projectID string, name string, slug string, infra Infrastructure, tenancy DataTenancy, mode IntegrationMode) (*Environment, error) {
	if slug == "" {
		slug = slugify(name)
	}

	env := &Environment{
		ID:              uuid.New().String(),
		ProjectID:       projectID,
		Name:            name,
		Slug:            slug,
		Infrastructure:  infra,
		DataTenancy:     tenancy,
		IntegrationMode: mode,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	if err := env.Validate(); err != nil {
		return nil, err
	}

	// Check for duplicate slug in project.
	existing, err := s.store.GetBySlug(ctx, projectID, slug)
	if err != nil && err != ErrNotFound {
		return nil, err
	}
	if existing != nil {
		return nil, ErrDuplicateSlug
	}

	// If this is the first environment, mark it as default.
	count, err := s.store.CountByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		env.IsDefault = true
	}

	if err := s.store.Create(ctx, env); err != nil {
		return nil, err
	}

	_ = s.bus.Publish(ctx, events.Event{
		Type:    events.EventEnvironmentCreated,
		Payload: map[string]any{"environment_id": env.ID, "project_id": projectID, "slug": env.Slug},
	})

	return env, nil
}

// GetEnvironment returns an environment by ID.
func (s *Service) GetEnvironment(ctx context.Context, id string) (*Environment, error) {
	return s.store.GetByID(ctx, id)
}

// GetEnvironmentBySlug returns an environment by project ID and slug.
func (s *Service) GetEnvironmentBySlug(ctx context.Context, projectID, slug string) (*Environment, error) {
	return s.store.GetBySlug(ctx, projectID, slug)
}

// ListEnvironments returns all environments for a project.
func (s *Service) ListEnvironments(ctx context.Context, projectID string) ([]*Environment, error) {
	return s.store.ListByProject(ctx, projectID)
}

// CountEnvironments returns the number of environments for a project.
func (s *Service) CountEnvironments(ctx context.Context, projectID string) (int, error) {
	return s.store.CountByProject(ctx, projectID)
}

// UpdateEnvironment updates an existing environment. Validates the compound tuple.
func (s *Service) UpdateEnvironment(ctx context.Context, id string, name string, slug string, infra Infrastructure, tenancy DataTenancy, mode IntegrationMode) (*Environment, error) {
	env, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if name != "" {
		env.Name = name
	}
	if slug != "" {
		env.Slug = slug
	}
	if infra != "" {
		env.Infrastructure = infra
	}
	if tenancy != "" {
		env.DataTenancy = tenancy
	}
	if mode != "" {
		env.IntegrationMode = mode
	}

	if err := env.Validate(); err != nil {
		return nil, err
	}

	// Check slug uniqueness if changed.
	existing, err := s.store.GetBySlug(ctx, env.ProjectID, env.Slug)
	if err != nil && err != ErrNotFound {
		return nil, err
	}
	if existing != nil && existing.ID != env.ID {
		return nil, ErrDuplicateSlug
	}

	if err := s.store.Update(ctx, env); err != nil {
		return nil, err
	}

	_ = s.bus.Publish(ctx, events.Event{
		Type:    events.EventEnvironmentUpdated,
		Payload: map[string]any{"environment_id": env.ID, "project_id": env.ProjectID},
	})

	return env, nil
}

// SetDefault sets an environment as the default for its project (unsets previous default).
func (s *Service) SetDefault(ctx context.Context, id string) error {
	env, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.store.ClearDefault(ctx, env.ProjectID); err != nil {
		return err
	}

	env.IsDefault = true
	return s.store.Update(ctx, env)
}

// DeleteEnvironment deletes an environment. Enforces minimum-two constraint and
// disallows deleting the default environment.
func (s *Service) DeleteEnvironment(ctx context.Context, id string) error {
	env, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if env.IsDefault {
		return ErrCannotDeleteDefault
	}

	count, err := s.store.CountByProject(ctx, env.ProjectID)
	if err != nil {
		return err
	}
	if count <= 2 {
		return ErrLastEnvironment
	}

	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}

	_ = s.bus.Publish(ctx, events.Event{
		Type:    events.EventEnvironmentDeleted,
		Payload: map[string]any{"environment_id": id, "project_id": env.ProjectID},
	})

	return nil
}

// EnsureMinimumEnvironments checks that a project has at least 2 environments.
// Returns an error if the constraint is violated.
func (s *Service) EnsureMinimumEnvironments(ctx context.Context, projectID string) error {
	count, err := s.store.CountByProject(ctx, projectID)
	if err != nil {
		return err
	}
	if count < 2 {
		return fmt.Errorf("%w: project has %d, needs at least 2", ErrMinimumEnvironments, count)
	}
	return nil
}

// ProvisionDefaults creates two default environments for a new project:
// dev (dev/synthetic/sandbox) and staging (staging/anonymized/test).
func (s *Service) ProvisionDefaults(ctx context.Context, projectID string) ([]*Environment, error) {
	dev, err := s.CreateEnvironment(ctx, projectID, "Development", "dev",
		InfraDev, DataSynthetic, IntegrationSandbox)
	if err != nil {
		return nil, fmt.Errorf("creating dev environment: %w", err)
	}

	staging, err := s.CreateEnvironment(ctx, projectID, "Staging", "staging",
		InfraStaging, DataAnonymized, IntegrationTest)
	if err != nil {
		return nil, fmt.Errorf("creating staging environment: %w", err)
	}

	return []*Environment{dev, staging}, nil
}

// slugify creates a URL-safe slug from a name.
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	// Remove non-alphanumeric characters (except hyphens).
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteRune(c)
		}
	}
	return b.String()
}
