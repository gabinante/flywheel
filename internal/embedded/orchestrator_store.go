package embedded

import (
	"context"
	"database/sql"
	"slices"
	"time"

	"github.com/gabinante/flywheel/internal/orchestrator"
)

type OrchestratorStore struct{ db *sql.DB }

func NewOrchestratorStore(db *sql.DB) *OrchestratorStore { return &OrchestratorStore{db: db} }

func (s *OrchestratorStore) Create(_ context.Context, msg *orchestrator.Message) error {
	_, err := s.db.Exec(
		`INSERT INTO orchestrator_messages (id, project_id, role, content, created_at) VALUES (?, ?, ?, ?, ?)`,
		msg.ID, msg.ProjectID, string(msg.Role), msg.Content, msg.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *OrchestratorStore) ListByProjectID(_ context.Context, projectID string, limit int) ([]orchestrator.Message, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.Query(
		`SELECT id, project_id, role, content, created_at
		 FROM orchestrator_messages
		 WHERE project_id = ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		projectID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []orchestrator.Message
	for rows.Next() {
		var msg orchestrator.Message
		var role string
		var createdAt string
		if err := rows.Scan(&msg.ID, &msg.ProjectID, &role, &msg.Content, &createdAt); err != nil {
			return nil, err
		}
		msg.Role = orchestrator.Role(role)
		msg.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.Reverse(out)
	return out, nil
}
