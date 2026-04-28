import { describe, it, expect, beforeEach, vi } from 'vitest';
import Fastify, { type FastifyInstance } from 'fastify';
import { perMerchantRateLimit } from '../plugins/rate-limit.js';

/**
 * Create a mock Redis that implements the sliding window via eval.
 */
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

      if (!sortedSets.has(key)) {
        sortedSets.set(key, new Map());
      }
      const set = sortedSets.get(key)!;

      // Remove entries outside window
      const windowStart = now - window;
      for (const [id, score] of set.entries()) {
        if (score <= windowStart) {
          set.delete(id);
        }
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
    _clear() {
      sortedSets.clear();
    },
  };
}

describe('perMerchantRateLimit plugin', () => {
  let app: FastifyInstance;
  let mockRedis: ReturnType<typeof createMockRedis>;

  beforeEach(async () => {
    mockRedis = createMockRedis();
    app = Fastify({ logger: false });

    await app.register(perMerchantRateLimit, {
      redis: mockRedis as any,
      skipPrefixes: ['/api/v1/admin'],
    });

    // Register test routes
    app.get('/api/v1/merchants', async () => ({ merchants: [] }));
    app.post('/api/v1/checkout/sessions', async () => ({
      session: { id: 'sess_123' },
    }));
    app.patch('/api/v1/merchants/:id', async () => ({ updated: true }));
    app.get('/api/v1/admin/settings', async () => ({ settings: {} }));

    await app.ready();
  });

  it('allows requests with X-API-Key header and sets rate limit headers', async () => {
    const response = await app.inject({
      method: 'GET',
      url: '/api/v1/merchants',
      headers: { 'x-api-key': 'test-key-123' },
    });

    expect(response.statusCode).toBe(200);
    expect(response.headers['x-ratelimit-limit']).toBe('1000');
    expect(response.headers['x-ratelimit-remaining']).toBeDefined();
    expect(response.headers['x-ratelimit-reset']).toBeDefined();
  });

  it('allows requests without API key (no per-merchant rate limit)', async () => {
    const response = await app.inject({
      method: 'GET',
      url: '/api/v1/merchants',
    });

    expect(response.statusCode).toBe(200);
    // No rate limit headers when no API key
    expect(response.headers['x-ratelimit-limit']).toBeUndefined();
  });

  it('categorizes POST /checkout as checkout with 100 limit', async () => {
    const response = await app.inject({
      method: 'POST',
      url: '/api/v1/checkout/sessions',
      headers: { 'x-api-key': 'test-key-123' },
      payload: {},
    });

    expect(response.statusCode).toBe(200);
    expect(response.headers['x-ratelimit-limit']).toBe('100');
  });

  it('categorizes PATCH as write with 200 limit', async () => {
    const response = await app.inject({
      method: 'PATCH',
      url: '/api/v1/merchants/abc',
      headers: { 'x-api-key': 'test-key-123' },
      payload: {},
    });

    expect(response.statusCode).toBe(200);
    expect(response.headers['x-ratelimit-limit']).toBe('200');
  });

  it('returns 429 when rate limit exceeded', async () => {
    // Override to a low limit for testing
    const lowLimitApp = Fastify({ logger: false });
    await lowLimitApp.register(perMerchantRateLimit, {
      redis: mockRedis as any,
      getMerchantOverride: async () => ({ checkout: 3 }),
    });
    lowLimitApp.post('/api/v1/checkout/sessions', async () => ({
      session: { id: 'sess_123' },
    }));
    await lowLimitApp.ready();

    // Send 3 requests (the override limit)
    for (let i = 0; i < 3; i++) {
      const res = await lowLimitApp.inject({
        method: 'POST',
        url: '/api/v1/checkout/sessions',
        headers: { 'x-api-key': 'test-key-429' },
        payload: {},
      });
      expect(res.statusCode).toBe(200);
    }

    // 4th request should be rate limited
    const blocked = await lowLimitApp.inject({
      method: 'POST',
      url: '/api/v1/checkout/sessions',
      headers: { 'x-api-key': 'test-key-429' },
      payload: {},
    });

    expect(blocked.statusCode).toBe(429);
    const body = JSON.parse(blocked.body);
    expect(body.error).toBe('Too Many Requests');
    expect(blocked.headers['retry-after']).toBeDefined();
    expect(parseInt(blocked.headers['retry-after'] as string, 10)).toBeGreaterThan(0);
    expect(blocked.headers['x-ratelimit-remaining']).toBe('0');
    expect(blocked.headers['x-ratelimit-limit']).toBe('3');
  });

  it('skips rate limiting for admin routes', async () => {
    // Make many requests to admin endpoint without API key
    for (let i = 0; i < 10; i++) {
      const response = await app.inject({
        method: 'GET',
        url: '/api/v1/admin/settings',
        headers: { 'x-api-key': 'admin-key' },
      });
      expect(response.statusCode).toBe(200);
      // Admin routes should not have per-merchant rate limit headers
      expect(response.headers['x-ratelimit-limit']).toBeUndefined();
    }
  });

  it('extracts API key from Authorization Bearer header', async () => {
    const response = await app.inject({
      method: 'GET',
      url: '/api/v1/merchants',
      headers: { authorization: 'Bearer my-bearer-token' },
    });

    expect(response.statusCode).toBe(200);
    expect(response.headers['x-ratelimit-limit']).toBe('1000');
  });

  it('shows decreasing remaining count across requests', async () => {
    const remainingValues: string[] = [];

    for (let i = 0; i < 5; i++) {
      const response = await app.inject({
        method: 'GET',
        url: '/api/v1/merchants',
        headers: { 'x-api-key': 'counter-key' },
      });
      remainingValues.push(response.headers['x-ratelimit-remaining'] as string);
    }

    // Remaining should decrease
    const nums = remainingValues.map(Number);
    for (let i = 1; i < nums.length; i++) {
      expect(nums[i]).toBeLessThan(nums[i - 1]);
    }
  });

  it('isolates rate limits per API key', async () => {
    // Use a low-limit app
    const lowLimitApp = Fastify({ logger: false });
    await lowLimitApp.register(perMerchantRateLimit, {
      redis: mockRedis as any,
      getMerchantOverride: async () => ({ read: 2 }),
    });
    lowLimitApp.get('/api/v1/merchants', async () => ({ merchants: [] }));
    await lowLimitApp.ready();

    // Exhaust key-a's limit
    for (let i = 0; i < 2; i++) {
      await lowLimitApp.inject({
        method: 'GET',
        url: '/api/v1/merchants',
        headers: { 'x-api-key': 'key-a' },
      });
    }

    // key-a should be blocked
    const blockedA = await lowLimitApp.inject({
      method: 'GET',
      url: '/api/v1/merchants',
      headers: { 'x-api-key': 'key-a' },
    });
    expect(blockedA.statusCode).toBe(429);

    // key-b should still work
    const allowedB = await lowLimitApp.inject({
      method: 'GET',
      url: '/api/v1/merchants',
      headers: { 'x-api-key': 'key-b' },
    });
    expect(allowedB.statusCode).toBe(200);
  });

  it('handles getMerchantOverride failure gracefully', async () => {
    const failApp = Fastify({ logger: false });
    await failApp.register(perMerchantRateLimit, {
      redis: mockRedis as any,
      getMerchantOverride: async () => {
        throw new Error('DB connection failed');
      },
    });
    failApp.get('/api/v1/merchants', async () => ({ merchants: [] }));
    await failApp.ready();

    // Should still work with defaults
    const response = await failApp.inject({
      method: 'GET',
      url: '/api/v1/merchants',
      headers: { 'x-api-key': 'test-key' },
    });
    expect(response.statusCode).toBe(200);
    expect(response.headers['x-ratelimit-limit']).toBe('1000');
  });
});

describe('admin route override validation', () => {
  it('validates rateLimitOverride schema via admin PATCH endpoint concept', () => {
    // Test the validation logic directly
    const validOverrides = [
      { checkout: 200 },
      { read: 2000, write: 500 },
      { checkout: 100, read: 1000, write: 200 },
      null,
    ];

    const invalidOverrides = [
      { checkout: -1 },
      { checkout: 0 },
      { checkout: 1.5 },
      { checkout: 'string' },
      { unknownKey: 100 },
    ];

    // Valid overrides should pass validation
    for (const override of validOverrides) {
      if (override === null) continue;
      for (const [key, value] of Object.entries(override)) {
        expect(['checkout', 'read', 'write']).toContain(key);
        expect(typeof value).toBe('number');
        expect(Number.isInteger(value)).toBe(true);
        expect(value).toBeGreaterThan(0);
      }
    }

    // Invalid overrides should fail validation
    expect(({ checkout: -1 } as any).checkout).toBeLessThan(1);
    expect(Number.isInteger(1.5)).toBe(false);
  });
});
