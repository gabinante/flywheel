package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	leaseKeyPrefix         = "flywheel:lease:"
	leaseExpiresKey        = "flywheel:lease:expires"
	leaseTTLDefault        = 10 * time.Minute
	claimIdempotencyPrefix = "flywheel:idempotency_claim:"
	claimIdempotencyTTL    = 24 * time.Hour
)

// RedisStore handles lease data in Redis (key per ticket, sorted set for expiry).
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisStore creates a Redis store for leases.
func NewRedisStore(client *redis.Client, ttl time.Duration) *RedisStore {
	if ttl <= 0 {
		ttl = leaseTTLDefault
	}
	return &RedisStore{client: client, ttl: ttl}
}

// TTL returns the default lease TTL (for renewals).
func (r *RedisStore) TTL() time.Duration { return r.ttl }

// LeaseData is the value stored in Redis for a lease.
type LeaseData struct {
	Token     string    `json:"token"`
	AgentID   string    `json:"agent_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CreateLease stores a new lease and adds to the expires sorted set.
func (r *RedisStore) CreateLease(ctx context.Context, ticketID, agentID string) (token string, expiresAt time.Time, err error) {
	token = uuid.Must(uuid.NewV7()).String()
	expiresAt = time.Now().UTC().Add(r.ttl)
	data := LeaseData{Token: token, AgentID: agentID, ExpiresAt: expiresAt}
	raw, _ := json.Marshal(data)
	key := leaseKeyPrefix + ticketID
	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, raw, r.ttl)
	pipe.ZAdd(ctx, leaseExpiresKey, redis.Z{Score: float64(expiresAt.Unix()), Member: ticketID})
	_, err = pipe.Exec(ctx)
	return token, expiresAt, err
}

// GetLease returns lease data if it exists.
func (r *RedisStore) GetLease(ctx context.Context, ticketID string) (*LeaseData, error) {
	key := leaseKeyPrefix + ticketID
	b, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var data LeaseData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// ValidateToken returns the lease data if the token matches.
func (r *RedisStore) ValidateToken(ctx context.Context, ticketID, token string) (*LeaseData, error) {
	data, err := r.GetLease(ctx, ticketID)
	if err != nil || data == nil {
		return nil, err
	}
	if data.Token != token {
		return nil, fmt.Errorf("invalid lease token")
	}
	return data, nil
}

// RenewLease extends the lease TTL and updates the expires set score. Extend is typically r.ttl.
func (r *RedisStore) RenewLease(ctx context.Context, ticketID, token string, extend time.Duration) (newExpiresAt time.Time, err error) {
	data, err := r.ValidateToken(ctx, ticketID, token)
	if err != nil || data == nil {
		return time.Time{}, err
	}
	if extend <= 0 {
		extend = r.ttl
	}
	newExpiresAt = time.Now().UTC().Add(extend)
	data.ExpiresAt = newExpiresAt
	raw, _ := json.Marshal(data)
	key := leaseKeyPrefix + ticketID
	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, raw, extend)
	pipe.ZAdd(ctx, leaseExpiresKey, redis.Z{Score: float64(newExpiresAt.Unix()), Member: ticketID})
	_, err = pipe.Exec(ctx)
	return newExpiresAt, err
}

// ReleaseLease removes the lease from Redis.
func (r *RedisStore) ReleaseLease(ctx context.Context, ticketID, token string) error {
	data, err := r.ValidateToken(ctx, ticketID, token)
	if err != nil || data == nil {
		return err
	}
	key := leaseKeyPrefix + ticketID
	pipe := r.client.Pipeline()
	pipe.Del(ctx, key)
	pipe.ZRem(ctx, leaseExpiresKey, ticketID)
	_, err = pipe.Exec(ctx)
	return err
}

// GetExpiredLeaseTicketIDs returns ticket IDs whose lease has expired (score <= now).
func (r *RedisStore) GetExpiredLeaseTicketIDs(ctx context.Context, limit int64) ([]string, error) {
	now := float64(time.Now().UTC().Unix())
	ids, err := r.client.ZRangeByScore(ctx, leaseExpiresKey, &redis.ZRangeBy{Min: "0", Max: fmt.Sprintf("%f", now), Count: limit}).Result()
	return ids, err
}

// RemoveExpired removes a ticket from the expires set and deletes its lease key (call after handling expiry).
func (r *RedisStore) RemoveExpired(ctx context.Context, ticketID string) error {
	key := leaseKeyPrefix + ticketID
	pipe := r.client.Pipeline()
	pipe.Del(ctx, key)
	pipe.ZRem(ctx, leaseExpiresKey, ticketID)
	_, err := pipe.Exec(ctx)
	return err
}

// claimIdempotencyKey returns Redis key for (projectID, agentID, idempotencyKey).
func claimIdempotencyKey(projectID, agentID, idempotencyKey string) string {
	return claimIdempotencyPrefix + projectID + ":" + agentID + ":" + idempotencyKey
}

// SetClaimIdempotency records (project_id, agent_id, idempotency_key) -> ticket_id for claim_ticket idempotency.
func (r *RedisStore) SetClaimIdempotency(ctx context.Context, projectID, agentID, idempotencyKey, ticketID string) error {
	key := claimIdempotencyKey(projectID, agentID, idempotencyKey)
	return r.client.Set(ctx, key, ticketID, claimIdempotencyTTL).Err()
}

// GetClaimIdempotencyTicketID returns the ticket_id previously claimed by this agent with this idempotency key, or "" if none.
func (r *RedisStore) GetClaimIdempotencyTicketID(ctx context.Context, projectID, agentID, idempotencyKey string) (string, error) {
	key := claimIdempotencyKey(projectID, agentID, idempotencyKey)
	s, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return s, err
}
