package execution

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mustUUID() string { return uuid.Must(uuid.NewV7()).String() }

// Store persists execution steps.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// AppendStep inserts a step and returns the created step ID.
func (s *Store) AppendStep(ctx context.Context, ticketID, agentID string, step Step) error {
	payloadJSON, _ := json.Marshal(step.Payload)
	if step.ID == "" {
		step.ID = mustUUID()
	}
	var agentIDValue any
	if agentID != "" {
		agentIDValue = agentID
	}
	// Use worker_type if set on the step; column is nullable for backward compat.
	var workerType *string
	if step.WorkerType != "" {
		workerType = &step.WorkerType
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO execution_steps (id, ticket_id, agent_id, type, payload, worker_type, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		step.ID, ticketID, agentIDValue, string(step.Type), payloadJSON, workerType, step.CreatedAt)
	return err
}

// GetStepsByTicketID returns all steps for a ticket in order.
func (s *Store) GetStepsByTicketID(ctx context.Context, ticketID string) ([]Step, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, type, payload, worker_type, created_at FROM execution_steps WHERE ticket_id = $1 ORDER BY created_at`,
		ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []Step
	for rows.Next() {
		var st Step
		var payloadJSON []byte
		var workerType *string
		if err := rows.Scan(&st.ID, &st.Type, &payloadJSON, &workerType, &st.CreatedAt); err != nil {
			return nil, err
		}
		st.Payload = make(map[string]any)
		_ = json.Unmarshal(payloadJSON, &st.Payload)
		if workerType != nil {
			st.WorkerType = *workerType
		}
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

// GetAgentIDByTicketID returns the agent_id for the most recent step (current worker). Used for trace summary.
func (s *Store) GetAgentIDByTicketID(ctx context.Context, ticketID string) (string, error) {
	var agentID string
	err := s.pool.QueryRow(ctx,
		`SELECT agent_id FROM execution_steps WHERE ticket_id = $1 AND COALESCE(agent_id, '') <> '' ORDER BY created_at DESC LIMIT 1`, ticketID).
		Scan(&agentID)
	return agentID, err
}
