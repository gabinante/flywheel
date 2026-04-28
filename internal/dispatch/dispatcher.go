package dispatch

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/cost"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/workflow"
)

// TicketGetter retrieves tickets and their dependencies.
type TicketGetter interface {
	GetTicket(ctx context.Context, id string) (*ticket.Ticket, error)
	GetTicketsByIDs(ctx context.Context, ids []string) ([]*ticket.Ticket, error)
	ListByState(ctx context.Context, projectID string, state ticket.State) ([]*ticket.Ticket, error)
	ListByWorkStream(ctx context.Context, projectID string, workStreamID string) ([]*ticket.Ticket, error)
}

// ProjectGetter retrieves projects.
type ProjectGetter interface {
	GetProject(ctx context.Context, id string) (*project.Project, error)
}

// RepoResolver resolves the repository URL and local path for a ticket's target repo.
// Returns repoURL, defaultBranch, error. Used for multi-repo project support.
type RepoResolver interface {
	ResolveRepo(ctx context.Context, projectID, targetRepo string) (repoURL, defaultBranch string, err error)
}

// LeaseReleaser releases a ticket's lease and transitions it back to draft.
// This is Layer 1 of failure recovery: immediate cleanup on worker process exit.
type LeaseReleaser interface {
	ForceReleaseLease(ctx context.Context, ticketID string) error
}

// TicketTransitioner applies state transitions to tickets.
// Matches the ticket.Service.TransitionTicket signature.
type TicketTransitioner interface {
	TransitionTicket(ctx context.Context, id string, trigger string, actor ticket.Actor, payload map[string]any) error
}

// TraceAppender persists server-side execution trace steps.
type TraceAppender interface {
	AppendSystemStep(ctx context.Context, ticketID string, step execution.Step) error
}

// maxMergeAttempts is the number of merge failures before escalating to a human.
const maxMergeAttempts = 5

// Config holds dispatcher settings.
type Config struct {
	MaxWorkers        int
	ClaudePath        string
	WorktreeDir       string
	RepoDir           string        // path to the main git repository
	ServerURL         string        // Flywheel server URL for MCP connections
	AgentID           string        // agent identity for workers
	APIKey            string        // Flywheel API key for worker MCP authentication
	ProjectID         string        // only dispatch tickets for this project (empty = all)
	AutoApprove       bool          // auto-approve tickets when acceptance tests pass
	ReconcileInterval time.Duration // periodic reconciliation interval (default: 60s)
	AgentRunner       string        // execution backend: cli, docker, openai-responses, or openai-compatible
	// Agent driver selection.
	AgentDriver          string   // driver name: "claude" (default), "generic", or custom
	AgentCLIPath         string   // override CLI path for the agent binary
	AgentArgs            []string // extra static arguments for the agent command
	AgentModel           string   // API-native model name (used by API-native runners)
	AgentReasoningEffort string   // API-native reasoning effort
	AgentAPIBaseURL      string   // API-native base URL
	// Docker isolation settings.
	DockerEnabled  bool
	DockerImage    string
	DockerMemory   string
	DockerCPUs     string
	DockerFirewall bool
	AgentAPIKey    string
	CostSvc        *cost.Service
	TraceSvc       TraceAppender
}

// Dispatcher listens for ticket events and spawns workers.
// Supports both the legacy Bus interface (exact Subscribe) and the new DurableEventBus
// (pattern-based SubscribePattern) for at-least-once delivery.
type Dispatcher struct {
	cfg                Config
	bus                events.Bus
	durableBus         events.DurableEventBus // nil if bus doesn't support durability
	tickets            TicketGetter
	projects           ProjectGetter
	worker             Worker
	workerRouter       *ProjectWorkerRouter
	worktrees          *WorktreeManager
	clones             *MultiRepoCloneManager // nil-safe: only used for multi-repo projects
	repoResolver       RepoResolver           // nil-safe: only used for multi-repo projects
	leaseReleaser      LeaseReleaser          // nil-safe: if nil, worker exit does not release lease (Layer 2 TTL handles it)
	ticketTransitioner TicketTransitioner     // nil-safe: if nil, merged tickets are not auto-closed
	workflowEngine     *workflow.Engine       // nil-safe: if nil, workflow-aware dispatching is disabled

	mu             sync.Mutex
	active         map[string]context.CancelFunc // ticketID/role-prefixed key → cancel
	activeProjects map[string]string             // active key → projectID
	mergeAttempts  map[string]int                // ticketID → failed merge count
	wg             sync.WaitGroup
	scanning       int32              // atomic CAS guard for reconcile
	stopCancel     context.CancelFunc // cancels the internal context on Stop()
}

// New creates a dispatcher that subscribes to the event bus.
func New(cfg Config, bus events.Bus, tickets TicketGetter, projects ProjectGetter) *Dispatcher {
	d := &Dispatcher{
		cfg:          cfg,
		bus:          bus,
		tickets:      tickets,
		projects:     projects,
		worker:       NewWorker(cfg),
		workerRouter: NewProjectWorkerRouter(cfg),
		worktrees: &WorktreeManager{
			BaseDir: cfg.WorktreeDir,
			RepoDir: cfg.RepoDir,
		},
		clones:         NewMultiRepoCloneManager(filepath.Join(cfg.WorktreeDir, ".clones")),
		active:         make(map[string]context.CancelFunc),
		activeProjects: make(map[string]string),
		mergeAttempts:  make(map[string]int),
	}
	// Detect if bus supports durable event delivery.
	if durable, ok := bus.(events.DurableEventBus); ok {
		d.durableBus = durable
	}
	return d
}

// Start subscribes to events and begins dispatching. Call Stop to shut down.
// When a DurableEventBus is available, uses pattern-based subscriptions with
// at-least-once delivery guarantees. Falls back to exact subscriptions on legacy Bus.
func (d *Dispatcher) Start(ctx context.Context) {
	// Derive an internal context so Stop() can cancel background goroutines
	// even if the caller's context is still alive.
	ctx, d.stopCancel = context.WithCancel(ctx)

	if d.durableBus != nil {
		// Use pattern-based subscriptions for durable delivery.
		_ = d.durableBus.SubscribePattern("ticket.created", "dispatcher:ready", func(_ context.Context, e events.Event) {
			d.handleTicketReady(ctx, e)
		})
		_ = d.durableBus.SubscribePattern("ticket.unblocked", "dispatcher:ready", func(_ context.Context, e events.Event) {
			d.handleTicketReady(ctx, e)
		})
		_ = d.durableBus.SubscribePattern("ticket.rejected", "dispatcher:rejected", func(_ context.Context, e events.Event) {
			d.handleTicketRejected(ctx, e)
		})
		_ = d.durableBus.SubscribePattern("ticket.submitted", "dispatcher:submitted", func(_ context.Context, e events.Event) {
			d.handleTicketSubmitted(ctx, e)
		})
		_ = d.durableBus.SubscribePattern("ticket.closed", "dispatcher:done", func(_ context.Context, e events.Event) {
			d.handleTicketDone(ctx, e)
		})
		_ = d.durableBus.SubscribePattern("ticket.approved", "dispatcher:done", func(_ context.Context, e events.Event) {
			d.handleTicketDone(ctx, e)
		})
		_ = d.durableBus.SubscribePattern("tests.failed", "dispatcher:tests-failed", func(_ context.Context, e events.Event) {
			d.handleTestsFailed(ctx, e)
		})
		_ = d.durableBus.SubscribePattern("ticket.rolled_back", "dispatcher:rolled-back", func(_ context.Context, e events.Event) {
			d.handleTicketRolledBack(ctx, e)
		})
	} else {
		// Legacy exact subscriptions (backward compatible).
		d.bus.Subscribe(events.EventTicketCreated, func(_ context.Context, e events.Event) {
			d.handleTicketReady(ctx, e)
		})
		d.bus.Subscribe(events.EventTicketUnblocked, func(_ context.Context, e events.Event) {
			d.handleTicketReady(ctx, e)
		})
		d.bus.Subscribe(events.EventTicketRejected, func(_ context.Context, e events.Event) {
			d.handleTicketRejected(ctx, e)
		})
		d.bus.Subscribe(events.EventTicketSubmitted, func(_ context.Context, e events.Event) {
			d.handleTicketSubmitted(ctx, e)
		})
		d.bus.Subscribe(events.EventTicketDone, func(_ context.Context, e events.Event) {
			d.handleTicketDone(ctx, e)
		})
		d.bus.Subscribe(events.EventTicketApproved, func(_ context.Context, e events.Event) {
			d.handleTicketDone(ctx, e)
		})
		d.bus.Subscribe(events.EventTestsFailed, func(_ context.Context, e events.Event) {
			d.handleTestsFailed(ctx, e)
		})
		d.bus.Subscribe(events.EventTicketRolledBack, func(_ context.Context, e events.Event) {
			d.handleTicketRolledBack(ctx, e)
		})
	}

	slog.Info("dispatch started", "max_workers", d.cfg.MaxWorkers, "worktree_dir", d.cfg.WorktreeDir, "project", d.cfg.ProjectID, "durable", d.durableBus != nil)

	// Scan for existing pending tickets on startup.
	go d.reconcile(ctx)

	// Periodic reconciliation: retry validated tickets with pending CI, pick up
	// any tickets that fell through the cracks between events.
	reconcileInterval := d.cfg.ReconcileInterval
	if reconcileInterval <= 0 {
		reconcileInterval = 60 * time.Second
	}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		ticker := time.NewTicker(reconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.reconcile(ctx)
			}
		}
	}()
}

// Stop cancels all active workers and background goroutines, then waits for completion.
func (d *Dispatcher) Stop() {
	if d.stopCancel != nil {
		d.stopCancel()
	}
	d.mu.Lock()
	for key, cancel := range d.active {
		cancel()
		delete(d.activeProjects, key)
	}
	d.mu.Unlock()
	d.wg.Wait()
	slog.Info("dispatch stopped")
}

// SetLeaseReleaser configures the lease releaser for immediate cleanup on worker exit.
// Call this after construction to wire the queue service without circular imports.
func (d *Dispatcher) SetLeaseReleaser(lr LeaseReleaser) {
	d.leaseReleaser = lr
}

// SetTicketTransitioner configures the ticket transitioner for post-merge lifecycle.
// Call this after construction to wire the ticket service without circular imports.
func (d *Dispatcher) SetTicketTransitioner(tt TicketTransitioner) {
	d.ticketTransitioner = tt
}

// SetRepoResolver configures multi-repo resolution. When set, tickets with
// target_repo are resolved to the correct repository for worktree creation.
// SetWorkflowEngine sets the optional workflow engine for workflow-aware dispatching.
func (d *Dispatcher) SetWorkflowEngine(we *workflow.Engine) {
	d.workflowEngine = we
}

func (d *Dispatcher) SetRepoResolver(rr RepoResolver) {
	d.repoResolver = rr
}

// reconcile wraps scanPending with an atomic CAS to prevent concurrent runs.
func (d *Dispatcher) reconcile(ctx context.Context) bool {
	if !atomic.CompareAndSwapInt32(&d.scanning, 0, 1) {
		return false
	}
	defer atomic.StoreInt32(&d.scanning, 0)
	d.scanPending(ctx)
	return true
}

func (d *Dispatcher) scanPending(ctx context.Context) {
	// Small delay to let the server finish starting.
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Second):
	}

	// Reviewers first — finish in-progress work before starting new work.
	// This handles tickets stuck in awaiting_review when the reviewer was
	// deferred due to capacity (worker still occupied the slot at submit time).
	reviewing, err := d.tickets.ListByState(ctx, d.cfg.ProjectID, ticket.StateAwaitingReview)
	if err != nil {
		slog.Error("dispatch: scan awaiting_review failed", "error", err)
	} else {
		for _, t := range reviewing {
			if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
				continue
			}
			d.spawnReviewer(ctx, t)
		}
	}

	// Scan for validated tickets with unmerged PRs — try merge or spawn resolver.
	validated, err := d.tickets.ListByState(ctx, d.cfg.ProjectID, ticket.StateValidated)
	if err != nil {
		slog.Error("dispatch: scan validated failed", "error", err)
	} else {
		for _, t := range validated {
			if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
				continue
			}
			prURL, ok := t.Outputs["pr_url"].(string)
			if !ok || prURL == "" {
				continue
			}
			d.mu.Lock()
			_, resolving := d.active["resolve:"+t.ID]
			d.mu.Unlock()
			if resolving {
				continue
			}
			d.autoMergePR(ctx, t, prURL)
		}
	}

	pending, err := d.tickets.ListByState(ctx, d.cfg.ProjectID, ticket.StatePending)
	if err != nil {
		slog.Error("dispatch: scan pending failed", "error", err)
		return
	}
	slog.Info("dispatch: scan found pending tickets", "count", len(pending))
	for _, t := range pending {
		if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
			continue
		}
		d.tryDispatch(ctx, t)
	}
}

// isProjectDispatchEnabled checks whether dispatch is enabled for a project.
// Returns true if the project cannot be found (fail-open for backward compat
// with the legacy DISPATCH_PROJECT_ID single-project approach).
func (d *Dispatcher) isProjectDispatchEnabled(ctx context.Context, projectID string) bool {
	proj, err := d.projects.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		return true // fail-open: don't block dispatch if project lookup fails
	}
	return proj.DispatchEnabled
}

// projectHasRepo checks whether a project has a repo_url configured.
// Returns false if the project has no repo — workers must not execute against
// the server's own codebase when no target repo is defined.
func (d *Dispatcher) projectHasRepo(ctx context.Context, projectID string) bool {
	proj, err := d.projects.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		return false // fail-closed: don't dispatch if we can't verify the repo
	}
	return proj.RepoURL != ""
}

// resolveProjectRepoDir returns a local directory for the project's git repo.
// Uses the clone manager when available; falls back to d.cfg.RepoDir for legacy
// single-project deployments.
func (d *Dispatcher) resolveProjectRepoDir(ctx context.Context, projectID string) (string, error) {
	proj, err := d.projects.GetProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("get project: %w", err)
	}
	if proj.RepoURL == "" {
		return "", fmt.Errorf("project %s has no repo_url configured", projectID)
	}
	if d.clones != nil {
		return d.clones.EnsureClone(proj.RepoURL, proj.ID)
	}
	return d.cfg.RepoDir, nil
}

func (d *Dispatcher) handleTicketReady(ctx context.Context, e events.Event) {
	ticketID, _ := e.Payload["ticket_id"].(string)
	if ticketID == "" {
		return
	}

	// Verify ticket is eligible (pending + dependencies met).
	t, err := d.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		slog.Error("dispatch: get ticket failed", "ticket", ticketID, "error", err)
		return
	}

	// Filter by project if configured (legacy env var approach).
	if d.cfg.ProjectID != "" && t.ProjectID != d.cfg.ProjectID {
		return
	}

	// Check per-project dispatch toggle.
	if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
		return
	}

	d.tryDispatch(ctx, t)
}

// handleTicketRejected re-spawns a worker when a reviewer rejects a ticket.
// The ticket is in executing state (the reject transition goes awaiting_review → executing),
// so the worker can pick it up with prior_attempts context.
func (d *Dispatcher) handleTicketRejected(ctx context.Context, e events.Event) {
	ticketID, _ := e.Payload["ticket_id"].(string)
	if ticketID == "" {
		return
	}
	t, err := d.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		slog.Error("dispatch: rejected get ticket failed", "ticket", ticketID, "error", err)
		return
	}
	if d.cfg.ProjectID != "" && t.ProjectID != d.cfg.ProjectID {
		return
	}
	if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
		return
	}
	if t.State != ticket.StateExecuting {
		return
	}
	slog.Info("dispatch: ticket rejected, re-spawning worker", "ticket", ticketID)
	d.spawn(ctx, t)
}

// handleTestsFailed is called when CI checks fail on a validated ticket's PR.
// It logs the failure for visibility. The ticket remains in validated state —
// the next scan cycle will retry after the branch is fixed.
func (d *Dispatcher) handleTestsFailed(ctx context.Context, e events.Event) {
	ticketID, _ := e.Payload["ticket_id"].(string)
	prURL, _ := e.Payload["pr_url"].(string)
	output, _ := e.Payload["output"].(string)
	slog.Error("dispatch: tests failed", "ticket", ticketID, "pr_url", prURL, "output", output)

	// Spawn a conflict resolver to fix the build — the branch likely needs
	// a rebase or build fix after main diverged.
	if ticketID == "" {
		return
	}
	t, err := d.tickets.GetTicket(ctx, ticketID)
	if err != nil || t == nil {
		return
	}
	if prURL != "" {
		d.spawnConflictResolver(ctx, t, prURL)
	}
}

// handleTicketRolledBack cancels any active worker and cleans up when a ticket is rolled back.
func (d *Dispatcher) handleTicketRolledBack(_ context.Context, e events.Event) {
	ticketID, _ := e.Payload["ticket_id"].(string)
	if ticketID == "" {
		return
	}

	// Cancel any active worker for this ticket.
	d.mu.Lock()
	if cancel, ok := d.active[ticketID]; ok {
		cancel()
		delete(d.active, ticketID)
		delete(d.activeProjects, ticketID)
	}
	// Also cancel reviewer if running.
	reviewKey := "review:" + ticketID
	if cancel, ok := d.active[reviewKey]; ok {
		cancel()
		delete(d.active, reviewKey)
		delete(d.activeProjects, reviewKey)
	}
	// Also cancel conflict resolver if running.
	resolveKey := "resolve:" + ticketID
	if cancel, ok := d.active[resolveKey]; ok {
		cancel()
		delete(d.active, resolveKey)
		delete(d.activeProjects, resolveKey)
	}
	d.mu.Unlock()

	// Worktree cleanup is handled by the rollback service, but clean up
	// any remaining worktree as a safety net.
	if err := d.worktrees.Remove(ticketID); err != nil {
		// Not critical — the rollback service may have already removed it.
		slog.Warn("dispatch: rollback worktree cleanup", "ticket", ticketID, "error", err)
	}

	slog.Info("dispatch: ticket rolled back", "ticket", ticketID)
}

func (d *Dispatcher) tryDispatch(ctx context.Context, t *ticket.Ticket) {
	if t.State != ticket.StatePending {
		return
	}

	// Safety: refuse to dispatch tickets for projects with no repo configured.
	if !d.projectHasRepo(ctx, t.ProjectID) {
		slog.Error("dispatch: project has no repo_url, skipping ticket", "ticket", t.ID, "project", t.ProjectID)
		return
	}

	// Check capacity.
	active, limit, hasCapacity := d.projectCapacity(ctx, t.ProjectID)
	if !hasCapacity {
		slog.Warn("dispatch: at capacity, skipping", "active", active, "max", limit, "ticket", t.ID, "project", t.ProjectID)
		return
	}
	d.mu.Lock()
	if _, running := d.active[t.ID]; running {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()

	if len(t.DependsOn) > 0 {
		deps, err := d.tickets.GetTicketsByIDs(ctx, t.DependsOn)
		if err != nil {
			slog.Error("dispatch: get deps failed", "ticket", t.ID, "error", err)
			return
		}
		for _, dep := range deps {
			if dep.State != ticket.StateDone && dep.State != ticket.StateValidated {
				return
			}
		}
	}

	d.spawn(ctx, t)
}

// handleTicketSubmitted fires when a worker submits a ticket (→ awaiting_review).
// It spawns a reviewer agent to check the PR and approve or reject.
func (d *Dispatcher) handleTicketSubmitted(ctx context.Context, e events.Event) {
	ticketID, _ := e.Payload["ticket_id"].(string)
	if ticketID == "" {
		return
	}
	t, err := d.tickets.GetTicket(ctx, ticketID)
	if err != nil || t == nil {
		slog.Error("dispatch: reviewer get ticket failed", "ticket", ticketID, "error", err)
		return
	}
	if d.cfg.ProjectID != "" && t.ProjectID != d.cfg.ProjectID {
		return
	}
	if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
		return
	}
	if t.State != ticket.StateAwaitingReview {
		return
	}
	d.spawnReviewer(ctx, t)
}

func (d *Dispatcher) handleTicketDone(ctx context.Context, e events.Event) {
	ticketID, _ := e.Payload["ticket_id"].(string)
	if ticketID == "" {
		return
	}

	// Cancel any active worker/reviewer for this ticket.
	d.mu.Lock()
	if cancel, ok := d.active[ticketID]; ok {
		cancel()
		delete(d.active, ticketID)
		delete(d.activeProjects, ticketID)
	}
	d.mu.Unlock()

	// Auto-merge the PR if outputs contain a pr_url.
	t, err := d.tickets.GetTicket(ctx, ticketID)
	if err == nil && t != nil {
		if prURL, ok := t.Outputs["pr_url"].(string); ok && prURL != "" {
			d.autoMergePR(ctx, t, prURL)
		}
	}

	if err := d.worktrees.Remove(ticketID); err != nil {
		slog.Warn("dispatch: worktree cleanup failed", "ticket", ticketID, "error", err)
	}

	// Check work stream completion.
	if t != nil && t.WorkStreamID != "" {
		d.checkWorkStreamCompletion(ctx, t)
	}
}

// checkWorkStreamCompletion checks whether all tickets in the work stream are
// done (closed). If so, it logs a completion message — the foundation for
// coordinator follow-up work.
func (d *Dispatcher) checkWorkStreamCompletion(ctx context.Context, completed *ticket.Ticket) {
	tickets, err := d.tickets.ListByWorkStream(ctx, completed.ProjectID, completed.WorkStreamID)
	if err != nil {
		slog.Error("dispatch: work stream completion check failed", "work_stream", completed.WorkStreamID, "error", err)
		return
	}
	if len(tickets) == 0 {
		return
	}

	for _, t := range tickets {
		if t.State != ticket.StateClosed {
			return
		}
	}

	slog.Info("dispatch: work stream complete", "work_stream", completed.WorkStreamID, "ticket_count", len(tickets), "project", completed.ProjectID)
	d.bus.Publish(ctx, events.Event{
		Type: events.EventWorkStreamCompleted,
		Payload: map[string]any{
			"work_stream_id": completed.WorkStreamID,
			"project_id":     completed.ProjectID,
			"ticket_count":   len(tickets),
		},
	})
}

func (d *Dispatcher) spawn(ctx context.Context, t *ticket.Ticket) {
	// Register as active.
	workerCtx, _, active, limit, started := d.startActive(ctx, t.ID, t.ProjectID)
	if !started {
		slog.Info("dispatch: at capacity, deferring worker", "ticket", t.ID, "project", t.ProjectID, "active", active, "max", limit)
		return
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			delete(d.active, t.ID)
			delete(d.activeProjects, t.ID)
			d.mu.Unlock()

			// Re-scan for pending tickets to fill the freed slot.
			go d.reconcile(ctx)
		}()

		if err := d.runWorker(workerCtx, t); err != nil {
			slog.Error("dispatch: worker failed", "ticket", t.ID, "error", err)
		}

		// Layer 1 failure recovery: if the worker exited without submitting or
		// escalating, immediately release the lease so the ticket returns to
		// draft (pending) for retry. This avoids waiting for TTL expiry (Layer 2).
		d.handleWorkerExit(t.ID)
	}()

	slog.Info("dispatch: spawned worker", "ticket", t.ID, "project", t.ProjectID, "active", active, "max", limit)
}

// DetermineWorkerType selects the appropriate worker type based on ticket state
// and context. This is the routing logic that decides what kind of agent to spawn.
func DetermineWorkerType(t *ticket.Ticket) WorkerType {
	// Check if the ticket has an explicit worker_type in its inputs.
	if wt, ok := t.Inputs["worker_type"].(string); ok {
		parsed := WorkerType(wt)
		if parsed.IsValid() {
			return parsed
		}
	}

	// Route based on ticket state and type.
	switch t.State {
	case ticket.StateAwaitingValidation: // also matches StateAwaitingReview (alias)
		return WorkerTypeValidator
	default:
		// Default to executor for implementation work.
		return WorkerTypeExecutor
	}
}

func (d *Dispatcher) runWorker(ctx context.Context, t *ticket.Ticket) error {
	proj, err := d.projects.GetProject(ctx, t.ProjectID)
	if err != nil {
		return err
	}
	role, wt := resolveTicketWorkerRole(proj, t)
	return d.runTypedWorkerWithProject(ctx, t, proj, role, wt)
}

func (d *Dispatcher) runTypedWorker(ctx context.Context, t *ticket.Ticket, wt WorkerType) error {
	// Get project for context pack.
	proj, err := d.projects.GetProject(ctx, t.ProjectID)
	if err != nil {
		return err
	}
	return d.runTypedWorkerWithProject(ctx, t, proj, string(wt), wt)
}

func (d *Dispatcher) runTypedWorkerWithProject(ctx context.Context, t *ticket.Ticket, proj *project.Project, role string, wt WorkerType) error {
	// Gather dependency outputs.
	depOutputs := make(map[string]map[string]any)
	if len(t.DependsOn) > 0 {
		deps, err := d.tickets.GetTicketsByIDs(ctx, t.DependsOn)
		if err != nil {
			return err
		}
		for _, dep := range deps {
			if len(dep.Outputs) > 0 {
				depOutputs[dep.ID] = dep.Outputs
			}
		}
	}

	// Assemble type-specific prompt.
	prompt := AssembleTypedWorkerPrompt(wt, proj, t, depOutputs, d.cfg.ServerURL, d.cfg.AgentID)
	if roleDef, ok := dispatchRoleDefinition(proj, role); ok {
		prompt = appendCustomRoleContext(prompt, roleDef)
	}

	// Determine working directory.
	// SAFETY: refuse to dispatch if the project has no repo_url configured.
	// Without this guard, workers fall back to d.cfg.RepoDir (the server's cwd)
	// which may be a completely unrelated codebase.
	if proj.RepoURL == "" {
		return fmt.Errorf("dispatch: project %s (%s) has no repo_url configured — refusing to execute ticket %s against the server's own codebase", proj.ID, proj.Name, t.ID)
	}

	var workDir string
	repoDir := d.cfg.RepoDir
	if t.TargetRepo != "" && d.repoResolver != nil && d.clones != nil {
		repoURL, _, resolveErr := d.repoResolver.ResolveRepo(ctx, t.ProjectID, t.TargetRepo)
		if resolveErr != nil {
			slog.Warn("dispatch: resolve repo failed, falling back to primary", "target_repo", t.TargetRepo, "ticket", t.ID, "error", resolveErr)
		} else if repoURL != "" {
			cloneDir, cloneErr := d.clones.EnsureClone(repoURL, t.ProjectID+"/"+t.TargetRepo)
			if cloneErr != nil {
				return fmt.Errorf("clone repo %s: %w", t.TargetRepo, cloneErr)
			}
			repoDir = cloneDir
		}
	}

	if d.cfg.DockerEnabled {
		// Docker mode: container handles its own workspace; pass repo dir for context.
		workDir = repoDir
	} else {
		// Host mode: create git worktree for isolation.
		branch := "ticket/" + t.ID
		var err error
		workDir, err = d.worktrees.CreateFromRepo(t.ID, branch, repoDir)
		if err != nil {
			return err
		}
	}

	// Add a small delay to avoid thundering herd on startup.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}

	// Build type-specific task prompt.
	taskMsg := buildTypedTaskPrompt(wt, t.ID, t.ProjectID)

	slog.Info("dispatch: running worker", "type", string(wt), "role", role, "ticket", t.ID)

	// Spawn worker.
	result, selected, err := d.spawnWorker(ctx, proj, t.ID, t.ProjectID, role, wt, prompt, taskMsg, workDir)
	if err != nil {
		return err
	}
	usageRole := role
	if role == string(wt) {
		usageRole = workerRoleForType(wt)
	}
	d.recordUsage(ctx, selected.Config, t.ProjectID, t.ID, usageRole, operationTypeForType(wt), prompt, taskMsg, result)

	if !result.Success {
		slog.Error("dispatch: worker completed with error", "type", string(wt), "role", role, "ticket", t.ID, "error", result.Error, "output", result.Output)
	} else {
		slog.Info("dispatch: worker completed", "type", string(wt), "role", role, "ticket", t.ID)
	}

	return nil
}

func resolveTicketWorkerRole(proj *project.Project, t *ticket.Ticket) (string, WorkerType) {
	if hint := ticketWorkerRoleHint(t); hint != "" {
		role := normalizePolicyRole(hint)
		if wt, ok := workerTypeForConfiguredRole(proj, role); ok {
			return role, wt
		}
		if wt := WorkerType(role); wt.IsValid() {
			return role, wt
		}
	}
	wt := DetermineWorkerType(t)
	return string(wt), wt
}

func ticketWorkerRoleHint(t *ticket.Ticket) string {
	if t == nil || t.Inputs == nil {
		return ""
	}
	for _, key := range []string{"worker_role", "worker_type"} {
		if value, ok := t.Inputs[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func workerTypeForConfiguredRole(proj *project.Project, role string) (WorkerType, bool) {
	roleDef, ok := dispatchRoleDefinition(proj, role)
	if !ok {
		return "", false
	}
	wt := WorkerType(normalizePolicyRole(roleDef.BaseType))
	if wt.IsValid() {
		return wt, true
	}
	return WorkerTypeExecutor, true
}

func dispatchRoleDefinition(proj *project.Project, role string) (project.DispatchWorkerRole, bool) {
	if proj == nil {
		return project.DispatchWorkerRole{}, false
	}
	role = normalizePolicyRole(role)
	for _, roleDef := range proj.DispatchConfig.Normalized().Roles {
		if normalizePolicyRole(roleDef.ID) == role {
			return roleDef, true
		}
	}
	return project.DispatchWorkerRole{}, false
}

func appendCustomRoleContext(prompt string, role project.DispatchWorkerRole) string {
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n## Custom dispatch role\n\n")
	b.WriteString(fmt.Sprintf("- **Role:** %s (`%s`)\n", role.Name, role.ID))
	b.WriteString(fmt.Sprintf("- **Base worker type:** %s\n", role.BaseType))
	if role.Description != "" {
		b.WriteString("\n")
		b.WriteString(role.Description)
		b.WriteString("\n")
	}
	return b.String()
}

// handleWorkerExit is Layer 1 of failure recovery. When a worker process exits
// (success or failure), this checks whether the ticket is still in a state that
// indicates the worker didn't complete its work (planning or executing). If so,
// it immediately releases the lease via ForceReleaseLease, which:
//   - Removes the Redis lease key (invalidating the token — fencing preserved)
//   - Transitions the ticket back to draft (= pending, retriable)
//
// This is a no-op if:
//   - leaseReleaser is nil (graceful degradation; Layer 2 TTL will handle it)
//   - The ticket already transitioned to a terminal/advanced state (submitted, escalated, etc.)
//   - The ticket is not found (already cleaned up)
func (d *Dispatcher) handleWorkerExit(ticketID string) {
	if d.leaseReleaser == nil {
		return
	}

	// Use a background context: the worker context may be cancelled, but we
	// still want to clean up the lease.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check current ticket state. Only release if still in planning or executing —
	// these are the states where the worker holds the lease but hasn't completed.
	t, err := d.tickets.GetTicket(ctx, ticketID)
	if err != nil || t == nil {
		return
	}

	// If the ticket moved past the worker's responsibility (submitted, escalated,
	// awaiting_input, awaiting_validation, etc.), don't release.
	if t.State != ticket.StatePlanning && t.State != ticket.StateExecuting {
		return
	}

	slog.Warn("dispatch: worker exited without completing, releasing lease", "ticket", ticketID, "state", t.State)
	if err := d.leaseReleaser.ForceReleaseLease(ctx, ticketID); err != nil {
		slog.Error("dispatch: failed to release lease", "ticket", ticketID, "error", err)
	}
}

func (d *Dispatcher) activeCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.active)
}

func (d *Dispatcher) projectWorkerLimit(ctx context.Context, projectID string) int {
	limit := d.cfg.MaxWorkers
	if d.projects == nil || projectID == "" {
		return limit
	}
	proj, err := d.projects.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		return limit
	}
	if configured := proj.DispatchConfig.Normalized().MaxActiveWorkers; configured > 0 {
		return configured
	}
	return limit
}

func (d *Dispatcher) activeCountForProjectLocked(projectID string) int {
	count := 0
	for key := range d.active {
		activeProjectID := d.activeProjects[key]
		if projectID == "" || activeProjectID == "" || activeProjectID == projectID {
			count++
		}
	}
	return count
}

func (d *Dispatcher) projectCapacity(ctx context.Context, projectID string) (active int, limit int, hasCapacity bool) {
	limit = d.projectWorkerLimit(ctx, projectID)
	d.mu.Lock()
	defer d.mu.Unlock()
	active = d.activeCountForProjectLocked(projectID)
	return active, limit, active < limit
}

func (d *Dispatcher) startActive(ctx context.Context, key, projectID string) (context.Context, context.CancelFunc, int, int, bool) {
	limit := d.projectWorkerLimit(ctx, projectID)
	workerCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, running := d.active[key]; running {
		cancel()
		return nil, nil, d.activeCountForProjectLocked(projectID), limit, false
	}
	active := d.activeCountForProjectLocked(projectID)
	if active >= limit {
		cancel()
		return nil, nil, active, limit, false
	}
	d.active[key] = cancel
	d.activeProjects[key] = projectID
	return workerCtx, cancel, active + 1, limit, true
}

// spawnReviewer launches a reviewer agent for a ticket in awaiting_review.
// Reviewers count against the worker capacity limit.
func (d *Dispatcher) spawnReviewer(ctx context.Context, t *ticket.Ticket) {
	reviewKey := "review:" + t.ID

	workerCtx, _, active, limit, started := d.startActive(ctx, reviewKey, t.ProjectID)
	if !started {
		slog.Info("dispatch: at capacity, deferring review", "ticket", t.ID, "project", t.ProjectID, "active", active, "max", limit)
		return
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			delete(d.active, reviewKey)
			delete(d.activeProjects, reviewKey)
			d.mu.Unlock()
			go d.reconcile(ctx)
		}()

		if err := d.runReviewer(workerCtx, t); err != nil {
			slog.Error("dispatch: reviewer failed", "ticket", t.ID, "error", err)
		}
	}()

	slog.Info("dispatch: spawned reviewer", "ticket", t.ID, "project", t.ProjectID, "active", active, "max", limit)
}

// runReviewer spawns a validator worker that reviews the ticket's PR and approves or rejects.
func (d *Dispatcher) runReviewer(ctx context.Context, t *ticket.Ticket) error {
	proj, err := d.projects.GetProject(ctx, t.ProjectID)
	if err != nil {
		return err
	}

	// Use the typed validator prompt for consistency.
	depOutputs := make(map[string]map[string]any)
	prompt := AssembleTypedWorkerPrompt(WorkerTypeValidator, proj, t, depOutputs, d.cfg.ServerURL, d.cfg.AgentID)

	// Reviewer works in the repo dir (needs access to the code for `gh` and `make test`).
	// Use the existing worktree if available (the worker's branch), otherwise fall back.
	workDir := d.worktrees.Path(t.ID)
	if workDir == "" {
		workDir = d.cfg.RepoDir
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}

	slog.Info("dispatch: running validator worker", "ticket", t.ID)

	taskMsg := buildTypedTaskPrompt(WorkerTypeValidator, t.ID, t.ProjectID)
	result, selected, err := d.spawnWorker(ctx, proj, t.ID, t.ProjectID, string(WorkerTypeValidator), WorkerTypeValidator, prompt, taskMsg, workDir)
	if err != nil {
		return err
	}
	d.recordUsage(ctx, selected.Config, t.ProjectID, t.ID, "review", cost.OpReview, prompt, taskMsg, result)

	if !result.Success {
		slog.Error("dispatch: validator completed with error", "ticket", t.ID, "error", result.Error, "output", result.Output)
	} else {
		slog.Info("dispatch: validator completed", "ticket", t.ID)
	}

	return nil
}

// autoMergePR merges the PR after a ticket is approved/validated.
// It first validates that CI checks pass, then merges. If the merge fails due
// to conflicts, it spawns a conflict resolver worker. On success, it advances
// the ticket through deploying → observing → closed.
func (d *Dispatcher) autoMergePR(ctx context.Context, t *ticket.Ticket, prURL string) {
	// State guard: only merge from validated state. Prevents re-entrancy when
	// ticket.closed event re-enters handleTicketDone.
	if t.State != ticket.StateValidated {
		return
	}

	// Check escalation threshold before attempting.
	d.mu.Lock()
	attempts := d.mergeAttempts[t.ID]
	d.mu.Unlock()
	if attempts >= maxMergeAttempts {
		d.escalateMergeFailure(ctx, t, fmt.Sprintf("merge failed %d times", attempts))
		return
	}

	_ = d.worktrees.Remove(t.ID)

	repoDir, err := d.resolveProjectRepoDir(ctx, t.ProjectID)
	if err != nil {
		slog.Error("dispatch: cannot resolve repo for auto-merge", "ticket", t.ID, "error", err)
		return
	}

	// Validate CI checks before attempting merge.
	if !d.validatePRChecks(ctx, t, prURL, repoDir) {
		return
	}

	cmd := exec.Command("gh", "pr", "merge", prURL, "--squash")
	cmd.Dir = repoDir
	out, mergeErr := cmd.CombinedOutput()
	if mergeErr != nil {
		output := string(out)
		slog.Error("dispatch: auto-merge failed", "ticket", t.ID, "error", mergeErr, "output", output)

		d.mu.Lock()
		d.mergeAttempts[t.ID]++
		d.mu.Unlock()

		if strings.Contains(output, "not mergeable") || strings.Contains(output, "CONFLICT") || strings.Contains(output, "cannot be cleanly created") {
			d.spawnConflictResolver(ctx, t, prURL)
		}
	} else {
		slog.Info("dispatch: auto-merged PR", "ticket", t.ID)
		// Delete remote branch (best-effort).
		branch := "ticket/" + t.ID
		delCmd := exec.Command("git", "push", "origin", "--delete", branch)
		delCmd.Dir = repoDir
		_ = delCmd.Run()

		d.closeMergedTicket(ctx, t)
	}
}

// closeMergedTicket advances a ticket through the post-merge lifecycle.
// If the ticket has a workflow, advances through remaining workflow phases.
// Otherwise, uses the hardcoded path: validated → deploying → observing → closed.
// Nil-safe: if ticketTransitioner is nil, logs and returns.
func (d *Dispatcher) closeMergedTicket(ctx context.Context, t *ticket.Ticket) {
	if d.ticketTransitioner == nil {
		slog.Warn("dispatch: ticket transitioner not set, cannot close merged ticket", "ticket", t.ID)
		return
	}

	actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}

	// Workflow-aware: advance through remaining phases instead of hardcoded triggers.
	if t.WorkflowID != "" && d.workflowEngine != nil && t.WorkflowPhase != "" {
		for i := 0; i < 20; i++ { // safety limit
			next, err := d.workflowEngine.AdvancePhase(ctx, t.ID, t.WorkflowID, t.WorkflowPhase, "success", nil)
			if err != nil {
				slog.Error("dispatch: workflow advance failed", "ticket", t.ID, "phase", t.WorkflowPhase, "error", err)
				return
			}
			if next == nil {
				// Workflow complete — close the ticket.
				if err := d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerClose, actor, nil); err != nil {
					// May already be closed or in wrong state; log and move on.
					slog.Warn("dispatch: close after workflow complete failed", "ticket", t.ID, "error", err)
				}
				break
			}
			t.WorkflowPhase = next.ID
			// For phases that need external action (agent, manual), stop advancing.
			if next.Type == workflow.PhaseAgent || next.Type == workflow.PhaseManual {
				break
			}
			// For deploy/observe/automated, fire the corresponding state triggers.
			switch next.Type {
			case workflow.PhaseDeploy:
				_ = d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerDeploy, actor, nil)
			case workflow.PhaseObserve:
				_ = d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerObserve, actor, nil)
			}
		}
		slog.Info("dispatch: ticket advanced through workflow after merge", "ticket", t.ID)
		d.mu.Lock()
		delete(d.mergeAttempts, t.ID)
		d.mu.Unlock()
		return
	}

	// Legacy path: hardcoded post-merge transitions.
	for _, trigger := range []string{ticket.TriggerDeploy, ticket.TriggerObserve, ticket.TriggerClose} {
		if err := d.ticketTransitioner.TransitionTicket(ctx, t.ID, trigger, actor, nil); err != nil {
			slog.Error("dispatch: post-merge transition failed", "trigger", trigger, "ticket", t.ID, "error", err)
			return
		}
	}

	slog.Info("dispatch: ticket closed after merge", "ticket", t.ID)

	d.mu.Lock()
	delete(d.mergeAttempts, t.ID)
	d.mu.Unlock()
}

// escalateMergeFailure publishes an escalation event when merge attempts exceed the threshold.
// The ticket stays in validated state for manual intervention.
func (d *Dispatcher) escalateMergeFailure(ctx context.Context, t *ticket.Ticket, reason string) {
	slog.Warn("dispatch: escalating merge failure", "ticket", t.ID, "reason", reason)
	_ = d.bus.Publish(ctx, events.Event{
		Type: events.EventTicketEscalated,
		Payload: map[string]any{
			"ticket_id":  t.ID,
			"project_id": t.ProjectID,
			"reason":     reason,
			"source":     "auto_merge",
		},
	})
}

// validatePRChecks verifies CI checks pass on the PR before merging.
// Returns true if checks pass (or no checks exist), false if failing/pending.
// Emits EventTestsFailed when checks fail.
func (d *Dispatcher) validatePRChecks(ctx context.Context, t *ticket.Ticket, prURL, repoDir string) bool {
	// Use `gh pr checks` to get CI status.
	cmd := exec.Command("gh", "pr", "checks", prURL)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	output := string(out)

	if err != nil {
		// gh pr checks exits non-zero if any check failed or is pending.
		if strings.Contains(output, "fail") || strings.Contains(output, "X") {
			slog.Warn("dispatch: CI checks failed", "ticket", t.ID)
			_ = d.bus.Publish(ctx, events.Event{
				Type: events.EventTestsFailed,
				Payload: map[string]any{
					"ticket_id":  t.ID,
					"project_id": t.ProjectID,
					"pr_url":     prURL,
					"output":     truncate(output, 1000),
				},
			})
			return false
		}
		// Checks still pending — skip for now, will retry on next scan.
		if strings.Contains(output, "pending") || strings.Contains(output, "-") {
			slog.Info("dispatch: CI checks pending, will retry later", "ticket", t.ID)
			return false
		}
		// No checks configured or other error — allow merge.
		slog.Info("dispatch: pr checks error, proceeding with merge", "ticket", t.ID, "error", err)
	}

	return true
}

// truncate returns s truncated to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// spawnConflictResolver launches a worker to rebase a PR branch onto main and resolve conflicts.
func (d *Dispatcher) spawnConflictResolver(ctx context.Context, t *ticket.Ticket, prURL string) {
	resolveKey := "resolve:" + t.ID

	workerCtx, _, active, limit, started := d.startActive(ctx, resolveKey, t.ProjectID)
	if !started {
		slog.Info("dispatch: at capacity, deferring conflict resolution", "ticket", t.ID, "project", t.ProjectID, "active", active, "max", limit)
		return
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			delete(d.active, resolveKey)
			delete(d.activeProjects, resolveKey)
			d.mu.Unlock()
			go d.reconcile(ctx)
		}()

		if err := d.runConflictResolver(workerCtx, t, prURL); err != nil {
			slog.Error("dispatch: conflict resolver failed", "ticket", t.ID, "error", err)
		}
	}()

	slog.Info("dispatch: spawned conflict resolver", "ticket", t.ID, "project", t.ProjectID, "active", active, "max", limit)
}

// runConflictResolver rebases a ticket's branch onto main and retries the merge.
func (d *Dispatcher) runConflictResolver(ctx context.Context, t *ticket.Ticket, prURL string) error {
	branch := "ticket/" + t.ID

	proj, err := d.projects.GetProject(ctx, t.ProjectID)
	if err != nil {
		return err
	}

	// Resolve the repo dir from the project (not the server's cwd).
	repoDir, err := d.resolveProjectRepoDir(ctx, t.ProjectID)
	if err != nil {
		return fmt.Errorf("resolve repo for conflict resolver: %w", err)
	}

	workDir, err := d.worktrees.CreateFromRepo(t.ID, branch, repoDir)
	if err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}

	prompt := assembleConflictResolverPrompt(t, prURL, branch)
	taskMsg := fmt.Sprintf(
		"Rebase branch %s onto main and resolve any merge conflicts. "+
			"Then force-push the result. The goal is to make PR %s mergeable.",
		branch, prURL,
	)

	result, selected, err := d.spawnWorker(ctx, proj, t.ID, t.ProjectID, WorkerRoleConflictResolver, WorkerType(WorkerRoleConflictResolver), prompt, taskMsg, workDir)
	if err != nil {
		return err
	}
	d.recordUsage(ctx, selected.Config, t.ProjectID, t.ID, "conflict_resolution", cost.OpCodeGeneration, prompt, taskMsg, result)

	if !result.Success {
		slog.Error("dispatch: conflict resolver failed", "ticket", t.ID, "error", result.Error, "output", result.Output)
		return fmt.Errorf("resolver failed: %s", result.Error)
	}

	slog.Info("dispatch: conflict resolver completed, retrying merge", "ticket", t.ID)

	_ = d.worktrees.Remove(t.ID)
	cmd := exec.Command("gh", "pr", "merge", prURL, "--squash")
	cmd.Dir = repoDir
	out, mergeErr := cmd.CombinedOutput()
	if mergeErr != nil {
		slog.Error("dispatch: retry merge still failed", "ticket", t.ID, "error", mergeErr, "output", string(out))

		d.mu.Lock()
		d.mergeAttempts[t.ID]++
		d.mu.Unlock()

		return fmt.Errorf("retry merge: %w", mergeErr)
	}

	slog.Info("dispatch: auto-merged PR after conflict resolution", "ticket", t.ID)
	// Delete remote branch (best-effort).
	delCmd := exec.Command("git", "push", "origin", "--delete", branch)
	delCmd.Dir = repoDir
	_ = delCmd.Run()

	d.closeMergedTicket(ctx, t)
	return nil
}

func (d *Dispatcher) spawnWorker(ctx context.Context, proj *project.Project, ticketID, projectID, role string, wt WorkerType, systemPrompt, taskMessage, workDir string) (*WorkerResult, RoutedWorker, error) {
	router := d.workerRouter
	if router == nil {
		router = NewProjectWorkerRouter(d.cfg)
	}
	candidates := router.Candidates(proj, role)
	var lastResult *WorkerResult
	var lastErr error
	var lastWorker RoutedWorker
	for index, candidate := range candidates {
		worker := d.resolveWorker(candidate)
		if worker == nil {
			lastErr = fmt.Errorf("worker %s is not configured", candidate.ID)
			lastWorker = candidate
			continue
		}
		slog.Info("dispatch: selected worker",
			"worker_id", candidate.ID,
			"worker_name", candidate.Name,
			"role", role,
			"ticket", ticketID,
			"runner", resolveRunnerType(candidate.Config),
			"driver", candidate.Config.AgentDriver,
			"model", candidate.Config.AgentModel,
		)

		var (
			result *WorkerResult
			err    error
		)
		if streamable, ok := worker.(StreamableWorker); ok && d.cfg.TraceSvc != nil {
			onOutput, waitForTrace := d.traceWorkerOutput(ticketID, wt)
			result, err = streamable.SpawnStream(
				ctx,
				ticketID,
				projectID,
				systemPrompt,
				taskMessage,
				workDir,
				d.cfg.ServerURL,
				onOutput,
			)
			waitForTrace()
		} else {
			result, err = worker.Spawn(ctx, ticketID, projectID, systemPrompt, taskMessage, workDir, d.cfg.ServerURL)
		}
		if err == nil && result != nil && result.Success {
			return result, candidate, nil
		}

		lastResult = result
		lastErr = err
		lastWorker = candidate
		if index < len(candidates)-1 && ShouldFailoverToNextWorker(err, result) {
			slog.Warn("dispatch: worker failed, trying next candidate", "worker", candidate.ID, "ticket", ticketID)
			continue
		}
		if err != nil {
			return nil, candidate, err
		}
		return result, candidate, nil
	}
	if lastErr != nil {
		return nil, lastWorker, lastErr
	}
	return lastResult, lastWorker, nil
}

func (d *Dispatcher) resolveWorker(candidate RoutedWorker) Worker {
	if candidate.UseDefault {
		if d.worker != nil {
			return d.worker
		}
		return NewWorker(candidate.Config)
	}
	return NewWorker(candidate.Config)
}

func (d *Dispatcher) traceWorkerOutput(ticketID string, wt WorkerType) (WorkerOutputHandler, func()) {
	type outputChunk struct {
		stream string
		text   string
	}

	chunks := make(chan outputChunk, 256)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for chunk := range chunks {
			if err := d.cfg.TraceSvc.AppendSystemStep(context.Background(), ticketID, execution.Step{
				Type:       execution.StepTypeObservation,
				WorkerType: string(wt),
				Payload: map[string]any{
					"kind":   "worker_output",
					"stream": chunk.stream,
					"text":   chunk.text,
				},
			}); err != nil {
				slog.Error("dispatch: append worker output failed", "ticket", ticketID, "error", err)
			}
		}
	}()

	return func(stream, text string) {
			if strings.TrimSpace(text) == "" {
				return
			}
			chunks <- outputChunk{stream: stream, text: text}
		}, func() {
			close(chunks)
			wg.Wait()
		}
}

func workerRoleForType(wt WorkerType) string {
	switch wt {
	case WorkerTypePlanner:
		return "planning"
	case WorkerTypeValidator:
		return "review"
	case WorkerTypeDeployer:
		return "deployment"
	case WorkerTypeInvestigator:
		return "investigation"
	default:
		return "implementation"
	}
}

func operationTypeForType(wt WorkerType) cost.OperationType {
	switch wt {
	case WorkerTypePlanner:
		return cost.OpPlanning
	case WorkerTypeValidator:
		return cost.OpReview
	case WorkerTypeInvestigator:
		return cost.OpStructuralQuery
	case WorkerTypeDeployer:
		return cost.OpGeneral
	default:
		return cost.OpCodeGeneration
	}
}

func (d *Dispatcher) recordUsage(ctx context.Context, workerCfg Config, projectID, ticketID, workerRole string, op cost.OperationType, systemPrompt, taskMessage string, result *WorkerResult) {
	if d.cfg.CostSvc == nil || result == nil {
		return
	}
	provider, model := cost.InferProviderModel(workerCfg.AgentRunner, workerCfg.AgentDriver, workerCfg.AgentModel)
	output := strings.TrimSpace(result.Output)
	if output == "" {
		output = strings.TrimSpace(result.Error)
	}
	record := &cost.LLMCallRecord{
		ProjectID:     projectID,
		TicketID:      ticketID,
		WorkerRole:    workerRole,
		Provider:      provider,
		Model:         model,
		OperationType: op,
		InputTokens:   cost.EstimateTokens(systemPrompt, taskMessage),
		OutputTokens:  cost.EstimateTokens(output),
	}
	_, _ = d.cfg.CostSvc.RecordAndCheck(ctx, record)
}
