import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  hashApiKey,
  categorizeEndpoint,
  resolveLimit,
  checkRateLimit,
  DEFAULT_LIMITS,
  type RateLimitOverride,
  type EndpointCategory,
} from '../utils/rate-limiter.js';

// ---------- Unit tests (no Redis needed) ----------

describe('hashApiKey', () => {
  it('returns a 16-character hex string', () => {
    const hash = hashApiKey('test-api-key-123');
    expect(hash).toHaveLength(16);
    expect(hash).toMatch(/^[0-9a-f]{16}$/);
  });

  it('returns consistent results for the same key', () => {
    const a = hashApiKey('my-key');
    const b = hashApiKey('my-key');
    expect(a).toBe(b);
  });

  it('returns different results for different keys', () => {
    const a = hashApiKey('key-a');
    const b = hashApiKey('key-b');
    expect(a).not.toBe(b);
  });
});

describe('categorizeEndpoint', () => {
  it('classifies POST /api/v1/checkout/sessions as checkout', () => {
    expect(categorizeEndpoint('POST', '/api/v1/checkout/sessions')).toBe(
      'checkout'
    );
  });

  it('classifies POST /api/v1/checkout/sessions/abc/confirm as checkout', () => {
    expect(
      categorizeEndpoint('POST', '/api/v1/checkout/sessions/abc/confirm')
    ).toBe('checkout');
  });

  it('classifies GET /api/v1/checkout/sessions as read (GET overrides checkout path)', () => {
    expect(categorizeEndpoint('GET', '/api/v1/checkout/sessions')).toBe('read');
  });

  it('classifies GET /api/v1/merchants as read', () => {
    expect(categorizeEndpoint('GET', '/api/v1/merchants')).toBe('read');
  });

  it('classifies POST /api/v1/webhooks as write', () => {
    expect(categorizeEndpoint('POST', '/api/v1/webhooks')).toBe('write');
  });

  it('classifies PATCH /api/v1/merchants/123 as write', () => {
    expect(categorizeEndpoint('PATCH', '/api/v1/merchants/123')).toBe('write');
  });

  it('classifies DELETE /api/v1/merchants/123 as write', () => {
    expect(categorizeEndpoint('DELETE', '/api/v1/merchants/123')).toBe(
      'write'
    );
  });

  it('classifies PUT /api/v1/merchants/123 as write', () => {
    expect(categorizeEndpoint('PUT', '/api/v1/merchants/123')).toBe('write');
  });

  it('is case-insensitive for method', () => {
    expect(categorizeEndpoint('get', '/api/v1/merchants')).toBe('read');
    expect(categorizeEndpoint('post', '/api/v1/checkout/sessions')).toBe(
      'checkout'
    );
  });
});

describe('resolveLimit', () => {
  it('returns default limits when no override provided', () => {
    expect(resolveLimit('checkout')).toBe(100);
    expect(resolveLimit('read')).toBe(1000);
    expect(resolveLimit('write')).toBe(200);
  });

  it('returns default limits when override is null', () => {
    expect(resolveLimit('checkout', null)).toBe(100);
    expect(resolveLimit('read', null)).toBe(1000);
  });

  it('returns override values when provided', () => {
    const override: RateLimitOverride = { checkout: 200, read: 2000, write: 500 };
    expect(resolveLimit('checkout', override)).toBe(200);
    expect(resolveLimit('read', override)).toBe(2000);
    expect(resolveLimit('write', override)).toBe(500);
  });

  it('returns default for categories not in the override', () => {
    const override: RateLimitOverride = { checkout: 200 };
    expect(resolveLimit('checkout', override)).toBe(200);
    expect(resolveLimit('read', override)).toBe(1000); // default
    expect(resolveLimit('write', override)).toBe(200); // default
  });
});

describe('DEFAULT_LIMITS', () => {
  it('has expected default values', () => {
    expect(DEFAULT_LIMITS.checkout).toBe(100);
    expect(DEFAULT_LIMITS.read).toBe(1000);
    expect(DEFAULT_LIMITS.write).toBe(200);
  });
});

// ---------- Integration tests with mock Redis ----------

/**
 * Mock Redis implementation using an in-memory sorted set.
 * Simulates the Lua eval for the sliding window algorithm.
 */
function createMockRedis() {
  const sortedSets = new Map<string, Map<string, number>>();
  const ttls = new Map<string, number>();

  return {
    status: 'ready',
    async eval(
      _script: string,
      keyCount: number,
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
        // Add request
        set.set(requestId, now);
        ttls.set(key, now + window);

        const remaining = limit - current - 1;
        const resetAt = Math.ceil((now + window) / 1000);
        return [1, remaining, resetAt];
      } else {
        // Rate limited
        ttls.set(key, now + window);

        // Find oldest entry
        let oldestScore = now;
        for (const score of set.values()) {
          if (score < oldestScore) {
            oldestScore = score;
          }
        }
        const resetAt = Math.ceil((oldestScore + window) / 1000);
        return [0, 0, resetAt];
      }
    },
    // For test inspection
    _sortedSets: sortedSets,
    _ttls: ttls,
    _clear() {
      sortedSets.clear();
      ttls.clear();
    },
  };
}

describe('checkRateLimit (with mock Redis)', () => {
  let mockRedis: ReturnType<typeof createMockRedis>;

  beforeEach(() => {
    mockRedis = createMockRedis();
  });

  it('allows requests within the limit', async () => {
    const result = await checkRateLimit(
      mockRedis as any,
      'test-key',
      'checkout'
    );
    expect(result.allowed).toBe(true);
    expect(result.remaining).toBe(99); // 100 limit - 1 request
    expect(result.limit).toBe(100);
  });

  it('returns decreasing remaining count', async () => {
    const results: number[] = [];
    for (let i = 0; i < 5; i++) {
      const result = await checkRateLimit(
        mockRedis as any,
        'test-key',
        'checkout'
      );
      results.push(result.remaining);
    }
    expect(results).toEqual([99, 98, 97, 96, 95]);
  });

  it('blocks requests after limit is exceeded', async () => {
    // Send 100 requests (the limit for checkout)
    for (let i = 0; i < 100; i++) {
      const result = await checkRateLimit(
        mockRedis as any,
        'test-key',
        'checkout'
      );
      expect(result.allowed).toBe(true);
    }

    // Request 101 should be blocked
    const blocked = await checkRateLimit(
      mockRedis as any,
      'test-key',
      'checkout'
    );
    expect(blocked.allowed).toBe(false);
    expect(blocked.remaining).toBe(0);
  });

  it('uses different buckets for different API keys', async () => {
    // Fill up key-a's checkout limit
    for (let i = 0; i < 100; i++) {
      await checkRateLimit(mockRedis as any, 'key-a', 'checkout');
    }

    // key-b should still be allowed
    const result = await checkRateLimit(
      mockRedis as any,
      'key-b',
      'checkout'
    );
    expect(result.allowed).toBe(true);
    expect(result.remaining).toBe(99);
  });

  it('uses different buckets for different endpoint categories', async () => {
    // Fill up checkout limit
    for (let i = 0; i < 100; i++) {
      await checkRateLimit(mockRedis as any, 'test-key', 'checkout');
    }

    // Read should still be allowed (different category)
    const result = await checkRateLimit(
      mockRedis as any,
      'test-key',
      'read'
    );
    expect(result.allowed).toBe(true);
    expect(result.remaining).toBe(999);
  });

  it('respects per-merchant overrides', async () => {
    const override: RateLimitOverride = { checkout: 5 };

    // Send 5 requests (the override limit)
    for (let i = 0; i < 5; i++) {
      const result = await checkRateLimit(
        mockRedis as any,
        'test-key',
        'checkout',
        override
      );
      expect(result.allowed).toBe(true);
      expect(result.limit).toBe(5);
    }

    // Request 6 should be blocked
    const blocked = await checkRateLimit(
      mockRedis as any,
      'test-key',
      'checkout',
      override
    );
    expect(blocked.allowed).toBe(false);
    expect(blocked.limit).toBe(5);
  });

  it('uses defaults for categories not in override', async () => {
    const override: RateLimitOverride = { checkout: 5 };

    // Read should use default (1000)
    const result = await checkRateLimit(
      mockRedis as any,
      'test-key',
      'read',
      override
    );
    expect(result.limit).toBe(1000);
    expect(result.allowed).toBe(true);
  });

  it('returns correct resetAt timestamp', async () => {
    const result = await checkRateLimit(
      mockRedis as any,
      'test-key',
      'checkout'
    );
    const now = Math.floor(Date.now() / 1000);
    // resetAt should be roughly 60 seconds from now
    expect(result.resetAt).toBeGreaterThanOrEqual(now + 59);
    expect(result.resetAt).toBeLessThanOrEqual(now + 61);
  });

  it('handles read endpoint with 1000 req/min limit', async () => {
    // Verify first request succeeds with correct limit
    const result = await checkRateLimit(
      mockRedis as any,
      'test-key',
      'read'
    );
    expect(result.allowed).toBe(true);
    expect(result.limit).toBe(1000);
    expect(result.remaining).toBe(999);
  });

  it('handles write endpoint with 200 req/min limit', async () => {
    const result = await checkRateLimit(
      mockRedis as any,
      'test-key',
      'write'
    );
    expect(result.allowed).toBe(true);
    expect(result.limit).toBe(200);
    expect(result.remaining).toBe(199);
  });
});
