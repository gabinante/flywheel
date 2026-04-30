/**
 * Tests for the GHL queryUrl payment processing endpoint.
 *
 * POST /api/v1/ghl/query
 *
 * Tests cover:
 * - HMAC signature verification (missing, invalid, valid)
 * - Merchant not found for locationId
 * - Merchant without NMI credentials
 * - Charge: success, NMI decline
 * - Refund: success, original transaction not found
 * - Unknown payment type
 */

import { jest } from '@jest/globals';
import crypto from 'crypto';

// ---------------------------------------------------------------------------
// Shared mock state — updated per-test in beforeEach
// ---------------------------------------------------------------------------

const envState: Record<string, string> = {
  GHL_CLIENT_SECRET: 'test-ghl-secret',
  GHL_CLIENT_ID: 'test-client-id',
  GHL_REDIRECT_URI: 'http://localhost:3000/api/v1/ghl/callback',
  GHL_APP_ID: '',
  GHL_SSO_KEY: '',
  MERCHANT_URL: 'http://localhost:5175',
  ENCRYPTION_KEY: 'a'.repeat(64),
  JWT_SECRET: 'test-jwt-secret',
  JWT_REFRESH_SECRET: 'test-jwt-refresh-secret',
  JWT_EXPIRES_IN: '15m',
  JWT_REFRESH_EXPIRES_IN: '7d',
  NMI_MOCK_MODE: 'false',
};

// Mock merchant returned by prisma
const mockMerchant = {
  id: 'merchant-123',
  businessName: 'Test Merchant',
  contactEmail: 'test@merchant.com',
  status: 'ACTIVE',
  ghlLocationId: 'loc-456',
  nmiSecurityKey: 'nmi-security-key-test',
  nmiTokenizationKey: null,
};

// Mock Prisma
const mockPrisma = {
  merchant: {
    findUnique: jest.fn<() => Promise<typeof mockMerchant | null>>(),
  },
  transaction: {
    create: jest.fn<() => Promise<{ id: string; amountCents: number; nmiTransactionId: string | null }>>(),
    update: jest.fn<() => Promise<{ id: string }>>(),
    findFirst: jest.fn<() => Promise<{ id: string; nmiTransactionId: string | null; amountCents: number } | null>>(),
  },
  auditLog: {
    create: jest.fn<() => Promise<{ id: string }>>(),
  },
  ghlOAuthState: {
    findUnique: jest.fn(),
    create: jest.fn(),
    delete: jest.fn(),
  },
  application: {
    create: jest.fn(),
  },
};

// Mock NMI service
const mockNmi = {
  chargeCard: jest.fn<() => Promise<{
    success: boolean;
    transactionId: string;
    responseCode: string;
    responseText: string;
    authCode?: string;
    cardBrand?: string;
    cardLast4?: string;
  }>>(),
  refundTransaction: jest.fn<() => Promise<{
    success: boolean;
    transactionId: string;
    responseCode: string;
    responseText: string;
  }>>(),
  voidTransaction: jest.fn(),
};

// ---------------------------------------------------------------------------
// Module-level variables for dynamically imported items
// ---------------------------------------------------------------------------

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let ghlRoutes: any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let Fastify: any;

// ---------------------------------------------------------------------------
// Setup — register mocks then dynamically import
// ---------------------------------------------------------------------------

beforeAll(async () => {
  await (jest as any).unstable_mockModule('../../utils/env', () => ({ env: envState }));
  await (jest as any).unstable_mockModule('../../utils/prisma', () => ({ prisma: mockPrisma }));
  await (jest as any).unstable_mockModule('../../services/nmi.service', () => mockNmi);
  // Mock encryption utils so prisma module doesn't fail validateEncryptionKey
  await (jest as any).unstable_mockModule('../../utils/encryption', () => ({
    validateEncryptionKey: jest.fn(),
    encrypt: (v: string) => v,
    decrypt: (v: string) => v,
    isEncrypted: () => false,
  }));

  const ghlModule = await import('../../routes/ghl.js');
  ghlRoutes = ghlModule.ghlRoutes;

  const fastifyModule = await import('fastify');
  Fastify = fastifyModule.default;
});

// ---------------------------------------------------------------------------
// Per-test setup
// ---------------------------------------------------------------------------

let app: ReturnType<typeof Fastify>;

beforeEach(async () => {
  jest.clearAllMocks();

  // Reset env state
  envState.GHL_CLIENT_SECRET = 'test-ghl-secret';
  envState.NMI_MOCK_MODE = 'false';

  // Default mock responses
  (mockPrisma.merchant.findUnique as any).mockResolvedValue(mockMerchant);
  (mockPrisma.transaction.create as any).mockResolvedValue({
    id: 'txn-001',
    amountCents: 5000,
    nmiTransactionId: null,
  });
  (mockPrisma.transaction.update as any).mockResolvedValue({ id: 'txn-001' });
  (mockPrisma.transaction.findFirst as any).mockResolvedValue({
    id: 'txn-001',
    nmiTransactionId: 'nmi-txn-original',
    amountCents: 5000,
  });
  (mockPrisma.auditLog.create as any).mockResolvedValue({ id: 'log-1' });

  (mockNmi.chargeCard as any).mockResolvedValue({
    success: true,
    transactionId: 'nmi-charge-123',
    responseCode: '100',
    responseText: 'SUCCESS',
    authCode: 'AUTH123',
    cardBrand: 'Visa',
    cardLast4: '4242',
  });
  (mockNmi.refundTransaction as any).mockResolvedValue({
    success: true,
    transactionId: 'nmi-txn-original',
    responseCode: '100',
    responseText: 'SUCCESS',
  });

  // Fresh Fastify instance for each test
  app = Fastify({ logger: false });
  // Register minimal JWT stub so that routes that reference app.jwt don't fail
  // at registration time. The /query route itself doesn't use JWT.
  app.decorate('jwt', {
    sign: () => 'stub-token',
    verify: () => ({ id: 'stub' }),
  });
  await app.register(ghlRoutes, { prefix: '/api/v1/ghl' });
  await app.ready();
});

afterEach(async () => {
  await app.close();
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const TEST_SECRET = 'test-ghl-secret';

function buildGhlPayload(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    type: 'charge',
    amount: 5000,
    currency: 'USD',
    source: 'tok_test_4242',
    meta: {
      contactId: 'ghl_contact_1',
      locationId: 'loc-456',
      email: 'customer@example.com',
      name: 'John Doe',
      orderId: 'ghl_order_789',
    },
    ...overrides,
  };
}

function signPayload(body: unknown, secret: string = TEST_SECRET): string {
  return crypto.createHmac('sha256', secret).update(JSON.stringify(body)).digest('hex');
}

async function postQuery(
  body: unknown,
  headers: Record<string, string> = {},
): Promise<{ statusCode: number; json: () => unknown }> {
  const bodyStr = JSON.stringify(body);
  const sig = headers['x-ghl-signature'] !== undefined
    ? headers['x-ghl-signature']
    : signPayload(body);

  const response = await app.inject({
    method: 'POST',
    url: '/api/v1/ghl/query',
    headers: {
      'content-type': 'application/json',
      'x-ghl-signature': sig,
      ...headers,
    },
    body: bodyStr,
  });

  return {
    statusCode: response.statusCode,
    json: () => JSON.parse(response.body),
  };
}

// ---------------------------------------------------------------------------
// Signature verification
// ---------------------------------------------------------------------------

describe('GHL HMAC signature verification', () => {
  it('returns 401 when x-ghl-signature header is missing', async () => {
    const payload = buildGhlPayload();
    const res = await app.inject({
      method: 'POST',
      url: '/api/v1/ghl/query',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(payload),
    });
    expect(res.statusCode).toBe(401);
    const json = JSON.parse(res.body);
    expect(json.success).toBe(false);
    expect(json.message).toMatch(/missing/i);
  });

  it('returns 401 when signature is invalid', async () => {
    const payload = buildGhlPayload();
    const res = await app.inject({
      method: 'POST',
      url: '/api/v1/ghl/query',
      headers: {
        'content-type': 'application/json',
        'x-ghl-signature': 'deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef',
      },
      body: JSON.stringify(payload),
    });
    expect(res.statusCode).toBe(401);
    const json = JSON.parse(res.body);
    expect(json.success).toBe(false);
    expect(json.message).toMatch(/invalid/i);
  });

  it('accepts signatures with sha256= prefix', async () => {
    const payload = buildGhlPayload();
    const sig = `sha256=${signPayload(payload)}`;
    const res = await app.inject({
      method: 'POST',
      url: '/api/v1/ghl/query',
      headers: {
        'content-type': 'application/json',
        'x-ghl-signature': sig,
      },
      body: JSON.stringify(payload),
    });
    // Should not be 401 (might be 200 or other code, but not auth failure)
    expect(res.statusCode).not.toBe(401);
  });

  it('accepts x-hub-signature-256 header as alternative', async () => {
    const payload = buildGhlPayload();
    const sig = signPayload(payload);
    const res = await app.inject({
      method: 'POST',
      url: '/api/v1/ghl/query',
      headers: {
        'content-type': 'application/json',
        'x-hub-signature-256': sig,
      },
      body: JSON.stringify(payload),
    });
    expect(res.statusCode).not.toBe(401);
  });

  it('returns 401 when signature uses wrong secret', async () => {
    const payload = buildGhlPayload();
    const wrongSig = signPayload(payload, 'wrong-secret');
    const res = await app.inject({
      method: 'POST',
      url: '/api/v1/ghl/query',
      headers: {
        'content-type': 'application/json',
        'x-ghl-signature': wrongSig,
      },
      body: JSON.stringify(payload),
    });
    expect(res.statusCode).toBe(401);
    expect(JSON.parse(res.body).success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Merchant lookup
// ---------------------------------------------------------------------------

describe('merchant lookup', () => {
  it('returns success:false when merchant not found for locationId', async () => {
    (mockPrisma.merchant.findUnique as any).mockResolvedValue(null);
    const res = await postQuery(buildGhlPayload());
    expect(res.statusCode).toBe(200);
    const json = res.json() as any;
    expect(json.success).toBe(false);
    expect(json.message).toMatch(/merchant not configured/i);
  });

  it('logs an audit entry when merchant not found', async () => {
    (mockPrisma.merchant.findUnique as any).mockResolvedValue(null);
    await postQuery(buildGhlPayload());
    expect(mockPrisma.auditLog.create).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          action: 'ghl_query_merchant_not_found',
        }),
      }),
    );
  });

  it('returns success:false when merchant has no NMI credentials', async () => {
    (mockPrisma.merchant.findUnique as any).mockResolvedValue({
      ...mockMerchant,
      nmiSecurityKey: null,
    });
    const res = await postQuery(buildGhlPayload());
    expect(res.statusCode).toBe(200);
    const json = res.json() as any;
    expect(json.success).toBe(false);
    expect(json.message).toMatch(/not configured/i);
  });
});

// ---------------------------------------------------------------------------
// Charge
// ---------------------------------------------------------------------------

describe('charge', () => {
  it('returns success:true with transactionId on NMI approval', async () => {
    const res = await postQuery(buildGhlPayload());
    expect(res.statusCode).toBe(200);
    const json = res.json() as any;
    expect(json.success).toBe(true);
    expect(json.transactionId).toBe('txn-001');
    expect(json.message).toBe('Payment successful');
  });

  it('calls chargeCard with correct params', async () => {
    await postQuery(buildGhlPayload());
    expect(mockNmi.chargeCard).toHaveBeenCalledWith(
      expect.objectContaining({
        paymentToken: 'tok_test_4242',
        amountCents: 5000,
        customerEmail: 'customer@example.com',
        customerFirstName: 'John',
        customerLastName: 'Doe',
        securityKey: 'nmi-security-key-test',
      }),
    );
  });

  it('creates a transaction with ghlOrderId', async () => {
    await postQuery(buildGhlPayload());
    expect(mockPrisma.transaction.create).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          merchantId: 'merchant-123',
          amountCents: 5000,
          currency: 'USD',
          ghlOrderId: 'ghl_order_789',
          paymentMethod: 'CARD',
        }),
      }),
    );
  });

  it('updates transaction status to CAPTURED on success', async () => {
    await postQuery(buildGhlPayload());
    expect(mockPrisma.transaction.update).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          status: 'CAPTURED',
          nmiTransactionId: 'nmi-charge-123',
        }),
      }),
    );
  });

  it('returns success:false with decline reason when NMI declines', async () => {
    (mockNmi.chargeCard as any).mockResolvedValue({
      success: false,
      transactionId: '',
      responseCode: '300',
      responseText: 'Card declined: insufficient funds',
    });
    const res = await postQuery(buildGhlPayload());
    expect(res.statusCode).toBe(200);
    const json = res.json() as any;
    expect(json.success).toBe(false);
    expect(json.message).toBe('Card declined: insufficient funds');
  });

  it('updates transaction to DECLINED when NMI declines', async () => {
    (mockNmi.chargeCard as any).mockResolvedValue({
      success: false,
      transactionId: '',
      responseCode: '300',
      responseText: 'Do Not Honor',
    });
    await postQuery(buildGhlPayload());
    expect(mockPrisma.transaction.update).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ status: 'DECLINED' }),
      }),
    );
  });

  it('returns 400 when source/token is missing for charge', async () => {
    const payload = buildGhlPayload({ source: undefined });
    const res = await postQuery(payload);
    expect(res.statusCode).toBe(400);
    const json = res.json() as any;
    expect(json.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Refund
// ---------------------------------------------------------------------------

describe('refund', () => {
  const refundPayload = () => ({
    type: 'refund',
    amount: 5000,
    currency: 'USD',
    meta: {
      locationId: 'loc-456',
      orderId: 'ghl_order_789',
      transactionId: 'nmi-txn-original',
    },
  });

  it('returns success:true on successful refund', async () => {
    const res = await postQuery(refundPayload());
    expect(res.statusCode).toBe(200);
    const json = res.json() as any;
    expect(json.success).toBe(true);
    expect(json.message).toBe('Refund successful');
  });

  it('calls refundTransaction with NMI transaction ID', async () => {
    await postQuery(refundPayload());
    expect(mockNmi.refundTransaction).toHaveBeenCalledWith(
      expect.objectContaining({
        transactionId: 'nmi-txn-original',
        securityKey: 'nmi-security-key-test',
      }),
    );
  });

  it('updates transaction status to REFUNDED', async () => {
    await postQuery(refundPayload());
    expect(mockPrisma.transaction.update).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ status: 'REFUNDED' }),
      }),
    );
  });

  it('returns success:false when original transaction not found', async () => {
    (mockPrisma.transaction.findFirst as any).mockResolvedValue(null);
    const res = await postQuery(refundPayload());
    expect(res.statusCode).toBe(200);
    const json = res.json() as any;
    expect(json.success).toBe(false);
    expect(json.message).toMatch(/not found/i);
  });

  it('returns success:false when NMI refund fails', async () => {
    (mockNmi.refundTransaction as any).mockResolvedValue({
      success: false,
      transactionId: 'nmi-txn-original',
      responseCode: '300',
      responseText: 'Refund not allowed',
    });
    const res = await postQuery(refundPayload());
    expect(res.statusCode).toBe(200);
    const json = res.json() as any;
    expect(json.success).toBe(false);
    expect(json.message).toBe('Refund not allowed');
  });

  it('falls back to ghlOrderId lookup when nmiTransactionId lookup returns null', async () => {
    // First call (lookup by nmiTransactionId) returns null → should try ghlOrderId
    (mockPrisma.transaction.findFirst as any)
      .mockResolvedValueOnce(null) // by nmiTransactionId: not found
      .mockResolvedValueOnce({     // by ghlOrderId: found
        id: 'txn-001',
        nmiTransactionId: 'nmi-txn-original',
        amountCents: 5000,
      });

    const payload = {
      type: 'refund',
      amount: 5000,
      meta: {
        locationId: 'loc-456',
        orderId: 'ghl_order_789',
        transactionId: 'nmi-txn-not-found-in-db', // present but lookup misses
      },
    };
    const res = await postQuery(payload);
    expect(res.statusCode).toBe(200);
    const json = res.json() as any;
    expect(json.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

describe('edge cases', () => {
  it('returns success:false for subscription type (not yet supported)', async () => {
    const payload = buildGhlPayload({ type: 'subscription' });
    const res = await postQuery(payload);
    expect(res.statusCode).toBe(200);
    const json = res.json() as any;
    expect(json.success).toBe(false);
    expect(json.message).toMatch(/subscription/i);
  });

  it('returns success:false for unknown payment type', async () => {
    const payload = buildGhlPayload({ type: 'unknown_type' });
    const res = await postQuery(payload);
    expect(res.statusCode).toBe(200);
    const json = res.json() as any;
    expect(json.success).toBe(false);
    expect(json.message).toMatch(/unknown/i);
  });

  it('returns 400 when locationId is missing from meta', async () => {
    const payload = {
      type: 'charge',
      amount: 5000,
      source: 'tok_test',
      meta: { orderId: 'order-1' }, // no locationId
    };
    const res = await postQuery(payload);
    expect(res.statusCode).toBe(400);
    const json = res.json() as any;
    expect(json.success).toBe(false);
    expect(json.message).toMatch(/locationId/i);
  });
});
