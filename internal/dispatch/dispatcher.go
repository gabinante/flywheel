package dispatch

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/matt0x6f/warrant/events"
	"github.com/matt0x6f/warrant/internal/project"
	"github.com/matt0x6f/warrant/internal/ticket"
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

// Config holds dispatcher settings.
type Config struct {
	MaxWorkers   int
	ClaudePath   string
	WorktreeDir  string
	RepoDir      string // path to the main git repository
	ServerURL    string // warrant server URL for MCP connections
	AgentID      string // agent identity for workers
	APIKey       string // warrant API key for worker MCP authentication
	ProjectID    string // only dispatch tickets for this project (empty = all)
	AutoApprove  bool   // auto-approve tickets when acceptance tests pass
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
	cfg       Config
	bus       events.Bus
	tickets   TicketGetter
	projects  ProjectGetter
	worker    Worker
	worktrees *WorktreeManager

	mu       sync.Mutex
	active   map[string]context.CancelFunc // ticketID → cancel
	wg       sync.WaitGroup
}

// New creates a dispatcher that subscribes to the event bus.
func New(cfg Config, bus events.Bus, tickets TicketGetter, projects ProjectGetter) *Dispatcher {
	var worker Worker
	if cfg.DockerEnabled {
		worker = &DockerWorker{
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
			ClaudePath: cfg.ClaudePath,
			APIKey:     cfg.APIKey,
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
		d.handleTicketReady(ctx, e)
	})
	d.bus.Subscribe(events.EventTicketDone, func(_ context.Context, e events.Event) {
		d.handleTicketDone(e)
	})
	d.bus.Subscribe(events.EventTicketApproved, func(_ context.Context, e events.Event) {
		d.handleTicketDone(e)
	})

	log.Printf("dispatch: started (max_workers=%d, worktree_dir=%s, project=%s)", d.cfg.MaxWorkers, d.cfg.WorktreeDir, d.cfg.ProjectID)

	// Scan for existing pending tickets on startup.
	if d.cfg.ProjectID != "" {
		go d.scanPending(ctx)
	}
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

func (d *Dispatcher) scanPending(ctx context.Context) {
	// Small delay to let the server finish starting.
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Second):
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
			if dep.State != ticket.StateDone {
				return
			}
		}
	}

	d.spawn(ctx, t)
}

func (d *Dispatcher) handleTicketDone(e events.Event) {
	ticketID, _ := e.Payload["ticket_id"].(string)
	if ticketID == "" {
		return
	}

	// Clean up worktree.
	d.mu.Lock()
	if cancel, ok := d.active[ticketID]; ok {
		cancel()
		delete(d.active, ticketID)
	}
	d.mu.Unlock()

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
		}()

		if err := d.runWorker(workerCtx, t); err != nil {
			log.Printf("dispatch: worker %s failed: %v", t.ID, err)
		}
	}()

	log.Printf("dispatch: spawned worker for %s (%d/%d active)", t.ID, d.activeCount(), d.cfg.MaxWorkers)
}

func (d *Dispatcher) runWorker(ctx context.Context, t *ticket.Ticket) error {
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

	// Assemble prompt.
	prompt := AssembleWorkerPrompt(proj, t, depOutputs, d.cfg.ServerURL, d.cfg.AgentID)

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

	// Spawn worker.
	result, err := d.worker.Spawn(ctx, t.ID, prompt, workDir, d.cfg.ServerURL)
	if err != nil {
		return err
	}

	if !result.Success {
		log.Printf("dispatch: worker %s completed with error: %s\nOutput: %s", t.ID, result.Error, result.Output)
	} else {
		log.Printf("dispatch: worker %s completed successfully", t.ID)
	}

	return nil
}

func (d *Dispatcher) activeCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.active)
}
