package claims

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a claim or conflict is not found.
var ErrNotFound = errors.New("claims: not found")

// Store provides persistence for the claims registry.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new claims store backed by Postgres.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateClaim persists a new claim.
func (s *Store) CreateClaim(ctx context.Context, c *Claim) error {
	metaJSON, err := json.Marshal(c.Metadata)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO claims (id, ticket_id, entity_id, environment, claim_type, state, metadata, claimed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		c.ID, c.TicketID, c.EntityID, c.Environment, string(c.ClaimType), string(c.State), metaJSON, c.ClaimedAt,
	)
	return err
}

// GetClaim returns a claim by ID.
func (s *Store) GetClaim(ctx context.Context, id string) (*Claim, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, ticket_id, entity_id, environment, claim_type, state, metadata, claimed_at, released_at
		 FROM claims WHERE id = $1`, id)
	return scanClaim(row)
}

// GetActiveByEntity returns all active claims for a given entity+environment.
func (s *Store) GetActiveByEntity(ctx context.Context, entityID, environment string) ([]*Claim, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ticket_id, entity_id, environment, claim_type, state, metadata, claimed_at, released_at
		 FROM claims WHERE entity_id = $1 AND environment = $2 AND state = 'active'`,
		entityID, environment)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanClaims(rows)
}

// GetActiveByTicket returns all active claims for a ticket.
func (s *Store) GetActiveByTicket(ctx context.Context, ticketID string) ([]*Claim, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ticket_id, entity_id, environment, claim_type, state, metadata, claimed_at, released_at
		 FROM claims WHERE ticket_id = $1 AND state = 'active'`,
		ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanClaims(rows)
}

// GetActiveByEnvironment returns all active claims in an environment.
func (s *Store) GetActiveByEnvironment(ctx context.Context, environment string) ([]*Claim, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ticket_id, entity_id, environment, claim_type, state, metadata, claimed_at, released_at
		 FROM claims WHERE environment = $1 AND state = 'active'`,
		environment)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanClaims(rows)
}

// ReleaseClaim marks a claim as released.
func (s *Store) ReleaseClaim(ctx context.Context, id string, releasedAt time.Time) error {
	cmd, err := s.pool.Exec(ctx,
		`UPDATE claims SET state = 'released', released_at = $2 WHERE id = $1 AND state = 'active'`,
		id, releasedAt)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReleaseByTicket releases all active claims for a ticket.
func (s *Store) ReleaseByTicket(ctx context.Context, ticketID string, releasedAt time.Time) (int, error) {
	cmd, err := s.pool.Exec(ctx,
		`UPDATE claims SET state = 'released', released_at = $2 WHERE ticket_id = $1 AND state = 'active'`,
		ticketID, releasedAt)
	if err != nil {
		return 0, err
	}
	return int(cmd.RowsAffected()), nil
}

// CreateConflict persists a detected conflict.
func (s *Store) CreateConflict(ctx context.Context, c *Conflict) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO claim_conflicts (id, ticket_id, blocking_ticket, claim_id, conflict_type, severity, detected_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		c.ID, c.TicketID, c.BlockingTicket, c.ClaimID, string(c.ConflictType), string(c.Severity), c.DetectedAt,
	)
	return err
}

// GetUnresolvedConflicts returns unresolved conflicts for a ticket.
func (s *Store) GetUnresolvedConflicts(ctx context.Context, ticketID string) ([]*Conflict, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ticket_id, blocking_ticket, claim_id, conflict_type, severity, detected_at, resolved_at
		 FROM claim_conflicts WHERE ticket_id = $1 AND resolved_at IS NULL`,
		ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConflicts(rows)
}

// ResolveConflictsByTicket resolves all conflicts where the blocking ticket completes.
func (s *Store) ResolveConflictsByTicket(ctx context.Context, blockingTicketID string, resolvedAt time.Time) (int, error) {
	cmd, err := s.pool.Exec(ctx,
		`UPDATE claim_conflicts SET resolved_at = $2 WHERE blocking_ticket = $1 AND resolved_at IS NULL`,
		blockingTicketID, resolvedAt)
	if err != nil {
		return 0, err
	}
	return int(cmd.RowsAffected()), nil
}

// --- Scan helpers ---

func scanClaim(row pgx.Row) (*Claim, error) {
	var c Claim
	var metaJSON []byte
	var claimType, state string
	err := row.Scan(&c.ID, &c.TicketID, &c.EntityID, &c.Environment, &claimType, &state, &metaJSON, &c.ClaimedAt, &c.ReleasedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.ClaimType = ClaimType(claimType)
	c.State = ClaimState(state)
	if len(metaJSON) > 0 {
		_ = json.Unmarshal(metaJSON, &c.Metadata)
	}
	return &c, nil
}

func scanClaims(rows pgx.Rows) ([]*Claim, error) {
	var result []*Claim
	for rows.Next() {
		var c Claim
		var metaJSON []byte
		var claimType, state string
		if err := rows.Scan(&c.ID, &c.TicketID, &c.EntityID, &c.Environment, &claimType, &state, &metaJSON, &c.ClaimedAt, &c.ReleasedAt); err != nil {
			return nil, err
		}
		c.ClaimType = ClaimType(claimType)
		c.State = ClaimState(state)
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &c.Metadata)
		}
		result = append(result, &c)
	}
	return result, rows.Err()
}

func scanConflicts(rows pgx.Rows) ([]*Conflict, error) {
	var result []*Conflict
	for rows.Next() {
		var c Conflict
		var conflictType, severity string
		if err := rows.Scan(&c.ID, &c.TicketID, &c.BlockingTicket, &c.ClaimID, &conflictType, &severity, &c.DetectedAt, &c.ResolvedAt); err != nil {
			return nil, err
		}
		c.ConflictType = ConflictType(conflictType)
		c.Severity = Severity(severity)
		result = append(result, &c)
	}
	return result, rows.Err()
}
