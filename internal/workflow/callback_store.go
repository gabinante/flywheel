package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCallbackStore implements CallbackStore using Redis with TTL-based expiry.
type RedisCallbackStore struct {
	client *redis.Client
	prefix string
}

// NewRedisCallbackStore creates a callback store backed by Redis.
func NewRedisCallbackStore(client *redis.Client) *RedisCallbackStore {
	return &RedisCallbackStore{client: client, prefix: "wf:cb:"}
}

func (s *RedisCallbackStore) key(token string) string {
	return s.prefix + token
}

func (s *RedisCallbackStore) StoreCallbackToken(ctx context.Context, token, ticketID, workflowID, phaseID string, expiresAt time.Time) error {
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return errors.New("callback token already expired")
	}
	val := fmt.Sprintf("%s|%s|%s", ticketID, workflowID, phaseID)
	return s.client.Set(ctx, s.key(token), val, ttl).Err()
}

func (s *RedisCallbackStore) LookupCallbackToken(ctx context.Context, token string) (ticketID, workflowID, phaseID string, err error) {
	val, err := s.client.Get(ctx, s.key(token)).Result()
	if errors.Is(err, redis.Nil) {
		return "", "", "", errors.New("callback token not found or expired")
	}
	if err != nil {
		return "", "", "", err
	}
	parts := strings.SplitN(val, "|", 3)
	if len(parts) != 3 {
		return "", "", "", errors.New("malformed callback token data")
	}
	return parts[0], parts[1], parts[2], nil
}

func (s *RedisCallbackStore) DeleteCallbackToken(ctx context.Context, token string) error {
	return s.client.Del(ctx, s.key(token)).Err()
}
