package environment

import "context"

// EnvironmentStore is the persistence interface for environments.
// *Store (Postgres) and embedded.EnvironmentStore (SQLite) implement this.
type EnvironmentStore interface {
	Create(ctx context.Context, env *Environment) error
	GetByID(ctx context.Context, id string) (*Environment, error)
	GetBySlug(ctx context.Context, projectID, slug string) (*Environment, error)
	ListByProject(ctx context.Context, projectID string) ([]*Environment, error)
	CountByProject(ctx context.Context, projectID string) (int, error)
	Update(ctx context.Context, env *Environment) error
	Delete(ctx context.Context, id string) error
	ClearDefault(ctx context.Context, projectID string) error
}
