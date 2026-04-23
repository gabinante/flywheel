package orchestrator

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Create(ctx context.Context, msg *Message) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO orchestrator_messages (id, project_id, role, content, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		msg.ID, msg.ProjectID, string(msg.Role), msg.Content, msg.CreatedAt)
	return err
}

func (s *Store) ListByProjectID(ctx context.Context, projectID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, role, content, created_at
		 FROM (
		 	SELECT id, project_id, role, content, created_at
		 	FROM orchestrator_messages
		 	WHERE project_id = $1
		 	ORDER BY created_at DESC
		 	LIMIT $2
		 ) recent
		 ORDER BY created_at ASC`,
		projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var msg Message
		var role string
		if err := rows.Scan(&msg.ID, &msg.ProjectID, &role, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, err
		}
		msg.Role = Role(role)
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (s *Store) DeleteByProjectID(ctx context.Context, projectID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM orchestrator_messages WHERE project_id = $1`, projectID)
	return err
}
