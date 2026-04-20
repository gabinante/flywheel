package main

import (
	"context"
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
	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/auth"
	"github.com/gabinante/flywheel/internal/dispatch"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/policy"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/queue"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/user"
	"github.com/gabinante/flywheel/internal/workstream"
	"github.com/redis/go-redis/v9"
)

// leaseValidatorAdapter adapts queue.RedisStore to execution.LeaseValidator.
type leaseValidatorAdapter struct {
	redis *queue.RedisStore
}

func (a *leaseValidatorAdapter) ValidateLease(ctx context.Context, ticketID, token string) (string, error) {
	data, err := a.redis.ValidateToken(ctx, ticketID, token)
	if err != nil || data == nil {
		return "", err
	}
	return data.AgentID, nil
}

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.DB.URL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	bus := events.NewInProcessBus()

	orgStore := org.NewStore(pool)
	orgSvc := org.NewService(orgStore)
	projectStore := project.NewStore(pool)
	projectSvc := project.NewService(projectStore)
	workStreamStore := workstream.NewStore(pool)
	workStreamSvc := workstream.NewService(workStreamStore)
	ticketStore := ticket.NewStore(pool)
	ticketSvc := ticket.NewService(ticketStore, bus, projectSvc)
	if cfg.RunAcceptanceTestOnSubmit {
		ticketSvc.SetAcceptanceRunner(&ticket.ShellAcceptanceRunner{})
	}
	if cfg.Dispatch.AutoApproveOnAcceptancePass {
		ticketSvc.SetAutoApproveOnPass(true)
	}

	// Policy layer: composable rules with most-restrictive-wins semantics.
	policyStore := policy.NewPostgresStore(pool)
	policySvc := policy.NewService(policyStore, bus)
	policyAdapter := policy.NewTicketPolicyAdapter(policySvc)
	_ = policyAdapter // adapter available for ticket service integration
	_ = policySvc     // policy service available for API handlers
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
	go scheduler.Run(ctx)

	// Lease validator for execution trace: validate token and return agent ID
	leaseValidator := &leaseValidatorAdapter{redis: queueRedis}
	agentStore := agent.NewStore(pool)
	agentSvc := agent.NewService(agentStore)
	execStore := execution.NewStore(pool)
	execSvc := execution.NewService(execStore, leaseValidator)
	reviewStore := review.NewStore(pool)
	reviewSvc := review.NewService(reviewStore, ticketSvc, bus)
	userStore := user.NewStore(pool)

	strictServer := &rest.StrictServer{
		OrgSvc:        orgSvc,
		ProjectSvc:    projectSvc,
		WorkStreamSvc: workStreamSvc,
		TicketSvc:     ticketSvc,
		QueueSvc:      queueSvc,
		TraceSvc:      execSvc,
		ReviewSvc:     reviewSvc,
		AgentStore:    agentStore,
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
			Project:    projectSvc,
			WorkStream: workStreamSvc,
			Ticket:     ticketSvc,
			Queue:      queueSvc,
			Trace:      execSvc,
			Review:     reviewSvc,
			Org:        orgSvc,
			AgentStore: agentStore,
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

	router := rest.NewRouter(rest.RouterConfig{
		StrictServer:   strictServer,
		AuthMiddleware: authMiddleware,
		AuthHandler:    authHandler,
		OAuthHandler:   oauthHandler,
		MCPHandler:     mcpHandler,
		MCPSSEHandler:  mcpSSEHandler,
		AgentsHandler:  &rest.AgentsHandler{AgentSvc: agentSvc},
		WebDist:        cfg.Server.WebDist,
	})

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
			DockerEnabled:  cfg.Dispatch.DockerEnabled,
			DockerImage:    cfg.Dispatch.DockerImage,
			DockerMemory:   cfg.Dispatch.DockerMemory,
			DockerCPUs:     cfg.Dispatch.DockerCPUs,
			DockerFirewall: cfg.Dispatch.DockerFirewall,
			AnthropicKey:   cfg.Dispatch.AnthropicKey,
		}, bus, ticketSvc, projectSvc)
		dispatcher.Start(ctx)
	}

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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Println("server stopped")
}
