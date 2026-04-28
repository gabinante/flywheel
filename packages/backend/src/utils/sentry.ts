/**
 * Sentry Integration for GoHighPayment Backend
 *
 * Initializes Sentry for error tracking and performance monitoring.
 * Must be called before Fastify starts.
 *
 * Configuration via environment variables:
 * - SENTRY_DSN: Sentry Data Source Name (required to enable)
 * - SENTRY_ENVIRONMENT: Environment tag (dev/staging/prod, default: development)
 * - SENTRY_RELEASE: Release version (default: package version)
 * - SENTRY_TRACES_SAMPLE_RATE: Performance sampling rate (0.0 - 1.0, default: 0.1)
 */

import * as Sentry from '@sentry/node';
import type { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';

let initialized = false;

export interface SentryConfig {
  dsn?: string;
  environment?: string;
  release?: string;
  tracesSampleRate?: number;
}

/**
 * Initialize Sentry SDK. Safe to call multiple times — only initializes once.
 * No-op if SENTRY_DSN is not set.
 */
export function initSentry(config?: SentryConfig): void {
  if (initialized) return;

  const dsn = config?.dsn ?? process.env.SENTRY_DSN;
  if (!dsn) {
    return; // Sentry disabled when no DSN configured
  }

  Sentry.init({
    dsn,
    environment: config?.environment ?? process.env.SENTRY_ENVIRONMENT ?? 'development',
    release: config?.release ?? process.env.SENTRY_RELEASE ?? `gohighpayment-backend@${process.env.npm_package_version ?? '0.1.0'}`,
    tracesSampleRate: config?.tracesSampleRate ?? parseFloat(process.env.SENTRY_TRACES_SAMPLE_RATE ?? '0.1'),

    // Filter out health check transactions to reduce noise
    beforeSendTransaction(event) {
      if (event.transaction?.startsWith('/health')) {
        return null;
      }
      return event;
    },
  });

  initialized = true;
}

/**
 * Check if Sentry is initialized and active.
 */
export function isSentryEnabled(): boolean {
  return initialized;
}

/**
 * Set Sentry context for the current scope (e.g., merchantId, correlationId).
 */
export function setSentryContext(
  context: Record<string, string | number | boolean | undefined>
): void {
  if (!initialized) return;

  Sentry.getCurrentScope().setExtras(context);

  if (context.merchantId) {
    Sentry.getCurrentScope().setTag('merchantId', String(context.merchantId));
  }
  if (context.correlationId) {
    Sentry.getCurrentScope().setTag('correlationId', String(context.correlationId));
  }
}

/**
 * Capture an exception in Sentry with optional extra context.
 */
export function captureException(
  error: Error | unknown,
  context?: Record<string, string | number | boolean | undefined>
): void {
  if (!initialized) return;

  if (context) {
    Sentry.withScope((scope) => {
      scope.setExtras(context);
      if (context.merchantId) {
        scope.setTag('merchantId', String(context.merchantId));
      }
      if (context.correlationId) {
        scope.setTag('correlationId', String(context.correlationId));
      }
      Sentry.captureException(error);
    });
  } else {
    Sentry.captureException(error);
  }
}

/**
 * Register Sentry error handler as a Fastify plugin.
 * Captures unhandled route errors and enriches with request context.
 */
export function registerSentryErrorHandler(app: FastifyInstance): void {
  if (!initialized) return;

  app.addHook('onRequest', async (request: FastifyRequest) => {
    // Set request context for Sentry scope
    const scope = Sentry.getCurrentScope();
    scope.setTag('requestId', request.id);

    // Extract merchantId from headers or params if available
    const apiKey = request.headers['x-api-key'];
    if (apiKey) {
      scope.setTag('apiKey', '[present]');
    }

    // Extract correlationId from headers if propagated
    const correlationId = request.headers['x-correlation-id'];
    if (correlationId && typeof correlationId === 'string') {
      scope.setTag('correlationId', correlationId);
    }
  });

  app.addHook('onError', async (request: FastifyRequest, _reply: FastifyReply, error: Error) => {
    Sentry.withScope((scope) => {
      scope.setTag('requestId', request.id);
      scope.setExtra('method', request.method);
      scope.setExtra('url', request.url);
      scope.setExtra('userAgent', request.headers['user-agent']);

      const correlationId = request.headers['x-correlation-id'];
      if (correlationId && typeof correlationId === 'string') {
        scope.setTag('correlationId', correlationId);
      }

      Sentry.captureException(error);
    });
  });
}

/**
 * Start a Sentry span for tracing key operations.
 * Returns the result of the callback. No-op wrapper if Sentry is not initialized.
 *
 * Usage:
 *   const result = await sentryTrace('checkout.subscribe', { merchantId }, async () => {
 *     return doWork();
 *   });
 */
export async function sentryTrace<T>(
  name: string,
  attributes: Record<string, string | number | boolean | undefined>,
  callback: () => Promise<T>
): Promise<T> {
  if (!initialized) {
    return callback();
  }

  return Sentry.startSpan(
    {
      name,
      op: name,
      attributes: Object.fromEntries(
        Object.entries(attributes).filter(([, v]) => v !== undefined)
      ) as Record<string, string | number | boolean>,
    },
    async () => callback()
  );
}

/**
 * Flush Sentry events before shutdown.
 */
export async function flushSentry(timeout = 2000): Promise<void> {
  if (!initialized) return;
  await Sentry.flush(timeout);
}

// Re-export Sentry for advanced usage
export { Sentry };
