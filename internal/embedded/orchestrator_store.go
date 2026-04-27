package embedded

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"time"

	"github.com/gabinante/flywheel/internal/orchestrator"
)

type OrchestratorStore struct{ db *sql.DB }

func NewOrchestratorStore(db *sql.DB) *OrchestratorStore { return &OrchestratorStore{db: db} }

func (s *OrchestratorStore) CreateMessage(_ context.Context, msg *orchestrator.Message) error {
	_, err := s.db.Exec(
		`INSERT INTO orchestrator_messages (id, project_id, role, content, created_at) VALUES (?, ?, ?, ?, ?)`,
		msg.ID, msg.ProjectID, string(msg.Role), msg.Content, msg.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *OrchestratorStore) ListMessagesByProjectID(_ context.Context, projectID string, limit int) ([]orchestrator.Message, error) {
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

func (s *OrchestratorStore) CreateRun(_ context.Context, run *orchestrator.Run) error {
	_, err := s.db.Exec(
		`INSERT INTO orchestrator_runs (id, project_id, user_message_id, assistant_message_id, status, worker_id, worker_name, runner, driver, model, error, started_at, completed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID,
		run.ProjectID,
		run.UserMessageID,
		nullString(run.AssistantMessageID),
		string(run.Status),
		nullString(run.WorkerID),
		nullString(run.WorkerName),
		nullString(run.Runner),
		nullString(run.Driver),
		nullString(run.Model),
		nullString(run.Error),
		run.StartedAt.UTC().Format(time.RFC3339Nano),
		nullTime(run.CompletedAt),
	)
	return err
}

func (s *OrchestratorStore) UpdateRun(_ context.Context, run *orchestrator.Run) error {
	_, err := s.db.Exec(
		`UPDATE orchestrator_runs
		 SET assistant_message_id = ?,
		     status = ?,
		     worker_id = ?,
		     worker_name = ?,
		     runner = ?,
		     driver = ?,
		     model = ?,
		     error = ?,
		     completed_at = ?
		 WHERE id = ?`,
		nullString(run.AssistantMessageID),
		string(run.Status),
		nullString(run.WorkerID),
		nullString(run.WorkerName),
		nullString(run.Runner),
		nullString(run.Driver),
		nullString(run.Model),
		nullString(run.Error),
		nullTime(run.CompletedAt),
		run.ID,
	)
	return err
}

func (s *OrchestratorStore) AppendRunEvent(_ context.Context, event *orchestrator.RunEvent) error {
	payloadJSON, _ := json.Marshal(event.Payload)
	_, err := s.db.Exec(
		`INSERT INTO orchestrator_run_events (id, run_id, kind, payload, created_at) VALUES (?, ?, ?, ?, ?)`,
		event.ID, event.RunID, string(event.Kind), string(payloadJSON), event.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *OrchestratorStore) ListRunsByProjectID(_ context.Context, projectID string, limit int) ([]orchestrator.Run, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(
		`SELECT id, project_id, user_message_id, assistant_message_id, status, worker_id, worker_name, runner, driver, model, error, started_at, completed_at
		 FROM orchestrator_runs
		 WHERE project_id = ?
		 ORDER BY started_at DESC
		 LIMIT ?`,
		projectID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []orchestrator.Run
	for rows.Next() {
		var run orchestrator.Run
		var status string
		var assistantMessageID sql.NullString
		var workerID sql.NullString
		var workerName sql.NullString
		var runner sql.NullString
		var driver sql.NullString
		var model sql.NullString
		var runError sql.NullString
		var startedAt string
		var completedAt sql.NullString
		if err := rows.Scan(
			&run.ID,
			&run.ProjectID,
			&run.UserMessageID,
			&assistantMessageID,
			&status,
			&workerID,
			&workerName,
			&runner,
			&driver,
			&model,
			&runError,
			&startedAt,
			&completedAt,
		); err != nil {
			return nil, err
		}
		run.Status = orchestrator.RunStatus(status)
		run.StartedAt, _ = time.Parse(time.RFC3339Nano, startedAt)
		if assistantMessageID.Valid {
			run.AssistantMessageID = assistantMessageID.String
		}
		if workerID.Valid {
			run.WorkerID = workerID.String
		}
		if workerName.Valid {
			run.WorkerName = workerName.String
		}
		if runner.Valid {
			run.Runner = runner.String
		}
		if driver.Valid {
			run.Driver = driver.String
		}
		if model.Valid {
			run.Model = model.String
		}
		if runError.Valid {
			run.Error = runError.String
		}
		if completedAt.Valid {
			parsed, _ := time.Parse(time.RFC3339Nano, completedAt.String)
			run.CompletedAt = &parsed
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.Reverse(out)
	return out, nil
}

func (s *OrchestratorStore) ListRunEventsByRunIDs(_ context.Context, runIDs []string, limitPerRun int) (map[string][]orchestrator.RunEvent, error) {
	out := make(map[string][]orchestrator.RunEvent, len(runIDs))
	if len(runIDs) == 0 {
		return out, nil
	}
	if limitPerRun <= 0 {
		limitPerRun = 200
	}
	for _, runID := range runIDs {
		rows, err := s.db.Query(
			`SELECT id, kind, payload, created_at
			 FROM orchestrator_run_events
			 WHERE run_id = ?
			 ORDER BY created_at DESC
			 LIMIT ?`,
			runID, limitPerRun,
		)
		if err != nil {
			return nil, err
		}
		var events []orchestrator.RunEvent
		for rows.Next() {
			var event orchestrator.RunEvent
			var kind string
			var payloadJSON string
			var createdAt string
			if err := rows.Scan(&event.ID, &kind, &payloadJSON, &createdAt); err != nil {
				rows.Close()
				return nil, err
			}
			event.RunID = runID
			event.Kind = orchestrator.RunEventKind(kind)
			event.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
			event.Payload = map[string]any{}
			_ = json.Unmarshal([]byte(payloadJSON), &event.Payload)
			events = append(events, event)
		}
		rows.Close()
		slices.Reverse(events)
		out[runID] = events
	}
	return out, nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
