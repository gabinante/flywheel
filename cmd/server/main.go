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
	"github.com/gabinante/flywheel/events/hooks"
	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/auth"
	"github.com/gabinante/flywheel/internal/catalog"
	"github.com/gabinante/flywheel/internal/claims"
	"github.com/gabinante/flywheel/internal/cost"
	"github.com/gabinante/flywheel/internal/delivery"
	"github.com/gabinante/flywheel/internal/dispatch"
	"github.com/gabinante/flywheel/internal/entity"
	"github.com/gabinante/flywheel/internal/environment"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/investigation"
	"github.com/gabinante/flywheel/internal/mirror"
	"github.com/gabinante/flywheel/internal/mirror/jira"
	"github.com/gabinante/flywheel/internal/mirror/linear"
	"github.com/gabinante/flywheel/internal/notification"
	notifyemail "github.com/gabinante/flywheel/internal/notification/email"
	notifyslack "github.com/gabinante/flywheel/internal/notification/slack"
	notifysms "github.com/gabinante/flywheel/internal/notification/sms"
	"github.com/gabinante/flywheel/internal/observation"
	"github.com/gabinante/flywheel/internal/orchestrator"
	"github.com/gabinante/flywheel/internal/progress"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/projecttemplate"
	"github.com/gabinante/flywheel/internal/pillar"
	"github.com/gabinante/flywheel/internal/stateindex"
	"github.com/gabinante/flywheel/internal/plan"
	"github.com/gabinante/flywheel/internal/policy"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/queue"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/rollback"
	"github.com/gabinante/flywheel/internal/stream"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/user"
	"github.com/gabinante/flywheel/internal/workflow"
	"github.com/gabinante/flywheel/internal/workstream"

	"github.com/redis/go-redis/v9"
)

// ticketPolicyBridge adapts policy.TicketPolicyAdapter to ticket.PolicyEvaluator,
// converting between the policy and ticket package types to avoid circular imports.
type ticketPolicyBridge struct {
	adapter *policy.TicketPolicyAdapter
}

func (b *ticketPolicyBridge) EvaluateForTicket(ctx context.Context, t *ticket.Ticket, trigger string) (*ticket.PolicyDecision, error) {
	var services []string
	if len(t.Context.Constraints) > 0 {
		services = policy.ExtractServicesFromConstraints(t.Context.Constraints)
	}

	pd, err := b.adapter.EvaluateForTicketData(ctx, t.ProjectID, "", string(t.Type), int(t.Priority), services, trigger)
	if err != nil {
		return nil, err
	}
	if pd == nil {
		return nil, nil
	}

	result := &ticket.PolicyDecision{
		Action: string(pd.Action),
		Reason: "",
	}
	if pd.EffectiveRule != nil {
		result.Reason = pd.EffectiveRule.Reason
	}
	for _, mr := range pd.MatchedRules {
		result.MatchedRules = append(result.MatchedRules, ticket.MatchedRule{
			RuleID:   mr.RuleID,
			RuleName: mr.RuleName,
			Action:   string(mr.Action),
			Reason:   mr.Reason,
		})
	}
	for _, req := range pd.Requirements {
		result.Requirements = append(result.Requirements, ticket.PolicyRequirement{
			Type:   string(req.Type),
			Config: req.Config,
		})
	}
	return result, nil
}

// requirementCheckerBridge adapts policy.CheckerRegistry to ticket.RequirementChecker.
type requirementCheckerBridge struct {
	registry *policy.CheckerRegistry
}

func (b *requirementCheckerBridge) CheckRequirements(ctx context.Context, requirements []ticket.PolicyRequirement, ticketID, projectID, prURL string) []ticket.PolicyRequirementStatus {
	// Convert ticket requirements to policy requirements.
	policyReqs := make([]policy.GateRequirement, len(requirements))
	for i, r := range requirements {
		policyReqs[i] = policy.GateRequirement{
			Type:   policy.GateRequirementType(r.Type),
			Config: r.Config,
		}
	}

	rctx := policy.CheckContext{
		TicketID:  ticketID,
		ProjectID: projectID,
		PRURL:     prURL,
	}

	statuses := b.registry.CheckAll(ctx, policyReqs, rctx)

	// Convert back to ticket types.
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
	ctx := context.Background()
	runPostgres(ctx, cfg)
}

// runPostgres is the original Postgres+Redis startup path.
func runPostgres(ctx context.Context, cfg *config.Config) {
	// Create a cancellable context for all background services. Cancelling this
	// during shutdown ensures goroutines stop before the pool and Redis are closed,
	// preventing "closed pool" errors.
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

	// Create event bus: use Postgres durable bus by default for at-least-once delivery.
	// Falls back to in-process bus if DURABLE_BUS=false is set.
	var bus events.DurableEventBus
	if os.Getenv("DURABLE_BUS") == "false" {
		bus = events.NewInProcessBus()
		slog.Info("events: using in-process bus (no durability)")
	} else {
		pgBus := events.NewPostgresBus(pool, events.PostgresBusConfig{})
		bus = pgBus
		slog.Info("events: using Postgres durable bus (at-least-once delivery)")
	}

	orgStore := org.NewStore(pool)
	orgSvc := org.NewService(orgStore)
	projectStore := project.NewStore(pool)
	projectSvc := project.NewService(projectStore)
	projectTemplateStore := projecttemplate.NewStore(pool)
	projectTemplateSvc := projecttemplate.NewService(projectTemplateStore)
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

	// Workflow engine: configurable SDLC pipelines per system/org/project.
	workflowStore := workflow.NewStore(pool)
	workflowEngine := workflow.NewEngine(workflowStore, ticketStore)
	workflowResolver := workflow.NewResolver(workflowEngine)
	ticketSvc.SetWorkflowResolver(workflowResolver)

	// Policy layer: composable rules with most-restrictive-wins semantics.
	postureStore := policy.NewPostgresStore(pool)
	postureSvc := policy.NewPostureService(postureStore, bus)
	policyAdapter := policy.NewTicketPolicyAdapter(postureSvc)
	ticketSvc.SetPolicyEvaluator(&ticketPolicyBridge{adapter: policyAdapter})

	// Gate requirement checkers: automated conditions that must pass for transitions.
	checkerRegistry := policy.NewCheckerRegistry()
	checkerRegistry.Register(policy.RequireGitHubChecks, &policy.GitHubChecksChecker{})
	checkerRegistry.Register(policy.RequireHumanApproval, &policy.HumanApprovalChecker{})
	checkerRegistry.Register(policy.RequireHTTPCheck, &policy.HTTPCheckChecker{Client: &http.Client{Timeout: 10 * time.Second}})
	checkerRegistry.Register(policy.RequireWebhook, &policy.WebhookChecker{})
	ticketSvc.SetRequirementChecker(&requirementCheckerBridge{registry: checkerRegistry})

	_ = postureSvc // posture service available for API handlers
	slog.Info("policy config loaded", "default_posture", cfg.Policy.DefaultPosture, "auto_apply", cfg.Policy.AutoApplyDefault)

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

	// Workflow callback + external executor: async phase support.
	callbackSecret := []byte(cfg.Auth.JWTSecret) // reuse JWT secret for callback HMAC
	if len(callbackSecret) == 0 {
		callbackSecret = []byte(autoGenerateSecret())
	}
	callbackStore := workflow.NewPostgresCallbackStore(pool)
	callbackHandler := workflow.NewCallbackHandler(callbackSecret, callbackStore, workflowEngine)
	externalExecutor := workflow.NewExternalExecutor(callbackHandler, cfg.Auth.BaseURL)

	// Lease validator for execution trace: validate token and return agent ID
	leaseValidator := &leaseValidatorAdapter{leases: queueRedis}
	agentStore := agent.NewStore(pool)
	agentSvc := agent.NewService(agentStore)
	execStore := execution.NewStore(pool)
	execSvc := execution.NewService(execStore, leaseValidator)
	reviewStore := review.NewStore(pool)
	reviewSvc := review.NewService(reviewStore, ticketSvc, bus)
	entityStore := entity.NewStore(pool)
	entitySvc := entity.NewService(entityStore, bus)
	envStore := environment.NewStore(pool)
	envSvc := environment.NewService(envStore, bus)
	planStore := plan.NewStore(pool)
	planSvc := plan.NewService(planStore, bus)
	calibrationStore := policy.NewCalibrationStore(pool)
	calibrationSvc := policy.NewCalibrationService(calibrationStore, bus)
	userStore := user.NewStore(pool)

	// Cost management service (budget tracking, rate-limit handling, model routing).
	costNotifier := cost.NewBusNotifier(bus)
	costStore := cost.NewPostgresStore(pool)
	costCfg := cost.DefaultConfig()
	costSvc := cost.NewService(costStore, costCfg, costNotifier, costNotifier)
	slog.Info("cost: service initialized",
		"flagship", costCfg.FlagshipProvider+"/"+costCfg.FlagshipModel,
		"mid", costCfg.MidProvider+"/"+costCfg.MidModel,
		"fast", costCfg.FastProvider+"/"+costCfg.FastModel)

	// Mirror service: one-way ticket mirroring to Linear/Jira (opt-in per project).
	// Adapters are registered but only activated when a project's mirror_config is set.
	if cfg.Mirror.Enabled {
		mirrorSvc := mirror.NewService(ticketSvc, projectSvc, bus)
		if cfg.Mirror.LinearAPIKey != "" {
			linearClient := linear.NewClient(cfg.Mirror.LinearAPIKey)
			mirrorSvc.RegisterAdapter("linear", linear.NewAdapter(linearClient))
		}
		if cfg.Mirror.JiraAPIToken != "" {
			jiraClient := jira.NewClient(cfg.Mirror.JiraBaseURL, cfg.Mirror.JiraEmail, cfg.Mirror.JiraAPIToken)
			mirrorSvc.RegisterAdapter("jira", jira.NewAdapter(jiraClient))
		}
		_ = mirrorSvc // service runs via event subscriptions
		slog.Info("mirror: service started")
	}

	// Notification service: policy-driven async push to operators (Layer 12).
	// Adapters are pluggable; Slack is the default, email/SMS are stubs.
	var notifySvc *notification.Service
	if cfg.Notification.Enabled {
		notifyStore := notification.NewPostgresStore(pool)
		notifySvc = notification.NewService(notifyStore, bus)
		notifySvc.SetTicketGetter(ticketSvc)
		notifySvc.RegisterAdapter(notification.ChannelSlack, notifyslack.NewAdapter())
		notifySvc.RegisterAdapter(notification.ChannelEmail, notifyemail.NewAdapter())
		notifySvc.RegisterAdapter(notification.ChannelSMS, notifysms.NewAdapter())
		slog.Info("notification: service started")
	}

	// Claims registry for concurrency control (spec v0.2 §4.3).
	claimsStore := claims.NewStore(pool)
	claimsSvc := claims.NewService(claimsStore, bus)
	// Lifecycle handler: auto-register claims on ticket.started, auto-release on completion.
	_ = claims.NewLifecycleHandler(bus, claimsSvc, planSvc, ticketSvc)

	// Pillar and strategy layer (Layer 15).
	pillarStore := pillar.NewPostgresStore(pool)
	pillarSvc := pillar.NewService(pillarStore)

	// Rollback service for stage-specific rollback behavior.
	rollbackSvc := rollback.NewService(ticketSvc, ticketSvc, bus)
	rollbackSvc.SetLeaseRemover(queueSvc)

	// Observation service for production signal tracking and attribution.
	obsStore := observation.NewPostgresStore(pool)
	obsSvc := observation.NewService(obsStore, bus)

	// State index service (spec v0.2 Layer 10): observed infrastructure state.
	stateIndexStore := stateindex.NewPostgresStore(pool)
	stateIndexSvc := stateindex.NewService(stateIndexStore, bus)

	// Foundational streams (entity, state, change) per spec v0.2 section 2.2.
	streamStore := stream.NewPostgresStore(pool)
	streamSvc := stream.NewService(streamStore, bus)
	// Bridge existing ticket events to the change stream.
	streamSvc.SubscribeToTicketEvents(func(ticketID string) string {
		t, err := ticketSvc.GetTicket(ctx, ticketID)
		if err != nil || t == nil {
			return ""
		}
		return t.ProjectID
	})

	// Code intelligence: bundled Tree-sitter/Go-AST default (Layer 3).
	codeIntel := mcp.NewTreeSitterCodeIntel()
	slog.Info("code-intel: bundled default initialized (Tree-sitter + Go AST)")

	// Hooks: change event publication library + gap detection (spec v0.2 §2.4).
	hooksClient := hooks.NewClient(bus)
	gapDetector := hooks.NewGapDetector(bus, hooksClient)
	go gapDetector.Start(ctx)
	slog.Info("hooks: change event library initialized with gap detection")

	// Findings layer (Layer 4): semantic findings store.
	// Uses Weaviate if configured, otherwise falls back to in-memory store.
	var findingsProvider mcp.FindingsProvider
	if cfg.Findings.WeaviateURL != "" {
		findingsProvider = mcp.NewWeaviateFindingsStore(mcp.WeaviateFindingsConfig{
			URL:        cfg.Findings.WeaviateURL,
			APIKey:     cfg.Findings.WeaviateAPIKey,
			Vectorizer: cfg.Findings.WeaviateVectorizer,
		})
		slog.Info("findings: Weaviate backend", "url", cfg.Findings.WeaviateURL)
	} else {
		findingsProvider = mcp.NewMemoryFindingsStore()
		slog.Info("findings: in-memory backend (set WEAVIATE_URL for production)")
	}

	// Catalog service (Layer 14 project map).
	catalogStore := catalog.NewPostgresStore(pool)
	catalogSvc := catalog.NewService(catalogStore)
	catalogScanner := catalog.NewScanner()

	// Delivery service: manages integrations config, pipeline sync, and PR overview.
	deliverySvc := delivery.NewService(projectSvc, envSvc, catalogSvc, stateIndexSvc)
	deliverySvc.SetFlyIOFallback(os.Getenv("FLY_API_TOKEN"), os.Getenv("FLY_API_BASE_URL"))

	// Coordinator learning loop: auto-generate feedback findings on ticket
	// rejection, failure, replan, and invalidation events.
	_ = mcp.NewCoordinatorFeedbackSubscriber(bus, findingsProvider, ticketSvc)
	slog.Info("coordinator-feedback: learning loop subscriber active")

	strictServer := &rest.StrictServer{
		OrgSvc:        orgSvc,
		ProjectSvc:    projectSvc,
		WorkStreamSvc: workStreamSvc,
		TicketSvc:     ticketSvc,
		QueueSvc:      queueSvc,
		TraceSvc:      execSvc,
		ReviewSvc:     reviewSvc,
		EntitySvc:     entitySvc,
		EnvSvc:        envSvc,
		PlanSvc:       planSvc,
		PolicySvc:     calibrationSvc,
		AgentStore:         agentStore,
		CostSvc:            costSvc,
		ProjectTemplateSvc: projectTemplateSvc,
	}

	// Investigation service: uses the same worker infrastructure as dispatch.
	// Configured when dispatch is enabled; nil-safe in the MCP tool handler.
	var investigationSvc *investigation.Service
	repoDir, _ := os.Getwd()
	if cfg.Dispatch.Enabled {
		invWorker := dispatch.NewInvestigationWorker(dispatch.Config{
			ClaudePath:           cfg.Dispatch.ClaudePath,
			AgentRunner:          cfg.Dispatch.AgentRunner,
			AgentDriver:          cfg.Dispatch.AgentDriver,
			AgentCLIPath:         cfg.Dispatch.AgentCLIPath,
			AgentModel:           cfg.Dispatch.AgentModel,
			AgentReasoningEffort: cfg.Dispatch.AgentReasoningEffort,
			AgentAPIBaseURL:      cfg.Dispatch.AgentAPIBaseURL,
			APIKey:               cfg.Dispatch.APIKey,
			AgentAPIKey:          cfg.Dispatch.AgentAPIKey,
			RepoDir:              repoDir,
			CostSvc:              costSvc,
		})
		investigationSvc = investigation.NewService(invWorker, investigation.Config{
			ServerURL:   cfg.Auth.BaseURL,
			WorkDir:     repoDir,
			CostSvc:     costSvc,
			AgentRunner: cfg.Dispatch.AgentRunner,
			AgentDriver: cfg.Dispatch.AgentDriver,
			AgentModel:  cfg.Dispatch.AgentModel,
		})
	}

	var orchestratorWorker dispatch.Worker
	if cfg.Orchestrator.Enabled {
		orchestratorWorker = dispatch.NewWorker(dispatch.Config{
			ClaudePath:           cfg.Dispatch.ClaudePath,
			AgentRunner:          cfg.Orchestrator.AgentRunner,
			AgentDriver:          cfg.Orchestrator.AgentDriver,
			AgentCLIPath:         cfg.Orchestrator.AgentCLIPath,
			AgentModel:           cfg.Orchestrator.AgentModel,
			AgentReasoningEffort: cfg.Orchestrator.AgentReasoningEffort,
			AgentAPIBaseURL:      cfg.Orchestrator.AgentAPIBaseURL,
			APIKey:               cfg.Dispatch.APIKey,
			AgentAPIKey:          cfg.Orchestrator.AgentAPIKey,
			RepoDir:              repoDir,
			CostSvc:              costSvc,
		})
	}
	orchestratorSvc := orchestrator.NewService(ctx, orchestratorStore, projectSvc, orchestratorWorker, orchestrator.Config{
		Enabled:      cfg.Orchestrator.Enabled,
		RepoDir:      repoDir,
		ServerURL:    cfg.Auth.BaseURL,
		AgentID:      "command-center-orchestrator",
		HistoryLimit: cfg.Orchestrator.HistoryLimit,
		CostSvc:      costSvc,
		AgentRunner:  cfg.Orchestrator.AgentRunner,
		AgentDriver:  cfg.Orchestrator.AgentDriver,
		AgentModel:   cfg.Orchestrator.AgentModel,
		WorkerConfig: dispatch.Config{
			ClaudePath:           cfg.Dispatch.ClaudePath,
			AgentRunner:          cfg.Orchestrator.AgentRunner,
			AgentDriver:          cfg.Orchestrator.AgentDriver,
			AgentCLIPath:         cfg.Orchestrator.AgentCLIPath,
			AgentModel:           cfg.Orchestrator.AgentModel,
			AgentReasoningEffort: cfg.Orchestrator.AgentReasoningEffort,
			AgentAPIBaseURL:      cfg.Orchestrator.AgentAPIBaseURL,
			APIKey:               cfg.Dispatch.APIKey,
			AgentAPIKey:          cfg.Orchestrator.AgentAPIKey,
			RepoDir:              repoDir,
			CostSvc:              costSvc,
		},
	})

	// Progress monitor: bridges ticket lifecycle events → command center system messages.
	_ = progress.NewMonitor(bus, orchestratorSvc, ticketSvc)
	slog.Info("progress: monitor started")

	var authMiddleware func(http.Handler) http.Handler
	var authHandler *rest.AuthHandler
	var oauthHandler *rest.OAuthHandler
	var mcpHandler http.Handler
	var mcpSSEHandler http.Handler
	if cfg.Auth.GitHubClientID != "" && cfg.Auth.JWTSecret != "" {
		authMiddleware = rest.AuthMiddleware(cfg.Auth.JWTSecret, agentSvc)
		authCfg := auth.Config{
			ClientID:           cfg.Auth.GitHubClientID,
			ClientSecret:       cfg.Auth.GitHubClientSecret,
			BaseURL:            cfg.Auth.BaseURL,
			RedirectPath:       "/auth/github/callback",
			SuccessRedirectURL: cfg.Auth.SuccessRedirectURL,
		}
		provisioner := &auth.Provisioner{UserStore: userStore, AgentStore: agentStore}
		oauthStore := auth.NewOAuthStore(redisClient)
		authHandler = rest.NewAuthHandler(authCfg, provisioner, oauthStore, orgSvc, cfg.Auth.JWTSecret, 0)
		oauthHandler = &rest.OAuthHandler{
			BaseURL:      cfg.Auth.BaseURL,
			AuthConfig:   authCfg,
			OAuthStore:   oauthStore,
			Provisioner:  provisioner,
			JWTSecret:    cfg.Auth.JWTSecret,
			JWTExpirySec: 604800, // 7 days in seconds for token response
		}
		mcpSrv, err := mcp.NewServer(&mcp.Backend{
			Project:         projectSvc,
			WorkStream:      workStreamSvc,
			Ticket:          ticketSvc,
			Queue:           queueSvc,
			Trace:           execSvc,
			Review:          reviewSvc,
			Org:             orgSvc,
			Entity:          entitySvc,
			AgentStore:      agentStore,
			Investigation:   investigationSvc,
			Claims:          claimsSvc,
			CodeIntel:       codeIntel,
			Findings:        findingsProvider,
			Notification:    notifySvc,
			Catalog:         catalogSvc,
			CatalogScanner:  catalogScanner,
			Pillar:          pillarSvc,
			StateIndex:      stateIndexSvc,
			Rollback:        rollbackSvc,
			Workflow:        workflowEngine,
			ProjectTemplate: projectTemplateSvc,
		})
		if err != nil {
			slog.Error("mcp server init failed", "error", err)
			os.Exit(1)
		}
		streamable := mcp.NewStreamableHTTPHandler(mcpSrv)
		mcpHandler = &rest.MCPHTTPHandler{
			Handler:   streamable,
			BaseURL:   cfg.Auth.BaseURL,
			JWTSecret: cfg.Auth.JWTSecret,
			AgentSvc:  agentSvc,
		}
		sseHandler := mcp.NewSSEHandler(mcpSrv)
		mcpSSEHandler = &rest.MCPHTTPHandler{
			Handler:   sseHandler,
			BaseURL:   cfg.Auth.BaseURL,
			JWTSecret: cfg.Auth.JWTSecret,
			AgentSvc:  agentSvc,
		}
	}

	// Start dispatcher if enabled.
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
			AgentAPIBaseURL:      cfg.Dispatch.AgentAPIBaseURL,
			DockerEnabled:        cfg.Dispatch.DockerEnabled,
			DockerImage:          cfg.Dispatch.DockerImage,
			DockerMemory:         cfg.Dispatch.DockerMemory,
			DockerCPUs:           cfg.Dispatch.DockerCPUs,
			DockerFirewall:       cfg.Dispatch.DockerFirewall,
			AgentAPIKey:          cfg.Dispatch.AgentAPIKey,
			ReconcileInterval:    cfg.Dispatch.ReconcileInterval,
			CostSvc:              costSvc,
			TraceSvc:             execSvc,
		}, bus, ticketSvc, projectSvc)
		dispatcher.SetLeaseReleaser(queueSvc)
		dispatcher.SetFailureSummarizer(ticketSvc)
		dispatcher.SetTicketTransitioner(ticketSvc)
		dispatcher.SetWorkflowEngine(workflowEngine)
		dispatcher.SetExternalExecutor(externalExecutor)
		actionRegistry := workflow.NewActionRegistry()
		dispatcher.SetActionRegistry(actionRegistry)
		dispatcher.SetCheckerRegistry(checkerRegistry)
		dispatcher.SetOutputPatcher(ticketStore)
		dispatcher.SetWorkflowPhaseUpdater(ticketStore)
		// Wire worktree cleanup for rollback when dispatcher manages worktrees.
		rollbackSvc.SetWorktreeRemover(&dispatch.WorktreeManager{
			BaseDir: cfg.Dispatch.WorktreeDir,
			RepoDir: repoDir,
		})
		dispatcher.Start(ctx)
	}

	// Start durable bus delivery (LISTEN/NOTIFY + polling) after all subscriptions are registered.
	if err := bus.Start(ctx); err != nil {
		slog.Error("event bus start failed", "error", err)
		os.Exit(1)
	}

	router := rest.NewRouter(rest.RouterConfig{
		StrictServer:    strictServer,
		AuthMiddleware:  authMiddleware,
		AuthHandler:     authHandler,
		OAuthHandler:    oauthHandler,
		MCPHandler:      mcpHandler,
		MCPSSEHandler:   mcpSSEHandler,
		AgentsHandler:   &rest.AgentsHandler{AgentSvc: agentSvc},
		EntitiesHandler: &rest.EntitiesHandler{EntitySvc: entitySvc},
		DispatchHandler: &rest.DispatchHandler{Dispatcher: dispatcher},
		OrchestratorHandler: &rest.OrchestratorHandler{
			Service:    orchestratorSvc,
			ProjectSvc: projectSvc,
			OrgSvc:     orgSvc,
			AgentStore: agentStore,
		},
		UsageHandler: &rest.UsageHandler{
			CostSvc:    costSvc,
			ProjectSvc: projectSvc,
			OrgSvc:     orgSvc,
			AgentStore: agentStore,
		},
		PlansHandler:       &rest.PlansHandler{PlanSvc: planSvc},
		ObservationHandler: &rest.ObservationHandler{Svc: obsSvc},
		StateIndexHandler:  &rest.StateIndexHandler{Svc: stateIndexSvc},
		StreamsHandler:     &rest.StreamsHandler{Svc: streamSvc},
		CatalogHandler:     &rest.CatalogHandler{Svc: catalogSvc, Scanner: catalogScanner},
		PoliciesHandler: &rest.PoliciesHandler{
			PolicySvc:  calibrationSvc,
			ProjectSvc: projectSvc,
			OrgSvc:     orgSvc,
			AgentStore: agentStore,
		},
		EnvironmentsHandler: &rest.EnvironmentsHandler{
			EnvSvc:     envSvc,
			ProjectSvc: projectSvc,
			OrgSvc:     orgSvc,
			AgentStore: agentStore,
		},
		ClaimsHandler:  &rest.ClaimsHandler{ClaimsSvc: claimsSvc},
		HooksHandler:   &rest.HooksHandler{Client: hooksClient},
		PillarsHandler: &rest.PillarsHandler{PillarSvc: pillarSvc},
		DeliveryHandler: &rest.DeliveryHandler{
			DeliverySvc: deliverySvc,
			ProjectSvc:  projectSvc,
			OrgSvc:      orgSvc,
			AgentStore:  agentStore,
		},
		ProjectTemplatesHandler: &rest.ProjectTemplatesHandler{
			Svc:        projectTemplateSvc,
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
		InvitesHandler: &rest.InvitesHandler{
			OrgSvc:     orgSvc,
			AgentStore: agentStore,
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

	serve(ctx, cfg, router, dispatcher, bus, cancelServices)
}

// serve starts the HTTP server and blocks until SIGINT/SIGTERM.
// cancelServices cancels the context shared by background goroutines (scheduler,
// gap detector, stream bridge, etc.) so they stop before the pool is closed.
func serve(_ context.Context, cfg *config.Config, router http.Handler, dispatcher *dispatch.Dispatcher, bus events.DurableEventBus, cancelServices context.CancelFunc) {
	srv := &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout is intentionally omitted — it kills SSE connections.
	}

	go func() {
		slog.Info("server listening", "port", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Cancel the background services context first so goroutines using the pool
	// stop before we close the pool/redis connections. This prevents "closed pool" errors.
	cancelServices()

	if dispatcher != nil {
		dispatcher.Stop()
	}
	_ = bus.Stop()

	// Brief pause for background goroutines (scheduler, gap detector) to observe
	// the cancelled context and exit before pool.Close() runs.
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
