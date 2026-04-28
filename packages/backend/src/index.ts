import Fastify from 'fastify';
import rateLimit from '@fastify/rate-limit';
import Redis from 'ioredis';
import { PrismaClient } from '@prisma/client';
import { perMerchantRateLimit } from './plugins/rate-limit.js';
import { registerAdminRoutes } from './routes/admin.js';
import { registerSubscriptionRoutes } from './routes/subscriptions.js';
import { registerCheckoutRoutes } from './routes/checkout.js';
import { registerWebhookRoutes } from './routes/webhooks.js';

/**
 * Build and configure the Fastify application.
 * Exported for testing; the server is started at the bottom of this file.
 */
export async function buildApp(opts?: {
  redis?: Redis;
  prisma?: PrismaClient;
  logger?: boolean;
}) {
  const app = Fastify({
    logger: opts?.logger ?? true,
  });

  // ---- Prisma client ----
  const prisma = opts?.prisma ?? new PrismaClient();

  // ---- Redis client ----
  const redis =
    opts?.redis ??
    new Redis(process.env.REDIS_URL ?? 'redis://localhost:6379', {
      maxRetriesPerRequest: 3,
      lazyConnect: true,
    });

  // Connect if not already connected
  if (redis.status === 'wait') {
    await redis.connect();
  }

  // ---- Global rate limit (outer backstop) ----
  await app.register(rateLimit, {
    max: 5000,
    timeWindow: '1 minute',
    redis,
  });

  // ---- Per-merchant rate limit (inner, per-API-key) ----
  await app.register(perMerchantRateLimit, {
    redis,
    getMerchantOverride: async (apiKey: string) => {
      try {
        const merchant = await prisma.merchant.findUnique({
          where: { apiKey },
          select: { rateLimitOverride: true },
        });
        return merchant?.rateLimitOverride as { checkout?: number; read?: number; write?: number } | null;
      } catch {
        return null;
      }
    },
    skipPrefixes: ['/api/v1/admin', '/api/v1/webhooks'],
  });

  // ---- Routes ----
  await app.register(registerAdminRoutes, { prefix: '/api/v1/admin' });

  // Merchant subscription management routes
  await app.register(
    async (instance) => {
      await registerSubscriptionRoutes(instance, { prisma });
    },
    { prefix: '/api/v1/merchant' }
  );

  // Checkout routes (customer-facing)
  await app.register(
    async (instance) => {
      await registerCheckoutRoutes(instance, { prisma });
    },
    { prefix: '/api/v1/checkout' }
  );

  // Webhook routes (NMI callbacks)
  await app.register(
    async (instance) => {
      await registerWebhookRoutes(instance, { prisma });
    },
    { prefix: '/api/v1/webhooks' }
  );

  // Health check
  app.get('/health', async () => ({ status: 'ok' }));

  // Graceful shutdown
  app.addHook('onClose', async () => {
    await prisma.$disconnect();
    redis.disconnect();
  });

  return app;
}

// ---- Start server (only when run directly) ----
const isMainModule =
  typeof process !== 'undefined' &&
  process.argv[1] &&
  (process.argv[1].endsWith('/index.ts') ||
    process.argv[1].endsWith('/index.js'));

if (isMainModule) {
  buildApp({ logger: true })
    .then((app) => {
      const port = parseInt(process.env.PORT ?? '3000', 10);
      const host = process.env.HOST ?? '0.0.0.0';
      return app.listen({ port, host });
    })
    .then((address) => {
      console.log(`Server listening on ${address}`);
    })
    .catch((err) => {
      console.error('Failed to start server:', err);
      process.exit(1);
    });
}
