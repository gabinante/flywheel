import type { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import fp from 'fastify-plugin';
import type { Redis } from 'ioredis';
import {
  checkRateLimit,
  categorizeEndpoint,
  type RateLimitOverride,
} from '../utils/rate-limiter.js';

/**
 * Options for the per-merchant rate limit plugin.
 */
export interface PerMerchantRateLimitOptions {
  /** ioredis client instance for sliding window state */
  redis: Redis;
  /**
   * Function to look up per-merchant rate limit overrides by API key.
   * Returns null/undefined if no overrides are configured.
   */
  getMerchantOverride?: (
    apiKey: string
  ) => Promise<RateLimitOverride | null | undefined>;
  /**
   * Route prefixes that should be skipped (e.g. admin routes).
   * Defaults to ['/api/v1/admin'].
   */
  skipPrefixes?: string[];
}

/**
 * Fastify plugin that enforces per-API-key rate limits using a Redis
 * sliding window algorithm.
 *
 * Wrapped with fastify-plugin to avoid encapsulation — the preHandler
 * hook applies to ALL routes in the parent scope (and children).
 *
 * This is registered as a preHandler hook and runs AFTER the global
 * @fastify/rate-limit plugin (which acts as an outer backstop).
 *
 * Usage:
 *   await app.register(perMerchantRateLimit, { redis, getMerchantOverride });
 */
async function perMerchantRateLimitPlugin(
  app: FastifyInstance,
  opts: PerMerchantRateLimitOptions
): Promise<void> {
  const {
    redis,
    getMerchantOverride,
    skipPrefixes = ['/api/v1/admin'],
  } = opts;

  app.addHook(
    'preHandler',
    async (request: FastifyRequest, reply: FastifyReply) => {
      // Skip rate limiting for admin routes
      if (skipPrefixes.some((prefix) => request.url.startsWith(prefix))) {
        return;
      }

      // Extract API key from X-API-Key header or auth context
      const apiKey = extractApiKey(request);

      if (!apiKey) {
        // No API key — skip per-merchant rate limiting.
        // Auth middleware will handle unauthorized requests separately.
        return;
      }

      // Determine endpoint category from method + URL
      const endpoint = categorizeEndpoint(request.method, request.url);

      // Look up per-merchant overrides if a resolver is provided
      let override: RateLimitOverride | null | undefined = null;
      if (getMerchantOverride) {
        try {
          override = await getMerchantOverride(apiKey);
        } catch (err) {
          // Log but don't block on override lookup failure — use defaults
          request.log.warn(
            { err },
            'Failed to look up merchant rate limit override'
          );
        }
      }

      // Check rate limit
      const result = await checkRateLimit(redis, apiKey, endpoint, override);

      // Always set rate limit headers on the response
      reply.header('X-RateLimit-Limit', result.limit);
      reply.header('X-RateLimit-Remaining', result.remaining);
      reply.header('X-RateLimit-Reset', result.resetAt);

      if (!result.allowed) {
        const retryAfter = Math.max(1, result.resetAt - Math.floor(Date.now() / 1000));
        reply.header('Retry-After', retryAfter);
        return reply.code(429).send({
          statusCode: 429,
          error: 'Too Many Requests',
          message: `Rate limit exceeded. Try again in ${retryAfter} seconds.`,
        });
      }
    }
  );
}

/**
 * Extract the API key from the request. Checks:
 * 1. X-API-Key header
 * 2. Authorization Bearer token (as fallback)
 * 3. Auth context on the request (if set by prior middleware)
 */
function extractApiKey(request: FastifyRequest): string | null {
  // Check X-API-Key header first
  const xApiKey = request.headers['x-api-key'];
  if (typeof xApiKey === 'string' && xApiKey.length > 0) {
    return xApiKey;
  }

  // Check Authorization Bearer token as fallback
  const authHeader = request.headers['authorization'];
  if (typeof authHeader === 'string' && authHeader.startsWith('Bearer ')) {
    return authHeader.substring(7);
  }

  // Check if API key is stored in request context (set by prior auth middleware)
  const requestAny = request as Record<string, unknown>;
  if (
    requestAny.merchant &&
    typeof requestAny.merchant === 'object' &&
    (requestAny.merchant as Record<string, unknown>).apiKey
  ) {
    return (requestAny.merchant as Record<string, unknown>).apiKey as string;
  }

  return null;
}

/**
 * Export with fastify-plugin wrapper to prevent encapsulation.
 * This ensures the preHandler hook applies to all routes, not just
 * routes registered within this plugin's scope.
 */
export const perMerchantRateLimit = fp(perMerchantRateLimitPlugin, {
  name: 'per-merchant-rate-limit',
  fastify: '4.x',
});

export default perMerchantRateLimit;
