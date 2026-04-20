package agent

import "context"

// AgentStore is the persistence interface for agents.
// *Store (Postgres) and embedded.AgentStore (SQLite) implement this.
type AgentStore interface {
	Create(ctx context.Context, a *Agent) error
	GetByID(ctx context.Context, id string) (*Agent, error)
	GetByAPIKey(ctx context.Context, apiKey string) (*Agent, error)
	GetByUserID(ctx context.Context, userID string) (*Agent, error)
}
