package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/cost"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/policy"
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

// FailureSummarizer injects failure context into a ticket's PriorAttempts so
// the next agent knows why the previous attempt failed.
type FailureSummarizer interface {
	AppendFailureSummary(ctx context.Context, ticketID, reason string) error
}

// TicketOutputPatcher persists merge metadata into ticket outputs.
type TicketOutputPatcher interface {
	PatchOutputs(ctx context.Context, id string, patch map[string]any) error
}

// WorkflowPhaseUpdater provides workflow phase status operations for the dispatcher.
type WorkflowPhaseUpdater interface {
	UpdateWorkflowPhaseStatus(ctx context.Context, id string, status string) error
	ListByWorkflowPhaseStatus(ctx context.Context, projectID, status string) ([]*ticket.Ticket, error)
	CASWorkflowPhaseStatus(ctx context.Context, id, expected, desired string) (bool, error)
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

// defaultScanLimit caps the number of tickets processed per scan phase.
const defaultScanLimit = 50

// maxWorkerAttempts is the number of consecutive worker crashes (exit without
// submit/escalate) before the dispatcher escalates to awaiting_input instead
// of retrying. This prevents infinite crash loops from consuming capacity.
const maxWorkerAttempts = 3

// projectCache is a per-reconcile cache for project lookups.
// Scoped to a single scanPending call, cleared on exit. Max staleness = reconcile interval.
type projectCache struct {
	getter ProjectGetter
	cache  map[string]*project.Project
}

func newProjectCache(getter ProjectGetter) *projectCache {
	return &projectCache{getter: getter, cache: make(map[string]*project.Project)}
}

func (c *projectCache) GetProject(ctx context.Context, id string) (*project.Project, error) {
	if p, ok := c.cache[id]; ok {
		return p, nil
	}
	p, err := c.getter.GetProject(ctx, id)
	if err == nil {
		c.cache[id] = p
	}
	return p, err
}

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
	BranchGCInterval  time.Duration // stale branch GC interval (default: 6h)
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
	ScanLimit      int // max tickets per scan phase (default 50, 0 = unlimited)
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
	failureSummarizer  FailureSummarizer      // nil-safe: if nil, worker exit does not inject failure context
	ticketTransitioner TicketTransitioner        // nil-safe: if nil, merged tickets are not auto-closed
	workflowEngine     *workflow.Engine              // nil-safe: if nil, workflow-aware dispatching is disabled
	externalExecutor   *workflow.ExternalExecutor   // nil-safe: if nil, external phases auto-advance
	actionRegistry     *workflow.ActionRegistry     // nil-safe: if nil, action phases auto-advance
	checkerRegistry    *policy.CheckerRegistry      // nil-safe: if nil, gate requirements are not auto-checked
	outputPatcher        TicketOutputPatcher          // nil-safe: if nil, merge state is not persisted
	workflowPhaseUpdater WorkflowPhaseUpdater        // nil-safe: if nil, async phase processing is disabled

	ghLimiter      *ghRateLimiter  // rate limiter for gh CLI subprocess calls
	reconcileCache *projectCache  // per-reconcile project lookup cache; nil outside scanPending

	mu             sync.Mutex
	active         map[string]context.CancelFunc // ticketID/role-prefixed key → cancel
	activeProjects map[string]string             // active key → projectID
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
		ghLimiter:      newGHRateLimiter(1, 10), // 1 token/sec sustained, burst 10
		active:         make(map[string]context.CancelFunc),
		activeProjects: make(map[string]string),
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
		_ = d.durableBus.SubscribePattern(events.EventTestsFailed, "dispatcher:tests-failed", func(_ context.Context, e events.Event) {
			d.handleTestsFailed(ctx, e)
		})
		_ = d.durableBus.SubscribePattern("ticket.rolled_back", "dispatcher:rolled-back", func(_ context.Context, e events.Event) {
			d.handleTicketRolledBack(ctx, e)
		})
		_ = d.durableBus.SubscribePattern("ticket.cancelled", "dispatcher:cancelled", func(_ context.Context, e events.Event) {
			d.handleTicketCancelled(ctx, e)
		})
		_ = d.durableBus.SubscribePattern("ticket.input_provided", "dispatcher:input-provided", func(_ context.Context, e events.Event) {
			d.handleTicketInputProvided(ctx, e)
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
		d.bus.Subscribe(events.EventTicketCancelled, func(_ context.Context, e events.Event) {
			d.handleTicketCancelled(ctx, e)
		})
		d.bus.Subscribe(events.EventTicketInputProvided, func(_ context.Context, e events.Event) {
			d.handleTicketInputProvided(ctx, e)
		})
	}

	slog.Info("dispatch started", "max_workers", d.cfg.MaxWorkers, "worktree_dir", d.cfg.WorktreeDir, "project", d.cfg.ProjectID, "durable", d.durableBus != nil)

	// Scan for existing pending tickets on startup.
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.reconcile(ctx)
	}()

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

	// Start periodic branch GC.
	d.startBranchGC(ctx)
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

// SetFailureSummarizer configures the failure summarizer for injecting context on worker exit.
// Call this after construction to wire the ticket service without circular imports.
func (d *Dispatcher) SetFailureSummarizer(fs FailureSummarizer) {
	d.failureSummarizer = fs
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

// SetExternalExecutor sets the optional external executor for workflow external phases.
func (d *Dispatcher) SetExternalExecutor(ee *workflow.ExternalExecutor) {
	d.externalExecutor = ee
}

// SetCheckerRegistry sets the optional policy checker registry for gate requirement auto-checking.
func (d *Dispatcher) SetActionRegistry(ar *workflow.ActionRegistry) {
	d.actionRegistry = ar
}

func (d *Dispatcher) SetCheckerRegistry(cr *policy.CheckerRegistry) {
	d.checkerRegistry = cr
}

func (d *Dispatcher) SetRepoResolver(rr RepoResolver) {
	d.repoResolver = rr
}

// SetOutputPatcher wires the ticket output patcher for durable merge state.
func (d *Dispatcher) SetOutputPatcher(op TicketOutputPatcher) {
	d.outputPatcher = op
}

// SetWorkflowPhaseUpdater wires the workflow phase status updater for async phase processing.
func (d *Dispatcher) SetWorkflowPhaseUpdater(wpu WorkflowPhaseUpdater) {
	d.workflowPhaseUpdater = wpu
}

// scanLimitValue returns the configured scan limit or the default.
func (d *Dispatcher) scanLimitValue() int {
	if d.cfg.ScanLimit > 0 {
		return d.cfg.ScanLimit
	}
	return defaultScanLimit
}

// ghCommand creates a rate-limited exec.Cmd for the GitHub CLI.
// All gh subprocess calls should go through this method.
func (d *Dispatcher) ghCommand(ctx context.Context, args ...string) *exec.Cmd {
	_ = d.ghLimiter.Wait(ctx)
	return exec.CommandContext(ctx, "gh", args...)
}

// cachedGetProject returns a project, using the per-reconcile cache if active.
// Outside of scanPending (reconcileCache == nil), goes directly to the DB.
func (d *Dispatcher) cachedGetProject(ctx context.Context, id string) (*project.Project, error) {
	if d.reconcileCache != nil {
		return d.reconcileCache.GetProject(ctx, id)
	}
	return d.projects.GetProject(ctx, id)
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

	// Set up per-reconcile project cache; cleared on exit.
	d.reconcileCache = newProjectCache(d.projects)
	defer func() { d.reconcileCache = nil }()

	scanLimit := d.scanLimitValue()

	// Reviewers first — finish in-progress work before starting new work.
	// This handles tickets stuck in awaiting_review when the reviewer was
	// deferred due to capacity (worker still occupied the slot at submit time).
	reviewing, err := d.tickets.ListByState(ctx, d.cfg.ProjectID, ticket.StateAwaitingValidation)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("dispatch: scan awaiting_review failed", "error", err)
	} else {
		if len(reviewing) > scanLimit {
			reviewing = reviewing[:scanLimit]
		}
		for _, t := range reviewing {
			if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
				continue
			}
			d.spawnReviewer(ctx, t)
		}
	}

	// Reconcile awaiting_validation PRs: merged externally, approved, or changes requested.
	// Single pass replaces the old reconcileExternalMerges + reconcileGitHubReviewStatus.
	if ctx.Err() != nil {
		return
	}
	d.reconcileAwaitingValidationPRs(ctx, reviewing)

	// Scan for validated tickets with unmerged PRs — try merge or spawn resolver.
	if ctx.Err() != nil {
		return
	}
	validated, err := d.tickets.ListByState(ctx, d.cfg.ProjectID, ticket.StateValidated)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("dispatch: scan validated failed", "error", err)
	} else {
		if len(validated) > scanLimit {
			validated = validated[:scanLimit]
		}
		for _, t := range validated {
			if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
				continue
			}
			prURL, _ := t.Outputs["pr_url"].(string)

			// Backfill pr_url from artifacts for already-stuck tickets.
			if !isValidPRURL(prURL) {
				if artifacts, ok := t.Outputs["artifacts"].([]any); ok {
					for _, a := range artifacts {
						if m, ok := a.(map[string]any); ok {
							if tp, _ := m["type"].(string); tp == "pr" || tp == "pull_request" {
								if u, _ := m["url"].(string); isValidPRURL(u) {
									prURL = u
									if d.outputPatcher != nil {
										_ = d.outputPatcher.PatchOutputs(ctx, t.ID, map[string]any{"pr_url": u})
									}
									break
								}
							}
						}
					}
				}
			}

			// Try to discover PR via branch convention if still no pr_url.
			if !isValidPRURL(prURL) {
				if repoDir, err := d.resolveProjectRepoDir(ctx, t.ProjectID); err == nil {
					branch := d.branchForTicket(ctx, t)
					cmd := d.ghCommand(ctx, "pr", "list", "--head", branch,
						"--state", "all", "--json", "url,state", "--jq", ".[0]")
					cmd.Dir = repoDir
					if out, err := cmd.Output(); err == nil {
						var prInfo struct {
							URL   string `json:"url"`
							State string `json:"state"`
						}
						if json.Unmarshal(out, &prInfo) == nil && isValidPRURL(prInfo.URL) {
							prURL = prInfo.URL
							if d.outputPatcher != nil {
								_ = d.outputPatcher.PatchOutputs(ctx, t.ID, map[string]any{"pr_url": prURL})
							}
							// If PR is already merged, close the ticket directly.
							if prInfo.State == "MERGED" {
								slog.Info("dispatch: discovered merged PR for validated ticket", "ticket", t.ID, "pr_url", prURL)
								d.cleanupTicketBranch(ctx, t.ID, t.ProjectID)
								d.publishMergedEvent(ctx, t, prURL)
								d.closeMergedTicket(ctx, t)
								continue
							}
						}
					}
				}
			}

			// No PR found anywhere — close as no-code validated ticket.
			if !isValidPRURL(prURL) {
				if d.ticketTransitioner != nil {
					actor := ticket.Actor{ID: "dispatcher", Type: ticket.ActorSystem}
					if err := d.ticketTransitioner.TransitionTicket(ctx, t.ID, ticket.TriggerClose, actor, nil); err != nil {
						slog.Warn("dispatch: close no-code validated ticket failed", "ticket", t.ID, "error", err)
					} else {
						slog.Info("dispatch: closed no-code validated ticket", "ticket", t.ID)
					}
				}
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

	// Process workflow phases that are ready for advancement.
	if ctx.Err() != nil {
		return
	}
	d.processReadyWorkflowPhases(ctx)

	// Re-check blocked gates (http_check polling, github_checks re-eval).
	if ctx.Err() != nil {
		return
	}
	d.recheckBlockedGates(ctx)

	// Check for phase timeouts.
	if ctx.Err() != nil {
		return
	}
	d.checkPhaseTimeouts(ctx)

	if ctx.Err() != nil {
		return
	}
	pending, err := d.tickets.ListByState(ctx, d.cfg.ProjectID, ticket.StateDraft)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("dispatch: scan pending failed", "error", err)
		return
	}

	if len(pending) > scanLimit {
		pending = pending[:scanLimit]
	}

	// Recover draft tickets that have orphaned open PRs (e.g. from rollback after submit).
	recovered := d.reconcileOrphanedPRs(ctx, pending)

	slog.Info("dispatch: scan found pending tickets", "count", len(pending))
	for _, t := range pending {
		if recovered[t.ID] {
			continue // already recovered to awaiting_validation
		}
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
	proj, err := d.cachedGetProject(ctx, projectID)
	if err != nil || proj == nil {
		return true // fail-open: don't block dispatch if project lookup fails
	}
	return proj.DispatchEnabled
}

// projectHasRepo checks whether a project has a repo_url configured.
// Returns false if the project has no repo — workers must not execute against
// the server's own codebase when no target repo is defined.
func (d *Dispatcher) projectHasRepo(ctx context.Context, projectID string) bool {
	proj, err := d.cachedGetProject(ctx, projectID)
	if err != nil || proj == nil {
		return false // fail-closed: don't dispatch if we can't verify the repo
	}
	return proj.RepoURL != ""
}

// resolveProjectRepoDir returns a local directory for the project's git repo.
// Always uses the clone manager to create an isolated clone — never falls back
// to the server's own codebase. This prevents cross-project repo contamination.
func (d *Dispatcher) resolveProjectRepoDir(ctx context.Context, projectID string) (string, error) {
	proj, err := d.cachedGetProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("get project: %w", err)
	}
	if proj.RepoURL == "" {
		return "", fmt.Errorf("project %s has no repo_url configured", projectID)
	}
	if d.clones == nil {
		return "", fmt.Errorf("clone manager not initialized — cannot safely resolve repo for project %s", projectID)
	}
	return d.clones.EnsureClone(proj.RepoURL, proj.ID)
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
	d.advanceWorkflowIfNeeded(ctx, t, "failed")
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

// handleTicketCancelled cancels workers and cleans up the branch on cancellation.
func (d *Dispatcher) handleTicketCancelled(ctx context.Context, e events.Event) {
	ticketID, _ := e.Payload["ticket_id"].(string)
	projectID, _ := e.Payload["project_id"].(string)
	if ticketID == "" {
		return
	}

	// Cancel any active worker for this ticket.
	d.mu.Lock()
	for _, key := range []string{ticketID, "review:" + ticketID, "resolve:" + ticketID} {
		if cancel, ok := d.active[key]; ok {
			cancel()
			delete(d.active, key)
			delete(d.activeProjects, key)
		}
	}
	d.mu.Unlock()

	// Safe to delete branch: ticket explicitly cancelled.
	// Only clean up branches if the project has a repo configured.
	if projectID != "" {
		if d.projectHasRepo(ctx, projectID) {
			d.cleanupTicketBranch(ctx, ticketID, projectID)
		} else {
			_ = d.worktrees.Remove(ticketID)
		}
	} else {
		_ = d.worktrees.Remove(ticketID)
	}

	slog.Info("dispatch: ticket cancelled, branch cleaned up", "ticket", ticketID)
}

// handleTicketInputProvided re-dispatches a ticket after a human provides input.
// The ticket exits awaiting_input → executing or planning, but the original worker
// has already exited. We release the stale lease and re-dispatch immediately.
func (d *Dispatcher) handleTicketInputProvided(ctx context.Context, e events.Event) {
	ticketID, _ := e.Payload["ticket_id"].(string)
	if ticketID == "" {
		return
	}

	t, err := d.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		slog.Error("dispatch: input_provided get ticket failed", "ticket", ticketID, "error", err)
		return
	}
	if d.cfg.ProjectID != "" && t.ProjectID != d.cfg.ProjectID {
		return
	}
	if !d.isProjectDispatchEnabled(ctx, t.ProjectID) {
		return
	}

	switch t.State {
	case ticket.StateDraft:
		// Lease already expired before event arrived — just dispatch.
		slog.Info("dispatch: input_provided, ticket already draft, dispatching", "ticket", ticketID)
		d.tryDispatch(ctx, t)

	case ticket.StateExecuting, ticket.StatePlanning:
		// At-least-once delivery guard: skip if a worker is already running.
		d.mu.Lock()
		_, running := d.active[ticketID]
		d.mu.Unlock()
		if running {
			slog.Info("dispatch: input_provided, worker already active, skipping", "ticket", ticketID)
			return
		}

		// Release the stale lease to transition back to draft.
		if d.leaseReleaser == nil {
			slog.Warn("dispatch: input_provided, no lease releaser configured", "ticket", ticketID)
			return
		}
		if err := d.leaseReleaser.ForceReleaseLease(ctx, ticketID); err != nil {
			slog.Error("dispatch: input_provided, lease release failed", "ticket", ticketID, "error", err)
			return
		}

		// Re-read ticket after lease release (now draft) and dispatch.
		t, err = d.tickets.GetTicket(ctx, ticketID)
		if err != nil {
			slog.Error("dispatch: input_provided, re-read ticket failed", "ticket", ticketID, "error", err)
			return
		}
		slog.Info("dispatch: input_provided, lease released, dispatching", "ticket", ticketID)
		d.tryDispatch(ctx, t)

	default:
		slog.Warn("dispatch: input_provided, unexpected state", "ticket", ticketID, "state", t.State)
	}
}

func (d *Dispatcher) tryDispatch(ctx context.Context, t *ticket.Ticket) {
	if t.State != ticket.StateDraft {
		return
	}

	// Priority guard: don't start new work if review/merge work is waiting.
	if d.hasHigherPriorityWork(ctx, t.ProjectID) {
		slog.Info("dispatch: deferring new work, higher-priority tickets need attention",
			"ticket", t.ID, "project", t.ProjectID)
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
		// Require deps to be closed, matching guardDependenciesMet (the claim
		// guard). Accepting "validated" here only spawns a worker that then
		// fails its self-claim, churning retries.
		for _, dep := range deps {
			if dep.State != ticket.StateClosed {
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
	if t.State != ticket.StateAwaitingValidation {
		return
	}
	// Reset review attempt counter on fresh submission so the count doesn't
	// carry over from a previous reject→re-execute→submit cycle.
	if t.Outputs != nil {
		if _, had := t.Outputs["_review_attempts"]; had {
			d.persistReviewAttempt(ctx, t.ID, 0)
		}
	}
	nextPhase := d.advanceWorkflowIfNeeded(ctx, t, "success")
	if nextPhase != nil {
		switch nextPhase.Type {
		case workflow.PhaseAgent:
			// If the next phase is a validator-like agent, spawn reviewer.
			agentCfg, _ := workflow.ParseAgentConfig(nextPhase.Config)
			if agentCfg != nil && agentCfg.Role == "validator" {
				d.spawnReviewer(ctx, t)
				return
			}
			// Other agent phases: spawn a regular worker.
			d.spawn(ctx, t)
			return
		case workflow.PhaseGate, workflow.PhaseExternal, workflow.PhaseAction:
			// Non-agent phases: set status=ready and let processReadyPhase handle it.
			if d.workflowPhaseUpdater != nil {
				_ = d.workflowPhaseUpdater.UpdateWorkflowPhaseStatus(ctx, t.ID, "ready")
			}
			return
		}
	}
	// No workflow or legacy: fall through to default behavior.
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

	// Advance workflow and auto-merge the PR if outputs contain a pr_url.
	t, err := d.tickets.GetTicket(ctx, ticketID)
	if err == nil && t != nil {
		d.advanceWorkflowIfNeeded(ctx, t, "success")
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

// advanceWorkflowIfNeeded advances the workflow phase for a ticket if it has an active workflow.
// Returns the next phase so callers can make routing decisions. Returns nil if
// there is no workflow, the workflow is complete, or an error occurred.
//
// Contract: ticket state controls lifecycle permissions (who can do what);
// workflow phase controls dispatch routing (what work to do next). They advance
// at the same integration points but are driven by different actors.
func (d *Dispatcher) advanceWorkflowIfNeeded(ctx context.Context, t *ticket.Ticket, outcome string) *workflow.Phase {
	if t.WorkflowID == "" || t.WorkflowPhase == "" || d.workflowEngine == nil {
		return nil
	}
	next, err := d.workflowEngine.AdvancePhase(ctx, t.ID, t.WorkflowID, t.WorkflowPhase, outcome, nil, t.WorkflowVersion)
	if err != nil {
		slog.Error("dispatch: workflow advance failed", "ticket", t.ID, "error", err)
		return nil
	}
	if next != nil {
		t.WorkflowPhase = next.ID
	}
	return next
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

			// Priority: spawn any waiting reviewers before scanning for new work.
			d.spawnWaitingReviewers(ctx, t.ProjectID)

			// Then scan for remaining work.
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
	case ticket.StateAwaitingValidation:
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

	// Resolve phase overrides from workflow definition if available.
	var phaseOverrides *PhaseOverrides
	if t.WorkflowID != "" && t.WorkflowPhase != "" && d.workflowEngine != nil {
		if pos, err := d.workflowEngine.GetPosition(ctx, t.ID, t.WorkflowID, t.WorkflowPhase, t.WorkflowVersion); err == nil && pos != nil && pos.CurrentPhase != nil {
			if pos.CurrentPhase.Type == workflow.PhaseAgent {
				if agentCfg, parseErr := workflow.ParseAgentConfig(pos.CurrentPhase.Config); parseErr == nil {
					phaseOverrides = &PhaseOverrides{Goal: agentCfg.Goal, Prompt: agentCfg.Prompt}
					// Phase role takes highest precedence: phase config > ticket input hint > state default.
					if agentCfg.Role != "" {
						role = agentCfg.Role
						if projectWt, ok := workerTypeForConfiguredRole(proj, agentCfg.Role); ok {
							wt = projectWt
						} else if parsed := WorkerType(agentCfg.Role); parsed.IsValid() {
							wt = parsed
						}
					}
				}
			}
		}
	}

	// Assemble type-specific prompt.
	prompt := AssembleTypedWorkerPrompt(wt, proj, t, depOutputs, d.cfg.ServerURL, d.cfg.AgentID, phaseOverrides)
	if roleDef, ok := dispatchRoleDefinition(proj, role); ok {
		prompt = appendCustomRoleContext(prompt, roleDef)
	}

	// Determine working directory.
	var workDir string
	if proj.RepoURL == "" {
		// No-repo mode: create a temp scratch directory. Workers get MCP access
		// but no git workspace. Suitable for operators, triage, and non-code tasks.
		tmpDir, tmpErr := os.MkdirTemp("", "flywheel-norepo-"+t.ID+"-")
		if tmpErr != nil {
			return fmt.Errorf("dispatch: create temp workdir: %w", tmpErr)
		}
		workDir = tmpDir
		slog.Info("dispatch: no-repo mode, using temp workdir", "ticket", t.ID, "workdir", workDir)
	} else {
		// Repo mode: every project gets its own isolated clone via resolveProjectRepoDir.
		// Workers must never use d.cfg.RepoDir (the server's own codebase).
		repoDir, err := d.resolveProjectRepoDir(ctx, t.ProjectID)
		if err != nil {
			return fmt.Errorf("dispatch: resolve project repo: %w", err)
		}

		// If ticket targets a secondary repo, resolve that instead.
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
			branch := d.branchForTicket(ctx, t)
			var err error
			workDir, err = d.worktrees.CreateFromRepo(t.ID, branch, repoDir, proj.DefaultBranch)
			if err != nil {
				// If ancestry validation failed, try resetting the clone and retrying once.
				if strings.Contains(err.Error(), "no common history") && d.clones != nil {
					slog.Warn("dispatch: ancestry validation failed, resetting clone and retrying", "ticket", t.ID, "error", err)
					cloneAlias := proj.ID
					if t.TargetRepo != "" {
						cloneAlias = proj.ID + "/" + t.TargetRepo
					}
					if resetErr := d.clones.ResetClone(cloneAlias); resetErr != nil {
						slog.Error("dispatch: clone reset failed", "ticket", t.ID, "error", resetErr)
					} else {
						// Re-ensure the clone after reset.
						repoURL := proj.RepoURL
						if t.TargetRepo != "" && d.repoResolver != nil {
							if resolved, _, resolveErr := d.repoResolver.ResolveRepo(ctx, t.ProjectID, t.TargetRepo); resolveErr == nil && resolved != "" {
								repoURL = resolved
							}
						}
						if newCloneDir, cloneErr := d.clones.EnsureClone(repoURL, cloneAlias); cloneErr == nil {
							repoDir = newCloneDir
							workDir, err = d.worktrees.CreateFromRepo(t.ID, branch, repoDir, proj.DefaultBranch)
						}
					}
				}
				if err != nil {
					// Inject failure context so the next attempt knows why this failed.
					if d.failureSummarizer != nil {
						reason := fmt.Sprintf("Worktree creation failed: %s", err)
						_ = d.failureSummarizer.AppendFailureSummary(ctx, t.ID, reason)
					}
					return err
				}
			}
			// Persist branch name so merge/cleanup know exactly which branch to target.
			if d.outputPatcher != nil {
				_ = d.outputPatcher.PatchOutputs(ctx, t.ID, map[string]any{
					"_branch": branch,
				})
			}
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
	// If no repo is configured and we'd default to executor, use operator instead.
	if wt == WorkerTypeExecutor && proj.RepoURL == "" {
		wt = WorkerTypeOperator
	}
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
// After maxWorkerAttempts consecutive crashes, the ticket is escalated to
// awaiting_input instead of being retried, breaking the crash loop.
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

	// Count consecutive worker_exit attempts to detect crash loops.
	exitAttempts := 0
	for _, a := range t.Context.PriorAttempts {
		if a.Outcome == "worker_exit" {
			exitAttempts++
		}
	}

	reason := fmt.Sprintf("Worker exited without submitting or escalating (state: %s). "+
		"Check the execution trace via get_trace for the previous agent's detailed log.", t.State)

	// Inject failure context before taking action.
	if d.failureSummarizer != nil {
		if err := d.failureSummarizer.AppendFailureSummary(ctx, ticketID, reason); err != nil {
			slog.Error("dispatch: failed to inject failure summary", "ticket", ticketID, "error", err)
		}
	}

	// After maxWorkerAttempts crashes, escalate to awaiting_input instead of retrying.
	// The +1 accounts for the attempt we just appended above.
	if exitAttempts+1 >= maxWorkerAttempts && d.ticketTransitioner != nil {
		escalationReason := fmt.Sprintf(
			"Worker crashed %d times without submitting or escalating. "+
				"The ticket has been moved to awaiting_input to prevent further retries. "+
				"Review the execution trace and prior_attempts, then provide_input to retry or cancel the ticket.",
			exitAttempts+1)

		slog.Warn("dispatch: worker crash loop detected, escalating to awaiting_input",
			"ticket", ticketID, "attempts", exitAttempts+1, "max", maxWorkerAttempts)

		// Transition using the leaseholder identity (the lease is still held by the worker agent).
		actor := ticket.Actor{ID: t.AssignedTo, Type: ticket.ActorAgent}
		// TriggerEscalate only works from executing; use TriggerRequestInput for planning.
		trigger := ticket.TriggerEscalate
		payload := map[string]any{"reason": escalationReason}
		if t.State == ticket.StatePlanning {
			trigger = ticket.TriggerRequestInput
		}
		if err := d.ticketTransitioner.TransitionTicket(ctx, ticketID, trigger, actor, payload); err != nil {
			slog.Error("dispatch: failed to escalate crash-looping ticket", "ticket", ticketID, "error", err)
			// Fall through to ForceReleaseLease as a last resort.
		} else {
			// Publish escalation event for observers/notifications.
			_ = d.bus.Publish(ctx, events.Event{
				Type: events.EventTicketEscalated,
				Payload: map[string]any{
					"ticket_id":  t.ID,
					"project_id": t.ProjectID,
					"reason":     escalationReason,
					"source":     "worker_crash_loop",
				},
			})
			return
		}
	}

	slog.Warn("dispatch: worker exited without completing, releasing lease",
		"ticket", ticketID, "state", t.State, "attempt", exitAttempts+1)

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
	proj, err := d.cachedGetProject(ctx, projectID)
	if err != nil || proj == nil {
		return limit
	}
	if configured := proj.DispatchConfig.Normalized().MaxActiveWorkers; configured > 0 {
		// Per-project config can restrict below the server limit but never exceed it.
		if limit <= 0 || configured < limit {
			return configured
		}
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

// hasHigherPriorityWork returns true if there are awaiting_review or validated
// tickets for the project that need attention but don't have an active worker.
// It actively tries to resolve blockers: spawns reviewers for unreviewed tickets
// and triggers autoMergePR for validated tickets with PRs. Only blocks if work
// remains that genuinely needs attention.
func (d *Dispatcher) hasHigherPriorityWork(ctx context.Context, projectID string) bool {
	blocked := false

	// Check for awaiting_review tickets that need a reviewer.
	reviewing, err := d.tickets.ListByState(ctx, projectID, ticket.StateAwaitingValidation)
	if err == nil {
		for _, t := range reviewing {
			d.mu.Lock()
			_, active := d.active["review:"+t.ID]
			d.mu.Unlock()
			if !active {
				d.spawnReviewer(ctx, t)
				blocked = true
			}
		}
	}

	// Check for validated tickets with PRs that need merge.
	validated, err := d.tickets.ListByState(ctx, projectID, ticket.StateValidated)
	if err == nil {
		for _, t := range validated {
			prURL, _ := t.Outputs["pr_url"].(string)
			if !isValidPRURL(prURL) {
				continue
			}
			// Skip tickets whose merge has been escalated or exhausted attempts —
			// they need human intervention and should not block new work.
			if mergeStatus, _ := t.Outputs["_merge_status"].(string); mergeStatus == "escalated" {
				continue
			}
			if v, ok := t.Outputs["_merge_attempts"]; ok {
				var att int
				switch n := v.(type) {
				case float64:
					att = int(n)
				case int:
					att = n
				}
				if att >= maxMergeAttempts {
					continue
				}
			}
			d.mu.Lock()
			_, resolving := d.active["resolve:"+t.ID]
			d.mu.Unlock()
			if !resolving {
				// Try to merge/close — autoMergePR handles already-merged PRs.
				d.autoMergePR(ctx, t, prURL)
				// Re-check: if autoMergePR closed the ticket, it's no longer blocking.
				fresh, ferr := d.tickets.GetTicket(ctx, t.ID)
				if ferr == nil && fresh != nil && fresh.State == ticket.StateValidated {
					blocked = true
				}
			}
		}
	}
	return blocked
}

// Review orchestration (spawnWaitingReviewers, spawnReviewer, prHeadCommit,
// persistReviewedCommit, runReviewer), PR reconciliation (reconcileAwaitingValidationPRs,
// isValidPRURL), and merge automation (autoMergePR, publishMergedEvent,
// closeMergedTicket) are in merge.go.

// processReadyPhase handles exactly one workflow phase for a ticket.
// It reads the current phase from the workflow definition and executes the
// appropriate handler. Phases that complete synchronously advance to the next
// phase (setting status=ready for the reconcile loop to pick up).
// Phase processing (processReadyPhase, processReadyWorkflowPhases,
// recheckBlockedGates, checkPhaseTimeouts) are in phase_executor.go.

// Merge helpers (escalateMergeFailure, persistMergeState, branchForTicket,
// branchFromOutputs, cleanupTicketBranch, validatePRChecks, truncate,
// spawnConflictResolver, runConflictResolver) are in merge.go.

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

// PR/merge/review functions are in merge.go.
