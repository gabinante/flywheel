package workflow

import (
	"context"
	"errors"
	"fmt"
	"github.com/gabinante/flywheel/db"
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
	tag, err := db.Executor(ctx, s.pool).Exec(ctx,
		`INSERT INTO workflow_callback_tokens (token, ticket_id, workflow_id, phase_id, expires_at,workflow_version,phase_entered_at)
 SELECT $1,$2,$3,$4,$5,workflow_version,workflow_phase_entered_at FROM tickets WHERE id=$2 AND workflow_id=$3 AND workflow_phase=$4`,
		token, ticketID, workflowID, phaseID, expiresAt)
	if err == nil && tag.RowsAffected() != 1 {
		return fmt.Errorf("phase is no longer current")
	}
	return err
}

func (s *PostgresCallbackStore) LookupCallbackToken(ctx context.Context, token string) (ticketID, workflowID, phaseID string, err error) {
	err = db.Executor(ctx, s.pool).QueryRow(ctx,
		`SELECT c.ticket_id, c.workflow_id, c.phase_id FROM workflow_callback_tokens c JOIN tickets t ON t.id=c.ticket_id
 WHERE c.token = $1 AND c.expires_at > now() AND c.phase_id=t.workflow_phase AND c.workflow_version=t.workflow_version AND c.phase_entered_at=t.workflow_phase_entered_at`, token).
		Scan(&ticketID, &workflowID, &phaseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", fmt.Errorf("callback token not found or expired")
	}
	return
}

func (s *PostgresCallbackStore) DeleteCallbackToken(ctx context.Context, token string) error {
	_, err := db.Executor(ctx, s.pool).Exec(ctx, `DELETE FROM workflow_callback_tokens WHERE token = $1`, token)
	return err
}

// DeleteExpired removes expired callback tokens. Called from the reconcile loop.
func (s *PostgresCallbackStore) DeleteExpired(ctx context.Context) error {
	_, err := db.Executor(ctx, s.pool).Exec(ctx, `DELETE FROM workflow_callback_tokens WHERE expires_at <= now()`)
	return err
}

func (s *PostgresCallbackStore) Transaction(ctx context.Context, fn func(context.Context) error) error {
	return db.Transaction(ctx, s.pool, fn)
}
func (s *PostgresCallbackStore) Attempt(ctx context.Context, token string) (int, string, error) {
	var version int
	var entered time.Time
	err := db.Executor(ctx, s.pool).QueryRow(ctx, "SELECT workflow_version,phase_entered_at FROM workflow_callback_tokens WHERE token=$1 FOR UPDATE", token).Scan(&version, &entered)
	return version, entered.Format(time.RFC3339Nano), err
}
