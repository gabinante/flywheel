package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gabinante/flywheel/internal/dispatch"
	"github.com/gabinante/flywheel/internal/project"
)

type mockStore struct {
	messages []Message
	runs     []Run
	events   []RunEvent
}

func (m *mockStore) CreateMessage(_ context.Context, msg *Message) error {
	m.messages = append(m.messages, *msg)
	return nil
}

func (m *mockStore) ListMessagesByProjectID(_ context.Context, projectID string, limit int) ([]Message, error) {
	var out []Message
	for _, msg := range m.messages {
		if msg.ProjectID == projectID {
			out = append(out, msg)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (m *mockStore) CreateRun(_ context.Context, run *Run) error {
	m.runs = append(m.runs, *run)
	return nil
}

func (m *mockStore) UpdateRun(_ context.Context, run *Run) error {
	for index := range m.runs {
		if m.runs[index].ID == run.ID {
			m.runs[index] = *run
			return nil
		}
	}
	m.runs = append(m.runs, *run)
	return nil
}

func (m *mockStore) AppendRunEvent(_ context.Context, event *RunEvent) error {
	m.events = append(m.events, *event)
	return nil
}

func (m *mockStore) ListRunsByProjectID(_ context.Context, projectID string, limit int) ([]Run, error) {
	var out []Run
	for _, run := range m.runs {
		if run.ProjectID == projectID {
			out = append(out, run)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func (m *mockStore) ListRunEventsByRunIDs(_ context.Context, runIDs []string, limitPerRun int) (map[string][]RunEvent, error) {
	allowed := make(map[string]struct{}, len(runIDs))
	for _, id := range runIDs {
		allowed[id] = struct{}{}
	}
	out := make(map[string][]RunEvent, len(runIDs))
	for _, event := range m.events {
		if _, ok := allowed[event.RunID]; !ok {
			continue
		}
		out[event.RunID] = append(out[event.RunID], event)
	}
	if limitPerRun > 0 {
		for runID, events := range out {
			if len(events) > limitPerRun {
				out[runID] = events[len(events)-limitPerRun:]
			}
		}
	}
	return out, nil
}

type mockProjectGetter struct {
	project *project.Project
	err     error
}

func (m *mockProjectGetter) GetProject(_ context.Context, id string) (*project.Project, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.project, nil
}

type mockWorker struct {
	result      *dispatch.WorkerResult
	err         error
	lastWorkDir string
	lastTask    string
	lastPrompt  string
}

func (m *mockWorker) Spawn(_ context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*dispatch.WorkerResult, error) {
	m.lastWorkDir = workDir
	m.lastTask = taskMessage
	m.lastPrompt = systemPrompt
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

type mockStreamWorker struct {
	mockWorker
	outputs []string
}

func (m *mockStreamWorker) SpawnStream(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string, onOutput dispatch.WorkerOutputHandler) (*dispatch.WorkerResult, error) {
	for _, output := range m.outputs {
		onOutput("stdout", output)
	}
	return m.Spawn(ctx, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL)
}

func TestSendUserMessageRequiresWorker(t *testing.T) {
	store := &mockStore{}
	svc := NewService(store, &mockProjectGetter{
		project: &project.Project{ID: "proj-1", RepoURL: "/tmp/repo"},
	}, nil, Config{})

	_, err := svc.SendUserMessage(context.Background(), "proj-1", "hello")
	if !errors.Is(err, ErrWorkerNotConfigured) {
		t.Fatalf("expected ErrWorkerNotConfigured, got %v", err)
	}
	if len(store.messages) != 0 {
		t.Fatalf("expected no persisted messages, got %d", len(store.messages))
	}
}

func TestSendUserMessageUsesProjectRepoWhenServiceRepoUnset(t *testing.T) {
	store := &mockStore{}
	worker := &mockWorker{
		result: &dispatch.WorkerResult{Success: true, Output: "Created two tickets."},
	}
	svc := NewService(store, &mockProjectGetter{
		project: &project.Project{ID: "proj-1", Name: "Test", RepoURL: "/tmp/project-repo"},
	}, worker, Config{ServerURL: "http://localhost:8080", AgentID: "orch"})

	thread, err := svc.SendUserMessage(context.Background(), "proj-1", "Add chat orchestration")
	if err != nil {
		t.Fatalf("SendUserMessage() error = %v", err)
	}
	if worker.lastWorkDir != "/tmp/project-repo" {
		t.Fatalf("expected worker workdir /tmp/project-repo, got %q", worker.lastWorkDir)
	}
	if len(thread.Messages) != 2 {
		t.Fatalf("expected 2 messages in thread, got %d", len(thread.Messages))
	}
	if thread.Messages[0].Role != RoleUser || thread.Messages[1].Role != RoleAssistant {
		t.Fatalf("unexpected roles: %+v", thread.Messages)
	}
	if worker.lastTask == "" || worker.lastPrompt == "" {
		t.Fatal("expected prompt and task to be populated")
	}
	if len(thread.Runs) != 1 {
		t.Fatalf("expected 1 run in thread, got %d", len(thread.Runs))
	}
	if thread.Runs[0].Status != RunStatusCompleted {
		t.Fatalf("expected completed run, got %s", thread.Runs[0].Status)
	}
}

func TestSendUserMessageStoresRunEventsFromStreamableWorker(t *testing.T) {
	store := &mockStore{}
	worker := &mockStreamWorker{
		mockWorker: mockWorker{
			result: &dispatch.WorkerResult{Success: true, Output: "Created a work stream and two tickets."},
		},
		outputs: []string{
			"inspect_project: ok",
			"create_work_stream: warrant-123",
		},
	}
	svc := NewService(store, &mockProjectGetter{
		project: &project.Project{ID: "proj-1", Name: "Test", RepoURL: "/tmp/project-repo"},
	}, worker, Config{ServerURL: "http://localhost:8080", AgentID: "orch"})

	thread, err := svc.SendUserMessage(context.Background(), "proj-1", "Plan the auth feature")
	if err != nil {
		t.Fatalf("SendUserMessage() error = %v", err)
	}
	if len(thread.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(thread.Runs))
	}
	run := thread.Runs[0]
	if run.Status != RunStatusCompleted {
		t.Fatalf("expected completed run, got %s", run.Status)
	}
	if len(run.Events) < 4 {
		t.Fatalf("expected run events to include lifecycle and worker output, got %d", len(run.Events))
	}
	foundOutput := false
	for _, event := range run.Events {
		if event.Kind == RunEventKindWorkerOutput && event.Payload["text"] == "create_work_stream: warrant-123" {
			foundOutput = true
			break
		}
	}
	if !foundOutput {
		t.Fatalf("expected streamed worker output event, got %+v", run.Events)
	}
}

func TestGetThreadInitializesEmptySlices(t *testing.T) {
	store := &mockStore{}
	svc := NewService(store, &mockProjectGetter{
		project: &project.Project{ID: "proj-1", Name: "Test"},
	}, nil, Config{})

	thread, err := svc.GetThread(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("GetThread() error = %v", err)
	}
	if thread.Messages == nil {
		t.Fatal("expected messages slice to be initialized")
	}
	if thread.Runs == nil {
		t.Fatal("expected runs slice to be initialized")
	}
}

func TestInjectSystemEventDeduplicates(t *testing.T) {
	store := &mockStore{}
	svc := NewService(store, &mockProjectGetter{}, nil, Config{})

	ctx := context.Background()
	if err := svc.InjectSystemEvent(ctx, "proj-1", "escalation", "ticket-1 needs input"); err != nil {
		t.Fatal(err)
	}
	if err := svc.InjectSystemEvent(ctx, "proj-1", "escalation", "ticket-1 needs input"); err != nil {
		t.Fatal(err)
	}

	if len(store.messages) != 1 {
		t.Fatalf("expected 1 message (deduplicated), got %d", len(store.messages))
	}
}

func TestInjectSystemEventAllowsDifferentContent(t *testing.T) {
	store := &mockStore{}
	svc := NewService(store, &mockProjectGetter{}, nil, Config{})

	ctx := context.Background()
	if err := svc.InjectSystemEvent(ctx, "proj-1", "escalation", "ticket-1 needs input"); err != nil {
		t.Fatal(err)
	}
	if err := svc.InjectSystemEvent(ctx, "proj-1", "escalation", "ticket-2 failed"); err != nil {
		t.Fatal(err)
	}

	if len(store.messages) != 2 {
		t.Fatalf("expected 2 messages (different content), got %d", len(store.messages))
	}
}

func TestFailRunPersistsErrorAndCompletion(t *testing.T) {
	store := &mockStore{}
	run := Run{
		ID:        "run-1",
		ProjectID: "proj-1",
		Status:    RunStatusRunning,
		StartedAt: time.Now().UTC(),
	}
	store.runs = append(store.runs, run)
	svc := NewService(store, &mockProjectGetter{}, nil, Config{})

	svc.failRun(context.Background(), &run, dispatch.RoutedWorker{Name: "Planner"}, &dispatch.WorkerResult{Output: "partial output"}, errors.New("boom"))

	if store.runs[0].Status != RunStatusFailed {
		t.Fatalf("expected failed run, got %s", store.runs[0].Status)
	}
	if store.runs[0].CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
	if len(store.events) < 2 {
		t.Fatalf("expected failure events, got %d", len(store.events))
	}
}
