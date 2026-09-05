package agent

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists agents.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create inserts an agent (api_key and user_id optional).
func (s *Store) Create(ctx context.Context, a *Agent) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agents (id, user_id, name, type, api_key, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		a.ID, nullIfEmpty(a.UserID), a.Name, string(a.Type), nullIfEmpty(a.APIKey), a.CreatedAt)
	return err
}

// GetByID returns an agent by ID.
func (s *Store) GetByID(ctx context.Context, id string) (*Agent, error) {
	var a Agent
	var apiKey, userID *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, name, type, api_key, created_at FROM agents WHERE id = $1`, id).
		Scan(&a.ID, &userID, &a.Name, &a.Type, &apiKey, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	if apiKey != nil {
		a.APIKey = *apiKey
	}
	if userID != nil {
		a.UserID = *userID
	}
	return &a, nil
}

// GetByAPIKey returns an agent by API key (for authentication).
func (s *Store) GetByAPIKey(ctx context.Context, apiKey string) (*Agent, error) {
	if apiKey == "" {
		return nil, nil
	}
	var a Agent
	var apiKeyVal, userID *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, name, type, api_key, created_at FROM agents WHERE api_key = $1`, apiKey).
		Scan(&a.ID, &userID, &a.Name, &a.Type, &apiKeyVal, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	if apiKeyVal != nil {
		a.APIKey = *apiKeyVal
	}
	if userID != nil {
		a.UserID = *userID
	}
	return &a, nil
}

// GetByUserID returns the agent linked to a local user.
func (s *Store) GetByUserID(ctx context.Context, userID string) (*Agent, error) {
	var a Agent
	var apiKey, uid *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, name, type, api_key, created_at FROM agents WHERE user_id = $1 ORDER BY (api_key IS NULL) DESC, created_at, id LIMIT 1`, userID).
		Scan(&a.ID, &uid, &a.Name, &a.Type, &apiKey, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	if apiKey != nil {
		a.APIKey = *apiKey
	}
	if uid != nil {
		a.UserID = *uid
	}
	return &a, nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
