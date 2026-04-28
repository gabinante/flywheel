/**
 * Health Check Routes
 *
 * Three tiers of health checks for deployment orchestration:
 *
 * GET /health/live  — Liveness probe: is the process alive?
 * GET /health/ready — Readiness probe: can it serve traffic?
 * GET /health       — Detailed status: everything from /ready + system info
 */

import type { FastifyInstance } from 'fastify';
import type { Redis } from 'ioredis';
import type { PrismaClient } from '@prisma/client';

const startTime = Date.now();

export interface HealthCheckDeps {
  prisma: PrismaClient;
  redis: Redis;
  pgBoss?: { getQueueSize: (name: string, options?: Record<string, unknown>) => Promise<number> } | null;
}

export interface CheckResult {
  status: 'ok' | 'error';
  latency_ms?: number;
  error?: string;
}

export interface ReadinessResponse {
  status: 'ok' | 'degraded';
  checks: {
    postgres: CheckResult;
    redis: CheckResult;
    pgboss: CheckResult;
  };
}

export interface DetailedHealthResponse extends ReadinessResponse {
  version: string;
  uptime_seconds: number;
  memory: {
    rss_mb: number;
    heap_used_mb: number;
    heap_total_mb: number;
  };
}

/**
 * Check PostgreSQL connectivity by running SELECT 1.
 */
async function checkPostgres(prisma: PrismaClient): Promise<CheckResult> {
  const start = performance.now();
  try {
    await prisma.$queryRaw`SELECT 1`;
    return {
      status: 'ok',
      latency_ms: Math.round(performance.now() - start),
    };
  } catch (err) {
    return {
      status: 'error',
      latency_ms: Math.round(performance.now() - start),
      error: err instanceof Error ? err.message : 'Unknown error',
    };
  }
}

/**
 * Check Redis connectivity by sending PING.
 */
async function checkRedis(redis: Redis): Promise<CheckResult> {
  const start = performance.now();
  try {
    const result = await redis.ping();
    if (result !== 'PONG') {
      return {
        status: 'error',
        latency_ms: Math.round(performance.now() - start),
        error: `Unexpected PING response: ${result}`,
      };
    }
    return {
      status: 'ok',
      latency_ms: Math.round(performance.now() - start),
    };
  } catch (err) {
    return {
      status: 'error',
      latency_ms: Math.round(performance.now() - start),
      error: err instanceof Error ? err.message : 'Unknown error',
    };
  }
}

/**
 * Check pg-boss connectivity by verifying connection.
 * If pg-boss is not configured, returns ok (optional dependency).
 */
async function checkPgBoss(
  pgBoss?: HealthCheckDeps['pgBoss'] | null
): Promise<CheckResult> {
  const start = performance.now();
  if (!pgBoss) {
    // pg-boss not initialized — treat as ok (may be initializing)
    return {
      status: 'ok',
      latency_ms: Math.round(performance.now() - start),
    };
  }

  try {
    // Attempt to query queue size as a connectivity check
    await pgBoss.getQueueSize('__health_check__');
    return {
      status: 'ok',
      latency_ms: Math.round(performance.now() - start),
    };
  } catch (err) {
    return {
      status: 'error',
      latency_ms: Math.round(performance.now() - start),
      error: err instanceof Error ? err.message : 'Unknown error',
    };
  }
}

/**
 * Run all readiness checks and return combined result.
 */
export async function runReadinessChecks(deps: HealthCheckDeps): Promise<ReadinessResponse> {
  const [postgres, redis, pgboss] = await Promise.all([
    checkPostgres(deps.prisma),
    checkRedis(deps.redis),
    checkPgBoss(deps.pgBoss),
  ]);

  const allOk = postgres.status === 'ok' && redis.status === 'ok' && pgboss.status === 'ok';

  return {
    status: allOk ? 'ok' : 'degraded',
    checks: { postgres, redis, pgboss },
  };
}

/**
 * Register health check routes on the given Fastify instance.
 */
export function registerHealthRoutes(
  app: FastifyInstance,
  deps: HealthCheckDeps
): void {
  /**
   * GET /health/live — Liveness probe
   *
   * Returns 200 always. If the process can respond, it's alive.
   */
  app.get('/health/live', async (_request, reply) => {
    return reply.code(200).send({ status: 'ok' });
  });

  /**
   * GET /health/ready — Readiness probe
   *
   * Checks PostgreSQL, Redis, and pg-boss.
   * Returns 200 if all pass, 503 if any fail.
   */
  app.get('/health/ready', async (_request, reply) => {
    const result = await runReadinessChecks(deps);
    const statusCode = result.status === 'ok' ? 200 : 503;
    return reply.code(statusCode).send(result);
  });

  /**
   * GET /health — Detailed health status
   *
   * Everything from /ready plus version, uptime, and memory usage.
   */
  app.get('/health', async (_request, reply) => {
    const readiness = await runReadinessChecks(deps);
    const mem = process.memoryUsage();

    const detailed: DetailedHealthResponse = {
      ...readiness,
      version: process.env.SENTRY_RELEASE ?? process.env.npm_package_version ?? '0.1.0',
      uptime_seconds: Math.round((Date.now() - startTime) / 1000),
      memory: {
        rss_mb: Math.round(mem.rss / 1024 / 1024 * 100) / 100,
        heap_used_mb: Math.round(mem.heapUsed / 1024 / 1024 * 100) / 100,
        heap_total_mb: Math.round(mem.heapTotal / 1024 / 1024 * 100) / 100,
      },
    };

    const statusCode = readiness.status === 'ok' ? 200 : 503;
    return reply.code(statusCode).send(detailed);
  });
}
