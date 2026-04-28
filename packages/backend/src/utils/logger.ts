/**
 * Structured Logger Configuration
 *
 * Provides pino-based structured logging with:
 * - JSON output (machine-parseable for Datadog, CloudWatch)
 * - Request ID tracking (via Fastify)
 * - Correlation ID propagation for transaction flows
 * - Sensitive field redaction (PCI compliance)
 * - Configurable log level via LOG_LEVEL env var
 *
 * Usage:
 *   import { loggerConfig, createCorrelationId } from './utils/logger.js';
 *   const app = Fastify({ logger: loggerConfig });
 */

import { randomUUID } from 'crypto';
import type { FastifyBaseLogger } from 'fastify';

// ─── Correlation ID ────────────���──────────────────────────────────────

/**
 * Generate a unique correlation ID for tracing a transaction flow
 * across checkout, payment processing, webhooks, and post-payment pipeline.
 */
export function createCorrelationId(): string {
  return `corr_${randomUUID().replace(/-/g, '').slice(0, 24)}`;
}

// ─── Redaction paths ───────────���──────────────────────────────────────

/**
 * Pino redact configuration.
 * These paths will be replaced with '[REDACTED]' in all log output.
 *
 * Covers:
 * - HTTP auth headers (Authorization, x-api-key)
 * - NMI credentials (security key, tokenization key)
 * - Seamlesschex API key
 * - Password hashes and plaintext passwords
 * - PII (SSN, bank details)
 * - Card data (PAN, CVV)
 * - Encryption keys
 */
/**
 * Sensitive field names that must be redacted from all log output.
 *
 * Pino's fast-redact wildcard `*` matches nested object keys but NOT
 * root-level properties. To cover both cases, we generate paths for:
 *   - Root level: `fieldName`
 *   - One-level nested: `*.fieldName`
 */
const SENSITIVE_FIELDS = [
  // NMI credentials
  'nmiSecurityKey',
  'nmiTokenizationKey',
  'securityKey',
  'security_key',

  // Seamlesschex credentials
  'seamlesschexApiKey',

  // Passwords
  'passwordHash',
  'password',

  // PII
  'ssnLast4',
  'ownerSsn',

  // Bank info
  'bankRoutingNumber',
  'bankAccountNumber',
  'bank_routing_number',
  'bank_account_number',

  // Card data (PCI)
  'cardNumber',
  'cvv',
  'card_number',
  'cc_number',

  // Encryption / API keys
  'encryptionKey',
  'apiKey',
  'partnerKey',
  'partner_key',
];

export const REDACT_PATHS = [
  // HTTP headers (always nested under req.headers)
  'req.headers.authorization',
  'req.headers["x-api-key"]',

  // Generate root-level and wildcard paths for all sensitive fields
  ...SENSITIVE_FIELDS.flatMap((field) => [field, `*.${field}`]),
];

// ─��─ Pino configuration ──��───────────────────────────────────────────

/**
 * Pino logger options for Fastify.
 *
 * Pass this to `Fastify({ logger: loggerConfig })` for structured JSON logging.
 */
export const loggerConfig = {
  level: process.env.LOG_LEVEL || 'info',
  timestamp: () => `,"time":"${new Date().toISOString()}"`,
  formatters: {
    level(label: string) {
      return { level: label };
    },
  },
  redact: {
    paths: REDACT_PATHS,
    censor: '[REDACTED]',
  },
  serializers: {
    req(request: { method: string; url: string; headers: Record<string, string> }) {
      return {
        method: request.method,
        url: request.url,
        headers: {
          host: request.headers?.host,
          'user-agent': request.headers?.['user-agent'],
          'content-type': request.headers?.['content-type'],
          // Authorization header is redacted via pino redact paths
          authorization: request.headers?.authorization,
          'x-api-key': request.headers?.['x-api-key'],
        },
      };
    },
    res(response: { statusCode: number }) {
      return {
        statusCode: response.statusCode,
      };
    },
  },
};

// ─── Timer helper ──────────────────────────────────��──────────────────

/**
 * Create a timer for measuring operation duration.
 *
 * Usage:
 *   const timer = createTimer();
 *   await someOperation();
 *   log.info({ duration_ms: timer.elapsed() }, 'Operation complete');
 */
export function createTimer(): { elapsed: () => number } {
  const start = performance.now();
  return {
    elapsed: () => Math.round(performance.now() - start),
  };
}

// ─── Service logger factory ─────────────���─────────────────────────────

/**
 * Create a child logger for a service that doesn't have access to a
 * Fastify request context (background jobs, scheduled tasks, etc.).
 *
 * The returned logger includes the service name and optional correlation ID.
 */
export function createServiceLogger(
  baseLogger: FastifyBaseLogger,
  service: string,
  correlationId?: string
): FastifyBaseLogger {
  const bindings: Record<string, string> = { service };
  if (correlationId) {
    bindings.correlationId = correlationId;
  }
  return baseLogger.child(bindings);
}

export type { FastifyBaseLogger as Logger };
