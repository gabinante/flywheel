/**
 * Integration tests for structured logging output.
 *
 * Validates that pino with our configuration produces:
 * - Valid JSON log lines
 * - Correct field names (level, time, msg, reqId)
 * - Proper redaction of sensitive fields
 * - correlationId propagation
 */
import { describe, it, expect } from 'vitest';
import { Writable } from 'stream';
import pino from 'pino';
import { loggerConfig, REDACT_PATHS } from '../utils/logger.js';

/**
 * Create a pino logger that writes to a buffer for assertion.
 */
function createTestLogger(): { logger: pino.Logger; getLines: () => any[] } {
  const lines: string[] = [];
  const stream = new Writable({
    write(chunk, _encoding, callback) {
      lines.push(chunk.toString().trim());
      callback();
    },
  });

  const logger = pino(
    {
      ...loggerConfig,
      // Force sync for testing
    },
    stream
  );

  return {
    logger,
    getLines: () => lines.map((line) => JSON.parse(line)),
  };
}

describe('Structured JSON output', () => {
  it('should produce valid JSON for every log entry', () => {
    const { logger, getLines } = createTestLogger();

    logger.info('simple message');
    logger.warn({ action: 'test_action' }, 'warning message');
    logger.error({ err: new Error('test') }, 'error message');

    const entries = getLines();
    expect(entries.length).toBe(3);

    for (const entry of entries) {
      // Each entry should be a valid object (parsed from JSON)
      expect(typeof entry).toBe('object');
      expect(entry).not.toBeNull();
    }
  });

  it('should include level as string label', () => {
    const { logger, getLines } = createTestLogger();

    logger.info('info message');
    logger.warn('warn message');
    logger.error('error message');

    const entries = getLines();
    expect(entries[0].level).toBe('info');
    expect(entries[1].level).toBe('warn');
    expect(entries[2].level).toBe('error');
  });

  it('should include ISO 8601 timestamp', () => {
    const { logger, getLines } = createTestLogger();

    logger.info('timestamped');

    const entries = getLines();
    expect(entries[0].time).toBeDefined();
    // Verify it's a valid ISO 8601 date
    const date = new Date(entries[0].time);
    expect(date.toISOString()).toBe(entries[0].time);
  });

  it('should include msg field', () => {
    const { logger, getLines } = createTestLogger();

    logger.info('hello world');

    const entries = getLines();
    expect(entries[0].msg).toBe('hello world');
  });

  it('should include structured fields in log entries', () => {
    const { logger, getLines } = createTestLogger();

    logger.info(
      {
        merchantId: 'merchant_123',
        action: 'charge_card',
        amount: 5000,
        correlationId: 'corr_abc123',
      },
      'NMI charge initiated'
    );

    const entries = getLines();
    const entry = entries[0];
    expect(entry.merchantId).toBe('merchant_123');
    expect(entry.action).toBe('charge_card');
    expect(entry.amount).toBe(5000);
    expect(entry.correlationId).toBe('corr_abc123');
    expect(entry.msg).toBe('NMI charge initiated');
  });

  it('should include duration_ms for timed operations', () => {
    const { logger, getLines } = createTestLogger();

    logger.info(
      {
        action: 'nmi_sale_completed',
        duration_ms: 234,
        correlationId: 'corr_test',
      },
      'NMI sale completed'
    );

    const entries = getLines();
    expect(entries[0].duration_ms).toBe(234);
  });
});

describe('Sensitive field redaction', () => {
  it('should redact nmiSecurityKey', () => {
    const { logger, getLines } = createTestLogger();

    logger.info({ nmiSecurityKey: 'super-secret-key-12345' }, 'merchant data');

    const entries = getLines();
    expect(entries[0].nmiSecurityKey).toBe('[REDACTED]');
  });

  it('should redact nmiTokenizationKey', () => {
    const { logger, getLines } = createTestLogger();

    logger.info({ nmiTokenizationKey: 'tok-key-abc' }, 'merchant data');

    const entries = getLines();
    expect(entries[0].nmiTokenizationKey).toBe('[REDACTED]');
  });

  it('should redact seamlesschexApiKey', () => {
    const { logger, getLines } = createTestLogger();

    logger.info({ seamlesschexApiKey: 'chex-secret' }, 'merchant data');

    const entries = getLines();
    expect(entries[0].seamlesschexApiKey).toBe('[REDACTED]');
  });

  it('should redact passwordHash', () => {
    const { logger, getLines } = createTestLogger();

    logger.info({ passwordHash: '$2b$12$abc...' }, 'user data');

    const entries = getLines();
    expect(entries[0].passwordHash).toBe('[REDACTED]');
  });

  it('should redact password', () => {
    const { logger, getLines } = createTestLogger();

    logger.info({ password: 'plaintext-password' }, 'login attempt');

    const entries = getLines();
    expect(entries[0].password).toBe('[REDACTED]');
  });

  it('should redact ssnLast4', () => {
    const { logger, getLines } = createTestLogger();

    logger.info({ ssnLast4: '1234' }, 'application data');

    const entries = getLines();
    expect(entries[0].ssnLast4).toBe('[REDACTED]');
  });

  it('should redact bankRoutingNumber', () => {
    const { logger, getLines } = createTestLogger();

    logger.info({ bankRoutingNumber: '021000021' }, 'bank data');

    const entries = getLines();
    expect(entries[0].bankRoutingNumber).toBe('[REDACTED]');
  });

  it('should redact bankAccountNumber', () => {
    const { logger, getLines } = createTestLogger();

    logger.info({ bankAccountNumber: '123456789012' }, 'bank data');

    const entries = getLines();
    expect(entries[0].bankAccountNumber).toBe('[REDACTED]');
  });

  it('should redact cardNumber (PCI)', () => {
    const { logger, getLines } = createTestLogger();

    logger.info({ cardNumber: '4111111111111111' }, 'card data');

    const entries = getLines();
    expect(entries[0].cardNumber).toBe('[REDACTED]');
  });

  it('should redact cvv (PCI)', () => {
    const { logger, getLines } = createTestLogger();

    logger.info({ cvv: '123' }, 'card data');

    const entries = getLines();
    expect(entries[0].cvv).toBe('[REDACTED]');
  });

  it('should redact encryptionKey', () => {
    const { logger, getLines } = createTestLogger();

    logger.info({ encryptionKey: 'aes-256-key-value' }, 'config data');

    const entries = getLines();
    expect(entries[0].encryptionKey).toBe('[REDACTED]');
  });

  it('should redact apiKey', () => {
    const { logger, getLines } = createTestLogger();

    logger.info({ apiKey: 'api-key-secret' }, 'api data');

    const entries = getLines();
    expect(entries[0].apiKey).toBe('[REDACTED]');
  });

  it('should redact nested sensitive fields', () => {
    const { logger, getLines } = createTestLogger();

    logger.info(
      {
        merchant: {
          id: 'merch_123',
          nmiSecurityKey: 'should-be-hidden',
          name: 'Test Merchant',
        },
      },
      'merchant record'
    );

    const entries = getLines();
    // The wildcard path *.nmiSecurityKey should catch nested fields
    expect(entries[0].merchant.nmiSecurityKey).toBe('[REDACTED]');
    expect(entries[0].merchant.name).toBe('Test Merchant');
    expect(entries[0].merchant.id).toBe('merch_123');
  });

  it('should NOT redact non-sensitive fields', () => {
    const { logger, getLines } = createTestLogger();

    logger.info(
      {
        merchantId: 'merch_123',
        action: 'checkout',
        amount: 5000,
        correlationId: 'corr_test',
      },
      'checkout data'
    );

    const entries = getLines();
    expect(entries[0].merchantId).toBe('merch_123');
    expect(entries[0].action).toBe('checkout');
    expect(entries[0].amount).toBe(5000);
    expect(entries[0].correlationId).toBe('corr_test');
  });
});

describe('Child logger propagation', () => {
  it('should propagate service name via child logger', () => {
    const { logger, getLines } = createTestLogger();

    const childLogger = logger.child({ service: 'nmi-service' });
    childLogger.info({ action: 'sale' }, 'NMI sale');

    const entries = getLines();
    expect(entries[0].service).toBe('nmi-service');
    expect(entries[0].action).toBe('sale');
  });

  it('should propagate correlationId via child logger', () => {
    const { logger, getLines } = createTestLogger();

    const childLogger = logger.child({
      service: 'checkout',
      correlationId: 'corr_propagated',
    });
    childLogger.info({ action: 'step1' }, 'first step');
    childLogger.info({ action: 'step2' }, 'second step');

    const entries = getLines();
    expect(entries[0].correlationId).toBe('corr_propagated');
    expect(entries[1].correlationId).toBe('corr_propagated');
  });
});

describe('LOG_LEVEL configurability', () => {
  it('should respect LOG_LEVEL from environment (if set)', () => {
    // The loggerConfig.level reads from process.env.LOG_LEVEL || 'info'
    expect(loggerConfig.level).toBe(process.env.LOG_LEVEL || 'info');
  });

  it('should suppress debug logs when level is info', () => {
    const { logger, getLines } = createTestLogger();

    logger.debug('debug message');
    logger.info('info message');

    const entries = getLines();
    // Only info should appear (debug is below info level)
    expect(entries.length).toBe(1);
    expect(entries[0].level).toBe('info');
  });
});
