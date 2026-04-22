package queue

import (
	"context"
	"time"
)

// LeaseStore manages lease data for the ticket queue.
// *RedisStore implements this for production; embedded mode uses miniredis.
type LeaseStore interface {
	TTL() time.Duration
	CreateLease(ctx context.Context, ticketID, agentID string) (token string, expiresAt time.Time, err error)
	GetLease(ctx context.Context, ticketID string) (*LeaseData, error)
	ValidateToken(ctx context.Context, ticketID, token string) (*LeaseData, error)
	RenewLease(ctx context.Context, ticketID, token string, extend time.Duration) (newExpiresAt time.Time, err error)
	ReleaseLease(ctx context.Context, ticketID, token string) error
	GetExpiredLeaseTicketIDs(ctx context.Context, limit int64) ([]string, error)
	RemoveExpired(ctx context.Context, ticketID string) error
	SetClaimIdempotency(ctx context.Context, projectID, agentID, idempotencyKey, ticketID string) error
	GetClaimIdempotencyTicketID(ctx context.Context, projectID, agentID, idempotencyKey string) (string, error)
}
