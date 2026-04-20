// Package main runs the Flywheel MCP server over stdio. Use this from IDEs (Cursor, Claude Code, etc.)
// or agents that speak MCP. Requires DATABASE_URL and REDIS_URL (same as the REST server).
package main

import (
	"context"
	"log"
	"time"

	"github.com/gabinante/flywheel/internal/queue"

	"github.com/gabinante/flywheel/api/mcp"
	"github.com/gabinante/flywheel/config"
	"github.com/gabinante/flywheel/db"
	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.DB.URL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	redisOpts, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis ping: %v", err)
	}

	bus := events.NewInProcessBus()
	leaseTTL := time.Duration(cfg.Queue.LeaseTTLMinutes) * time.Minute
	queueRedis := queue.NewRedisStore(redisClient, leaseTTL)

	orgStore := org.NewStore(pool)
	orgSvc := org.NewService(orgStore)
	projectStore := project.NewStore(pool)
	projectSvc := project.NewService(projectStore)
	ticketStore := ticket.NewStore(pool)
	ticketSvc := ticket.NewService(ticketStore, bus, projectSvc)
	queueSvc := queue.NewService(ticketSvc, ticketSvc, queueRedis)

	leaseValidator := &leaseValidatorAdapter{redis: queueRedis}
	agentStore := agent.NewStore(pool)
	_ = agent.NewService(agentStore)
	execStore := execution.NewStore(pool)
	execSvc := execution.NewService(execStore, leaseValidator)
	reviewStore := review.NewStore(pool)
	reviewSvc := review.NewService(reviewStore, ticketSvc, bus)

	backend := &mcp.Backend{
		Project:    projectSvc,
		Ticket:     ticketSvc,
		Queue:      queueSvc,
		Trace:      execSvc,
		Review:     reviewSvc,
		Org:        orgSvc,
		AgentStore: agentStore,
	}

	mcp.RunStdio(backend)
}

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
