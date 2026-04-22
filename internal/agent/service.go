package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

// Service provides agent identity operations.
type Service struct {
	store AgentStore
}

// NewService returns a new Service. The store parameter accepts any AgentStore
// implementation (Postgres *Store, embedded SQLite, etc.).
func NewService(store AgentStore) *Service {
	return &Service{store: store}
}

// RegisterAgent creates an agent and returns it with a new API key. The key is only returned once.
func (s *Service) RegisterAgent(ctx context.Context, name string, typ Type) (*Agent, string, error) {
	id := uuid.Must(uuid.NewV7()).String()
	apiKey, err := generateAPIKey()
	if err != nil {
		return nil, "", err
	}
	a := &Agent{
		ID:        id,
		Name:      name,
		Type:      typ,
		APIKey:    apiKey,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.Create(ctx, a); err != nil {
		return nil, "", err
	}
	return a, apiKey, nil
}

// AuthenticateAgent returns the agent for the given API key, or nil if invalid.
func (s *Service) AuthenticateAgent(ctx context.Context, apiKey string) (*Agent, error) {
	return s.store.GetByAPIKey(ctx, apiKey)
}

// GetAgent returns an agent by ID.
func (s *Service) GetAgent(ctx context.Context, id string) (*Agent, error) {
	return s.store.GetByID(ctx, id)
}

func generateAPIKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "wf_" + hex.EncodeToString(b), nil
}
