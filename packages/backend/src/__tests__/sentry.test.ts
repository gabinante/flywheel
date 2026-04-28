/**
 * Sentry Integration Tests
 *
 * Tests the Sentry utility module for the backend.
 * Uses mocked @sentry/node to verify initialization and error capture.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock @sentry/node before importing sentry utils
vi.mock('@sentry/node', () => {
  const scope = {
    setExtras: vi.fn(),
    setTag: vi.fn(),
    setExtra: vi.fn(),
  };

  return {
    init: vi.fn(),
    captureException: vi.fn(),
    flush: vi.fn().mockResolvedValue(true),
    getCurrentScope: vi.fn().mockReturnValue(scope),
    withScope: vi.fn((callback: (s: typeof scope) => void) => callback(scope)),
  };
});

describe('Sentry Utils', () => {
  // Re-import for each test to reset the initialized state
  let sentryUtils: typeof import('../utils/sentry.js');
  let SentryMock: typeof import('@sentry/node');

  beforeEach(async () => {
    vi.resetModules();
    vi.clearAllMocks();

    // Re-mock after reset
    vi.mock('@sentry/node', () => {
      const scope = {
        setExtras: vi.fn(),
        setTag: vi.fn(),
        setExtra: vi.fn(),
      };

      return {
        init: vi.fn(),
        captureException: vi.fn(),
        flush: vi.fn().mockResolvedValue(true),
        getCurrentScope: vi.fn().mockReturnValue(scope),
        withScope: vi.fn((callback: (s: typeof scope) => void) => callback(scope)),
      };
    });

    sentryUtils = await import('../utils/sentry.js');
    SentryMock = await import('@sentry/node');
  });

  describe('initSentry', () => {
    it('does not initialize when no DSN is provided', () => {
      delete process.env.SENTRY_DSN;
      sentryUtils.initSentry();
      expect(SentryMock.init).not.toHaveBeenCalled();
      expect(sentryUtils.isSentryEnabled()).toBe(false);
    });

    it('initializes with DSN from config', () => {
      sentryUtils.initSentry({ dsn: 'https://test@sentry.io/123' });
      expect(SentryMock.init).toHaveBeenCalledWith(
        expect.objectContaining({
          dsn: 'https://test@sentry.io/123',
          environment: 'development',
        })
      );
      expect(sentryUtils.isSentryEnabled()).toBe(true);
    });

    it('initializes with DSN from env', () => {
      process.env.SENTRY_DSN = 'https://env@sentry.io/456';
      sentryUtils.initSentry();
      expect(SentryMock.init).toHaveBeenCalledWith(
        expect.objectContaining({
          dsn: 'https://env@sentry.io/456',
        })
      );
      delete process.env.SENTRY_DSN;
    });

    it('uses custom environment and release', () => {
      sentryUtils.initSentry({
        dsn: 'https://test@sentry.io/123',
        environment: 'staging',
        release: 'v1.2.3',
      });
      expect(SentryMock.init).toHaveBeenCalledWith(
        expect.objectContaining({
          environment: 'staging',
          release: 'v1.2.3',
        })
      );
    });

    it('only initializes once', () => {
      sentryUtils.initSentry({ dsn: 'https://test@sentry.io/123' });
      sentryUtils.initSentry({ dsn: 'https://test@sentry.io/456' });
      expect(SentryMock.init).toHaveBeenCalledTimes(1);
    });
  });

  describe('captureException', () => {
    it('does nothing when Sentry is not initialized', () => {
      sentryUtils.captureException(new Error('test'));
      expect(SentryMock.captureException).not.toHaveBeenCalled();
    });

    it('captures exception when initialized', () => {
      sentryUtils.initSentry({ dsn: 'https://test@sentry.io/123' });
      const error = new Error('test error');
      sentryUtils.captureException(error);
      expect(SentryMock.captureException).toHaveBeenCalledWith(error);
    });

    it('captures exception with context', () => {
      sentryUtils.initSentry({ dsn: 'https://test@sentry.io/123' });
      const error = new Error('test error');
      sentryUtils.captureException(error, {
        merchantId: 'merchant_123',
        correlationId: 'corr_abc',
      });
      expect(SentryMock.withScope).toHaveBeenCalled();
    });
  });

  describe('setSentryContext', () => {
    it('does nothing when Sentry is not initialized', () => {
      sentryUtils.setSentryContext({ merchantId: 'test' });
      expect(SentryMock.getCurrentScope).not.toHaveBeenCalled();
    });

    it('sets context when initialized', () => {
      sentryUtils.initSentry({ dsn: 'https://test@sentry.io/123' });
      sentryUtils.setSentryContext({ merchantId: 'merchant_123', correlationId: 'corr_abc' });

      const scope = SentryMock.getCurrentScope();
      expect(scope.setExtras).toHaveBeenCalledWith({
        merchantId: 'merchant_123',
        correlationId: 'corr_abc',
      });
      expect(scope.setTag).toHaveBeenCalledWith('merchantId', 'merchant_123');
      expect(scope.setTag).toHaveBeenCalledWith('correlationId', 'corr_abc');
    });
  });

  describe('flushSentry', () => {
    it('does nothing when not initialized', async () => {
      await sentryUtils.flushSentry();
      expect(SentryMock.flush).not.toHaveBeenCalled();
    });

    it('flushes when initialized', async () => {
      sentryUtils.initSentry({ dsn: 'https://test@sentry.io/123' });
      await sentryUtils.flushSentry(3000);
      expect(SentryMock.flush).toHaveBeenCalledWith(3000);
    });
  });
});
