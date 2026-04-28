/**
 * Sentry Initialization for React Frontends
 *
 * Common Sentry setup used by all frontend packages.
 *
 * Configuration:
 * - VITE_SENTRY_DSN: Sentry Data Source Name (required to enable)
 * - VITE_SENTRY_ENVIRONMENT: Environment (dev/staging/prod)
 * - VITE_SENTRY_RELEASE: Release version
 *
 * Usage:
 *   import { initSentryFrontend } from '@gohighpayment/shared/sentry-init';
 *   initSentryFrontend({ projectName: 'admin-dashboard' });
 */

import * as Sentry from '@sentry/react';

export interface SentryFrontendConfig {
  /** Name of the frontend project (e.g., 'admin-dashboard', 'checkout') */
  projectName: string;
  /** Override DSN (defaults to VITE_SENTRY_DSN env var) */
  dsn?: string;
  /** Override environment (defaults to VITE_SENTRY_ENVIRONMENT) */
  environment?: string;
  /** Override release (defaults to VITE_SENTRY_RELEASE) */
  release?: string;
  /** Traces sample rate (0.0-1.0, default: 0.1) */
  tracesSampleRate?: number;
  /** Replay session sample rate (0.0-1.0, default: 0.1) */
  replaysSessionSampleRate?: number;
  /** Replay error sample rate (0.0-1.0, default: 1.0) */
  replaysOnErrorSampleRate?: number;
}

let initialized = false;

/**
 * Initialize Sentry for a frontend application.
 * Safe to call multiple times — only initializes once.
 * No-op if no DSN is configured.
 */
export function initSentryFrontend(config: SentryFrontendConfig): void {
  if (initialized) return;

  // Access Vite env vars via import.meta.env
  const dsn = config.dsn ?? (typeof import.meta !== 'undefined' ? (import.meta as any).env?.VITE_SENTRY_DSN : undefined);
  if (!dsn) {
    return; // Sentry disabled when no DSN configured
  }

  const environment = config.environment
    ?? (typeof import.meta !== 'undefined' ? (import.meta as any).env?.VITE_SENTRY_ENVIRONMENT : undefined)
    ?? 'development';

  const release = config.release
    ?? (typeof import.meta !== 'undefined' ? (import.meta as any).env?.VITE_SENTRY_RELEASE : undefined)
    ?? `gohighpayment-${config.projectName}@0.1.0`;

  Sentry.init({
    dsn,
    environment,
    release,
    tracesSampleRate: config.tracesSampleRate ?? 0.1,
    replaysSessionSampleRate: config.replaysSessionSampleRate ?? 0.1,
    replaysOnErrorSampleRate: config.replaysOnErrorSampleRate ?? 1.0,

    // Tag all events with the project name
    initialScope: {
      tags: {
        project: config.projectName,
      },
    },
  });

  initialized = true;
}

/**
 * Check if Sentry frontend is initialized.
 */
export function isSentryFrontendEnabled(): boolean {
  return initialized;
}

// Re-export Sentry for advanced usage
export { Sentry };
