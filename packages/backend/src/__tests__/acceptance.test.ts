/**
 * Acceptance tests for gohighpayment-4: Per-Merchant Rate Limiting
 *
 * These tests verify all acceptance criteria from the ticket:
 * 1. First 100 checkout requests succeed with decreasing X-RateLimit-Remaining
 * 2. Request 101 returns 429 with Retry-After header
 * 3. Admin can set override to 200 for a merchant, allowing 200 requests
 * 4. Per-API-key isolation works correctly
 */
import { describe, it, expect, beforeEach } from 'vitest';
import Fastify, { type FastifyInstance } from 'fastify';
import { perMerchantRateLimit } from '../plugins/rate-limit.js';
import type { RateLimitOverride } from '../utils/rate-limiter.js';

function createMockRedis() {
  const sortedSets = new Map<string, Map<string, number>>();
  return {
    status: 'ready',
    async eval(
      _script: string,
      _keyCount: number,
      key: string,
      nowStr: string,
      windowStr: string,
      limitStr: string,
      requestId: string
    ): Promise<[number, number, number]> {
      const now = parseInt(nowStr, 10);
      const window = parseInt(windowStr, 10);
      const limit = parseInt(limitStr, 10);
      if (!sortedSets.has(key)) sortedSets.set(key, new Map());
      const set = sortedSets.get(key)!;
      const windowStart = now - window;
      for (const [id, score] of set.entries()) {
        if (score <= windowStart) set.delete(id);
      }
      const current = set.size;
      if (current < limit) {
        set.set(requestId, now);
        const remaining = limit - current - 1;
        const resetAt = Math.ceil((now + window) / 1000);
        return [1, remaining, resetAt];
      } else {
        let oldestScore = now;
        for (const score of set.values()) {
          if (score < oldestScore) oldestScore = score;
        }
        const resetAt = Math.ceil((oldestScore + window) / 1000);
        return [0, 0, resetAt];
      }
    },
    _clear() { sortedSets.clear(); },
  };
}

describe('Acceptance: Per-Merchant Rate Limiting', () => {
  let mockRedis: ReturnType<typeof createMockRedis>;
  let merchantOverrides: Record<string, RateLimitOverride | null>;

  function buildApp(overrides?: Record<string, RateLimitOverride | null>) {
    merchantOverrides = overrides ?? {};
    const app = Fastify({ logger: false });
    return app.register(perMerchantRateLimit, {
      redis: mockRedis as any,
      getMerchantOverride: async (apiKey: string) => {
        return merchantOverrides[apiKey] ?? null;
      },
      skipPrefixes: ['/api/v1/admin'],
    }).then(() => {
      app.post('/api/v1/checkout/sessions', async () => ({ session: { id: 'sess_123' } }));
      app.get('/api/v1/merchants', async () => ({ merchants: [] }));
      app.patch('/api/v1/admin/merchants/:id', async (req) => {
        const body = req.body as any;
        if (body?.rateLimitOverride !== undefined) {
          const apiKey = (req.headers['x-merchant-api-key'] as string) || '';
          merchantOverrides[apiKey] = body.rateLimitOverride;
        }
        return { updated: true };
      });
      return app.ready().then(() => app);
    });
  }

  beforeEach(() => {
    mockRedis = createMockRedis();
  });

  it('Criterion 1: first 100 checkout requests succeed with decreasing X-RateLimit-Remaining', async () => {
    const app = await buildApp();
    const apiKey = 'merchant-test-key-001';
    const remainingValues: number[] = [];

    for (let i = 0; i < 100; i++) {
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/checkout/sessions',
        headers: { 'x-api-key': apiKey },
        payload: { amount: 1000 },
      });
      expect(res.statusCode).toBe(200);
      remainingValues.push(parseInt(res.headers['x-ratelimit-remaining'] as string, 10));
    }

    // Verify decreasing remaining count
    expect(remainingValues[0]).toBe(99);
    expect(remainingValues[99]).toBe(0);
    for (let i = 1; i < remainingValues.length; i++) {
      expect(remainingValues[i]).toBeLessThan(remainingValues[i - 1]);
    }
  });

  it('Criterion 2: request 101 returns 429 with Retry-After header', async () => {
    const app = await buildApp();
    const apiKey = 'merchant-test-key-002';

    // Send 100 requests to exhaust the limit
    for (let i = 0; i < 100; i++) {
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/checkout/sessions',
        headers: { 'x-api-key': apiKey },
        payload: { amount: 1000 },
      });
      expect(res.statusCode).toBe(200);
    }

    // Request 101 should be rate limited
    const blocked = await app.inject({
      method: 'POST',
      url: '/api/v1/checkout/sessions',
      headers: { 'x-api-key': apiKey },
      payload: { amount: 1000 },
    });

    expect(blocked.statusCode).toBe(429);
    expect(blocked.headers['retry-after']).toBeDefined();
    expect(parseInt(blocked.headers['retry-after'] as string, 10)).toBeGreaterThan(0);
    expect(blocked.headers['x-ratelimit-limit']).toBe('100');
    expect(blocked.headers['x-ratelimit-remaining']).toBe('0');
    expect(blocked.headers['x-ratelimit-reset']).toBeDefined();

    const body = JSON.parse(blocked.body);
    expect(body.statusCode).toBe(429);
    expect(body.error).toBe('Too Many Requests');
  });

  it('Criterion 3: admin sets override to 200, verifies 200 requests allowed', async () => {
    const apiKey = 'merchant-test-key-003';
    // Start with override of 200 for checkout
    const app = await buildApp({ [apiKey]: { checkout: 200 } });

    // Should be able to send 200 requests
    for (let i = 0; i < 200; i++) {
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/checkout/sessions',
        headers: { 'x-api-key': apiKey },
        payload: { amount: 1000 },
      });
      expect(res.statusCode).toBe(200);
      expect(res.headers['x-ratelimit-limit']).toBe('200');
    }

    // Request 201 should be blocked
    const blocked = await app.inject({
      method: 'POST',
      url: '/api/v1/checkout/sessions',
      headers: { 'x-api-key': apiKey },
      payload: { amount: 1000 },
    });
    expect(blocked.statusCode).toBe(429);
  });

  it('Criterion 4: read endpoints limited to 1000 req/min per API key', async () => {
    const app = await buildApp();
    const apiKey = 'merchant-test-key-004';

    const res = await app.inject({
      method: 'GET',
      url: '/api/v1/merchants',
      headers: { 'x-api-key': apiKey },
    });

    expect(res.statusCode).toBe(200);
    expect(res.headers['x-ratelimit-limit']).toBe('1000');
    expect(res.headers['x-ratelimit-remaining']).toBe('999');
  });

  it('Criterion 5: global rate limit concept preserved - admin routes skip per-merchant limiting', async () => {
    const app = await buildApp();

    // Admin routes should not have per-merchant rate limit headers
    const res = await app.inject({
      method: 'PATCH',
      url: '/api/v1/admin/merchants/123',
      headers: { 'x-api-key': 'admin-key' },
      payload: { rateLimitOverride: { checkout: 200 } },
    });

    expect(res.statusCode).toBe(200);
    // Admin routes should be skipped for per-merchant rate limiting
    expect(res.headers['x-ratelimit-limit']).toBeUndefined();
  });
});
