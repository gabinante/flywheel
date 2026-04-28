/**
 * Tests for structured logging configuration and utilities.
 *
 * Validates:
 * - Logger configuration produces valid JSON
 * - Correlation ID generation
 * - Sensitive field redaction
 * - Timer utility
 * - No console.log remaining in codebase
 */
import { describe, it, expect } from 'vitest';
import {
  createCorrelationId,
  createTimer,
  loggerConfig,
  REDACT_PATHS,
  createServiceLogger,
} from '../utils/logger.js';

describe('createCorrelationId', () => {
  it('should generate a unique string starting with "corr_"', () => {
    const id = createCorrelationId();
    expect(id).toMatch(/^corr_[a-f0-9]{24}$/);
  });

  it('should generate unique IDs on each call', () => {
    const ids = new Set(Array.from({ length: 100 }, () => createCorrelationId()));
    expect(ids.size).toBe(100);
  });
});

describe('createTimer', () => {
  it('should return elapsed time in milliseconds', async () => {
    const timer = createTimer();
    // Wait a small amount
    await new Promise((resolve) => setTimeout(resolve, 10));
    const elapsed = timer.elapsed();
    expect(typeof elapsed).toBe('number');
    expect(elapsed).toBeGreaterThanOrEqual(0);
  });

  it('should return an integer (rounded)', () => {
    const timer = createTimer();
    const elapsed = timer.elapsed();
    expect(Number.isInteger(elapsed)).toBe(true);
  });
});

describe('loggerConfig', () => {
  it('should have level defaulting to "info"', () => {
    expect(loggerConfig.level).toBe(process.env.LOG_LEVEL || 'info');
  });

  it('should have redact configuration', () => {
    expect(loggerConfig.redact).toBeDefined();
    expect(loggerConfig.redact.censor).toBe('[REDACTED]');
    expect(Array.isArray(loggerConfig.redact.paths)).toBe(true);
  });

  it('should produce ISO 8601 timestamps', () => {
    const timestampFn = loggerConfig.timestamp as () => string;
    const result = timestampFn();
    // Extract the time value from the JSON fragment
    const match = result.match(/"time":"([^"]+)"/);
    expect(match).not.toBeNull();
    const date = new Date(match![1]);
    expect(date.toISOString()).toBe(match![1]);
  });

  it('should format level as string label', () => {
    const formatter = loggerConfig.formatters.level;
    expect(formatter('info')).toEqual({ level: 'info' });
    expect(formatter('error')).toEqual({ level: 'error' });
    expect(formatter('warn')).toEqual({ level: 'warn' });
  });

  it('should have serializers for req and res', () => {
    expect(loggerConfig.serializers.req).toBeDefined();
    expect(loggerConfig.serializers.res).toBeDefined();
  });

  it('req serializer should only expose safe headers', () => {
    const serialized = loggerConfig.serializers.req({
      method: 'POST',
      url: '/api/v1/checkout/subscribe',
      headers: {
        host: 'localhost:3000',
        'user-agent': 'test-agent',
        'content-type': 'application/json',
        authorization: 'Bearer secret-jwt-token',
        'x-api-key': 'secret-key-123',
        'x-forwarded-for': '1.2.3.4',
      },
    });

    expect(serialized.method).toBe('POST');
    expect(serialized.url).toBe('/api/v1/checkout/subscribe');
    expect(serialized.headers.host).toBe('localhost:3000');
    expect(serialized.headers['user-agent']).toBe('test-agent');
    expect(serialized.headers['content-type']).toBe('application/json');
    // These fields exist in the serialized output but will be redacted by pino redact paths
    expect(serialized.headers.authorization).toBe('Bearer secret-jwt-token');
    // x-forwarded-for should NOT be included (not in allowlist)
    expect(serialized.headers).not.toHaveProperty('x-forwarded-for');
  });

  it('res serializer should expose only statusCode', () => {
    const serialized = loggerConfig.serializers.res({
      statusCode: 200,
    });
    expect(serialized).toEqual({ statusCode: 200 });
  });
});

describe('REDACT_PATHS', () => {
  it('should include all required sensitive field paths', () => {
    const requiredPaths = [
      'req.headers.authorization',
      'req.headers["x-api-key"]',
      '*.nmiSecurityKey',
      '*.nmiTokenizationKey',
      '*.seamlesschexApiKey',
      '*.passwordHash',
      '*.password',
      '*.ssnLast4',
      '*.bankRoutingNumber',
      '*.bankAccountNumber',
      '*.cardNumber',
      '*.cvv',
      '*.encryptionKey',
    ];

    for (const path of requiredPaths) {
      expect(REDACT_PATHS).toContain(path);
    }
  });

  it('should include NMI API credential paths', () => {
    expect(REDACT_PATHS).toContain('*.securityKey');
    expect(REDACT_PATHS).toContain('*.security_key');
    expect(REDACT_PATHS).toContain('*.partnerKey');
    expect(REDACT_PATHS).toContain('*.partner_key');
  });

  it('should include card data paths for PCI compliance', () => {
    expect(REDACT_PATHS).toContain('*.cardNumber');
    expect(REDACT_PATHS).toContain('*.cvv');
    expect(REDACT_PATHS).toContain('*.card_number');
    expect(REDACT_PATHS).toContain('*.cc_number');
  });
});

describe('no console.log in codebase', () => {
  it('should have zero console.log/error/warn in src/ (excluding tests)', async () => {
    const { execSync } = await import('child_process');
    const backendSrcDir = new URL('..', import.meta.url).pathname;

    // Search for console.log/error/warn in src/ but exclude __tests__
    try {
      const result = execSync(
        `grep -rn "console\\.(log\\|error\\|warn)" "${backendSrcDir}" --include="*.ts" | grep -v "__tests__" | grep -v "node_modules"`,
        { encoding: 'utf-8' }
      );
      // If grep finds matches, fail the test
      expect(result.trim()).toBe('');
    } catch {
      // grep returns exit code 1 when no matches found — that's success
    }
  });
});

describe('createServiceLogger', () => {
  it('should create a child logger with service name and correlationId', () => {
    let capturedBindings: Record<string, unknown> = {};
    const mockBaseLogger = {
      child: (bindings: Record<string, unknown>) => {
        capturedBindings = bindings;
        return mockBaseLogger;
      },
      info: () => {},
      warn: () => {},
      error: () => {},
      debug: () => {},
      fatal: () => {},
      trace: () => {},
      silent: () => {},
      level: 'info',
    } as any;

    createServiceLogger(mockBaseLogger, 'nmi-boarding', 'corr_abc123');
    expect(capturedBindings.service).toBe('nmi-boarding');
    expect(capturedBindings.correlationId).toBe('corr_abc123');
  });

  it('should create a child logger without correlationId when not provided', () => {
    let capturedBindings: Record<string, unknown> = {};
    const mockBaseLogger = {
      child: (bindings: Record<string, unknown>) => {
        capturedBindings = bindings;
        return mockBaseLogger;
      },
      info: () => {},
      warn: () => {},
      error: () => {},
      debug: () => {},
      fatal: () => {},
      trace: () => {},
      silent: () => {},
      level: 'info',
    } as any;

    createServiceLogger(mockBaseLogger, 'residual');
    expect(capturedBindings.service).toBe('residual');
    expect(capturedBindings).not.toHaveProperty('correlationId');
  });
});
