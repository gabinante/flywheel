package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresCallbackStore implements CallbackStore using the workflow_callback_tokens table.
type PostgresCallbackStore struct {
	pool *pgxpool.Pool
}

// NewPostgresCallbackStore creates a callback store backed by Postgres.
func NewPostgresCallbackStore(pool *pgxpool.Pool) *PostgresCallbackStore {
	return &PostgresCallbackStore{pool: pool}
}

func (s *PostgresCallbackStore) StoreCallbackToken(ctx context.Context, token, ticketID, workflowID, phaseID string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO workflow_callback_tokens (token, ticket_id, workflow_id, phase_id, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		token, ticketID, workflowID, phaseID, expiresAt)
	return err
}

func (s *PostgresCallbackStore) LookupCallbackToken(ctx context.Context, token string) (ticketID, workflowID, phaseID string, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT ticket_id, workflow_id, phase_id FROM workflow_callback_tokens
		 WHERE token = $1 AND expires_at > now()`, token).
		Scan(&ticketID, &workflowID, &phaseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", fmt.Errorf("callback token not found or expired")
	}
	return
}

func (s *PostgresCallbackStore) DeleteCallbackToken(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM workflow_callback_tokens WHERE token = $1`, token)
	return err
}

// DeleteExpired removes expired callback tokens. Called from the reconcile loop.
func (s *PostgresCallbackStore) DeleteExpired(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM workflow_callback_tokens WHERE expires_at <= now()`)
	return err
}
