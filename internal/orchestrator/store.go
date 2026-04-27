package orchestrator

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) CreateMessage(ctx context.Context, msg *Message) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO orchestrator_messages (id, project_id, role, content, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		msg.ID, msg.ProjectID, string(msg.Role), msg.Content, msg.CreatedAt)
	return err
}

func (s *Store) ListMessagesByProjectID(ctx context.Context, projectID string, limit int) ([]Message, error) {
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

func (s *Store) CreateRun(ctx context.Context, run *Run) error {
	var assistantMessageID any
	if run.AssistantMessageID != "" {
		assistantMessageID = run.AssistantMessageID
	}
	var workerID any
	if run.WorkerID != "" {
		workerID = run.WorkerID
	}
	var workerName any
	if run.WorkerName != "" {
		workerName = run.WorkerName
	}
	var runner any
	if run.Runner != "" {
		runner = run.Runner
	}
	var driver any
	if run.Driver != "" {
		driver = run.Driver
	}
	var model any
	if run.Model != "" {
		model = run.Model
	}
	var runError any
	if run.Error != "" {
		runError = run.Error
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO orchestrator_runs
		 (id, project_id, user_message_id, assistant_message_id, status, worker_id, worker_name, runner, driver, model, error, started_at, completed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		run.ID, run.ProjectID, run.UserMessageID, assistantMessageID, string(run.Status), workerID, workerName, runner, driver, model, runError, run.StartedAt, run.CompletedAt)
	return err
}

func (s *Store) UpdateRun(ctx context.Context, run *Run) error {
	var assistantMessageID any
	if run.AssistantMessageID != "" {
		assistantMessageID = run.AssistantMessageID
	}
	var workerID any
	if run.WorkerID != "" {
		workerID = run.WorkerID
	}
	var workerName any
	if run.WorkerName != "" {
		workerName = run.WorkerName
	}
	var runner any
	if run.Runner != "" {
		runner = run.Runner
	}
	var driver any
	if run.Driver != "" {
		driver = run.Driver
	}
	var model any
	if run.Model != "" {
		model = run.Model
	}
	var runError any
	if run.Error != "" {
		runError = run.Error
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE orchestrator_runs
		 SET assistant_message_id = $2,
		     status = $3,
		     worker_id = $4,
		     worker_name = $5,
		     runner = $6,
		     driver = $7,
		     model = $8,
		     error = $9,
		     completed_at = $10
		 WHERE id = $1`,
		run.ID, assistantMessageID, string(run.Status), workerID, workerName, runner, driver, model, runError, run.CompletedAt)
	return err
}

func (s *Store) AppendRunEvent(ctx context.Context, event *RunEvent) error {
	payloadJSON, _ := json.Marshal(event.Payload)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO orchestrator_run_events (id, run_id, kind, payload, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		event.ID, event.RunID, string(event.Kind), payloadJSON, event.CreatedAt)
	return err
}

func (s *Store) ListRunsByProjectID(ctx context.Context, projectID string, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, user_message_id, assistant_message_id, status, worker_id, worker_name, runner, driver, model, error, started_at, completed_at
		 FROM (
		 	SELECT id, project_id, user_message_id, assistant_message_id, status, worker_id, worker_name, runner, driver, model, error, started_at, completed_at
		 	FROM orchestrator_runs
		 	WHERE project_id = $1
		 	ORDER BY started_at DESC
		 	LIMIT $2
		 ) recent
		 ORDER BY started_at ASC`,
		projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Run
	for rows.Next() {
		var run Run
		var status string
		var assistantMessageID *string
		var workerID *string
		var workerName *string
		var runner *string
		var driver *string
		var model *string
		var runError *string
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
			&run.StartedAt,
			&run.CompletedAt,
		); err != nil {
			return nil, err
		}
		run.Status = RunStatus(status)
		if assistantMessageID != nil {
			run.AssistantMessageID = *assistantMessageID
		}
		if workerID != nil {
			run.WorkerID = *workerID
		}
		if workerName != nil {
			run.WorkerName = *workerName
		}
		if runner != nil {
			run.Runner = *runner
		}
		if driver != nil {
			run.Driver = *driver
		}
		if model != nil {
			run.Model = *model
		}
		if runError != nil {
			run.Error = *runError
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (s *Store) ListRunEventsByRunIDs(ctx context.Context, runIDs []string, limitPerRun int) (map[string][]RunEvent, error) {
	out := make(map[string][]RunEvent, len(runIDs))
	if len(runIDs) == 0 {
		return out, nil
	}
	if limitPerRun <= 0 {
		limitPerRun = 200
	}
	rows, err := s.pool.Query(ctx,
		`SELECT run_id, id, kind, payload, created_at
		 FROM (
		 	SELECT run_id, id, kind, payload, created_at,
		 	       row_number() OVER (PARTITION BY run_id ORDER BY created_at DESC) AS rn
		 	FROM orchestrator_run_events
		 	WHERE run_id = ANY($1)
		 ) recent
		 WHERE rn <= $2
		 ORDER BY created_at ASC`,
		runIDs, limitPerRun)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var event RunEvent
		var kind string
		var payloadJSON []byte
		var runID string
		if err := rows.Scan(&runID, &event.ID, &kind, &payloadJSON, &event.CreatedAt); err != nil {
			return nil, err
		}
		event.RunID = runID
		event.Kind = RunEventKind(kind)
		event.Payload = map[string]any{}
		_ = json.Unmarshal(payloadJSON, &event.Payload)
		out[runID] = append(out[runID], event)
	}
	return out, rows.Err()
}

func (s *Store) DeleteByProjectID(ctx context.Context, projectID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM orchestrator_runs WHERE project_id = $1`, projectID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM orchestrator_messages WHERE project_id = $1`, projectID)
	return err
}
