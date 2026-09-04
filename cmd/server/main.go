package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/gabinante/flywheel/api/mcp"
	"github.com/gabinante/flywheel/api/rest"
	"github.com/gabinante/flywheel/config"
	"github.com/gabinante/flywheel/db"
	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/auth"
	"github.com/gabinante/flywheel/internal/codereview"
	"github.com/gabinante/flywheel/internal/dispatch"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/gate"
	"github.com/gabinante/flywheel/internal/harness"
	"github.com/gabinante/flywheel/internal/linear"
	"github.com/gabinante/flywheel/internal/orchestrator"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/progress"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/queue"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/sessions"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/user"
	"github.com/gabinante/flywheel/internal/workflow"
	"github.com/gabinante/flywheel/internal/workstream"

	"github.com/redis/go-redis/v9"
)

// requirementCheckerBridge adapts gate.CheckerRegistry to ticket.RequirementChecker.
type requirementCheckerBridge struct {
	registry *gate.CheckerRegistry
}

func (b *requirementCheckerBridge) CheckRequirements(ctx context.Context, requirements []ticket.PolicyRequirement, ticketID, projectID, prURL string) []ticket.PolicyRequirementStatus {
	reqs := make([]gate.GateRequirement, len(requirements))
	for i, r := range requirements {
		reqs[i] = gate.GateRequirement{
			Type:   gate.GateRequirementType(r.Type),
			Config: r.Config,
		}
	}
	statuses := b.registry.CheckAll(ctx, reqs, gate.CheckContext{
		TicketID:  ticketID,
		ProjectID: projectID,
		PRURL:     prURL,
	})
	result := make([]ticket.PolicyRequirementStatus, len(statuses))
	for i, s := range statuses {
		result[i] = ticket.PolicyRequirementStatus{
			Requirement: ticket.PolicyRequirement{
				Type:   string(s.Requirement.Type),
				Config: s.Requirement.Config,
			},
			Satisfied: s.Satisfied,
			Reason:    s.Reason,
		}
	}
	return result
}

// leaseValidatorAdapter adapts queue.LeaseStore to execution.LeaseValidator.
type leaseValidatorAdapter struct {
	leases queue.LeaseStore
}

func (a *leaseValidatorAdapter) ValidateLease(ctx context.Context, ticketID, token string) (string, error) {
	data, err := a.leases.ValidateToken(ctx, ticketID, token)
	if err != nil || data == nil {
		return "", err
	}
	return data.AgentID, nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := config.Load()
	run(context.Background(), cfg)
}

func run(ctx context.Context, cfg *config.Config) {
	// Cancellable context for all background services. Cancelling it during
	// shutdown stops goroutines before the pool and Redis are closed.
	ctx, cancelServices := context.WithCancel(ctx)

	pool, err := db.NewPool(ctx, cfg.DB.URL)
	if err != nil {
		cancelServices()
		slog.Error("db init failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		slog.Warn("pool.Close() called — this should only happen at shutdown",
			"stack", string(debug.Stack()))
		pool.Close()
	}()
	defer cancelServices() // LIFO: context cancelled BEFORE pool closes

	// Event bus: Postgres durable bus by default for at-least-once delivery.
	var bus events.DurableEventBus
	if os.Getenv("DURABLE_BUS") == "false" {
		bus = events.NewInProcessBus()
		slog.Info("events: using in-process bus (no durability)")
	} else {
		bus = events.NewPostgresBus(pool, events.PostgresBusConfig{})
		slog.Info("events: using Postgres durable bus (at-least-once delivery)")
	}

	orgStore := org.NewStore(pool)
	orgSvc := org.NewService(orgStore)
	projectStore := project.NewStore(pool)
	projectSvc := project.NewService(projectStore)
	repoSvc := project.NewRepositoryService(project.NewRepositoryStore(pool))
	orchestratorStore := orchestrator.NewStore(pool)
	workStreamStore := workstream.NewStore(pool)
	workStreamSvc := workstream.NewService(workStreamStore)
	ticketStore := ticket.NewStore(pool)
	transitionStore := ticket.NewTransitionStore(pool)
	ticketSvc := ticket.NewService(ticketStore, bus, projectSvc)
	ticketSvc.SetTransitionStore(transitionStore)
	if cfg.RunAcceptanceTestOnSubmit {
		ticketSvc.SetAcceptanceRunner(&ticket.ShellAcceptanceRunner{})
	}
	if cfg.Dispatch.AutoApproveOnAcceptancePass {
		ticketSvc.SetAutoApproveOnPass(true)
	}

	// Linear projections: ticket reads carry their Linear issue and accept identifiers.
	linearStore := linear.NewStore(pool)
	ticketSvc.SetExternalRefLookup(linearStore)

	// Workflow engine: configurable pipelines per system/org/project.
	workflowStore := workflow.NewStore(pool)
	workflowEngine := workflow.NewEngine(workflowStore, ticketStore)
	workflowResolver := workflow.NewResolver(workflowEngine)
	ticketSvc.SetWorkflowResolver(workflowResolver)

	// Gate requirement checkers: automated conditions that must pass for gate phases.
	checkerRegistry := gate.NewCheckerRegistry()
	checkerRegistry.Register(gate.RequireGitHubChecks, &gate.GitHubChecksChecker{})
	checkerRegistry.Register(gate.RequireHumanApproval, &gate.HumanApprovalChecker{})
	checkerRegistry.Register(gate.RequireHTTPCheck, &gate.HTTPCheckChecker{Client: &http.Client{Timeout: 10 * time.Second}})
	checkerRegistry.Register(gate.RequireWebhook, &gate.WebhookChecker{})
	ticketSvc.SetRequirementChecker(&requirementCheckerBridge{registry: checkerRegistry})

	redisOpts, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		cancelServices()
		slog.Error("redis URL parse failed", "error", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		cancelServices()
		slog.Error("redis ping failed", "error", err)
		os.Exit(1)
	}
	leaseTTL := time.Duration(cfg.Queue.LeaseTTLMinutes) * time.Minute
	queueRedis := queue.NewRedisStore(redisClient, leaseTTL)
	queueSvc := queue.NewService(ticketSvc, ticketSvc, queueRedis)
	scheduler := queue.NewScheduler(queueRedis, ticketSvc, ticketSvc, bus, 30*time.Second)
	scheduler.EnableStalenessSweep(ticketSvc, 0) // default: 2x lease TTL
	scheduler.SetFailureSummarizer(ticketSvc)
	go scheduler.Run(ctx)

	// Operator JWT secret. Generated per process when unset; the UI then needs
	// to sign in again after a restart.
	jwtSecret := cfg.Auth.JWTSecret
	if jwtSecret == "" {
		jwtSecret = autoGenerateSecret()
		slog.Warn("auth: JWT_SECRET unset; generated a per-process secret (UI sessions will not survive restarts)")
	}

	// Workflow callback + external executor: async phase support.
	callbackStore := workflow.NewPostgresCallbackStore(pool)
	callbackHandler := workflow.NewCallbackHandler([]byte(jwtSecret), callbackStore, workflowEngine)
	externalExecutor := workflow.NewExternalExecutor(callbackHandler, cfg.Auth.BaseURL)

	leaseValidator := &leaseValidatorAdapter{leases: queueRedis}
	agentStore := agent.NewStore(pool)
	agentSvc := agent.NewService(agentStore)
	execStore := execution.NewStore(pool)
	execSvc := execution.NewService(execStore, leaseValidator)
	reviewStore := review.NewStore(pool)
	reviewSvc := review.NewService(reviewStore, ticketSvc, bus)
	userStore := user.NewStore(pool)

	// Linear as the ticket store: discover led projects, mirror issues, push changes back.
	var linearClient *linear.Client
	if cfg.Linear.APIKey != "" && cfg.Linear.Enabled {
		linearClient = linear.NewClient(cfg.Linear.APIKey, "")
	}
	linearSvc := linear.NewSyncer(linearClient, linearStore, ticketSvc, projectSvc, bus, linear.Config{
		Enabled:        cfg.Linear.Enabled,
		ProjectIDs:     cfg.Linear.ProjectIDs,
		Interval:       cfg.Linear.Interval,
		DefaultTeamKey: cfg.Linear.DefaultTeamKey,
	})
	linearSvc.Start(ctx)

	// Session tracking: ingest Claude Code and Codex sessions from their local stores.
	sessionsSvc := sessions.NewService(sessions.NewStore(pool), sessions.Config{
		Enabled:   cfg.Sessions.Enabled,
		ClaudeDir: cfg.Sessions.ClaudeDir,
		CodexDir:  cfg.Sessions.CodexDir,
		Interval:  cfg.Sessions.Interval,
	})
	sessionsSvc.Start(ctx)

	// Code review: PR-keyed reviews by a local harness, posted through the operator's gh CLI.
	harnessCfg := harness.Config{ClaudeBin: cfg.Dispatch.ClaudePath}
	if cfg.Dispatch.AgentDriver == "codex" && cfg.Dispatch.AgentCLIPath != "" {
		harnessCfg.CodexBin = cfg.Dispatch.AgentCLIPath
	}
	harnessRunner := harness.New(harnessCfg)
	codeReviewSvc := codereview.New(codereview.NewStore(pool), codereview.NewGitHub(""), harnessRunner, sessionsSvc, bus, codereview.Config{
		Enabled:        cfg.Review.Enabled,
		Harness:        cfg.Review.Harness,
		Model:          cfg.Review.Model,
		Effort:         cfg.Review.Effort,
		Publish:        cfg.Review.Publish,
		PollInterval:   cfg.Review.PollInterval,
		MaxConcurrent:  cfg.Review.MaxConcurrent,
		RepoRoot:       cfg.Review.RepoRoot,
		WatchRequested: cfg.Review.WatchRequested,
		WatchAuthored:  cfg.Review.WatchAuthored,
		ReviewTimeout:  cfg.Review.Timeout,
		SkipDrafts:     cfg.Review.SkipDrafts,
	})
	codeReviewSvc.Start(ctx)

	strictServer := &rest.StrictServer{
		OrgSvc:        orgSvc,
		ProjectSvc:    projectSvc,
		WorkStreamSvc: workStreamSvc,
		TicketSvc:     ticketSvc,
		QueueSvc:      queueSvc,
		TraceSvc:      execSvc,
		ReviewSvc:     reviewSvc,
		SessionsSvc:   sessionsSvc,
		LinearSvc:     linearSvc,
		CodeReviewSvc: codeReviewSvc,
		AgentStore:    agentStore,
	}

	repoDir, _ := os.Getwd()
	orchestratorWorkerCfg := dispatch.Config{
		ClaudePath:           cfg.Dispatch.ClaudePath,
		AgentRunner:          cfg.Orchestrator.AgentRunner,
		AgentDriver:          cfg.Orchestrator.AgentDriver,
		AgentCLIPath:         cfg.Orchestrator.AgentCLIPath,
		AgentModel:           cfg.Orchestrator.AgentModel,
		AgentReasoningEffort: cfg.Orchestrator.AgentReasoningEffort,
		APIKey:               cfg.Dispatch.APIKey,
		AgentAPIKey:          cfg.Orchestrator.AgentAPIKey,
		RepoDir:              repoDir,
	}
	var orchestratorWorker dispatch.Worker
	if cfg.Orchestrator.Enabled {
		orchestratorWorker = dispatch.NewWorker(orchestratorWorkerCfg)
	}
	orchestratorSvc := orchestrator.NewService(ctx, orchestratorStore, projectSvc, orchestratorWorker, orchestrator.Config{
		Enabled:      cfg.Orchestrator.Enabled,
		RepoDir:      repoDir,
		ServerURL:    cfg.Auth.BaseURL,
		AgentID:      "command-center-orchestrator",
		HistoryLimit: cfg.Orchestrator.HistoryLimit,
		AgentRunner:  cfg.Orchestrator.AgentRunner,
		AgentDriver:  cfg.Orchestrator.AgentDriver,
		AgentModel:   cfg.Orchestrator.AgentModel,
		WorkerConfig: orchestratorWorkerCfg,
	})

	// Progress monitor: bridges ticket lifecycle events → command center system messages.
	_ = progress.NewMonitor(bus, orchestratorSvc, ticketSvc)

	// Auth: single local operator + API keys for agents.
	authMiddleware := rest.AuthMiddleware(jwtSecret, agentSvc)
	provisioner := &auth.Provisioner{UserStore: userStore, AgentStore: agentStore}
	authHandler := rest.NewAuthHandler(provisioner, orgSvc, jwtSecret, cfg.Auth.BaseURL, cfg.Auth.SuccessRedirectURL, 0)

	mcpSrv, err := mcp.NewServer(&mcp.Backend{
		Project:    projectSvc,
		WorkStream: workStreamSvc,
		Ticket:     ticketSvc,
		Queue:      queueSvc,
		Trace:      execSvc,
		Review:     reviewSvc,
		Org:        orgSvc,
		AgentStore: agentStore,
		Repos:      repoSvc,
		Workflow:   workflowEngine,
		Sessions:   sessionsSvc,
		CodeReview: codeReviewSvc,
	})
	if err != nil {
		slog.Error("mcp server init failed", "error", err)
		os.Exit(1)
	}
	mcpHandler := &rest.MCPHTTPHandler{
		Handler:   mcp.NewStreamableHTTPHandler(mcpSrv),
		BaseURL:   cfg.Auth.BaseURL,
		JWTSecret: jwtSecret,
		AgentSvc:  agentSvc,
	}
	mcpSSEHandler := &rest.MCPHTTPHandler{
		Handler:   mcp.NewSSEHandler(mcpSrv),
		BaseURL:   cfg.Auth.BaseURL,
		JWTSecret: jwtSecret,
		AgentSvc:  agentSvc,
	}

	// Dispatcher: background workers for tickets in agent phases.
	var dispatcher *dispatch.Dispatcher
	if cfg.Dispatch.Enabled {
		dispatcher = dispatch.New(dispatch.Config{
			MaxWorkers:           cfg.Dispatch.MaxWorkers,
			ClaudePath:           cfg.Dispatch.ClaudePath,
			WorktreeDir:          cfg.Dispatch.WorktreeDir,
			RepoDir:              repoDir,
			ServerURL:            cfg.Auth.BaseURL,
			AgentID:              "dispatch-worker",
			APIKey:               cfg.Dispatch.APIKey,
			ProjectID:            cfg.Dispatch.ProjectID,
			AutoApprove:          cfg.Dispatch.AutoApproveOnAcceptancePass,
			AgentRunner:          cfg.Dispatch.AgentRunner,
			AgentDriver:          cfg.Dispatch.AgentDriver,
			AgentCLIPath:         cfg.Dispatch.AgentCLIPath,
			AgentModel:           cfg.Dispatch.AgentModel,
			AgentReasoningEffort: cfg.Dispatch.AgentReasoningEffort,
			AgentAPIKey:          cfg.Dispatch.AgentAPIKey,
			ReconcileInterval:    cfg.Dispatch.ReconcileInterval,
			TraceSvc:             execSvc,
		}, bus, ticketSvc, projectSvc)
		dispatcher.SetLeaseReleaser(queueSvc)
		dispatcher.SetFailureSummarizer(ticketSvc)
		dispatcher.SetTicketTransitioner(ticketSvc)
		dispatcher.SetWorkflowEngine(workflowEngine)
		dispatcher.SetExternalExecutor(externalExecutor)
		dispatcher.SetActionRegistry(workflow.NewActionRegistry())
		dispatcher.SetCheckerRegistry(checkerRegistry)
		dispatcher.SetOutputPatcher(ticketStore)
		dispatcher.SetWorkflowPhaseUpdater(ticketStore)
		dispatcher.Start(ctx)
	}

	// Start durable bus delivery after all subscriptions are registered.
	if err := bus.Start(ctx); err != nil {
		slog.Error("event bus start failed", "error", err)
		os.Exit(1)
	}

	router := rest.NewRouter(rest.RouterConfig{
		StrictServer:    strictServer,
		AuthMiddleware:  authMiddleware,
		AuthHandler:     authHandler,
		MCPHandler:      mcpHandler,
		MCPSSEHandler:   mcpSSEHandler,
		AgentsHandler:   &rest.AgentsHandler{AgentSvc: agentSvc},
		DispatchHandler: &rest.DispatchHandler{Dispatcher: dispatcher},
		OrchestratorHandler: &rest.OrchestratorHandler{
			Service:    orchestratorSvc,
			ProjectSvc: projectSvc,
			OrgSvc:     orgSvc,
			AgentStore: agentStore,
		},
		WorkflowHandler: &rest.WorkflowHandler{
			Engine:          workflowEngine,
			Store:           workflowStore,
			TicketSvc:       ticketSvc,
			ProjectSvc:      projectSvc,
			OrgSvc:          orgSvc,
			AgentStore:      agentStore,
			CallbackHandler: callbackHandler,
		},
		WorkerConfigHandler: &rest.WorkerConfigHandler{
			BaseURL: cfg.Auth.BaseURL,
		},
		HealthCheckers: []rest.HealthChecker{
			&rest.PostgresHealthChecker{Pool: pool},
			&rest.RedisHealthChecker{Client: redisClient},
		},
		WebDist:        cfg.Server.WebDist,
		WebDevProxyURL: cfg.Server.WebDevProxyURL,
	})

	serve(cfg, router, dispatcher, bus, cancelServices)
}

// serve starts the HTTP server and blocks until SIGINT/SIGTERM. cancelServices
// stops background goroutines before the pool is closed.
func serve(cfg *config.Config, router http.Handler, dispatcher *dispatch.Dispatcher, bus events.DurableEventBus, cancelServices context.CancelFunc) {
	srv := &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout is intentionally omitted — it kills SSE connections.
	}

	go func() {
		slog.Info("server listening", "port", cfg.Server.Port, "url", cfg.Auth.BaseURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	cancelServices()
	if dispatcher != nil {
		dispatcher.Stop()
	}
	_ = bus.Stop()

	// Brief pause for background goroutines to observe the cancelled context
	// before pool.Close() runs.
	time.Sleep(100 * time.Millisecond)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("server stopped")
}

func autoGenerateSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
