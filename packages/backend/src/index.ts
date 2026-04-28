/**
 * GoHighPayment Backend — Fastify application entry point
 *
 * Structured JSON logging with:
 * - Request ID tracking (Fastify auto-generated)
 * - Correlation ID propagation for transaction flows
 * - Sensitive field redaction (PCI compliance)
 * - Configurable log level via LOG_LEVEL env var
 *
 * Observability:
 * - Sentry error tracking and performance monitoring
 * - Health check endpoints (liveness, readiness, detailed)
 */

// Initialize Sentry BEFORE importing Fastify (must be first)
import { initSentry, registerSentryErrorHandler, flushSentry } from './utils/sentry.js';
initSentry();

import Fastify from 'fastify';
import rateLimit from '@fastify/rate-limit';
import Redis from 'ioredis';
import { PrismaClient } from '@prisma/client';
import { perMerchantRateLimit } from './plugins/rate-limit.js';
import { registerAdminRoutes } from './routes/admin.js';
import { registerSubscriptionRoutes } from './routes/subscriptions.js';
import { registerCheckoutRoutes } from './routes/checkout.js';
import { registerWebhookRoutes } from './routes/webhooks.js';
import { registerHealthRoutes, type HealthCheckDeps } from './routes/health.js';
import { loggerConfig, createCorrelationId, createTimer, createServiceLogger } from './utils/logger.js';

// Re-export logging utilities for use across the codebase
export { createCorrelationId, createTimer, createServiceLogger, loggerConfig } from './utils/logger.js';
export { REDACT_PATHS } from './utils/logger.js';

// Re-export Sentry utilities
export { initSentry, captureException, setSentryContext, isSentryEnabled } from './utils/sentry.js';

// Re-export health check utilities
export { runReadinessChecks, type ReadinessResponse, type DetailedHealthResponse, type CheckResult } from './routes/health.js';

// Re-export services
export {
  createResidualService,
  TIER_BPS,
  TWO_TIER_BPS,
  type ResidualCalculationResult,
  type MerchantVolume,
  type NmiReportingClient,
  type ResidualServiceDeps,
} from './services/residual.service.js';

export {
  createEmailService,
  type EmailService,
  type EmailServiceConfig,
  type SendEmailParams,
  type SendEmailResult,
} from './services/email.service.js';

export {
  registerJobHandlers,
  RESIDUAL_CALCULATION_JOB,
  EMAIL_SEND_JOB,
  getPreviousMonthRange,
  type ResidualJobData,
  type EmailSendJobData,
  type JobHandlerDeps,
} from './jobs/handlers.js';

export {
  scheduleMonthlyResidualJob,
  enqueueResidualCalculation,
  enqueueEmail,
  enqueueEmailSimple,
  type EnqueueEmailParams,
} from './jobs/queue.js';

export {
  registerAdminRoutes,
} from './routes/admin.js';

/**
 * Build and configure the Fastify application.
 * Exported for testing; the server is started at the bottom of this file.
 */
export async function buildApp(opts?: {
  redis?: Redis;
  prisma?: PrismaClient;
  pgBoss?: HealthCheckDeps['pgBoss'] | null;
  logger?: boolean | object;
}) {
  const app = Fastify({
    logger: opts?.logger === false
      ? false
      : (typeof opts?.logger === 'object' ? opts.logger : loggerConfig),
    genReqId: () => `req_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`,
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

  // ---- Sentry error handler (before routes so it captures all errors) ----
  registerSentryErrorHandler(app);

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
    skipPrefixes: ['/api/v1/admin', '/api/v1/webhooks', '/health'],
  });

  // ---- Health check routes ----
  registerHealthRoutes(app, {
    prisma,
    redis,
    pgBoss: opts?.pgBoss ?? null,
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

  // Graceful shutdown
  app.addHook('onClose', async () => {
    await flushSentry();
    await prisma.$disconnect();
    redis.disconnect();
  });

  app.log.info({ action: 'app_built', logLevel: loggerConfig.level }, 'Fastify app configured with structured logging and health checks');

  return app;
}

// ---- Start server (only when run directly) ----
const isMainModule =
  typeof process !== 'undefined' &&
  process.argv[1] &&
  (process.argv[1].endsWith('/index.ts') ||
    process.argv[1].endsWith('/index.js'));

if (isMainModule) {
  buildApp()
    .then((app) => {
      const port = parseInt(process.env.PORT ?? '3000', 10);
      const host = process.env.HOST ?? '0.0.0.0';
      return app.listen({ port, host });
    })
    .then((address) => {
      // Fastify logs its own "Server listening at..." message automatically
    })
    .catch((err) => {
      // Use structured logging even for fatal startup errors
      const pino = require('pino');
      const logger = pino(loggerConfig);
      logger.fatal({ err, action: 'server_start_failed' }, 'Failed to start server');
      process.exit(1);
    });
}
