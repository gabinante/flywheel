package dispatch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func (m *mockTicketGetter) ListByWorkStream(_ context.Context, _ string, workStreamID string) ([]*ticket.Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	var result []*ticket.Ticket
	for _, t := range m.tickets {
		if t.WorkStreamID == workStreamID {
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

// seedTestClone pre-creates a git repo at the clone manager's expected path for a
// project ID, so EnsureClone finds it and skips network cloning. Returns the repo dir.
func seedTestClone(t *testing.T, d *Dispatcher, projectID string) string {
	t.Helper()
	repo := createBareRepo(t, "fixture")
	proj, err := d.projects.GetProject(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	proj.RepoURL = repo
	safe := strings.ReplaceAll(projectID, "/", "-")
	dir := filepath.Join(d.clones.BaseDir, safe)
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "clone", repo, dir).CombinedOutput(); err != nil {
		t.Fatalf("clone fixture: %s: %v", out, err)
	}

	return dir
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
	d.skipWorktrees = true
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

func TestNewDispatcherGenericDriver(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()

	cfg := Config{
		MaxWorkers:   1,
		AgentDriver:  "generic",
		AgentCLIPath: "/usr/bin/opencode",
		RepoDir:      "/repo",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
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
		State:     ticket.StateDraft,
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
		State:     ticket.StateDraft,
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

func TestProjectCapacityUsesProjectMaxActiveWorkers(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter(&project.Project{
		ID:      "p-1",
		RepoURL: "https://github.com/test/repo.git",
		DispatchConfig: project.DispatchConfig{
			MaxActiveWorkers: 2,
		},
	})
	d := New(Config{MaxWorkers: 5}, bus, tg, pg)

	_, cancel1 := context.WithCancel(context.Background())
	_, cancel2 := context.WithCancel(context.Background())
	defer cancel1()
	defer cancel2()

	d.mu.Lock()
	d.active["t-1"] = cancel1
	d.activeProjects["t-1"] = "p-1"
	d.active["review:t-2"] = cancel2
	d.activeProjects["review:t-2"] = "p-1"
	d.mu.Unlock()

	active, limit, hasCapacity := d.projectCapacity(context.Background(), "p-1")
	if active != 2 {
		t.Fatalf("expected 2 active project workers, got %d", active)
	}
	if limit != 2 {
		t.Fatalf("expected project worker limit 2, got %d", limit)
	}
	if hasCapacity {
		t.Fatal("expected project to be at capacity")
	}
}

func TestProjectCapacityCountsWorkersByProject(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter(
		&project.Project{ID: "p-1", RepoURL: "https://github.com/test/repo.git", DispatchConfig: project.DispatchConfig{MaxActiveWorkers: 1}},
		&project.Project{ID: "p-2", RepoURL: "https://github.com/test/repo2.git", DispatchConfig: project.DispatchConfig{MaxActiveWorkers: 1}},
	)
	d := New(Config{MaxWorkers: 1}, bus, tg, pg)

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	d.mu.Lock()
	d.active["t-2"] = cancel
	d.activeProjects["t-2"] = "p-2"
	d.mu.Unlock()

	active, limit, hasCapacity := d.projectCapacity(context.Background(), "p-1")
	if active != 0 {
		t.Fatalf("expected no active workers for p-1, got %d", active)
	}
	if limit != 1 {
		t.Fatalf("expected project worker limit 1, got %d", limit)
	}
	if !hasCapacity {
		t.Fatal("expected p-1 to have capacity independent of p-2")
	}
}

func TestProjectCapacityCapsAtServerLimit(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter(&project.Project{
		ID:      "p-1",
		RepoURL: "https://github.com/test/repo.git",
		DispatchConfig: project.DispatchConfig{
			MaxActiveWorkers: 10, // exceeds server limit
		},
	})
	d := New(Config{MaxWorkers: 4}, bus, tg, pg)

	// Fill to server limit.
	for i := 0; i < 4; i++ {
		_, cancel := context.WithCancel(context.Background())
		defer cancel()
		key := fmt.Sprintf("t-%d", i)
		d.mu.Lock()
		d.active[key] = cancel
		d.activeProjects[key] = "p-1"
		d.mu.Unlock()
	}

	active, limit, hasCapacity := d.projectCapacity(context.Background(), "p-1")
	if limit != 4 {
		t.Fatalf("expected server limit 4 to cap project limit 10, got %d", limit)
	}
	if active != 4 {
		t.Fatalf("expected 4 active, got %d", active)
	}
	if hasCapacity {
		t.Fatal("expected project to be at capacity (capped by server limit)")
	}
}

func TestTryDispatchAlreadyRunning(t *testing.T) {
	tk := &ticket.Ticket{
		ID:        "t-1",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
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
		State:     ticket.StateDraft,
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
		State:   ticket.StateClosed,
		Outputs: map[string]any{"summary": "done"},
	}
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}
	tk := &ticket.Ticket{
		ID:        "t-1",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
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
		MaxWorkers:  5,
		WorktreeDir: t.TempDir(),
		ServerURL:   "http://localhost",
		AgentID:     "test-agent",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker // inject mock worker
	seedTestClone(t, d, "p-1")

	d.tryDispatch(context.Background(), tk)

	// Wait for the actual worker invocation; Git fetch duration varies under load.
	deadline := time.Now().Add(5 * time.Second)
	for worker.callCount() < 1 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	if worker.callCount() != 1 {
		t.Errorf("expected 1 worker call, got %d", worker.callCount())
	}
}

func TestHandleTicketDone(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(&ticket.Ticket{ID: "t-1", State: ticket.StateClosed})
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
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}
	tk := &ticket.Ticket{
		ID:        "t-dup",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
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
		MaxWorkers:  5,
		WorktreeDir: t.TempDir(),
		ServerURL:   "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	seedTestClone(t, d, "p-1")

	ctx := context.Background()

	// First spawn.
	d.spawn(ctx, tk)
	time.Sleep(50 * time.Millisecond) // let goroutine register in active map

	// Second spawn of same ticket should be rejected (already in active).
	d.spawn(ctx, tk)
	// Wait for the first worker to be invoked (repo resolution plus the 100ms
	// startup delay make a fixed sleep flaky), then give a duplicate time to appear.
	deadline := time.Now().Add(5 * time.Second)
	for worker.callCount() < 1 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond)

	// Only 1 call should have been made.
	if worker.callCount() != 1 {
		t.Errorf("expected 1 worker call (dup prevented), got %d", worker.callCount())
	}

	close(blockCh) // release blocked worker
	d.Stop()
}

func TestRunWorkerContextCancelled(t *testing.T) {
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}
	tk := &ticket.Ticket{
		ID:        "t-1",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
		Title:     "t",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}

	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	worker := &mockWorker{}
	cfg := Config{
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
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
		State:     ticket.StateDraft,
		Title:     "t",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}

	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter() // no projects

	worker := &mockWorker{}
	cfg := Config{
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker

	err := d.runWorker(context.Background(), tk)
	if err == nil {
		t.Error("expected error when project not found")
	}
}

// --- Lease Release Tests (Layer 1 failure recovery) ---

// mockLeaseReleaser implements LeaseReleaser for tests.
type mockLeaseReleaser struct {
	mu    sync.Mutex
	calls []string // ticket IDs passed to ForceReleaseLease
	err   error    // error to return
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
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}
	tk := &ticket.Ticket{
		ID:        "t-crash",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
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
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
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
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}
	tk := &ticket.Ticket{
		ID:        "t-submit",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
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
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
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
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}
	tk := &ticket.Ticket{
		ID:        "t-planning",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
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
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
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
		State:     ticket.StateDraft,
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
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
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
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}
	tk := &ticket.Ticket{
		ID:        "t-nil",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
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
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
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
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}
	tk := &ticket.Ticket{
		ID:        "t-escalate",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
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
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
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
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}
	tk := &ticket.Ticket{
		ID:        "t-cleanup",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
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
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
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
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}
	tk := &ticket.Ticket{
		ID:        "t-err",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
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
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
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
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}
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
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
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
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git", DispatchEnabled: true}
	tk := &ticket.Ticket{
		ID:        "t-evt",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
		Title:     "event test",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}

	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	worker := &mockWorker{}
	cfg := Config{
		MaxWorkers:  5,
		ProjectID:   "p-1",
		WorktreeDir: t.TempDir(),
		ServerURL:   "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	seedTestClone(t, d, "p-1")

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

func TestDispatchDisabledSkipsTickets(t *testing.T) {
	// When dispatch_enabled=false, the dispatcher should skip the project's tickets.
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git", DispatchEnabled: false}
	tk := &ticket.Ticket{
		ID:        "t-disabled",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
		Title:     "disabled dispatch test",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}

	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	worker := &mockWorker{}
	cfg := Config{
		MaxWorkers: 5,
		ProjectID:  "p-1",
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	defer d.Stop()

	// Publish a ticket.created event for the disabled project.
	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketCreated,
		Payload: map[string]any{"ticket_id": "t-disabled"},
	})

	// Wait for the goroutine to process.
	time.Sleep(500 * time.Millisecond)

	if worker.callCount() != 0 {
		t.Errorf("expected 0 worker calls when dispatch disabled, got %d", worker.callCount())
	}
}

func TestDispatchEnabledAllowsTickets(t *testing.T) {
	// When dispatch_enabled=true, the dispatcher should process the project's tickets.
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git", DispatchEnabled: true}
	tk := &ticket.Ticket{
		ID:        "t-enabled",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
		Title:     "enabled dispatch test",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}

	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	worker := &mockWorker{}
	cfg := Config{
		MaxWorkers:  5,
		ProjectID:   "p-1",
		WorktreeDir: t.TempDir(),
		ServerURL:   "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	seedTestClone(t, d, "p-1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	defer d.Stop()

	// Publish a ticket.created event for the enabled project.
	_ = bus.Publish(ctx, events.Event{
		Type:    events.EventTicketCreated,
		Payload: map[string]any{"ticket_id": "t-enabled"},
	})

	// Wait for the goroutine to process.
	time.Sleep(500 * time.Millisecond)

	if worker.callCount() != 1 {
		t.Errorf("expected 1 worker call when dispatch enabled, got %d", worker.callCount())
	}
}

func TestDispatchNoRepoUsesOperator(t *testing.T) {
	// Projects without repo_url should dispatch as operator (not executor).
	// Workers get a temp scratch dir instead of a git workspace.
	proj := &project.Project{ID: "p-no-repo", Name: "no-repo-project", DispatchEnabled: true}
	tk := &ticket.Ticket{
		ID:        "t-norepo",
		ProjectID: "p-no-repo",
		State:     ticket.StateDraft,
		Title:     "should dispatch as operator",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "test"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)
	worker := &mockWorker{}
	cfg := Config{MaxWorkers: 5, ProjectID: "p-no-repo", RepoDir: "/tmp", ServerURL: "http://localhost", AgentID: "test"}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	d.Start(context.Background())

	bus.Publish(context.Background(), events.Event{
		Type:    events.EventTicketCreated,
		Payload: map[string]any{"ticket_id": "t-norepo"},
	})

	time.Sleep(500 * time.Millisecond)
	d.Stop()

	if worker.callCount() != 1 {
		t.Errorf("expected 1 worker call for no-repo project (operator mode), got %d", worker.callCount())
	}
	if worker.callCount() > 0 {
		worker.mu.Lock()
		call := worker.calls[0]
		worker.mu.Unlock()
		if !strings.Contains(call.SystemPrompt, "operator") {
			t.Error("expected operator prompt for no-repo project")
		}
	}
}

// --- Ticket Transitioner Tests (post-merge lifecycle) ---

// mockTicketTransitioner records TransitionTicket calls.
type mockTicketTransitioner struct {
	mu          sync.Mutex
	transitions []mockTransition
	err         error  // error to return (nil = success)
	failOn      string // trigger name to fail on (empty = never fail)
}

type mockTransition struct {
	ID      string
	Trigger string
	Actor   ticket.Actor
}

func (m *mockTicketTransitioner) TransitionTicket(_ context.Context, id string, trigger string, actor ticket.Actor, _ map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transitions = append(m.transitions, mockTransition{ID: id, Trigger: trigger, Actor: actor})
	if m.failOn != "" && trigger == m.failOn {
		return m.err
	}
	return nil
}

func (m *mockTicketTransitioner) transitionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.transitions)
}

func (m *mockTicketTransitioner) triggers() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []string
	for _, t := range m.transitions {
		result = append(result, t.Trigger)
	}
	return result
}

func TestCloseMergedTicket_HappyPath(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)
	trans := &mockTicketTransitioner{}
	d.SetTicketTransitioner(trans)

	tk := &ticket.Ticket{ID: "t-1", ProjectID: "p-1", State: ticket.StateValidated}
	d.closeMergedTicket(context.Background(), tk)

	if trans.transitionCount() != 1 {
		t.Fatalf("expected 1 transition, got %d", trans.transitionCount())
	}
	triggers := trans.triggers()
	if triggers[0] != ticket.TriggerClose {
		t.Errorf("expected close trigger, got %q", triggers[0])
	}
}

func TestCloseMergedTicket_NilTransitioner(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)
	// Note: NOT setting ticketTransitioner — it's nil.

	tk := &ticket.Ticket{ID: "t-1", ProjectID: "p-1", State: ticket.StateValidated}

	// Should not panic.
	d.closeMergedTicket(context.Background(), tk)
}

func TestCloseMergedTicket_TransitionError(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)
	trans := &mockTicketTransitioner{
		failOn: ticket.TriggerClose,
		err:    fmt.Errorf("transition failed"),
	}
	d.SetTicketTransitioner(trans)

	tk := &ticket.Ticket{ID: "t-1", ProjectID: "p-1", State: ticket.StateValidated}
	d.closeMergedTicket(context.Background(), tk)

	// Should have attempted close (failure), then stopped.
	if trans.transitionCount() != 1 {
		t.Errorf("expected 1 transition (failed at close), got %d", trans.transitionCount())
	}
}

func TestAutoMergePR_StateGuard(t *testing.T) {
	bus := events.NewInProcessBus()
	tk := &ticket.Ticket{
		ID:        "t-guard",
		ProjectID: "p-1",
		State:     ticket.StateClosed, // not validated — should be skipped
		Outputs:   map[string]any{"pr_url": "https://github.com/test/pr/1"},
	}
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5, RepoDir: t.TempDir(), WorktreeDir: t.TempDir()}

	d := New(cfg, bus, tg, pg)
	trans := &mockTicketTransitioner{}
	d.SetTicketTransitioner(trans)

	d.autoMergePR(context.Background(), tk, "https://github.com/test/pr/1")

	// No transitions should happen — state guard blocks.
	if trans.transitionCount() != 0 {
		t.Errorf("expected 0 transitions (state guard), got %d", trans.transitionCount())
	}
}

func TestAutoMergePR_Escalation(t *testing.T) {
	bus := events.NewInProcessBus()
	tk := &ticket.Ticket{
		ID:        "t-esc",
		ProjectID: "p-1",
		State:     ticket.StateValidated,
		Outputs: map[string]any{
			"pr_url":          "https://github.com/test/pr/1",
			"_merge_attempts": float64(maxMergeAttempts), // persisted as JSON number
		},
	}
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5, RepoDir: t.TempDir(), WorktreeDir: t.TempDir()}

	d := New(cfg, bus, tg, pg)

	// Subscribe to escalation events.
	var escalated []events.Event
	bus.Subscribe(events.EventTicketEscalated, func(_ context.Context, e events.Event) {
		escalated = append(escalated, e)
	})

	d.autoMergePR(context.Background(), tk, "https://github.com/test/pr/1")

	if len(escalated) != 1 {
		t.Fatalf("expected 1 escalation event, got %d", len(escalated))
	}
	if escalated[0].Payload["ticket_id"] != "t-esc" {
		t.Errorf("expected ticket_id=t-esc, got %v", escalated[0].Payload["ticket_id"])
	}
	if escalated[0].Payload["source"] != "auto_merge" {
		t.Errorf("expected source=auto_merge, got %v", escalated[0].Payload["source"])
	}
}

func TestWorkStreamCompletionAllDone(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(
		&ticket.Ticket{ID: "t-1", ProjectID: "p-1", State: ticket.StateClosed, WorkStreamID: "ws-1"},
		&ticket.Ticket{ID: "t-2", ProjectID: "p-1", State: ticket.StateClosed, WorkStreamID: "ws-1"},
		&ticket.Ticket{ID: "t-3", ProjectID: "p-1", State: ticket.StateClosed, WorkStreamID: "ws-1"},
	)
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5, WorktreeDir: t.TempDir(), RepoDir: t.TempDir()}

	d := New(cfg, bus, tg, pg)

	// Subscribe to the work_stream.completed event.
	var received []events.Event
	bus.Subscribe(events.EventWorkStreamCompleted, func(_ context.Context, e events.Event) {
		received = append(received, e)
	})

	// Simulate ticket done event.
	d.handleTicketDone(context.Background(), events.Event{
		Type:    events.EventTicketDone,
		Payload: map[string]any{"ticket_id": "t-1"},
	})

	if len(received) != 1 {
		t.Fatalf("expected 1 work_stream.completed event, got %d", len(received))
	}
	if received[0].Payload["work_stream_id"] != "ws-1" {
		t.Errorf("expected work_stream_id=ws-1, got %v", received[0].Payload["work_stream_id"])
	}
	if received[0].Payload["ticket_count"] != 3 {
		t.Errorf("expected ticket_count=3, got %v", received[0].Payload["ticket_count"])
	}
}

func TestWorkStreamCompletionNotAllDone(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(
		&ticket.Ticket{ID: "t-1", ProjectID: "p-1", State: ticket.StateClosed, WorkStreamID: "ws-1"},
		&ticket.Ticket{ID: "t-2", ProjectID: "p-1", State: ticket.StateExecuting, WorkStreamID: "ws-1"},
	)
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5, WorktreeDir: t.TempDir(), RepoDir: t.TempDir()}

	d := New(cfg, bus, tg, pg)

	var received []events.Event
	bus.Subscribe(events.EventWorkStreamCompleted, func(_ context.Context, e events.Event) {
		received = append(received, e)
	})

	d.handleTicketDone(context.Background(), events.Event{
		Type:    events.EventTicketDone,
		Payload: map[string]any{"ticket_id": "t-1"},
	})

	if len(received) != 0 {
		t.Errorf("expected no work_stream.completed event when not all tickets are done, got %d", len(received))
	}
}

func TestWorkStreamCompletionNoWorkStream(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(
		&ticket.Ticket{ID: "t-1", ProjectID: "p-1", State: ticket.StateClosed, WorkStreamID: ""},
	)
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5, WorktreeDir: t.TempDir(), RepoDir: t.TempDir()}

	d := New(cfg, bus, tg, pg)

	var received []events.Event
	bus.Subscribe(events.EventWorkStreamCompleted, func(_ context.Context, e events.Event) {
		received = append(received, e)
	})

	d.handleTicketDone(context.Background(), events.Event{
		Type:    events.EventTicketDone,
		Payload: map[string]any{"ticket_id": "t-1"},
	})

	if len(received) != 0 {
		t.Errorf("expected no work_stream.completed event for ticket without work_stream_id, got %d", len(received))
	}
}

// --- Durable merge state tests ---

// mockOutputPatcher implements TicketOutputPatcher for tests.
type mockOutputPatcher struct {
	mu      sync.Mutex
	patches map[string]map[string]any // ticketID → merged patches
}

func newMockOutputPatcher() *mockOutputPatcher {
	return &mockOutputPatcher{patches: make(map[string]map[string]any)}
}

func (m *mockOutputPatcher) PatchOutputs(_ context.Context, id string, patch map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.patches[id] == nil {
		m.patches[id] = make(map[string]any)
	}
	for k, v := range patch {
		m.patches[id][k] = v
	}
	return nil
}

func (m *mockOutputPatcher) get(id, key string) any {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.patches[id] == nil {
		return nil
	}
	return m.patches[id][key]
}

func TestPersistMergeState(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)
	patcher := newMockOutputPatcher()
	d.SetOutputPatcher(patcher)

	ctx := context.Background()
	d.persistMergeState(ctx, "t-1", 3, "conflict", "merge conflict in main.go")

	if got := patcher.get("t-1", "_merge_attempts"); got != 3 {
		t.Errorf("expected _merge_attempts=3, got %v", got)
	}
	if got := patcher.get("t-1", "_merge_status"); got != "conflict" {
		t.Errorf("expected _merge_status=conflict, got %v", got)
	}
	if got := patcher.get("t-1", "_merge_last_error"); got != "merge conflict in main.go" {
		t.Errorf("expected _merge_last_error set, got %v", got)
	}
	if got := patcher.get("t-1", "_merge_last_attempt_at"); got == nil || got == "" {
		t.Error("expected _merge_last_attempt_at to be set")
	}
}

func TestPersistMergeState_NilPatcher(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)
	// outputPatcher is nil — should not panic.
	d.persistMergeState(context.Background(), "t-1", 1, "merging", "")
}

func TestAutoMergePR_ReadsAttemptsFromOutputs(t *testing.T) {
	bus := events.NewInProcessBus()

	// Ticket with 4 attempts persisted — repo failure should bump to 5 and escalate.
	tk := &ticket.Ticket{
		ID:        "t-4",
		ProjectID: "p-1",
		State:     ticket.StateValidated,
		Outputs: map[string]any{
			"pr_url":          "https://github.com/test/pr/4",
			"_merge_attempts": float64(4),
			"_merge_status":   "conflict",
		},
	}
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5, RepoDir: t.TempDir(), WorktreeDir: t.TempDir()}

	d := New(cfg, bus, tg, pg)
	patcher := newMockOutputPatcher()
	d.SetOutputPatcher(patcher)

	// autoMergePR will fail to resolve repo (no project in mock with repo_url).
	// With 4 prior attempts, failure increments to 5 and triggers escalation.
	d.autoMergePR(context.Background(), tk, "https://github.com/test/pr/4")

	if got := patcher.get("t-4", "_merge_status"); got != "escalated" {
		t.Errorf("expected _merge_status=escalated (5th attempt triggers escalation), got %v", got)
	}
	if got := patcher.get("t-4", "_merge_attempts"); got != 5 {
		t.Errorf("expected _merge_attempts=5, got %v (%T)", got, got)
	}
}

func TestBranchFromOutputs(t *testing.T) {
	tests := []struct {
		name     string
		outputs  map[string]any
		expected string
	}{
		{"with _branch", map[string]any{"_branch": "feature/custom"}, "feature/custom"},
		{"without _branch", map[string]any{}, "ticket/t-1"},
		{"empty _branch", map[string]any{"_branch": ""}, "ticket/t-1"},
		{"nil outputs", nil, "ticket/t-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := &ticket.Ticket{ID: "t-1", Outputs: tt.outputs}
			if tk.Outputs == nil {
				tk.Outputs = make(map[string]any)
			}
			got := branchFromOutputs(tk)
			if got != tt.expected {
				t.Errorf("branchFromOutputs() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestHandleTicketCancelled_CleansUpWorker(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5, WorktreeDir: t.TempDir(), RepoDir: t.TempDir()}

	d := New(cfg, bus, tg, pg)

	// Simulate an active worker.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.mu.Lock()
	d.active["t-cancel"] = cancel
	d.activeProjects["t-cancel"] = "p-1"
	d.mu.Unlock()

	d.handleTicketCancelled(ctx, events.Event{
		Type: events.EventTicketCancelled,
		Payload: map[string]any{
			"ticket_id":  "t-cancel",
			"project_id": "p-1",
		},
	})

	// Worker should be removed from active map.
	d.mu.Lock()
	_, stillActive := d.active["t-cancel"]
	d.mu.Unlock()
	if stillActive {
		t.Error("expected worker to be removed from active map after cancel")
	}
}

func TestEscalateMergeFailure_PersistsState(t *testing.T) {
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter()
	pg := newMockProjectGetter()
	cfg := Config{MaxWorkers: 5}

	d := New(cfg, bus, tg, pg)
	patcher := newMockOutputPatcher()
	d.SetOutputPatcher(patcher)

	var escalated []events.Event
	bus.Subscribe(events.EventTicketEscalated, func(_ context.Context, e events.Event) {
		escalated = append(escalated, e)
	})

	tk := &ticket.Ticket{ID: "t-esc2", ProjectID: "p-1", State: ticket.StateValidated}
	d.escalateMergeFailure(context.Background(), tk, "too many conflicts")

	if got := patcher.get("t-esc2", "_merge_status"); got != "escalated" {
		t.Errorf("expected _merge_status=escalated, got %v", got)
	}
	if got := patcher.get("t-esc2", "_merge_last_error"); got != "too many conflicts" {
		t.Errorf("expected _merge_last_error='too many conflicts', got %v", got)
	}
	if len(escalated) != 1 {
		t.Fatalf("expected 1 escalation event, got %d", len(escalated))
	}
}

func TestHasHigherPriorityWork_ReviewWaiting(t *testing.T) {
	reviewTicket := &ticket.Ticket{
		ID:        "t-review",
		ProjectID: "p-1",
		State:     ticket.StateAwaitingValidation,
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(reviewTicket)
	pg := newMockProjectGetter()
	d := New(Config{MaxWorkers: 5}, bus, tg, pg)

	// Awaiting_review ticket exists and no reviewer active → higher priority work.
	if !d.hasHigherPriorityWork(context.Background(), "p-1") {
		t.Error("expected hasHigherPriorityWork=true when awaiting_review ticket exists")
	}

	// Mark a reviewer as active → no longer higher priority.
	_, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.active["review:t-review"] = cancel
	d.activeProjects["review:t-review"] = "p-1"
	d.mu.Unlock()

	if d.hasHigherPriorityWork(context.Background(), "p-1") {
		t.Error("expected hasHigherPriorityWork=false when reviewer is active")
	}
	cancel()
}

func TestHasHigherPriorityWork_ValidatedPRWaiting(t *testing.T) {
	validatedTicket := &ticket.Ticket{
		ID:        "t-val",
		ProjectID: "p-1",
		State:     ticket.StateValidated,
		Outputs:   map[string]any{"pr_url": "https://github.com/test/repo/pull/1"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(validatedTicket)
	pg := newMockProjectGetter()
	d := New(Config{MaxWorkers: 5}, bus, tg, pg)

	// Validated ticket with PR, no resolver active → higher priority.
	if !d.hasHigherPriorityWork(context.Background(), "p-1") {
		t.Error("expected hasHigherPriorityWork=true when validated ticket with PR exists")
	}

	// Mark resolver as active → no longer higher priority.
	_, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	d.active["resolve:t-val"] = cancel
	d.activeProjects["resolve:t-val"] = "p-1"
	d.mu.Unlock()

	if d.hasHigherPriorityWork(context.Background(), "p-1") {
		t.Error("expected hasHigherPriorityWork=false when resolver is active")
	}
	cancel()
}

func TestHasHigherPriorityWork_NoPriorityWork(t *testing.T) {
	pendingTicket := &ticket.Ticket{
		ID:        "t-pending",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(pendingTicket)
	pg := newMockProjectGetter()
	d := New(Config{MaxWorkers: 5}, bus, tg, pg)

	// Only pending tickets exist → no higher priority work.
	if d.hasHigherPriorityWork(context.Background(), "p-1") {
		t.Error("expected hasHigherPriorityWork=false when only pending tickets exist")
	}
}

func TestTryDispatchDefersWhenHigherPriorityWork(t *testing.T) {
	reviewTicket := &ticket.Ticket{
		ID:        "t-review",
		ProjectID: "p-1",
		State:     ticket.StateAwaitingValidation,
	}
	pendingTicket := &ticket.Ticket{
		ID:        "t-pending",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(reviewTicket, pendingTicket)
	pg := newMockProjectGetter(&project.Project{
		ID:              "p-1",
		RepoURL:         "https://github.com/test/repo.git",
		DispatchEnabled: true,
	})
	d := New(Config{MaxWorkers: 5}, bus, tg, pg)

	// tryDispatch should skip the pending ticket because review work is waiting.
	d.tryDispatch(context.Background(), pendingTicket)

	// activeCount may be 1 because hasHigherPriorityWork spawns a reviewer.
	// The key assertion is that the pending ticket was NOT dispatched.
	d.mu.Lock()
	_, pendingActive := d.active[pendingTicket.ID]
	d.mu.Unlock()
	if pendingActive {
		t.Error("should not dispatch pending ticket when review work is waiting")
	}
}

// --- Input Provided Tests (awaiting_input → re-dispatch) ---

func TestHandleInputProvided_Executing_ReleasesAndDispatches(t *testing.T) {
	// Happy path: ticket in executing (stale lease, no active worker) →
	// ForceReleaseLease → re-read as draft → tryDispatch → spawn.
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git", DispatchEnabled: true}
	tk := &ticket.Ticket{
		ID:        "t-input",
		ProjectID: "p-1",
		State:     ticket.StateExecuting,
		Title:     "input provided test",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	worker := &mockWorker{}
	cfg := Config{MaxWorkers: 5, ProjectID: "p-1", WorktreeDir: t.TempDir(), ServerURL: "http://localhost"}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	d.SetLeaseReleaser(releaser)
	seedTestClone(t, d, "p-1")

	// After ForceReleaseLease, the re-read should return draft.
	releaserCalled := false
	originalRelease := releaser.ForceReleaseLease
	_ = originalRelease // use the mock directly
	releaser.err = nil

	// Simulate: ForceReleaseLease transitions ticket to draft.
	origForce := d.leaseReleaser
	d.leaseReleaser = &mockLeaseReleaser{
		err: nil,
	}
	// Wire up: when ForceReleaseLease is called, update mock ticket to draft.
	d.leaseReleaser = &leaseReleaserFunc{fn: func(_ context.Context, id string) error {
		releaserCalled = true
		tg.mu.Lock()
		tk.State = ticket.StateDraft
		tg.mu.Unlock()
		return nil
	}}
	_ = origForce

	ctx := context.Background()
	d.handleTicketInputProvided(ctx, events.Event{
		Type:    events.EventTicketInputProvided,
		Payload: map[string]any{"ticket_id": "t-input"},
	})

	// Wait for spawn goroutine.
	d.wg.Wait()

	if !releaserCalled {
		t.Error("expected ForceReleaseLease to be called")
	}
	if worker.callCount() != 1 {
		t.Errorf("expected 1 worker spawn after input_provided, got %d", worker.callCount())
	}
}

func TestHandleInputProvided_AlreadyDraft_DispatchesDirectly(t *testing.T) {
	// Lease expired before event arrived — ticket already draft → tryDispatch directly.
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git", DispatchEnabled: true}
	tk := &ticket.Ticket{
		ID:        "t-draft-input",
		ProjectID: "p-1",
		State:     ticket.StateDraft,
		Title:     "already draft",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	worker := &mockWorker{}
	cfg := Config{MaxWorkers: 5, ProjectID: "p-1", WorktreeDir: t.TempDir(), ServerURL: "http://localhost"}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	d.SetLeaseReleaser(releaser)
	seedTestClone(t, d, "p-1")

	ctx := context.Background()
	d.handleTicketInputProvided(ctx, events.Event{
		Type:    events.EventTicketInputProvided,
		Payload: map[string]any{"ticket_id": "t-draft-input"},
	})

	d.wg.Wait()

	// No lease release needed — ticket was already draft.
	if releaser.callCount() != 0 {
		t.Errorf("expected 0 ForceReleaseLease calls for already-draft ticket, got %d", releaser.callCount())
	}
	if worker.callCount() != 1 {
		t.Errorf("expected 1 worker spawn for draft ticket, got %d", worker.callCount())
	}
}

func TestHandleInputProvided_WorkerAlreadyActive_Skips(t *testing.T) {
	// At-least-once guard: if a worker is already running for this ticket, skip.
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git", DispatchEnabled: true}
	tk := &ticket.Ticket{
		ID:        "t-active-input",
		ProjectID: "p-1",
		State:     ticket.StateExecuting,
		Title:     "worker active",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	worker := &mockWorker{}
	cfg := Config{MaxWorkers: 5, ProjectID: "p-1", RepoDir: "/tmp", ServerURL: "http://localhost"}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	d.SetLeaseReleaser(releaser)

	// Simulate active worker.
	d.mu.Lock()
	d.active["t-active-input"] = func() {}
	d.mu.Unlock()

	ctx := context.Background()
	d.handleTicketInputProvided(ctx, events.Event{
		Type:    events.EventTicketInputProvided,
		Payload: map[string]any{"ticket_id": "t-active-input"},
	})

	if releaser.callCount() != 0 {
		t.Errorf("expected 0 ForceReleaseLease calls when worker active, got %d", releaser.callCount())
	}
	if worker.callCount() != 0 {
		t.Errorf("expected 0 worker spawns when worker active, got %d", worker.callCount())
	}
}

func TestHandleInputProvided_WrongProject_Filtered(t *testing.T) {
	// Ticket belongs to a different project — should be filtered out.
	proj := &project.Project{ID: "p-other", Name: "other", RepoURL: "https://github.com/test/repo.git", DispatchEnabled: true}
	tk := &ticket.Ticket{
		ID:        "t-wrong-proj",
		ProjectID: "p-other",
		State:     ticket.StateExecuting,
		Title:     "wrong project",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	worker := &mockWorker{}
	cfg := Config{MaxWorkers: 5, ProjectID: "p-1", RepoDir: "/tmp", ServerURL: "http://localhost"}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	d.SetLeaseReleaser(releaser)

	ctx := context.Background()
	d.handleTicketInputProvided(ctx, events.Event{
		Type:    events.EventTicketInputProvided,
		Payload: map[string]any{"ticket_id": "t-wrong-proj"},
	})

	if releaser.callCount() != 0 {
		t.Errorf("expected 0 ForceReleaseLease calls for wrong project, got %d", releaser.callCount())
	}
	if worker.callCount() != 0 {
		t.Errorf("expected 0 worker spawns for wrong project, got %d", worker.callCount())
	}
}

func TestHandleInputProvided_UnexpectedState_Skips(t *testing.T) {
	// Ticket in closed state — log warning and skip.
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git", DispatchEnabled: true}
	tk := &ticket.Ticket{
		ID:        "t-closed-input",
		ProjectID: "p-1",
		State:     ticket.StateClosed,
		Title:     "closed ticket",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	worker := &mockWorker{}
	cfg := Config{MaxWorkers: 5, ProjectID: "p-1", RepoDir: "/tmp", ServerURL: "http://localhost"}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	d.SetLeaseReleaser(releaser)

	ctx := context.Background()
	d.handleTicketInputProvided(ctx, events.Event{
		Type:    events.EventTicketInputProvided,
		Payload: map[string]any{"ticket_id": "t-closed-input"},
	})

	if releaser.callCount() != 0 {
		t.Errorf("expected 0 ForceReleaseLease calls for closed ticket, got %d", releaser.callCount())
	}
	if worker.callCount() != 0 {
		t.Errorf("expected 0 worker spawns for closed ticket, got %d", worker.callCount())
	}
}

func TestHandleInputProvided_ReleaserFails_NoDispatch(t *testing.T) {
	// ForceReleaseLease fails — should not proceed to tryDispatch.
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git", DispatchEnabled: true}
	tk := &ticket.Ticket{
		ID:        "t-release-fail",
		ProjectID: "p-1",
		State:     ticket.StateExecuting,
		Title:     "release fails",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{err: fmt.Errorf("redis unavailable")}
	worker := &mockWorker{}
	cfg := Config{MaxWorkers: 5, ProjectID: "p-1", RepoDir: "/tmp", ServerURL: "http://localhost"}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	d.SetLeaseReleaser(releaser)

	ctx := context.Background()
	d.handleTicketInputProvided(ctx, events.Event{
		Type:    events.EventTicketInputProvided,
		Payload: map[string]any{"ticket_id": "t-release-fail"},
	})

	if releaser.callCount() != 1 {
		t.Errorf("expected 1 ForceReleaseLease call, got %d", releaser.callCount())
	}
	if worker.callCount() != 0 {
		t.Errorf("expected 0 worker spawns after release failure, got %d", worker.callCount())
	}
}

func TestHandleInputProvided_NilReleaser_NoPanic(t *testing.T) {
	// When leaseReleaser is nil, should gracefully return without panic.
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git", DispatchEnabled: true}
	tk := &ticket.Ticket{
		ID:        "t-nil-release",
		ProjectID: "p-1",
		State:     ticket.StateExecuting,
		Title:     "nil releaser",
		Type:      ticket.TypeTask,
		Objective: ticket.Objective{Description: "d"},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	worker := &mockWorker{}
	cfg := Config{MaxWorkers: 5, ProjectID: "p-1", RepoDir: "/tmp", ServerURL: "http://localhost"}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	// Note: NOT setting leaseReleaser — it's nil.

	ctx := context.Background()
	// Should not panic.
	d.handleTicketInputProvided(ctx, events.Event{
		Type:    events.EventTicketInputProvided,
		Payload: map[string]any{"ticket_id": "t-nil-release"},
	})

	if worker.callCount() != 0 {
		t.Errorf("expected 0 worker spawns with nil releaser, got %d", worker.callCount())
	}
}

// leaseReleaserFunc adapts a function to the LeaseReleaser interface for tests.
type leaseReleaserFunc struct {
	fn func(ctx context.Context, ticketID string) error
}

func (f *leaseReleaserFunc) ForceReleaseLease(ctx context.Context, ticketID string) error {
	return f.fn(ctx, ticketID)
}

// --- Crash Loop Escalation Tests ---

// mockDispatchFailureSummarizer records AppendFailureSummary calls for assertions.
type mockDispatchFailureSummarizer struct {
	mu    sync.Mutex
	calls []string // ticket IDs
}

func (m *mockDispatchFailureSummarizer) AppendFailureSummary(_ context.Context, ticketID, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, ticketID)
	return nil
}

func TestHandleWorkerExit_CrashLoop_EscalatesToAwaitingInput(t *testing.T) {
	// Simulate: worker has crashed maxWorkerAttempts times (prior_attempts already has
	// maxWorkerAttempts-1 worker_exit entries). On this exit, the dispatcher should
	// escalate to awaiting_input instead of releasing the lease back to draft.
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}

	// Build prior_attempts with maxWorkerAttempts-1 worker_exit entries.
	var priorAttempts []ticket.AttemptSummary
	for i := 0; i < maxWorkerAttempts-1; i++ {
		priorAttempts = append(priorAttempts, ticket.AttemptSummary{
			AgentID: "system",
			Outcome: "worker_exit",
			Summary: "Worker exited without submitting",
		})
	}

	tk := &ticket.Ticket{
		ID:         "t-loop",
		ProjectID:  "p-1",
		State:      ticket.StateDraft,
		Title:      "crash loop test",
		Type:       ticket.TypeTask,
		AssignedTo: "agent-1",
		Objective:  ticket.Objective{Description: "d"},
		Context:    ticket.TicketContext{PriorAttempts: priorAttempts},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	fs := &mockDispatchFailureSummarizer{}
	trans := &mockTicketTransitioner{}
	worker := &mockWorker{
		spawnFunc: func(ctx context.Context, _, _, _, _, _, _ string) (*WorkerResult, error) {
			return nil, fmt.Errorf("process killed")
		},
	}

	cfg := Config{
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	d.SetLeaseReleaser(releaser)
	d.SetFailureSummarizer(fs)
	d.SetTicketTransitioner(trans)

	// Set ticket to executing (simulating that claim happened).
	tg.mu.Lock()
	tk.State = ticket.StateExecuting
	tg.mu.Unlock()

	d.spawn(context.Background(), tk)
	d.wg.Wait()

	// Should have escalated via TransitionTicket, NOT released the lease.
	if releaser.callCount() != 0 {
		t.Errorf("expected 0 ForceReleaseLease calls (escalated instead), got %d", releaser.callCount())
	}
	if trans.transitionCount() != 1 {
		t.Fatalf("expected 1 transition (escalate), got %d", trans.transitionCount())
	}
	triggers := trans.triggers()
	if triggers[0] != ticket.TriggerEscalate {
		t.Errorf("expected trigger %q, got %q", ticket.TriggerEscalate, triggers[0])
	}
}

func TestHandleWorkerExit_BelowMaxAttempts_ReleasesLease(t *testing.T) {
	// Simulate: worker has crashed once before (1 prior worker_exit entry).
	// On this exit, the dispatcher should still release the lease for retry
	// since we're below maxWorkerAttempts.
	proj := &project.Project{ID: "p-1", Name: "test", RepoURL: "https://github.com/test/repo.git"}
	tk := &ticket.Ticket{
		ID:         "t-retry",
		ProjectID:  "p-1",
		State:      ticket.StateDraft,
		Title:      "retry test",
		Type:       ticket.TypeTask,
		AssignedTo: "agent-1",
		Objective:  ticket.Objective{Description: "d"},
		Context: ticket.TicketContext{
			PriorAttempts: []ticket.AttemptSummary{
				{AgentID: "system", Outcome: "worker_exit", Summary: "crash 1"},
			},
		},
	}
	bus := events.NewInProcessBus()
	tg := newMockTicketGetter(tk)
	pg := newMockProjectGetter(proj)

	releaser := &mockLeaseReleaser{}
	fs := &mockDispatchFailureSummarizer{}
	trans := &mockTicketTransitioner{}
	worker := &mockWorker{
		spawnFunc: func(ctx context.Context, _, _, _, _, _, _ string) (*WorkerResult, error) {
			return nil, fmt.Errorf("process killed")
		},
	}

	cfg := Config{
		MaxWorkers: 5,
		RepoDir:    "/tmp",
		ServerURL:  "http://localhost",
	}

	d := New(cfg, bus, tg, pg)
	d.skipWorktrees = true
	d.worker = worker
	d.SetLeaseReleaser(releaser)
	d.SetFailureSummarizer(fs)
	d.SetTicketTransitioner(trans)

	tg.mu.Lock()
	tk.State = ticket.StateExecuting
	tg.mu.Unlock()

	d.spawn(context.Background(), tk)
	d.wg.Wait()

	// Should have released the lease for retry (not escalated).
	if releaser.callCount() != 1 {
		t.Errorf("expected 1 ForceReleaseLease call, got %d", releaser.callCount())
	}
	if trans.transitionCount() != 0 {
		t.Errorf("expected 0 transitions (below max attempts), got %d", trans.transitionCount())
	}
}
