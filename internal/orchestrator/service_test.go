package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/gabinante/flywheel/internal/dispatch"
	"github.com/gabinante/flywheel/internal/project"
)

type mockMessageStore struct {
	messages []Message
}

func (m *mockMessageStore) Create(_ context.Context, msg *Message) error {
	m.messages = append(m.messages, *msg)
	return nil
}

func (m *mockMessageStore) ListByProjectID(_ context.Context, projectID string, limit int) ([]Message, error) {
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

func TestSendUserMessageRequiresWorker(t *testing.T) {
	store := &mockMessageStore{}
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
	store := &mockMessageStore{}
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
}
