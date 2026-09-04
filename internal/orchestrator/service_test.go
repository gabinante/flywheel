package orchestrator

import (
	"context"
	"errors"
	"reflect"
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

type streamOutput struct {
	stream string
	text   string
}

type mockStreamWorker struct {
	mockWorker
	outputs       []string       // legacy: all emitted as "stdout"
	streamOutputs []streamOutput // stream-aware tuples
}

func (m *mockStreamWorker) SpawnStream(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string, onOutput dispatch.WorkerOutputHandler) (*dispatch.WorkerResult, error) {
	for _, so := range m.streamOutputs {
		onOutput(so.stream, so.text)
	}
	for _, output := range m.outputs {
		onOutput("stdout", output)
	}
	return m.Spawn(ctx, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL)
}

func TestSendUserMessageRequiresWorker(t *testing.T) {
	store := &mockStore{}
	svc := NewService(context.Background(), store, &mockProjectGetter{
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

// waitForRunDone polls until the run is no longer in "running" status.
func waitForRunDone(t *testing.T, svc *Service, projectID string, timeout time.Duration) *Thread {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		thread, err := svc.GetThread(context.Background(), projectID)
		if err != nil {
			t.Fatalf("GetThread() error = %v", err)
		}
		allDone := true
		for _, run := range thread.Runs {
			if run.Status == RunStatusRunning {
				allDone = false
				break
			}
		}
		if allDone {
			return thread
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for run to complete")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSendUserMessageUsesProjectRepoWhenServiceRepoUnset(t *testing.T) {
	store := &mockStore{}
	worker := &mockWorker{
		result: &dispatch.WorkerResult{Success: true, Output: "Created two tickets."},
	}
	svc := NewService(context.Background(), store, &mockProjectGetter{
		project: &project.Project{ID: "proj-1", Name: "Test", RepoURL: "/tmp/project-repo"},
	}, worker, Config{ServerURL: "http://localhost:8080", AgentID: "orch"})

	_, err := svc.SendUserMessage(context.Background(), "proj-1", "Add chat orchestration")
	if err != nil {
		t.Fatalf("SendUserMessage() error = %v", err)
	}

	// Wait for the background goroutine to complete the run.
	thread := waitForRunDone(t, svc, "proj-1", 5*time.Second)

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
		streamOutputs: []streamOutput{
			{stream: "tool_call", text: "create_work_stream"},
			{stream: "tool_result", text: "create_work_stream: warrant-123"},
		},
	}
	svc := NewService(context.Background(), store, &mockProjectGetter{
		project: &project.Project{ID: "proj-1", Name: "Test", RepoURL: "/tmp/project-repo"},
	}, worker, Config{ServerURL: "http://localhost:8080", AgentID: "orch"})

	_, err := svc.SendUserMessage(context.Background(), "proj-1", "Plan the auth feature")
	if err != nil {
		t.Fatalf("SendUserMessage() error = %v", err)
	}

	// Wait for the background goroutine to complete the run.
	thread := waitForRunDone(t, svc, "proj-1", 5*time.Second)

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
	foundToolCall := false
	foundToolResult := false
	for _, event := range run.Events {
		if event.Kind == RunEventKindToolCall && event.Payload["tool"] == "create_work_stream" {
			foundToolCall = true
		}
		if event.Kind == RunEventKindToolResult && event.Payload["tool"] == "create_work_stream" {
			foundToolResult = true
		}
	}
	if !foundToolCall {
		t.Fatalf("expected tool_call event for create_work_stream, got %+v", run.Events)
	}
	if !foundToolResult {
		t.Fatalf("expected tool_result event for create_work_stream, got %+v", run.Events)
	}
}

func TestGetThreadInitializesEmptySlices(t *testing.T) {
	store := &mockStore{}
	svc := NewService(context.Background(), store, &mockProjectGetter{
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
	svc := NewService(context.Background(), store, &mockProjectGetter{}, nil, Config{})

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
	svc := NewService(context.Background(), store, &mockProjectGetter{}, nil, Config{})

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

func TestClassifyWorkerOutput(t *testing.T) {
	tests := []struct {
		name       string
		stream     string
		text       string
		wantKind   RunEventKind
		wantTool   string
		wantStatus string
		wantSum    string
	}{
		{
			name:     "tool_call stream",
			stream:   "tool_call",
			text:     "create_work_stream",
			wantKind: RunEventKindToolCall,
			wantTool: "create_work_stream",
		},
		{
			name:       "tool_result stream success",
			stream:     "tool_result",
			text:       "create_work_stream: warrant-123",
			wantKind:   RunEventKindToolResult,
			wantTool:   "create_work_stream",
			wantStatus: "success",
		},
		{
			name:       "tool_result stream error",
			stream:     "tool_result",
			text:       "create_ticket failed: validation error",
			wantKind:   RunEventKindToolResult,
			wantTool:   "create_ticket",
			wantStatus: "error",
		},
		{
			name:     "tool_call stream with detail",
			stream:   "tool_call",
			text:     "Read: internal/foo.go",
			wantKind: RunEventKindToolCall,
			wantTool: "Read",
			wantSum:  "internal/foo.go",
		},
		{
			name:     "tool_call stream strips mcp prefix",
			stream:   "tool_call",
			text:     "mcp__flywheel__get_ticket: FLY-12",
			wantKind: RunEventKindToolCall,
			wantTool: "get_ticket",
		},
		{
			name:       "tool_result stream strips mcp prefix",
			stream:     "tool_result",
			text:       "mcp__flywheel__list_tickets: 3 tickets",
			wantKind:   RunEventKindToolResult,
			wantTool:   "list_tickets",
			wantStatus: "success",
		},
		{
			name:     "assistant prose mentioning a tool is not a tool call",
			stream:   "assistant",
			text:     "Next I will call create_ticket for each gap.",
			wantKind: RunEventKindWorkerOutput,
		},
		{
			name:     "system lifecycle note is worker output",
			stream:   "system",
			text:     "Claude Code session started · MCP flywheel connected",
			wantKind: RunEventKindWorkerOutput,
		},
		{
			name:     "stdout with known tool name (heuristic)",
			stream:   "stdout",
			text:     "create_work_stream: warrant-123",
			wantKind: RunEventKindToolCall,
			wantTool: "create_work_stream",
		},
		{
			name:     "stdout without tool name",
			stream:   "stdout",
			text:     "some random output",
			wantKind: RunEventKindWorkerOutput,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, payload := classifyWorkerOutput(tt.stream, tt.text)
			if kind != tt.wantKind {
				t.Fatalf("kind = %q, want %q", kind, tt.wantKind)
			}
			if tt.wantTool != "" {
				if got, _ := payload["tool"].(string); got != tt.wantTool {
					t.Fatalf("tool = %q, want %q", got, tt.wantTool)
				}
			}
			if tt.wantStatus != "" {
				if got, _ := payload["status"].(string); got != tt.wantStatus {
					t.Fatalf("status = %q, want %q", got, tt.wantStatus)
				}
			}
			if tt.wantSum != "" {
				if got, _ := payload["summary"].(string); got != tt.wantSum {
					t.Fatalf("summary = %q, want %q", got, tt.wantSum)
				}
			}
		})
	}
}

func TestParseToolResult(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantTool   string
		wantSum    string
		wantStatus string
	}{
		{
			name:       "success result",
			text:       "create_work_stream: warrant-123",
			wantTool:   "create_work_stream",
			wantSum:    "warrant-123",
			wantStatus: "success",
		},
		{
			name:       "error result",
			text:       "create_ticket failed: validation error",
			wantTool:   "create_ticket",
			wantSum:    "validation error",
			wantStatus: "error",
		},
		{
			name:       "no separator",
			text:       "some_tool",
			wantTool:   "some_tool",
			wantSum:    "",
			wantStatus: "success",
		},
		{
			name:       "status result with colon",
			text:       "list_tickets: status=completed",
			wantTool:   "list_tickets",
			wantSum:    "status=completed",
			wantStatus: "success",
		},
		{
			name:       "status result without colon",
			text:       "list_tickets status=completed",
			wantTool:   "list_tickets status=completed",
			wantSum:    "",
			wantStatus: "success",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, summary, status := parseToolResult(tt.text)
			if tool != tt.wantTool {
				t.Fatalf("tool = %q, want %q", tool, tt.wantTool)
			}
			if summary != tt.wantSum {
				t.Fatalf("summary = %q, want %q", summary, tt.wantSum)
			}
			if status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", status, tt.wantStatus)
			}
		})
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
	svc := NewService(context.Background(), store, &mockProjectGetter{}, nil, Config{})

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

func TestStreamedRunPhasesFollowToolActivity(t *testing.T) {
	store := &mockStore{}
	worker := &mockStreamWorker{
		mockWorker: mockWorker{
			result: &dispatch.WorkerResult{Success: true, Output: "Filed one ticket."},
		},
		streamOutputs: []streamOutput{
			{stream: "system", text: "Claude Code session started · MCP flywheel connected"},
			{stream: "tool_call", text: "Read: internal/foo.go"},
			{stream: "tool_result", text: "Read: package foo"},
			{stream: "tool_call", text: "mcp__flywheel__list_tickets"},
			{stream: "assistant", text: "I should create_ticket for the gap."},
			{stream: "tool_call", text: "create_ticket: Wire the thing"},
			{stream: "tool_result", text: "create_ticket: FLY-9"},
			{stream: "tool_call", text: "get_ticket: FLY-9"},
		},
	}
	svc := NewService(context.Background(), store, &mockProjectGetter{
		project: &project.Project{ID: "proj-1", Name: "Test", RepoURL: "/tmp/project-repo"},
	}, worker, Config{ServerURL: "http://localhost:8080", AgentID: "orch"})

	if _, err := svc.SendUserMessage(context.Background(), "proj-1", "Plan the auth feature"); err != nil {
		t.Fatalf("SendUserMessage() error = %v", err)
	}
	thread := waitForRunDone(t, svc, "proj-1", 5*time.Second)
	if len(thread.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(thread.Runs))
	}
	run := thread.Runs[0]

	var phases []string
	for _, event := range run.Events {
		if event.Kind == RunEventKindPhaseChange {
			phase, _ := event.Payload["phase"].(string)
			phases = append(phases, phase)
		}
		if event.Kind == RunEventKindToolCall && event.Payload["tool"] == "create_ticket" {
			if got, _ := event.Payload["summary"].(string); got != "Wire the thing" {
				t.Fatalf("expected tool_call detail to be kept as summary, got %q", got)
			}
		}
		if event.Kind == RunEventKindToolCall && event.Payload["tool"] == "mcp__flywheel__list_tickets" {
			t.Fatal("expected MCP-qualified tool names to be normalised")
		}
		if event.Kind != RunEventKindWorkerOutput && event.Payload["stream"] == "assistant" {
			t.Fatalf("assistant prose was classified as %s", event.Kind)
		}
	}
	want := []string{"queued", "connecting", "investigating", "authoring", "composing", "complete"}
	if !reflect.DeepEqual(phases, want) {
		t.Fatalf("phase sequence = %v, want %v", phases, want)
	}
}
