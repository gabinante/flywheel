package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	"github.com/gabinante/flywheel/internal/bootstrap"
	"github.com/gabinante/flywheel/internal/catalog"
	"github.com/gabinante/flywheel/internal/claims"
	"github.com/gabinante/flywheel/internal/cost"
	"github.com/gabinante/flywheel/internal/dispatch"
	"github.com/gabinante/flywheel/internal/embedded"
	"github.com/gabinante/flywheel/internal/entity"
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
	"github.com/gabinante/flywheel/internal/org"
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
	"github.com/gabinante/flywheel/internal/workstream"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

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
	cfg := config.Load()
	ctx := context.Background()

	if cfg.Embedded.Enabled {
		runEmbedded(ctx, cfg)
		return
	}
	runPostgres(ctx, cfg)
}

// runPostgres is the original Postgres+Redis startup path.
func runPostgres(ctx context.Context, cfg *config.Config) {
	pool, err := db.NewPool(ctx, cfg.DB.URL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	// Create event bus: use Postgres durable bus by default for at-least-once delivery.
	// Falls back to in-process bus if DURABLE_BUS=false is set.
	var bus events.DurableEventBus
	if os.Getenv("DURABLE_BUS") == "false" {
		bus = events.NewInProcessBus()
		log.Println("events: using in-process bus (no durability)")
	} else {
		pgBus := events.NewPostgresBus(pool, events.PostgresBusConfig{})
		bus = pgBus
		log.Println("events: using Postgres durable bus (at-least-once delivery)")
	}

	orgStore := org.NewStore(pool)
	orgSvc := org.NewService(orgStore)
	projectStore := project.NewStore(pool)
	projectSvc := project.NewService(projectStore)
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

	// Policy layer: composable rules with most-restrictive-wins semantics.
	postureStore := policy.NewPostgresStore(pool)
	postureSvc := policy.NewPostureService(postureStore, bus)
	policyAdapter := policy.NewTicketPolicyAdapter(postureSvc)
	_ = policyAdapter // adapter available for ticket service integration
	_ = postureSvc    // posture service available for API handlers
	log.Printf("policy: default_posture=%s auto_apply=%v", cfg.Policy.DefaultPosture, cfg.Policy.AutoApplyDefault)

	redisOpts, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis ping: %v", err)
	}
	leaseTTL := time.Duration(cfg.Queue.LeaseTTLMinutes) * time.Minute
	queueRedis := queue.NewRedisStore(redisClient, leaseTTL)
	queueSvc := queue.NewService(ticketSvc, ticketSvc, queueRedis)
	scheduler := queue.NewScheduler(queueRedis, ticketSvc, ticketSvc, bus, 30*time.Second)
	scheduler.EnableStalenessSweep(ticketSvc, 0) // default: 2x lease TTL
	go scheduler.Run(ctx)

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
	planStore := plan.NewStore(pool)
	planSvc := plan.NewService(planStore, bus)
	calibrationStore := policy.NewCalibrationStore(pool)
	calibrationSvc := policy.NewCalibrationService(calibrationStore, bus)
	userStore := user.NewStore(pool)

	// Cost management service (budget tracking, rate-limit handling, model routing).
	costNotifier := cost.NewBusNotifier(bus)
	costStore := cost.NewMemStore() // Uses in-memory store; Postgres store wired when migration runs.
	costCfg := cost.DefaultConfig()
	costSvc := cost.NewService(costStore, costCfg, costNotifier, costNotifier)
	log.Printf("cost: service initialized (fallback router: %s/%s → %s/%s → %s/%s)",
		costCfg.FlagshipProvider, costCfg.FlagshipModel,
		costCfg.MidProvider, costCfg.MidModel,
		costCfg.FastProvider, costCfg.FastModel)

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
		log.Println("mirror: service started")
	}

	// Notification service: policy-driven async push to operators (Layer 12).
	// Adapters are pluggable; Slack is the default, email/SMS are stubs.
	var notifySvc *notification.Service
	if cfg.Notification.Enabled {
		notifyStore := notification.NewPostgresStore(pool)
		notifySvc = notification.NewService(notifyStore, bus)
		notifySvc.RegisterAdapter(notification.ChannelSlack, notifyslack.NewAdapter())
		notifySvc.RegisterAdapter(notification.ChannelEmail, notifyemail.NewAdapter())
		notifySvc.RegisterAdapter(notification.ChannelSMS, notifysms.NewAdapter())
		log.Println("notification: service started")
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
	log.Println("code-intel: bundled default initialized (Tree-sitter + Go AST)")

	// Hooks: change event publication library + gap detection (spec v0.2 §2.4).
	hooksClient := hooks.NewClient(bus)
	gapDetector := hooks.NewGapDetector(bus, hooksClient)
	go gapDetector.Start(ctx)
	log.Println("hooks: change event library initialized with gap detection")

	// Findings layer (Layer 4): semantic findings store.
	// Uses Weaviate if configured, otherwise falls back to in-memory store.
	var findingsProvider mcp.FindingsProvider
	if cfg.Findings.WeaviateURL != "" {
		findingsProvider = mcp.NewWeaviateFindingsStore(mcp.WeaviateFindingsConfig{
			URL:        cfg.Findings.WeaviateURL,
			APIKey:     cfg.Findings.WeaviateAPIKey,
			Vectorizer: cfg.Findings.WeaviateVectorizer,
		})
		log.Printf("findings: Weaviate backend at %s", cfg.Findings.WeaviateURL)
	} else {
		findingsProvider = mcp.NewMemoryFindingsStore()
		log.Println("findings: in-memory backend (set WEAVIATE_URL for production)")
	}

	// Catalog service (Layer 14 project map).
	catalogStore := catalog.NewPostgresStore(pool)
	catalogSvc := catalog.NewService(catalogStore)
	catalogScanner := catalog.NewScanner()

	// Coordinator learning loop: auto-generate feedback findings on ticket
	// rejection, failure, replan, and invalidation events.
	_ = mcp.NewCoordinatorFeedbackSubscriber(bus, findingsProvider, ticketSvc)
	log.Println("coordinator-feedback: learning loop subscriber active")

	strictServer := &rest.StrictServer{
		OrgSvc:        orgSvc,
		ProjectSvc:    projectSvc,
		WorkStreamSvc: workStreamSvc,
		TicketSvc:     ticketSvc,
		QueueSvc:      queueSvc,
		TraceSvc:      execSvc,
		ReviewSvc:     reviewSvc,
		EntitySvc:     entitySvc,
		AgentStore:    agentStore,
		CostSvc:       costSvc,
	}

	// Investigation service: uses the same worker infrastructure as dispatch.
	// Configured when dispatch is enabled; nil-safe in the MCP tool handler.
	var investigationSvc *investigation.Service
	if cfg.Dispatch.Enabled {
		repoDir, _ := os.Getwd()
		invWorker := dispatch.NewInvestigationWorker(dispatch.Config{
			ClaudePath:   cfg.Dispatch.ClaudePath,
			AgentDriver:  cfg.Dispatch.AgentDriver,
			AgentCLIPath: cfg.Dispatch.AgentCLIPath,
			APIKey:       cfg.Dispatch.APIKey,
			RepoDir:      repoDir,
		})
		investigationSvc = investigation.NewService(invWorker, investigation.Config{
			ServerURL: cfg.Auth.BaseURL,
			WorkDir:   repoDir,
		})
	}

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
			Project:        projectSvc,
			WorkStream:     workStreamSvc,
			Ticket:         ticketSvc,
			Queue:          queueSvc,
			Trace:          execSvc,
			Review:         reviewSvc,
			Org:            orgSvc,
			Entity:         entitySvc,
			AgentStore:     agentStore,
			Investigation:  investigationSvc,
			Claims:         claimsSvc,
			CodeIntel:      codeIntel,
			Findings:       findingsProvider,
			Notification:   notifySvc,
			Catalog:        catalogSvc,
			CatalogScanner: catalogScanner,
			Pillar:         pillarSvc,
			StateIndex:     stateIndexSvc,
			Rollback:       rollbackSvc,
		})
		if err != nil {
			log.Fatalf("mcp server: %v", err)
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
		repoDir, _ := os.Getwd()
		dispatcher = dispatch.New(dispatch.Config{
			MaxWorkers:     cfg.Dispatch.MaxWorkers,
			ClaudePath:     cfg.Dispatch.ClaudePath,
			WorktreeDir:    cfg.Dispatch.WorktreeDir,
			RepoDir:        repoDir,
			ServerURL:      cfg.Auth.BaseURL,
			AgentID:        "dispatch-worker",
			APIKey:         cfg.Dispatch.APIKey,
			ProjectID:      cfg.Dispatch.ProjectID,
			AutoApprove:    cfg.Dispatch.AutoApproveOnAcceptancePass,
			AgentDriver:    cfg.Dispatch.AgentDriver,
			AgentCLIPath:   cfg.Dispatch.AgentCLIPath,
			DockerEnabled:  cfg.Dispatch.DockerEnabled,
			DockerImage:    cfg.Dispatch.DockerImage,
			DockerMemory:   cfg.Dispatch.DockerMemory,
			DockerCPUs:     cfg.Dispatch.DockerCPUs,
			DockerFirewall:    cfg.Dispatch.DockerFirewall,
			AnthropicKey:      cfg.Dispatch.AnthropicKey,
			ReconcileInterval: cfg.Dispatch.ReconcileInterval,
		}, bus, ticketSvc, projectSvc)
		dispatcher.SetLeaseReleaser(queueSvc)
		dispatcher.SetTicketTransitioner(ticketSvc)
		// Wire worktree cleanup for rollback when dispatcher manages worktrees.
		rollbackSvc.SetWorktreeRemover(&dispatch.WorktreeManager{
			BaseDir: cfg.Dispatch.WorktreeDir,
			RepoDir: repoDir,
		})
		dispatcher.Start(ctx)
	}

	// Start durable bus delivery (LISTEN/NOTIFY + polling) after all subscriptions are registered.
	if err := bus.Start(ctx); err != nil {
		log.Fatalf("event bus start: %v", err)
	}

	router := rest.NewRouter(rest.RouterConfig{
		StrictServer:       strictServer,
		AuthMiddleware:     authMiddleware,
		AuthHandler:        authHandler,
		OAuthHandler:       oauthHandler,
		MCPHandler:         mcpHandler,
		MCPSSEHandler:      mcpSSEHandler,
		AgentsHandler:      &rest.AgentsHandler{AgentSvc: agentSvc},
		EntitiesHandler:    &rest.EntitiesHandler{EntitySvc: entitySvc},
		DispatchHandler:    &rest.DispatchHandler{Dispatcher: dispatcher},
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
		ClaimsHandler:  &rest.ClaimsHandler{ClaimsSvc: claimsSvc},
		HooksHandler:   &rest.HooksHandler{Client: hooksClient},
		PillarsHandler: &rest.PillarsHandler{PillarSvc: pillarSvc},
		WebDist:        cfg.Server.WebDist,
	})

	serve(ctx, cfg, router, dispatcher, bus)
}

// runEmbedded starts the server in embedded mode: SQLite for storage, miniredis
// for leases, no external dependencies required. If this is the first run, it
// launches the interactive bootstrap wizard.
func runEmbedded(ctx context.Context, cfg *config.Config) {
	log.Println("starting in embedded mode (SQLite + in-memory Redis)")

	// Also load config from data dir if it exists.
	dataDir := cfg.Embedded.DataDir
	if dataDir == "" {
		dataDir = embedded.DefaultDataDir()
	}

	// Open SQLite database.
	sqliteDB, err := embedded.OpenDB("")
	if err != nil {
		log.Fatalf("embedded db: %v", err)
	}
	defer sqliteDB.Close()

	// Start miniredis for lease storage (in-memory, no persistence needed).
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	bus := events.NewInProcessBus()

	// Create embedded stores.
	orgSt := embedded.NewOrgStore(sqliteDB)
	orgSvc := org.NewService(orgSt)
	projectSt := embedded.NewProjectStore(sqliteDB)
	projectSvc := project.NewService(projectSt)
	workStreamSt := embedded.NewWorkStreamStore(sqliteDB)
	workStreamSvc := workstream.NewService(workStreamSt)
	ticketSt := embedded.NewTicketStore(sqliteDB)
	ticketSvc := ticket.NewService(ticketSt, bus, projectSvc)

	leaseTTL := time.Duration(cfg.Queue.LeaseTTLMinutes) * time.Minute
	queueRedis := queue.NewRedisStore(redisClient, leaseTTL)
	queueSvc := queue.NewService(ticketSvc, ticketSvc, queueRedis)
	scheduler := queue.NewScheduler(queueRedis, ticketSvc, ticketSvc, bus, 30*time.Second)
	scheduler.EnableStalenessSweep(ticketSvc, 0) // default: 2x lease TTL
	go scheduler.Run(ctx)

	leaseValidator := &leaseValidatorAdapter{leases: queueRedis}
	agentSt := embedded.NewAgentStore(sqliteDB)
	agentSvc := agent.NewService(agentSt)
	execSt := embedded.NewExecutionStepStore(sqliteDB)
	execSvc := execution.NewService(execSt, leaseValidator)
	reviewSt := embedded.NewReviewStore(sqliteDB)
	reviewSvc := review.NewService(reviewSt, ticketSvc, bus)
	pillarSt := embedded.NewPillarStore(sqliteDB)
	pillarSvc := pillar.NewService(pillarSt)

	// Hooks: change event publication library + gap detection (spec v0.2 §2.4).
	hooksClient := hooks.NewClient(bus)
	gapDetector := hooks.NewGapDetector(bus, hooksClient)
	go gapDetector.Start(ctx)

	// Catalog service (Layer 14 project map).
	catalogSt := catalog.NewSQLiteStore(sqliteDB)
	catalogSvc := catalog.NewService(catalogSt)
	catalogScanner := catalog.NewScanner()

	// Rollback service for embedded mode.
	rollbackSvcEmbed := rollback.NewService(ticketSvc, ticketSvc, bus)
	rollbackSvcEmbed.SetLeaseRemover(queueSvc)

	// Run first-run wizard if no data exists yet.
	firstRun := bootstrap.IsFirstRun(dataDir)
	if firstRun {
		if bootstrap.IsTTY() {
			log.Println("first run detected — launching interactive setup wizard")
			_, wizardErr := bootstrap.RunWizard(ctx, orgSvc, projectSvc, agentSvc)
			if wizardErr != nil {
				log.Fatalf("wizard: %v", wizardErr)
			}
		} else {
			log.Println("first run detected — running headless bootstrap (no TTY)")
			headlessCfg := bootstrap.HeadlessConfigFromEnv()
			_, wizardErr := bootstrap.RunHeadless(ctx, headlessCfg, orgSvc, projectSvc, agentSvc)
			if wizardErr != nil {
				log.Fatalf("headless bootstrap: %v", wizardErr)
			}
		}
	}

	// Ensure JWT secret exists (auto-generate if not set).
	jwtSecret := cfg.Auth.JWTSecret
	if jwtSecret == "" {
		jwtSecret = autoGenerateSecret()
		log.Printf("auto-generated JWT secret for embedded mode")
	}

	strictServer := &rest.StrictServer{
		OrgSvc:        orgSvc,
		ProjectSvc:    projectSvc,
		WorkStreamSvc: workStreamSvc,
		TicketSvc:     ticketSvc,
		QueueSvc:      queueSvc,
		TraceSvc:      execSvc,
		ReviewSvc:     reviewSvc,
		AgentStore:    agentSt,
	}

	// Code intelligence: bundled default for embedded mode.
	embeddedCodeIntel := mcp.NewTreeSitterCodeIntel()

	// Findings layer for embedded mode: always in-memory (no Weaviate dependency).
	var embeddedFindingsProvider mcp.FindingsProvider
	if cfg.Findings.WeaviateURL != "" {
		embeddedFindingsProvider = mcp.NewWeaviateFindingsStore(mcp.WeaviateFindingsConfig{
			URL:        cfg.Findings.WeaviateURL,
			APIKey:     cfg.Findings.WeaviateAPIKey,
			Vectorizer: cfg.Findings.WeaviateVectorizer,
		})
	} else {
		embeddedFindingsProvider = mcp.NewMemoryFindingsStore()
	}

	// Coordinator learning loop for embedded mode.
	_ = mcp.NewCoordinatorFeedbackSubscriber(bus, embeddedFindingsProvider, ticketSvc)
	log.Println("coordinator-feedback: learning loop subscriber active (embedded)")

	// In embedded mode, set up MCP with API key auth (no OAuth required).
	authMiddleware := rest.AuthMiddleware(jwtSecret, agentSvc)
	mcpSrv, err := mcp.NewServer(&mcp.Backend{
		Project:        projectSvc,
		WorkStream:     workStreamSvc,
		Ticket:         ticketSvc,
		Queue:          queueSvc,
		Trace:          execSvc,
		Review:         reviewSvc,
		Org:            orgSvc,
		AgentStore:     agentSt,
		CodeIntel:      embeddedCodeIntel,
		Findings:       embeddedFindingsProvider,
		Catalog:        catalogSvc,
		CatalogScanner: catalogScanner,
		Pillar:         pillarSvc,
		Rollback:       rollbackSvcEmbed,
	})
	if err != nil {
		log.Fatalf("mcp server: %v", err)
	}
	streamable := mcp.NewStreamableHTTPHandler(mcpSrv)
	mcpHandler := &rest.MCPHTTPHandler{
		Handler:   streamable,
		BaseURL:   cfg.Auth.BaseURL,
		JWTSecret: jwtSecret,
		AgentSvc:  agentSvc,
	}
	sseHandler := mcp.NewSSEHandler(mcpSrv)
	mcpSSEHandler := &rest.MCPHTTPHandler{
		Handler:   sseHandler,
		BaseURL:   cfg.Auth.BaseURL,
		JWTSecret: jwtSecret,
		AgentSvc:  agentSvc,
	}

	// Foundational streams (in-memory store for embedded mode).
	streamMemStore := stream.NewMemoryStore()
	streamSvc := stream.NewService(streamMemStore, bus)
	streamSvc.SubscribeToTicketEvents(func(ticketID string) string {
		t, err := ticketSvc.GetTicket(ctx, ticketID)
		if err != nil || t == nil {
			return ""
		}
		return t.ProjectID
	})

	router := rest.NewRouter(rest.RouterConfig{
		StrictServer:   strictServer,
		AuthMiddleware: authMiddleware,
		MCPHandler:     mcpHandler,
		MCPSSEHandler:  mcpSSEHandler,
		AgentsHandler:  &rest.AgentsHandler{AgentSvc: agentSvc},
		StreamsHandler: &rest.StreamsHandler{Svc: streamSvc},
		HooksHandler:   &rest.HooksHandler{Client: hooksClient},
		CatalogHandler: &rest.CatalogHandler{Svc: catalogSvc, Scanner: catalogScanner},
		PillarsHandler: &rest.PillarsHandler{PillarSvc: pillarSvc},
		WebDist:        cfg.Server.WebDist,
	})

	serve(ctx, cfg, router, nil, bus)
}

// serve starts the HTTP server and blocks until SIGINT/SIGTERM.
func serve(ctx context.Context, cfg *config.Config, router http.Handler, dispatcher *dispatch.Dispatcher, bus events.DurableEventBus) {
	srv := &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout is intentionally omitted — it kills SSE connections.
	}

	go func() {
		log.Printf("server listening on :%s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	if dispatcher != nil {
		dispatcher.Stop()
	}
	_ = bus.Stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Println("server stopped")
}

func autoGenerateSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
