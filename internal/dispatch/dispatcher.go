package dispatch

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
)

// TicketGetter retrieves tickets and their dependencies.
type TicketGetter interface {
	GetTicket(ctx context.Context, id string) (*ticket.Ticket, error)
	GetTicketsByIDs(ctx context.Context, ids []string) ([]*ticket.Ticket, error)
	ListByState(ctx context.Context, projectID string, state ticket.State) ([]*ticket.Ticket, error)
}

// ProjectGetter retrieves projects.
type ProjectGetter interface {
	GetProject(ctx context.Context, id string) (*project.Project, error)
}

// LeaseReleaser releases a ticket's lease and transitions it back to draft.
// This is Layer 1 of failure recovery: immediate cleanup on worker process exit.
type LeaseReleaser interface {
	ForceReleaseLease(ctx context.Context, ticketID string) error
}

// Config holds dispatcher settings.
type Config struct {
	MaxWorkers   int
	ClaudePath   string
	WorktreeDir  string
	RepoDir      string // path to the main git repository
	ServerURL    string // Flywheel server URL for MCP connections
	AgentID      string // agent identity for workers
	APIKey       string // Flywheel API key for worker MCP authentication
	ProjectID    string // only dispatch tickets for this project (empty = all)
	AutoApprove  bool   // auto-approve tickets when acceptance tests pass
	// Agent driver selection.
	AgentDriver  string // driver name: "claude" (default), "generic", or custom
	AgentCLIPath string // override CLI path for the agent binary
	AgentArgs    []string // extra static arguments for the agent command
	// Docker isolation settings.
	DockerEnabled  bool
	DockerImage    string
	DockerMemory   string
	DockerCPUs     string
	DockerFirewall bool
	AnthropicKey   string
}

// Dispatcher listens for ticket events and spawns workers.
type Dispatcher struct {
	cfg           Config
	bus           events.Bus
	tickets       TicketGetter
	projects      ProjectGetter
	worker        Worker
	worktrees     *WorktreeManager
	leaseReleaser LeaseReleaser // nil-safe: if nil, worker exit does not release lease (Layer 2 TTL handles it)

	mu       sync.Mutex
	active   map[string]context.CancelFunc // ticketID → cancel
	wg       sync.WaitGroup
}

// New creates a dispatcher that subscribes to the event bus.
func New(cfg Config, bus events.Bus, tickets TicketGetter, projects ProjectGetter) *Dispatcher {
	// Resolve the agent driver.
	driverName := cfg.AgentDriver
	if driverName == "" {
		driverName = "claude"
	}
	cliPath := cfg.AgentCLIPath
	if cliPath == "" {
		cliPath = cfg.ClaudePath // backward compat
	}
	driver, err := LookupDriver(driverName, DriverConfig{
		CLIPath:   cliPath,
		ExtraArgs: cfg.AgentArgs,
	})
	if err != nil {
		log.Printf("dispatch: %v, falling back to claude driver", err)
		driver = NewClaudeDriver(DriverConfig{CLIPath: cliPath})
	}

	var worker Worker
	if cfg.DockerEnabled {
		worker = &DockerWorker{
			Driver:       driver,
			Image:        cfg.DockerImage,
			APIKey:       cfg.APIKey,
			RepoDir:      cfg.RepoDir,
			AnthropicKey: cfg.AnthropicKey,
			Memory:       cfg.DockerMemory,
			CPUs:         cfg.DockerCPUs,
			Firewall:     cfg.DockerFirewall,
		}
	} else {
		worker = &CLIWorker{
			Driver: driver,
			APIKey: cfg.APIKey,
		}
	}
	d := &Dispatcher{
		cfg:      cfg,
		bus:      bus,
		tickets:  tickets,
		projects: projects,
		worker:   worker,
		worktrees: &WorktreeManager{
			BaseDir: cfg.WorktreeDir,
			RepoDir: cfg.RepoDir,
		},
		active: make(map[string]context.CancelFunc),
	}
	return d
}

// Start subscribes to events and begins dispatching. Call Stop to shut down.
func (d *Dispatcher) Start(ctx context.Context) {
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

	log.Printf("dispatch: started (max_workers=%d, worktree_dir=%s, project=%s)", d.cfg.MaxWorkers, d.cfg.WorktreeDir, d.cfg.ProjectID)

	// Scan for existing pending tickets on startup.
	go d.scanPending(ctx)
}

// Stop waits for all active workers to finish.
func (d *Dispatcher) Stop() {
	d.mu.Lock()
	for _, cancel := range d.active {
		cancel()
	}
	d.mu.Unlock()
	d.wg.Wait()
	log.Println("dispatch: stopped")
}

// SetLeaseReleaser configures the lease releaser for immediate cleanup on worker exit.
// Call this after construction to wire the queue service without circular imports.
func (d *Dispatcher) SetLeaseReleaser(lr LeaseReleaser) {
	d.leaseReleaser = lr
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
		log.Printf("dispatch: scan awaiting_review: %v", err)
	} else {
		for _, t := range reviewing {
			d.spawnReviewer(ctx, t)
		}
	}

	// Scan for validated tickets with unmerged PRs — try merge or spawn resolver.
	validated, err := d.tickets.ListByState(ctx, d.cfg.ProjectID, ticket.StateValidated)
	if err != nil {
		log.Printf("dispatch: scan validated: %v", err)
	} else {
		for _, t := range validated {
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
		log.Printf("dispatch: scan pending: %v", err)
		return
	}
	log.Printf("dispatch: scan found %d pending tickets", len(pending))
	for _, t := range pending {
		d.tryDispatch(ctx, t)
	}
}

func (d *Dispatcher) handleTicketReady(ctx context.Context, e events.Event) {
	ticketID, _ := e.Payload["ticket_id"].(string)
	if ticketID == "" {
		return
	}

	// Verify ticket is eligible (pending + dependencies met).
	t, err := d.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		log.Printf("dispatch: get ticket %s: %v", ticketID, err)
		return
	}

	// Filter by project if configured.
	if d.cfg.ProjectID != "" && t.ProjectID != d.cfg.ProjectID {
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
		log.Printf("dispatch: rejected get ticket %s: %v", ticketID, err)
		return
	}
	if d.cfg.ProjectID != "" && t.ProjectID != d.cfg.ProjectID {
		return
	}
	if t.State != ticket.StateExecuting {
		return
	}
	log.Printf("dispatch: ticket %s rejected, re-spawning worker for iteration", ticketID)
	d.spawn(ctx, t)
}

func (d *Dispatcher) tryDispatch(ctx context.Context, t *ticket.Ticket) {
	if t.State != ticket.StatePending {
		return
	}

	// Check capacity.
	d.mu.Lock()
	if len(d.active) >= d.cfg.MaxWorkers {
		d.mu.Unlock()
		log.Printf("dispatch: at capacity (%d/%d), skipping %s", len(d.active), d.cfg.MaxWorkers, t.ID)
		return
	}
	if _, running := d.active[t.ID]; running {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()

	if len(t.DependsOn) > 0 {
		deps, err := d.tickets.GetTicketsByIDs(ctx, t.DependsOn)
		if err != nil {
			log.Printf("dispatch: get deps for %s: %v", t.ID, err)
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
		log.Printf("dispatch: reviewer get ticket %s: %v", ticketID, err)
		return
	}
	if d.cfg.ProjectID != "" && t.ProjectID != d.cfg.ProjectID {
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
		log.Printf("dispatch: worktree cleanup %s: %v", ticketID, err)
	}
}

func (d *Dispatcher) spawn(ctx context.Context, t *ticket.Ticket) {
	// Register as active.
	workerCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	if _, running := d.active[t.ID]; running {
		d.mu.Unlock()
		cancel()
		return
	}
	d.active[t.ID] = cancel
	d.mu.Unlock()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			delete(d.active, t.ID)
			d.mu.Unlock()

			// Re-scan for pending tickets to fill the freed slot.
			go d.scanPending(ctx)
		}()

		if err := d.runWorker(workerCtx, t); err != nil {
			log.Printf("dispatch: worker %s failed: %v", t.ID, err)
		}

		// Layer 1 failure recovery: if the worker exited without submitting or
		// escalating, immediately release the lease so the ticket returns to
		// draft (pending) for retry. This avoids waiting for TTL expiry (Layer 2).
		d.handleWorkerExit(t.ID)
	}()

	log.Printf("dispatch: spawned worker for %s (%d/%d active)", t.ID, d.activeCount(), d.cfg.MaxWorkers)
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
	return d.runTypedWorker(ctx, t, DetermineWorkerType(t))
}

func (d *Dispatcher) runTypedWorker(ctx context.Context, t *ticket.Ticket, wt WorkerType) error {
	// Get project for context pack.
	proj, err := d.projects.GetProject(ctx, t.ProjectID)
	if err != nil {
		return err
	}

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

	// Determine working directory.
	var workDir string
	if d.cfg.DockerEnabled {
		// Docker mode: container handles its own workspace; pass repo dir for context.
		workDir = d.cfg.RepoDir
	} else {
		// Host mode: create git worktree for isolation.
		branch := "ticket/" + t.ID
		var err error
		workDir, err = d.worktrees.Create(t.ID, branch)
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

	log.Printf("dispatch: running %s worker for %s", wt, t.ID)

	// Spawn worker.
	result, err := d.worker.Spawn(ctx, t.ID, t.ProjectID, prompt, taskMsg, workDir, d.cfg.ServerURL)
	if err != nil {
		return err
	}

	if !result.Success {
		log.Printf("dispatch: %s worker %s completed with error: %s\nOutput: %s", wt, t.ID, result.Error, result.Output)
	} else {
		log.Printf("dispatch: %s worker %s completed successfully", wt, t.ID)
	}

	return nil
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

	log.Printf("dispatch: worker %s exited without completing — releasing lease (state=%s)", ticketID, t.State)
	if err := d.leaseReleaser.ForceReleaseLease(ctx, ticketID); err != nil {
		log.Printf("dispatch: failed to release lease for %s: %v", ticketID, err)
	}
}

func (d *Dispatcher) activeCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.active)
}

// spawnReviewer launches a reviewer agent for a ticket in awaiting_review.
// Reviewers count against the worker capacity limit.
func (d *Dispatcher) spawnReviewer(ctx context.Context, t *ticket.Ticket) {
	reviewKey := "review:" + t.ID

	d.mu.Lock()
	if len(d.active) >= d.cfg.MaxWorkers {
		d.mu.Unlock()
		log.Printf("dispatch: at capacity, deferring review of %s", t.ID)
		return
	}
	if _, running := d.active[reviewKey]; running {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()

	workerCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.active[reviewKey] = cancel
	d.mu.Unlock()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			delete(d.active, reviewKey)
			d.mu.Unlock()
			go d.scanPending(ctx)
		}()

		if err := d.runReviewer(workerCtx, t); err != nil {
			log.Printf("dispatch: reviewer %s failed: %v", t.ID, err)
		}
	}()

	log.Printf("dispatch: spawned reviewer for %s (%d/%d active)", t.ID, d.activeCount(), d.cfg.MaxWorkers)
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
	// Use the existing worktree if available (the worker's branch), otherwise the main repo.
	workDir := d.worktrees.Path(t.ID)
	if workDir == "" {
		workDir = d.cfg.RepoDir
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
	}

	log.Printf("dispatch: running validator worker for %s", t.ID)

	taskMsg := buildTypedTaskPrompt(WorkerTypeValidator, t.ID, t.ProjectID)
	result, err := d.worker.Spawn(ctx, t.ID, t.ProjectID, prompt, taskMsg, workDir, d.cfg.ServerURL)
	if err != nil {
		return err
	}

	if !result.Success {
		log.Printf("dispatch: validator %s completed with error: %s\nOutput: %s", t.ID, result.Error, result.Output)
	} else {
		log.Printf("dispatch: validator %s completed successfully", t.ID)
	}

	return nil
}

// autoMergePR merges the PR after a ticket is approved.
// If the merge fails due to conflicts, it spawns a conflict resolver worker.
func (d *Dispatcher) autoMergePR(ctx context.Context, t *ticket.Ticket, prURL string) {
	_ = d.worktrees.Remove(t.ID)

	cmd := exec.Command("gh", "pr", "merge", prURL, "--squash")
	cmd.Dir = d.cfg.RepoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := string(out)
		log.Printf("dispatch: auto-merge %s failed: %v\n%s", t.ID, err, output)
		if strings.Contains(output, "not mergeable") || strings.Contains(output, "CONFLICT") || strings.Contains(output, "cannot be cleanly created") {
			d.spawnConflictResolver(ctx, t, prURL)
		}
	} else {
		log.Printf("dispatch: auto-merged PR for %s", t.ID)
		// Delete remote branch (best-effort).
		branch := "ticket/" + t.ID
		delCmd := exec.Command("git", "push", "origin", "--delete", branch)
		delCmd.Dir = d.cfg.RepoDir
		_ = delCmd.Run()
	}
}

// spawnConflictResolver launches a worker to rebase a PR branch onto main and resolve conflicts.
func (d *Dispatcher) spawnConflictResolver(ctx context.Context, t *ticket.Ticket, prURL string) {
	resolveKey := "resolve:" + t.ID

	d.mu.Lock()
	if len(d.active) >= d.cfg.MaxWorkers {
		d.mu.Unlock()
		log.Printf("dispatch: at capacity, deferring conflict resolution for %s", t.ID)
		return
	}
	if _, running := d.active[resolveKey]; running {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()

	workerCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.active[resolveKey] = cancel
	d.mu.Unlock()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			delete(d.active, resolveKey)
			d.mu.Unlock()
			go d.scanPending(ctx)
		}()

		if err := d.runConflictResolver(workerCtx, t, prURL); err != nil {
			log.Printf("dispatch: conflict resolver %s failed: %v", t.ID, err)
		}
	}()

	log.Printf("dispatch: spawned conflict resolver for %s (%d/%d active)", t.ID, d.activeCount(), d.cfg.MaxWorkers)
}

// runConflictResolver rebases a ticket's branch onto main and retries the merge.
func (d *Dispatcher) runConflictResolver(ctx context.Context, t *ticket.Ticket, prURL string) error {
	branch := "ticket/" + t.ID

	workDir, err := d.worktrees.Create(t.ID, branch)
	if err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}

	prompt := assembleConflictResolverPrompt(t, prURL, branch)
	taskMsg := fmt.Sprintf(
		"Rebase branch %s onto main and resolve any merge conflicts. "+
			"Then force-push the result. The goal is to make PR %s mergeable.",
		branch, prURL,
	)

	result, err := d.worker.Spawn(ctx, t.ID, t.ProjectID, prompt, taskMsg, workDir, d.cfg.ServerURL)
	if err != nil {
		return err
	}

	if !result.Success {
		log.Printf("dispatch: conflict resolver %s failed: %s\nOutput: %s", t.ID, result.Error, result.Output)
		return fmt.Errorf("resolver failed: %s", result.Error)
	}

	log.Printf("dispatch: conflict resolver %s completed, retrying merge", t.ID)

	_ = d.worktrees.Remove(t.ID)
	cmd := exec.Command("gh", "pr", "merge", prURL, "--squash")
	cmd.Dir = d.cfg.RepoDir
	out, mergeErr := cmd.CombinedOutput()
	if mergeErr != nil {
		log.Printf("dispatch: retry merge %s still failed: %v\n%s", t.ID, mergeErr, string(out))
		return fmt.Errorf("retry merge: %w", mergeErr)
	}

	log.Printf("dispatch: auto-merged PR for %s (after conflict resolution)", t.ID)
	// Delete remote branch (best-effort).
	delCmd := exec.Command("git", "push", "origin", "--delete", branch)
	delCmd.Dir = d.cfg.RepoDir
	_ = delCmd.Run()
	return nil
}
