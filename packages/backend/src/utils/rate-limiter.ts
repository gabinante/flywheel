import { createHash } from 'crypto';
import type { Redis } from 'ioredis';

/**
 * Endpoint categories for rate limiting.
 * - checkout: POST /api/v1/checkout/* endpoints
 * - read: GET requests
 * - write: POST/PATCH/DELETE requests (non-checkout)
 */
export type EndpointCategory = 'checkout' | 'read' | 'write';

/**
 * Per-merchant rate limit overrides. Each field is optional;
 * if absent, the default limit for that category is used.
 */
export interface RateLimitOverride {
  checkout?: number;
  read?: number;
  write?: number;
}

/**
 * Result from checking rate limit for a request.
 */
export interface RateLimitResult {
  /** Whether the request is allowed */
  allowed: boolean;
  /** Number of requests remaining in the current window */
  remaining: number;
  /** Unix epoch (seconds) when the current window resets */
  resetAt: number;
  /** The configured limit for this endpoint category */
  limit: number;
}

/**
 * Default rate limits (requests per minute) per endpoint category.
 */
export const DEFAULT_LIMITS: Record<EndpointCategory, number> = {
  checkout: 100,
  read: 1000,
  write: 200,
};

/** Sliding window duration in seconds (1 minute). */
const WINDOW_SECONDS = 60;

/**
 * Lua script for Redis sliding window rate limiting.
 *
 * Uses a sorted set where each element is a unique request ID scored by timestamp.
 * The sliding window is maintained by removing entries older than (now - window).
 *
 * KEYS[1] = rate limit key
 * ARGV[1] = current timestamp in milliseconds
 * ARGV[2] = window size in milliseconds
 * ARGV[3] = max requests allowed in window
 * ARGV[4] = unique request ID (timestamp + random)
 *
 * Returns: [allowed (0/1), remaining count, reset timestamp in seconds]
 */
const SLIDING_WINDOW_SCRIPT = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local request_id = ARGV[4]

-- Remove entries outside the sliding window
local window_start = now - window
redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)

-- Count current entries in window
local current = redis.call('ZCARD', key)

if current < limit then
  -- Add the new request
  redis.call('ZADD', key, now, request_id)
  -- Set TTL to auto-expire the key after the window passes
  redis.call('PEXPIRE', key, window)

  local remaining = limit - current - 1
  local reset_at = math.ceil((now + window) / 1000)
  return {1, remaining, reset_at}
else
  -- Rate limited — do not add the request
  -- Set TTL in case it's missing
  redis.call('PEXPIRE', key, window)

  -- Find when the oldest entry in the window will expire
  local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
  local reset_at
  if #oldest > 0 then
    reset_at = math.ceil((tonumber(oldest[2]) + window) / 1000)
  else
    reset_at = math.ceil((now + window) / 1000)
  end

  return {0, 0, reset_at}
end
`;

/**
 * Hash an API key to create a safe Redis key component.
 * Uses SHA-256 to avoid storing raw API keys in Redis key names.
 */
export function hashApiKey(apiKey: string): string {
  return createHash('sha256').update(apiKey).digest('hex').substring(0, 16);
}

/**
 * Build the Redis key for a given API key hash and endpoint category.
 */
function buildRedisKey(apiKeyHash: string, endpoint: EndpointCategory): string {
  return `ratelimit:${endpoint}:${apiKeyHash}`;
}

/**
 * Determine the endpoint category from the request method and URL.
 */
export function categorizeEndpoint(
  method: string,
  url: string
): EndpointCategory {
  const upperMethod = method.toUpperCase();

  // Checkout endpoints: POST to /api/v1/checkout/*
  if (url.startsWith('/api/v1/checkout') && upperMethod === 'POST') {
    return 'checkout';
  }

  // Read endpoints: GET requests
  if (upperMethod === 'GET') {
    return 'read';
  }

  // Write endpoints: POST, PATCH, DELETE, PUT
  return 'write';
}

/**
 * Resolve the effective rate limit for a merchant + endpoint category.
 * Per-merchant overrides take priority over defaults.
 */
export function resolveLimit(
  endpoint: EndpointCategory,
  override?: RateLimitOverride | null
): number {
  if (override && typeof override[endpoint] === 'number') {
    return override[endpoint]!;
  }
  return DEFAULT_LIMITS[endpoint];
}

let requestCounter = 0;

/**
 * Check the rate limit for a given API key and endpoint category
 * using a Redis sliding window algorithm.
 *
 * @param redis - ioredis client instance
 * @param apiKey - The raw API key from the request
 * @param endpoint - The endpoint category
 * @param override - Optional per-merchant rate limit overrides
 * @returns RateLimitResult indicating whether the request is allowed
 */
export async function checkRateLimit(
  redis: Redis,
  apiKey: string,
  endpoint: EndpointCategory,
  override?: RateLimitOverride | null
): Promise<RateLimitResult> {
  const apiKeyHash = hashApiKey(apiKey);
  const key = buildRedisKey(apiKeyHash, endpoint);
  const limit = resolveLimit(endpoint, override);
  const now = Date.now();
  const windowMs = WINDOW_SECONDS * 1000;

  // Generate a unique request ID to avoid collisions in the sorted set
  requestCounter = (requestCounter + 1) % 1_000_000;
  const requestId = `${now}:${requestCounter}:${Math.random().toString(36).substring(2, 8)}`;

  const result = (await redis.eval(
    SLIDING_WINDOW_SCRIPT,
    1,
    key,
    now.toString(),
    windowMs.toString(),
    limit.toString(),
    requestId
  )) as [number, number, number];

  const [allowed, remaining, resetAt] = result;

  return {
    allowed: allowed === 1,
    remaining,
    resetAt,
    limit,
  };
}
