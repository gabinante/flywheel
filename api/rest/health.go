package rest

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// PostgresHealthChecker pings the Postgres connection pool.
type PostgresHealthChecker struct {
	Pool *pgxpool.Pool
}

func (p *PostgresHealthChecker) Name() string { return "postgres" }

func (p *PostgresHealthChecker) Check(ctx context.Context) error {
	return p.Pool.Ping(ctx)
}

// RedisHealthChecker pings the Redis client.
type RedisHealthChecker struct {
	Client *redis.Client
}

func (r *RedisHealthChecker) Name() string { return "redis" }

func (r *RedisHealthChecker) Check(ctx context.Context) error {
	return r.Client.Ping(ctx).Err()
}
