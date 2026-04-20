package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	oauthStatePrefix   = "flywheel:oauth:state:"
	oauthCodePrefix    = "flywheel:oauth:code:"
	oauthClientPrefix  = "flywheel:oauth:client:"
	oauthTTL           = 5 * time.Minute
	oauthClientTTL     = 365 * 24 * time.Hour // 1 year
)

// OAuthState holds data stored for the OAuth authorize redirect (CSRF + callback).
type OAuthState struct {
	RedirectURI   string `json:"redirect_uri"`
	ClientID      string `json:"client_id"`
	CodeChallenge string `json:"code_challenge,omitempty"`
	ClientState   string `json:"client_state"` // state from client to return on redirect
}

// OAuthStore stores OAuth state and one-time auth codes in Redis.
type OAuthStore struct {
	client *redis.Client
}

// NewOAuthStore creates an OAuth store backed by Redis.
func NewOAuthStore(client *redis.Client) *OAuthStore {
	return &OAuthStore{client: client}
}

// CreateState saves state and returns a new state string.
func (s *OAuthStore) CreateState(ctx context.Context, data OAuthState) (string, error) {
	if s == nil || s.client == nil {
		return "", fmt.Errorf("oauth store or redis client is nil")
	}
	state := uuid.Must(uuid.NewV7()).String()
	key := oauthStatePrefix + state
	raw, _ := json.Marshal(data)
	if err := s.client.Set(ctx, key, raw, oauthTTL).Err(); err != nil {
		return "", fmt.Errorf("oauth store set state: %w", err)
	}
	return state, nil
}

// GetAndDeleteState returns state data and removes it (one-time use).
func (s *OAuthStore) GetAndDeleteState(ctx context.Context, state string) (*OAuthState, error) {
	key := oauthStatePrefix + state
	b, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	_ = s.client.Del(ctx, key)
	var data OAuthState
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// CreateCode stores an auth code for the given agent ID and returns the code.
func (s *OAuthStore) CreateCode(ctx context.Context, agentID string) (string, error) {
	code := uuid.Must(uuid.NewV7()).String()
	key := oauthCodePrefix + code
	if err := s.client.Set(ctx, key, agentID, oauthTTL).Err(); err != nil {
		return "", fmt.Errorf("oauth store set code: %w", err)
	}
	return code, nil
}

// GetAndDeleteCode returns the agent ID for the code and removes it (one-time use).
func (s *OAuthStore) GetAndDeleteCode(ctx context.Context, code string) (string, error) {
	key := oauthCodePrefix + code
	agentID, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	_ = s.client.Del(ctx, key)
	return agentID, nil
}

// RegisterClient stores redirect_uris for a new client_id (RFC 7591 Dynamic Client Registration). Returns client_id.
func (s *OAuthStore) RegisterClient(ctx context.Context, redirectURIs []string) (string, error) {
	if len(redirectURIs) == 0 {
		return "", fmt.Errorf("redirect_uris required")
	}
	if s == nil || s.client == nil {
		return "", fmt.Errorf("oauth store or redis client is nil")
	}
	clientID := uuid.Must(uuid.NewV7()).String()
	key := oauthClientPrefix + clientID
	raw, _ := json.Marshal(redirectURIs)
	if err := s.client.Set(ctx, key, raw, oauthClientTTL).Err(); err != nil {
		return "", fmt.Errorf("oauth store set client: %w", err)
	}
	return clientID, nil
}
