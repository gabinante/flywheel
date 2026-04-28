/**
 * Health Check Endpoint Tests
 *
 * Tests the three health check tiers:
 * - /health/live — always returns 200
 * - /health/ready — checks Postgres, Redis, pg-boss
 * - /health — detailed status with version, uptime, memory
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import Fastify from 'fastify';
import { registerHealthRoutes, runReadinessChecks, type HealthCheckDeps } from '../routes/health.js';

// ─── Mock factories ───────────────────────────────────────────────

function createMockPrisma(options?: { failQuery?: boolean }) {
  return {
    $queryRaw: options?.failQuery
      ? vi.fn().mockRejectedValue(new Error('Connection refused'))
      : vi.fn().mockResolvedValue([{ '?column?': 1 }]),
    $disconnect: vi.fn().mockResolvedValue(undefined),
  };
}

function createMockRedis(options?: { failPing?: boolean; badResponse?: boolean }) {
  if (options?.failPing) {
    return {
      ping: vi.fn().mockRejectedValue(new Error('ECONNREFUSED')),
      disconnect: vi.fn(),
    };
  }
  if (options?.badResponse) {
    return {
      ping: vi.fn().mockResolvedValue('NOT_PONG'),
      disconnect: vi.fn(),
    };
  }
  return {
    ping: vi.fn().mockResolvedValue('PONG'),
    disconnect: vi.fn(),
  };
}

function createMockPgBoss(options?: { failQuery?: boolean }) {
  return {
    getQueueSize: options?.failQuery
      ? vi.fn().mockRejectedValue(new Error('pg-boss connection failed'))
      : vi.fn().mockResolvedValue(0),
  };
}

async function buildTestApp(deps: HealthCheckDeps) {
  const app = Fastify({ logger: false });
  registerHealthRoutes(app, deps);
  await app.ready();
  return app;
}

// ─── Tests ────────────────────────────────────────────────────────

describe('Health Check Endpoints', () => {
  // ─── GET /health/live ─────────────────────────────────────────
  describe('GET /health/live', () => {
    it('returns 200 always', async () => {
      const app = await buildTestApp({
        prisma: createMockPrisma() as any,
        redis: createMockRedis() as any,
        pgBoss: createMockPgBoss(),
      });

      const response = await app.inject({ method: 'GET', url: '/health/live' });
      expect(response.statusCode).toBe(200);
      expect(JSON.parse(response.body)).toEqual({ status: 'ok' });

      await app.close();
    });

    it('returns 200 even when dependencies are down', async () => {
      const app = await buildTestApp({
        prisma: createMockPrisma({ failQuery: true }) as any,
        redis: createMockRedis({ failPing: true }) as any,
        pgBoss: createMockPgBoss({ failQuery: true }),
      });

      const response = await app.inject({ method: 'GET', url: '/health/live' });
      expect(response.statusCode).toBe(200);
      expect(JSON.parse(response.body)).toEqual({ status: 'ok' });

      await app.close();
    });
  });

  // ─── GET /health/ready ────────────────────────────────────────
  describe('GET /health/ready', () => {
    it('returns 200 when all checks pass', async () => {
      const app = await buildTestApp({
        prisma: createMockPrisma() as any,
        redis: createMockRedis() as any,
        pgBoss: createMockPgBoss(),
      });

      const response = await app.inject({ method: 'GET', url: '/health/ready' });
      expect(response.statusCode).toBe(200);

      const body = JSON.parse(response.body);
      expect(body.status).toBe('ok');
      expect(body.checks.postgres.status).toBe('ok');
      expect(body.checks.redis.status).toBe('ok');
      expect(body.checks.pgboss.status).toBe('ok');
      expect(typeof body.checks.postgres.latency_ms).toBe('number');
      expect(typeof body.checks.redis.latency_ms).toBe('number');
      expect(typeof body.checks.pgboss.latency_ms).toBe('number');

      await app.close();
    });

    it('returns 503 when Postgres is down', async () => {
      const app = await buildTestApp({
        prisma: createMockPrisma({ failQuery: true }) as any,
        redis: createMockRedis() as any,
        pgBoss: createMockPgBoss(),
      });

      const response = await app.inject({ method: 'GET', url: '/health/ready' });
      expect(response.statusCode).toBe(503);

      const body = JSON.parse(response.body);
      expect(body.status).toBe('degraded');
      expect(body.checks.postgres.status).toBe('error');
      expect(body.checks.postgres.error).toContain('Connection refused');
      expect(body.checks.redis.status).toBe('ok');
      expect(body.checks.pgboss.status).toBe('ok');

      await app.close();
    });

    it('returns 503 when Redis is down', async () => {
      const app = await buildTestApp({
        prisma: createMockPrisma() as any,
        redis: createMockRedis({ failPing: true }) as any,
        pgBoss: createMockPgBoss(),
      });

      const response = await app.inject({ method: 'GET', url: '/health/ready' });
      expect(response.statusCode).toBe(503);

      const body = JSON.parse(response.body);
      expect(body.status).toBe('degraded');
      expect(body.checks.postgres.status).toBe('ok');
      expect(body.checks.redis.status).toBe('error');
      expect(body.checks.redis.error).toContain('ECONNREFUSED');

      await app.close();
    });

    it('returns 503 when Redis returns unexpected response', async () => {
      const app = await buildTestApp({
        prisma: createMockPrisma() as any,
        redis: createMockRedis({ badResponse: true }) as any,
        pgBoss: createMockPgBoss(),
      });

      const response = await app.inject({ method: 'GET', url: '/health/ready' });
      expect(response.statusCode).toBe(503);

      const body = JSON.parse(response.body);
      expect(body.status).toBe('degraded');
      expect(body.checks.redis.status).toBe('error');
      expect(body.checks.redis.error).toContain('Unexpected PING response');

      await app.close();
    });

    it('returns 503 when pg-boss is down', async () => {
      const app = await buildTestApp({
        prisma: createMockPrisma() as any,
        redis: createMockRedis() as any,
        pgBoss: createMockPgBoss({ failQuery: true }),
      });

      const response = await app.inject({ method: 'GET', url: '/health/ready' });
      expect(response.statusCode).toBe(503);

      const body = JSON.parse(response.body);
      expect(body.status).toBe('degraded');
      expect(body.checks.pgboss.status).toBe('error');
      expect(body.checks.pgboss.error).toContain('pg-boss connection failed');

      await app.close();
    });

    it('returns 503 when all dependencies are down', async () => {
      const app = await buildTestApp({
        prisma: createMockPrisma({ failQuery: true }) as any,
        redis: createMockRedis({ failPing: true }) as any,
        pgBoss: createMockPgBoss({ failQuery: true }),
      });

      const response = await app.inject({ method: 'GET', url: '/health/ready' });
      expect(response.statusCode).toBe(503);

      const body = JSON.parse(response.body);
      expect(body.status).toBe('degraded');
      expect(body.checks.postgres.status).toBe('error');
      expect(body.checks.redis.status).toBe('error');
      expect(body.checks.pgboss.status).toBe('error');

      await app.close();
    });

    it('returns ok when pgBoss is null (not configured)', async () => {
      const app = await buildTestApp({
        prisma: createMockPrisma() as any,
        redis: createMockRedis() as any,
        pgBoss: null,
      });

      const response = await app.inject({ method: 'GET', url: '/health/ready' });
      expect(response.statusCode).toBe(200);

      const body = JSON.parse(response.body);
      expect(body.status).toBe('ok');
      expect(body.checks.pgboss.status).toBe('ok');

      await app.close();
    });
  });

  // ─── GET /health ──────────────────────────────────────────────
  describe('GET /health', () => {
    it('returns 200 with detailed status when all checks pass', async () => {
      const app = await buildTestApp({
        prisma: createMockPrisma() as any,
        redis: createMockRedis() as any,
        pgBoss: createMockPgBoss(),
      });

      const response = await app.inject({ method: 'GET', url: '/health' });
      expect(response.statusCode).toBe(200);

      const body = JSON.parse(response.body);
      expect(body.status).toBe('ok');
      expect(body.checks).toBeDefined();
      expect(body.checks.postgres.status).toBe('ok');
      expect(body.checks.redis.status).toBe('ok');
      expect(body.checks.pgboss.status).toBe('ok');

      // Detailed fields
      expect(typeof body.version).toBe('string');
      expect(typeof body.uptime_seconds).toBe('number');
      expect(body.uptime_seconds).toBeGreaterThanOrEqual(0);

      // Memory
      expect(typeof body.memory.rss_mb).toBe('number');
      expect(typeof body.memory.heap_used_mb).toBe('number');
      expect(typeof body.memory.heap_total_mb).toBe('number');
      expect(body.memory.rss_mb).toBeGreaterThan(0);

      await app.close();
    });

    it('returns 503 with detailed status when any check fails', async () => {
      const app = await buildTestApp({
        prisma: createMockPrisma({ failQuery: true }) as any,
        redis: createMockRedis() as any,
        pgBoss: createMockPgBoss(),
      });

      const response = await app.inject({ method: 'GET', url: '/health' });
      expect(response.statusCode).toBe(503);

      const body = JSON.parse(response.body);
      expect(body.status).toBe('degraded');
      // Detailed fields should still be present
      expect(typeof body.version).toBe('string');
      expect(typeof body.uptime_seconds).toBe('number');
      expect(body.memory).toBeDefined();

      await app.close();
    });
  });

  // ─── runReadinessChecks (unit) ────────────────────────────────
  describe('runReadinessChecks', () => {
    it('runs all checks in parallel', async () => {
      const prisma = createMockPrisma();
      const redis = createMockRedis();
      const pgBoss = createMockPgBoss();

      const result = await runReadinessChecks({
        prisma: prisma as any,
        redis: redis as any,
        pgBoss,
      });

      expect(result.status).toBe('ok');
      expect(prisma.$queryRaw).toHaveBeenCalledTimes(1);
      expect(redis.ping).toHaveBeenCalledTimes(1);
      expect(pgBoss.getQueueSize).toHaveBeenCalledTimes(1);
    });

    it('reports degraded if any single check fails', async () => {
      const result = await runReadinessChecks({
        prisma: createMockPrisma() as any,
        redis: createMockRedis({ failPing: true }) as any,
        pgBoss: createMockPgBoss(),
      });

      expect(result.status).toBe('degraded');
      expect(result.checks.postgres.status).toBe('ok');
      expect(result.checks.redis.status).toBe('error');
      expect(result.checks.pgboss.status).toBe('ok');
    });

    it('includes latency for all checks', async () => {
      const result = await runReadinessChecks({
        prisma: createMockPrisma() as any,
        redis: createMockRedis() as any,
        pgBoss: createMockPgBoss(),
      });

      expect(typeof result.checks.postgres.latency_ms).toBe('number');
      expect(typeof result.checks.redis.latency_ms).toBe('number');
      expect(typeof result.checks.pgboss.latency_ms).toBe('number');
    });
  });
});
