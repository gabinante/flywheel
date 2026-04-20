package dispatch

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
)

// --- Mocks ---

// mockTicketGetter implements TicketGetter for tests.
type mockTicketGetter struct {
	mu      sync.Mutex
	tickets map[string]*ticket.Ticket
	err     error
}

func newMockTicketGetter(tickets ...*ticket.Ticket) *mockTicketGetter {
	m := &mockTicketGetter{tickets: make(map[string]*ticket.Ticket)}
	for _, t := range tickets {
		m.tickets[t.ID] = t
	}
	return m
}

func (m *mockTicketGetter) GetTicket(_ context.Context, id string) (*ticket.Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	t, ok := m.tickets[id]
	if !ok {
		return nil, fmt.Errorf("ticket %q not found", id)
	}
	return t, nil
}

func (m *mockTicketGetter) GetTicketsByIDs(_ context.Context, ids []string) ([]*ticket.Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	var result []*ticket.Ticket
	for _, id := range ids {
		if t, ok := m.tickets[id]; ok {
			result = append(result, t)
		}
	}
	return result, nil
}

func (m *mockTicketGetter) ListByState(_ context.Context, _ string, state ticket.State) ([]*ticket.Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	var result []*ticket.Ticket
	for _, t := range m.tickets {
		if t.State == state {
			result = append(result, t)
		}
	}
	return result, nil
}

// mockProjectGetter implements ProjectGetter for tests.
type mockProjectGetter struct {
	projects map[string]*project.Project
	err      error
}

func newMockProjectGetter(projects ...*project.Project) *mockProjectGetter {
	m := &mockProjectGetter{projects: make(map[string]*project.Project)}
	for _, p := range projects {
		m.projects[p.ID] = p
	}
	return m
}

func (m *mockProjectGetter) GetProject(_ context.Context, id string) (*project.Project, error) {
	if m.err != nil {
		return nil, m.err
	}
	p, ok := m.projects[id]
	if !ok {
		return nil, fmt.Errorf("project %q not found", id)
	}
	return p, nil
}

// mockWorker implements Worker for tests.
type mockWorker struct {
	mu        sync.Mutex
	calls     []mockWorkerCall
	result    *WorkerResult
	err       error
	spawnFunc func(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error)
}

type mockWorkerCall struct {
	TicketID     string
	ProjectID    string
	SystemPrompt string
	TaskMessage  string
	WorkDir      string
	ServerURL    string
}

func (w *mockWorker) Spawn(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
	w.mu.Lock()
	w.calls = append(w.calls, mockWorkerCall{
		TicketID:     ticketID,
		ProjectID:    projectID,
		SystemPrompt: systemPrompt,
		TaskMessage:  taskMessage,
		WorkDir:      workDir,
		ServerURL:    serverURL,
	})
	w.mu.Unlock()

	if w.spawnFunc != nil {
		return w.spawnFunc(ctx, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL)
	}
	if w.err != nil {
		return nil, w.err
	}
	if w.result != nil {
		return w.result, nil
	}
	return &WorkerResult{Success: true, Output: "ok"}, nil
}

func (w *mockWorker) callCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.calls)
}

// --- Tests ---

func TestNewDispatcherCLIWorker(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()

	cfg := Config{
		MaxWorkers:  2,
		ClaudePath:  "/usr/bin/claude",
		WorktreeDir: "/tmp/wt",
		RepoDir:     "/repo",
		ServerURL:   "http://localhost:8080",
		AgentID:     "agent-1",
		APIKey:      "key-123",
	}

	d := New(cfg, bus, tg, pg)
	if d == nil {
		t.Fatal("expected non-nil dispatcher")
	}
	if d.cfg.MaxWorkers != 2 {
		t.Errorf("expected MaxWorkers=2, got %d", d.cfg.MaxWorkers)
	}

	// Should use CLIWorker when DockerEnabled is false.
	cliWorker, ok := d.worker.(*CLIWorker)
	if !ok {
		t.Fatal("expected CLIWorker when DockerEnabled is false")
	}
	// Driver should be Claude by default.
	if cliWorker.Driver.Name() != "claude" {
		t.Errorf("expected claude driver, got %q", cliWorker.Driver.Name())
	}
}

func TestNewDispatcherDockerWorker(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()

	cfg := Config{
		MaxWorkers:    1,
		DockerEnabled: true,
		DockerImage:   "my-image",
		DockerMemory:  "8g",
		DockerCPUs:    "4",
		APIKey:        "key-456",
		RepoDir:       "/repo",
		AnthropicKey:  "sk-test",
	}

	d := New(cfg, bus, tg, pg)
	dw, ok := d.worker.(*DockerWorker)
	if !ok {
		t.Fatal("expected DockerWorker when DockerEnabled is true")
	}
	if dw.Image != "my-image" {
		t.Errorf("expected image 'my-image', got %q", dw.Image)
	}
	if dw.Memory != "8g" {
		t.Errorf("expected memory '8g', got %q", dw.Memory)
	}
	if dw.APIKey != "key-456" {
		t.Errorf("expected API key 'key-456', got %q", dw.APIKey)
	}
	// Driver should be Claude by default.
	if dw.Driver.Name() != "claude" {
		t.Errorf("expected claude driver, got %q", dw.Driver.Name())
	}
}

func TestNewDispatcherGenericDriver(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()

	cfg := Config{
		MaxWorkers:  1,
		AgentDriver: "generic",
		AgentCLIPath: "/usr/bin/opencode",
		RepoDir:     "/repo",
	}

	d := New(cfg, bus, tg, pg)
	cliWorker, ok := d.worker.(*CLIWorker)
	if !ok {
		t.Fatal("expected CLIWorker when DockerEnabled is false")
	}
	if cliWorker.Driver.Name() != "generic" {
		t.Errorf("expected generic driver, got %q", cliWorker.Driver.Name())
	}
}

func TestDispatcherActiveCount(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)
	if d.activeCount() != 0 {
		t.Errorf("expected 0 active, got %d", d.activeCount())
	}

	// Simulate adding active entries.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.mu.Lock()
	d.active["t1"] = cancel
	d.mu.Unlock()

	if d.activeCount() != 1 {
		t.Errorf("expected 1 active, got %d", d.activeCount())
	}
	_ = ctx // use ctx
}

func TestDispatcherStop(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)

	// Add a mock active worker that blocks until cancelled.
	// Mimic the real spawn goroutine: clean up active map on exit.
	ctx, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.active["t1"] = cancel
	d.mu.Unlock()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			delete(d.active, "t1")
			d.mu.Unlock()
		}()
		<-ctx.Done()
	}()

	// Stop should cancel all active workers and wait.
	done := make(chan struct{})
	go func() {
		d.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success: Stop completed.
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not complete in time")
	}

	if d.activeCount() != 0 {
		t.Errorf("expected 0 active after Stop, got %d", d.activeCount())
	}
}

func TestHandleTicketReadyEmptyPayload(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)

	// Empty payload should be silently ignored (no panic).
	d.handleTicketReady(context.Background(), events.Event{
		Type:    events.EventTicketCreated,
		Payload: map[string]any{},
	})
}

func TestHandleTicketReadyProjectFilter(t *testing.T) {
	tk := &ticket.Ticket{
		ID:        "t-1",
		ProjectID: "other-project",
		State:     ticket.StatePending,
		Title:     "t",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5, ProjectID: "my-project"}

	d := New(cfg, bus, tg, pg)

	// Ticket from a different project should be filtered out.
	d.handleTicketReady(context.Background(), events.Event{
		Type:    events.EventTicketCreated,
		Payload: map[string]any{"ticket_id": "t-1"},
	})

	// No workers should be spawned.
	if d.activeCount() != 0 {
		t.Errorf("expected 0 active (filtered), got %d", d.activeCount())
	}
}

func TestTryDispatchNonPendingTicket(t *testing.T) {
	tk := &ticket.Ticket{
		ID:    "t-1",
		State: ticket.StateExecuting,
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)
	d.tryDispatch(context.Background(), tk)

	if d.activeCount() != 0 {
		t.Error("should not dispatch non-pending ticket")
	}
}

func TestTryDispatchAtCapacity(t *testing.T) {
	tk := &ticket.Ticket{
		ID:        "t-1",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "t",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 1}

	d := New(cfg, bus, tg, pg)

	// Fill to capacity.
	_, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.active["existing"] = cancel
	d.mu.Unlock()

	d.tryDispatch(context.Background(), tk)

	if d.activeCount() != 1 {
		t.Error("should not exceed max workers capacity")
	}
}

func TestTryDispatchAlreadyRunning(t *testing.T) {
	tk := &ticket.Ticket{
		ID:        "t-1",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "t",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)

	// Mark ticket as already running.
	_, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.active["t-1"] = cancel
	d.mu.Unlock()

	d.tryDispatch(context.Background(), tk)

	// Should still be 1 (not re-dispatched).
	if d.activeCount() != 1 {
		t.Errorf("expected 1 active, got %d", d.activeCount())
	}
}

func TestTryDispatchBlockedByDependency(t *testing.T) {
	dep := &ticket.Ticket{
		ID:    "dep-1",
		State: ticket.StateExecuting, // not done
	}
	tk := &ticket.Ticket{
		ID:        "t-1",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		DependsOn: []string{"dep-1"},
		Title:     "t",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk, dep)
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)
	d.tryDispatch(context.Background(), tk)

	if d.activeCount() != 0 {
		t.Error("should not dispatch ticket with incomplete dependency")
	}
}

func TestTryDispatchDependenciesMet(t *testing.T) {
	dep := &ticket.Ticket{
		ID:      "dep-1",
		State:   ticket.StateDone,
		Outputs: map[string]any{"summary": "done"},
	}
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-1",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		DependsOn: []string{"dep-1"},
		Title:     "t",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk, dep)
	pg := newMockProjectGetter(proj)

	worker := &mockWorker{}
	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true, // use docker mode to avoid worktree creation
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
		AgentID:       "test-agent",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker // inject mock worker

	d.tryDispatch(context.Background(), tk)

	// Wait for the spawned goroutine to run.
	time.Sleep(500 * time.Millisecond)

	if worker.callCount() != 1 {
		t.Errorf("expected 1 worker call, got %d", worker.callCount())
	}
}

func TestHandleTicketDone(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5, WorktreeDir: t.TempDir(), RepoDir: t.TempDir()}

	d := New(cfg, bus, tg, pg)

	// Add a mock active entry.
	_, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.active["t-1"] = cancel
	d.mu.Unlock()

	d.handleTicketDone(context.Background(), events.Event{
		Type:    events.EventTicketDone,
		Payload: map[string]any{"ticket_id": "t-1"},
	})

	if d.activeCount() != 0 {
		t.Error("expected ticket to be removed from active after done event")
	}
}

func TestHandleTicketDoneEmptyPayload(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)

	// Should not panic with empty payload.
	d.handleTicketDone(context.Background(), events.Event{
		Type:    events.EventTicketDone,
		Payload: map[string]any{},
	})
}

func TestHandleTicketDoneUnknownTicket(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5, WorktreeDir: t.TempDir(), RepoDir: t.TempDir()}

	d := New(cfg, bus, tg, pg)

	// Should handle gracefully when ticket is not in active map.
	d.handleTicketDone(context.Background(), events.Event{
		Type:    events.EventTicketDone,
		Payload: map[string]any{"ticket_id": "unknown"},
	})
	// No panic = success.
}

func TestStartSubscribesEvents(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d.Start(ctx)
	defer d.Stop()

	// Verify that publishing events doesn't panic (handlers are registered).
	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketCreated,
		Payload: map[string]any{},
	})
	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketUnblocked,
		Payload: map[string]any{},
	})
	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketRejected,
		Payload: map[string]any{},
	})
	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketDone,
		Payload: map[string]any{},
	})
	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketApproved,
		Payload: map[string]any{},
	})
}

func TestSpawnDuplicatePrevented(t *testing.T) {
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-dup",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "t",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	blockCh := make(chan struct{})
	worker := &mockWorker{
		spawnFunc: func(ctx context.Context, _, _, _, _, _, _ string) (*WorkerResult, error) {
			<-blockCh // block until released
			return &WorkerResult{Success: true}, nil
		},
	}

	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker

	ctx := context.Background()

	// First spawn.
	d.spawn(ctx, tk)
	time.Sleep(50 * time.Millisecond) // let goroutine register in active map

	// Second spawn of same ticket should be rejected (already in active).
	d.spawn(ctx, tk)
	time.Sleep(200 * time.Millisecond) // wait past the 100ms startup delay in runWorker

	// Only 1 call should have been made.
	if worker.callCount() != 1 {
		t.Errorf("expected 1 worker call (dup prevented), got %d", worker.callCount())
	}

	close(blockCh) // release blocked worker
	d.Stop()
}

func TestRunWorkerDockerMode(t *testing.T) {
	proj := &project.Project{
		ID:   "p-1",
		Name: "test-proj",
		ContextPack: project.ContextPack{
			SystemPrompt: "Be helpful",
		},
	}
	dep := &ticket.Ticket{
		ID:      "dep-1",
		State:   ticket.StateDone,
		Outputs: map[string]any{"key": "value"},
	}
	tk := &ticket.Ticket{
		ID:        "t-1",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "test ticket",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "do work"},
		DependsOn: []string{"dep-1"},
	}

	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk, dep)
	pg := newMockProjectGetter(proj)

	worker := &mockWorker{}
	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost:8080",
		AgentID:       "agent-test",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker

	err := d.runWorker(context.Background(), tk)
	if err != nil {
		t.Fatalf("runWorker: %v", err)
	}

	if worker.callCount() != 1 {
		t.Fatalf("expected 1 worker call, got %d", worker.callCount())
	}

	worker.mu.Lock()
	call := worker.calls[0]
	worker.mu.Unlock()

	if call.TicketID != "t-1" {
		t.Errorf("expected ticketID 't-1', got %q", call.TicketID)
	}
	if call.WorkDir != "/tmp" {
		t.Errorf("expected workDir '/tmp' in docker mode, got %q", call.WorkDir)
	}
	if call.ServerURL != "http://localhost:8080" {
		t.Errorf("expected serverURL, got %q", call.ServerURL)
	}
	// System prompt should contain the project system prompt.
	if call.SystemPrompt == "" {
		t.Error("expected non-empty system prompt")
	}
}

func TestRunWorkerContextCancelled(t *testing.T) {
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-1",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "t",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}

	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	worker := &mockWorker{}
	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker

	// Cancel context immediately so the thundering-herd delay returns early.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := d.runWorker(ctx, tk)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestRunWorkerProjectNotFound(t *testing.T) {
	tk := &ticket.Ticket{
		ID:        "t-1",
		ProjectID: "missing-project",
		State:     ticket.StatePending,
		Title:     "t",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}

	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter() // no projects

	worker := &mockWorker{}
	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker

	err := d.runWorker(context.Background(), tk)
	if err == nil {
		t.Error("expected error when project not found")
	}
}

// --- Lease Release Tests (Layer 1 failure recovery) ---

// mockLeaseReleaser implements LeaseReleaser for tests.
type mockLeaseReleaser struct {
	mu       sync.Mutex
	calls    []string // ticket IDs passed to ForceReleaseLease
	err      error    // error to return
}

func (m *mockLeaseReleaser) ForceReleaseLease(_ context.Context, ticketID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, ticketID)
	return m.err
}

func (m *mockLeaseReleaser) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockLeaseReleaser) lastTicketID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return ""
	}
	return m.calls[len(m.calls)-1]
}

func TestHandleWorkerExit_WorkerCrash_ReleasesLease(t *testing.T) {
	// Simulate: worker exits with error while ticket is still in executing state.
	// Expected: lease is immediately released, ticket goes back to draft (pending).
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-crash",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "crash test",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	worker := &mockWorker{
		spawnFunc: func(ctx context.Context, _, _, _, _, _, _ string) (*WorkerResult, error) {
			// Simulate crash: return error.
			return nil, fmt.Errorf("process killed")
		},
	}

	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker
	d.SetLeaseReleaser(releaser)

	// Ticket state must be planning or executing for handleWorkerExit to trigger.
	// After spawn → runWorker fails → handleWorkerExit checks ticket state.
	// Update the mock ticket to be in executing state (simulating that claim happened).
	tg.mu.Lock()
	tk.State = ticket.StateExecuting
	tg.mu.Unlock()

	d.spawn(context.Background(), tk)

	// Wait for goroutine to complete.
	d.wg.Wait()

	if releaser.callCount() != 1 {
		t.Fatalf("expected 1 ForceReleaseLease call, got %d", releaser.callCount())
	}
	if releaser.lastTicketID() != "t-crash" {
		t.Errorf("expected ticketID 't-crash', got %q", releaser.lastTicketID())
	}
}

func TestHandleWorkerExit_SuccessfulSubmit_NoRelease(t *testing.T) {
	// Simulate: worker submits successfully, ticket moves to awaiting_validation.
	// Expected: no lease release (ticket already advanced past worker's responsibility).
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-submit",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "submit test",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	worker := &mockWorker{
		spawnFunc: func(ctx context.Context, _, _, _, _, _, _ string) (*WorkerResult, error) {
			// Simulate successful worker: it submits, ticket moves to awaiting_validation.
			tg.mu.Lock()
			tk.State = ticket.StateAwaitingValidation
			tg.mu.Unlock()
			return &WorkerResult{Success: true, Output: "done"}, nil
		},
	}

	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker
	d.SetLeaseReleaser(releaser)

	d.spawn(context.Background(), tk)
	d.wg.Wait()

	if releaser.callCount() != 0 {
		t.Errorf("expected 0 ForceReleaseLease calls (ticket submitted), got %d", releaser.callCount())
	}
}

func TestHandleWorkerExit_PlanningState_ReleasesLease(t *testing.T) {
	// Simulate: worker exits while ticket is in planning (claimed) state.
	// Expected: lease is released.
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-planning",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "planning crash",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	worker := &mockWorker{
		spawnFunc: func(ctx context.Context, _, _, _, _, _, _ string) (*WorkerResult, error) {
			return nil, fmt.Errorf("crash during planning")
		},
	}

	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker
	d.SetLeaseReleaser(releaser)

	// Simulate that ticket was claimed (moved to planning).
	tg.mu.Lock()
	tk.State = ticket.StatePlanning
	tg.mu.Unlock()

	d.spawn(context.Background(), tk)
	d.wg.Wait()

	if releaser.callCount() != 1 {
		t.Fatalf("expected 1 ForceReleaseLease call for planning state, got %d", releaser.callCount())
	}
}

func TestHandleWorkerExit_PreSpawnFailure_NoRelease(t *testing.T) {
	// Simulate: runWorker fails before reaching spawn (e.g. project not found).
	// Ticket is still in pending/draft state → handleWorkerExit should not release.
	tk := &ticket.Ticket{
		ID:        "t-prespawn",
		ProjectID: "missing-project",
		State:     ticket.StatePending,
		Title:     "pre-spawn fail",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter() // no projects → GetProject fails

	releaser := &mockLeaseReleaser{}
	worker := &mockWorker{}

	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker
	d.SetLeaseReleaser(releaser)

	d.spawn(context.Background(), tk)
	d.wg.Wait()

	// Ticket is still in pending (draft) state, not planning/executing,
	// so handleWorkerExit should be a no-op.
	if releaser.callCount() != 0 {
		t.Errorf("expected 0 ForceReleaseLease calls (ticket still pending), got %d", releaser.callCount())
	}
}

func TestHandleWorkerExit_NilReleaser_NoOp(t *testing.T) {
	// When leaseReleaser is nil, handleWorkerExit is a graceful no-op.
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-nil",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "nil releaser",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	worker := &mockWorker{
		spawnFunc: func(ctx context.Context, _, _, _, _, _, _ string) (*WorkerResult, error) {
			return nil, fmt.Errorf("crash")
		},
	}

	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker
	// Note: NOT setting leaseReleaser — it's nil.

	tg.mu.Lock()
	tk.State = ticket.StateExecuting
	tg.mu.Unlock()

	// Should not panic with nil leaseReleaser.
	d.spawn(context.Background(), tk)
	d.wg.Wait()
}

func TestHandleWorkerExit_Escalation_NoRelease(t *testing.T) {
	// Simulate: worker escalates, ticket moves to awaiting_input (needs_human).
	// Expected: no release — escalation is a valid worker exit.
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-escalate",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "escalation test",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	worker := &mockWorker{
		spawnFunc: func(ctx context.Context, _, _, _, _, _, _ string) (*WorkerResult, error) {
			// Simulate: worker escalates, ticket moves to awaiting_input.
			tg.mu.Lock()
			tk.State = ticket.StateAwaitingInput
			tg.mu.Unlock()
			return &WorkerResult{Success: true, Output: "escalated"}, nil
		},
	}

	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker
	d.SetLeaseReleaser(releaser)

	d.spawn(context.Background(), tk)
	d.wg.Wait()

	if releaser.callCount() != 0 {
		t.Errorf("expected 0 ForceReleaseLease calls (escalation), got %d", releaser.callCount())
	}
}

func TestHandleWorkerExit_ActiveMapCleanup(t *testing.T) {
	// Verify: after worker crash + lease release, the ticket is removed from the active map.
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-cleanup",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "cleanup test",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	worker := &mockWorker{
		spawnFunc: func(ctx context.Context, _, _, _, _, _, _ string) (*WorkerResult, error) {
			return nil, fmt.Errorf("crash")
		},
	}

	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker
	d.SetLeaseReleaser(releaser)

	tg.mu.Lock()
	tk.State = ticket.StateExecuting
	tg.mu.Unlock()

	d.spawn(context.Background(), tk)
	d.wg.Wait()

	// Worker should be removed from active map.
	if d.activeCount() != 0 {
		t.Errorf("expected 0 active workers after crash, got %d", d.activeCount())
	}
}

func TestHandleWorkerExit_ReleaserError_Logged(t *testing.T) {
	// Verify: if ForceReleaseLease returns an error, we don't panic and the
	// worker is still cleaned up from active map (Layer 2 TTL will handle it).
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-err",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "release error",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{err: fmt.Errorf("redis unavailable")}
	worker := &mockWorker{
		spawnFunc: func(ctx context.Context, _, _, _, _, _, _ string) (*WorkerResult, error) {
			return nil, fmt.Errorf("crash")
		},
	}

	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker
	d.SetLeaseReleaser(releaser)

	tg.mu.Lock()
	tk.State = ticket.StateExecuting
	tg.mu.Unlock()

	d.spawn(context.Background(), tk)
	d.wg.Wait()

	// ForceReleaseLease was called (and failed), but we don't crash.
	if releaser.callCount() != 1 {
		t.Errorf("expected 1 ForceReleaseLease call, got %d", releaser.callCount())
	}
	// Active map still cleaned up (defer handles that).
	if d.activeCount() != 0 {
		t.Errorf("expected 0 active after error, got %d", d.activeCount())
	}
}

func TestHandleWorkerExit_ConcurrentWorkers(t *testing.T) {
	// Verify: multiple workers crashing concurrently each get their lease released.
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk1 := &ticket.Ticket{
		ID:        "t-c1",
		ProjectID: "p-1",
		State:     ticket.StateExecuting,
		Title:     "concurrent 1",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	tk2 := &ticket.Ticket{
		ID:        "t-c2",
		ProjectID: "p-1",
		State:     ticket.StateExecuting,
		Title:     "concurrent 2",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk1, tk2)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	worker := &mockWorker{
		spawnFunc: func(ctx context.Context, _, _, _, _, _, _ string) (*WorkerResult, error) {
			return nil, fmt.Errorf("crash")
		},
	}

	cfg := Config{
		MaxWorkers:    5,
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker
	d.SetLeaseReleaser(releaser)

	// Spawn both workers concurrently (they have different IDs so no dup prevention).
	d.spawn(context.Background(), tk1)
	d.spawn(context.Background(), tk2)
	d.wg.Wait()

	if releaser.callCount() != 2 {
		t.Errorf("expected 2 ForceReleaseLease calls, got %d", releaser.callCount())
	}
	if d.activeCount() != 0 {
		t.Errorf("expected 0 active after concurrent crashes, got %d", d.activeCount())
	}
}

func TestEventBusIntegration(t *testing.T) {
	proj := &project.Project{ID: "p-1", Name: "test"}
	tk := &ticket.Ticket{
		ID:        "t-evt",
		ProjectID: "p-1",
		State:     ticket.StatePending,
		Title:     "event test",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}

	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	worker := &mockWorker{}
	cfg := Config{
		MaxWorkers:    5,
		ProjectID:     "p-1",
		DockerEnabled: true,
		RepoDir:       "/tmp",
		ServerURL:     "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.worker = worker

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	defer d.Stop()

	// Publish a ticket.created event.
	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketCreated,
		Payload: map[string]any{"ticket_id": "t-evt"},
	})

	// Wait for the goroutine to process.
	time.Sleep(500 * time.Millisecond)

	if worker.callCount() != 1 {
		t.Errorf("expected 1 worker call from event, got %d", worker.callCount())
	}
}
