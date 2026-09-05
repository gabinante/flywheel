package ticket

import (
	"context"
	"github.com/gabinante/flywheel/db"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StateTransition represents a recorded state change for a ticket.
type StateTransition struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	FromState string    `json:"from_state"`
	ToState   string    `json:"to_state"`
	Trigger   string    `json:"trigger"`
	ActorID   string    `json:"actor_id"`
	ActorType string    `json:"actor_type"`
	CreatedAt time.Time `json:"created_at"`
}

// TransitionStore persists and queries state transitions.
type TransitionStore struct {
	pool *pgxpool.Pool
}

// NewTransitionStore returns a new TransitionStore.
func NewTransitionStore(pool *pgxpool.Pool) *TransitionStore {
	return &TransitionStore{pool: pool}
}

// Record inserts a state transition record.
func (s *TransitionStore) Record(ctx context.Context, ticketID string, fromState, toState State, trigger string, actor Actor) error {
	_, err := db.Executor(ctx, s.pool).Exec(ctx,
		`INSERT INTO state_transitions (ticket_id, from_state, to_state, trigger, actor_id, actor_type)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		ticketID, string(fromState), string(toState), trigger, actor.ID, string(actor.Type))
	return err
}

// ListByTicket returns all transitions for a ticket ordered by created_at.
func (s *TransitionStore) ListByTicket(ctx context.Context, ticketID string) ([]StateTransition, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ticket_id, from_state, to_state, trigger, actor_id, actor_type, created_at
		 FROM state_transitions WHERE ticket_id = $1 ORDER BY created_at ASC`,
		ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []StateTransition
	for rows.Next() {
		var t StateTransition
		if err := rows.Scan(&t.ID, &t.TicketID, &t.FromState, &t.ToState, &t.Trigger, &t.ActorID, &t.ActorType, &t.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}
