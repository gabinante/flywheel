/**
 * Sentry Error Boundary for React Frontends
 *
 * Wraps the application to catch React rendering errors
 * and report them to Sentry with component stack traces.
 *
 * Usage:
 *   import { SentryErrorBoundary } from '@gohighpayment/shared/sentry-error-boundary';
 *
 *   <SentryErrorBoundary>
 *     <App />
 *   </SentryErrorBoundary>
 */

import React from 'react';
import * as Sentry from '@sentry/react';

interface FallbackProps {
  error: Error;
  resetError: () => void;
}

/**
 * Default fallback UI shown when a React error boundary catches an error.
 */
function DefaultFallback({ error, resetError }: FallbackProps) {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        minHeight: '100vh',
        backgroundColor: '#0a0a0a',
        color: '#e0e0e0',
        fontFamily: 'system-ui, -apple-system, sans-serif',
        padding: '2rem',
      }}
    >
      <div
        style={{
          maxWidth: '480px',
          textAlign: 'center',
          padding: '2rem',
          borderRadius: '12px',
          backgroundColor: 'rgba(255, 255, 255, 0.05)',
          border: '1px solid rgba(255, 255, 255, 0.1)',
        }}
      >
        <h1 style={{ fontSize: '1.5rem', marginBottom: '1rem', color: '#ff6b6b' }}>
          Something went wrong
        </h1>
        <p style={{ marginBottom: '1rem', color: '#999', fontSize: '0.9rem' }}>
          An unexpected error occurred. The error has been reported and we&apos;re looking into it.
        </p>
        {process.env.NODE_ENV === 'development' && (
          <pre
            style={{
              textAlign: 'left',
              padding: '1rem',
              backgroundColor: 'rgba(0, 0, 0, 0.3)',
              borderRadius: '8px',
              fontSize: '0.8rem',
              overflow: 'auto',
              maxHeight: '200px',
              marginBottom: '1rem',
            }}
          >
            {error.message}
          </pre>
        )}
        <button
          onClick={resetError}
          style={{
            padding: '0.75rem 1.5rem',
            backgroundColor: '#3b82f6',
            color: 'white',
            border: 'none',
            borderRadius: '8px',
            cursor: 'pointer',
            fontSize: '0.9rem',
          }}
        >
          Try again
        </button>
      </div>
    </div>
  );
}

interface SentryErrorBoundaryProps {
  children: React.ReactNode;
  fallback?: React.ComponentType<FallbackProps>;
}

/**
 * Error boundary that captures React errors and sends them to Sentry.
 * Falls back to a user-friendly error page.
 */
export function SentryErrorBoundary({
  children,
  fallback: FallbackComponent = DefaultFallback,
}: SentryErrorBoundaryProps) {
  return (
    <Sentry.ErrorBoundary
      fallback={({ error, resetError }) => (
        <FallbackComponent error={error as Error} resetError={resetError} />
      )}
    >
      {children}
    </Sentry.ErrorBoundary>
  );
}

export default SentryErrorBoundary;
