/**
 * Tests for NMI Partner Boarding API service.
 *
 * Uses jest.unstable_mockModule (ESM-compatible mocking) + dynamic imports.
 *
 * Tests cover:
 * - parseNmiResponse: XML, key=value, JSON formats
 * - isNmiConfigured: env var check
 * - submitBoardingApplication: mock mode, APPROVED, PENDING, DECLINED, HTTP errors
 * - checkBoardingStatus: APPROVED, PENDING, DECLINED, mock IDs, HTTP errors
 */

// In ESM mode with --experimental-vm-modules, jest must be explicitly imported
import { jest } from '@jest/globals';

// ---------------------------------------------------------------------------
// Mutable state shared between mock and tests
// nmiState is updated per-test in beforeEach — no jest globals needed here
// ---------------------------------------------------------------------------

const nmiState: Record<string, string> = {
  NMI_PARTNER_ID: '',
  NMI_PARTNER_KEY: '',
  NMI_BOARDING_API_URL: 'https://secure.nmi.com/api/boarding/',
};

// ---------------------------------------------------------------------------
// Module-level variables for dynamically imported functions
// (populated in beforeAll after mock setup)
// ---------------------------------------------------------------------------

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyFn = (...args: any[]) => any;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let parseNmiResponse: AnyFn;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let isNmiConfigured: AnyFn;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let submitBoardingApplication: AnyFn;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let checkBoardingStatus: AnyFn;

// ---------------------------------------------------------------------------
// Setup — import module after mocks are registered
// ---------------------------------------------------------------------------

beforeAll(async () => {
  // Mock env.ts to return nmiState object directly.
  // Tests modify nmiState per-test; the service reads from it via env.NMI_PARTNER_ID etc.
  // Path relative to this test file: ../../utils/env = src/utils/env
  // Cast jest to any because unstable_mockModule is experimental and not in @types/jest
  await (jest as any).unstable_mockModule('../../utils/env', () => ({
    env: nmiState,
  }));

  // Dynamic import AFTER mock registration so the service sees our env mock
  const svc = await import('../nmi-boarding.service.js');
  parseNmiResponse = svc.parseNmiResponse;
  isNmiConfigured = svc.isNmiConfigured;
  submitBoardingApplication = svc.submitBoardingApplication;
  checkBoardingStatus = svc.checkBoardingStatus;
});

// Reset env state and fetch mock before each test
beforeEach(() => {
  nmiState.NMI_PARTNER_ID = '';
  nmiState.NMI_PARTNER_KEY = '';
  nmiState.NMI_BOARDING_API_URL = 'https://secure.nmi.com/api/boarding/';
  global.fetch = jest.fn() as unknown as typeof fetch;
});

// Minimal valid boarding application for tests
const TEST_APP = {
  businessName: 'Acme Corp',
  businessType: 'retail',
  businessAddress: '123 Main St',
  businessCity: 'Springfield',
  businessState: 'IL',
  businessZip: '62701',
  ownerFirstName: 'John',
  ownerLastName: 'Doe',
  ownerEmail: 'john@acme.com',
  ownerDob: '1980-01-15',
  ownerSsnLast4: '1234',
  ownerAddress: '456 Oak Ave',
  ownerCity: 'Springfield',
  ownerState: 'IL',
  ownerZip: '62702',
  bankName: 'First National Bank',
  bankRoutingNumber: '021000021',
  bankAccountNumber: '123456789',
  bankAccountType: 'checking',
};

// ---------------------------------------------------------------------------
// parseNmiResponse
// ---------------------------------------------------------------------------

describe('parseNmiResponse', () => {
  it('parses XML tags', () => {
    const xml =
      '<status>APPROVED</status><merchant_id>MID123</merchant_id><security_key>sk_abc</security_key>';
    const result = parseNmiResponse(xml);
    expect(result.status).toBe('APPROVED');
    expect(result.merchant_id).toBe('MID123');
    expect(result.security_key).toBe('sk_abc');
  });

  it('parses URL-encoded key=value pairs', () => {
    const kv = 'status=PENDING&boarding_id=B001&response_code=200';
    const result = parseNmiResponse(kv);
    expect(result.status).toBe('PENDING');
    expect(result.boarding_id).toBe('B001');
    expect(result.response_code).toBe('200');
  });

  it('parses JSON', () => {
    const json = JSON.stringify({ status: 'DECLINED', decline_reason: 'Credit check failed' });
    const result = parseNmiResponse(json);
    expect(result.status).toBe('DECLINED');
    expect(result.decline_reason).toBe('Credit check failed');
  });

  it('handles empty string (returns raw field for non-parseable input)', () => {
    const result = parseNmiResponse('');
    // Empty string falls through to JSON.parse which throws, so raw is set
    expect(result).toBeDefined();
    expect(typeof result).toBe('object');
  });

  it('handles XML with multiple tags', () => {
    const xml = '<response_code>100</response_code><boarding_id>BD001</boarding_id>';
    const result = parseNmiResponse(xml);
    expect(result.response_code).toBe('100');
    expect(result.boarding_id).toBe('BD001');
  });
});

// ---------------------------------------------------------------------------
// isNmiConfigured
// ---------------------------------------------------------------------------

describe('isNmiConfigured', () => {
  it('returns false when both vars are missing', () => {
    nmiState.NMI_PARTNER_ID = '';
    nmiState.NMI_PARTNER_KEY = '';
    expect(isNmiConfigured()).toBe(false);
  });

  it('returns false when only NMI_PARTNER_ID is set', () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = '';
    expect(isNmiConfigured()).toBe(false);
  });

  it('returns false when only NMI_PARTNER_KEY is set', () => {
    nmiState.NMI_PARTNER_ID = '';
    nmiState.NMI_PARTNER_KEY = 'pkey';
    expect(isNmiConfigured()).toBe(false);
  });

  it('returns true when both vars are set', () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';
    expect(isNmiConfigured()).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// submitBoardingApplication
// ---------------------------------------------------------------------------

describe('submitBoardingApplication', () => {
  it('returns mock result when NMI credentials are not configured', async () => {
    nmiState.NMI_PARTNER_ID = '';
    nmiState.NMI_PARTNER_KEY = '';

    const result = await submitBoardingApplication(TEST_APP);

    expect(result.success).toBe(true);
    expect(result.status).toBe('PENDING');
    expect((result.boardingId as string)).toMatch(/^mock_boarding_/);
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it('throws when API URL is not HTTPS', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';
    nmiState.NMI_BOARDING_API_URL = 'http://insecure.example.com/api/boarding/';

    await expect(submitBoardingApplication(TEST_APP)).rejects.toThrow('must use HTTPS');
  });

  it('handles instant APPROVED response (XML)', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 200,
      ok: true,
      text: async () =>
        '<status>APPROVED</status><boarding_id>B999</boarding_id><merchant_id>MID999</merchant_id><security_key>sk_live_xyz</security_key><tokenization_key>tk_live_abc</tokenization_key>',
    });

    const result = await submitBoardingApplication(TEST_APP);

    expect(result.success).toBe(true);
    expect(result.status).toBe('APPROVED');
    expect(result.boardingId).toBe('B999');
    expect(result.nmiMerchantId).toBe('MID999');
    expect(result.securityKey).toBe('sk_live_xyz');
    expect(result.tokenizationKey).toBe('tk_live_abc');
  });

  it('handles PENDING response (key=value)', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 200,
      ok: true,
      text: async () => 'status=PENDING&boarding_id=B100&response_code=200',
    });

    const result = await submitBoardingApplication(TEST_APP);

    expect(result.success).toBe(true);
    expect(result.status).toBe('PENDING');
    expect(result.boardingId).toBe('B100');
    expect(result.securityKey).toBeUndefined();
  });

  it('handles DECLINED response (JSON)', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 200,
      ok: true,
      text: async () =>
        JSON.stringify({
          status: 'DECLINED',
          boarding_id: 'B101',
          decline_reason: 'Credit score too low',
        }),
    });

    const result = await submitBoardingApplication(TEST_APP);

    expect(result.success).toBe(false);
    expect(result.status).toBe('DECLINED');
    expect(result.error).toBe('Credit score too low');
  });

  it('returns error on HTTP 4xx response', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 401,
      ok: false,
      text: async () => 'Unauthorized',
    });

    const result = await submitBoardingApplication(TEST_APP);

    expect(result.success).toBe(false);
    expect((result.error as string)).toMatch(/HTTP 401/);
  });

  it('returns error on fetch network failure', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockRejectedValueOnce(new Error('Network timeout'));

    const result = await submitBoardingApplication(TEST_APP);

    expect(result.success).toBe(false);
    expect((result.error as string)).toContain('Network timeout');
  });

  it('treats unknown status as PENDING', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 200,
      ok: true,
      text: async () => 'status=PROCESSING&boarding_id=B200',
    });

    const result = await submitBoardingApplication(TEST_APP);

    expect(result.success).toBe(true);
    expect(result.status).toBe('PENDING');
  });

  it('sends POST with form-encoded Content-Type to configured URL', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 200,
      ok: true,
      text: async () => 'status=PENDING&boarding_id=B300',
    });

    await submitBoardingApplication(TEST_APP);

    expect(global.fetch).toHaveBeenCalledTimes(1);
    const [url, options] = (global.fetch as any).mock.calls[0] as [string, RequestInit];
    expect(url).toBe('https://secure.nmi.com/api/boarding/');
    expect(options.method).toBe('POST');
    expect((options.headers as Record<string, string>)['Content-Type']).toBe(
      'application/x-www-form-urlencoded'
    );
  });

  it('includes business and owner fields in the request body', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 200,
      ok: true,
      text: async () => 'status=PENDING&boarding_id=B301',
    });

    await submitBoardingApplication(TEST_APP);

    const [, options] = (global.fetch as any).mock.calls[0] as [string, RequestInit];
    const body = new URLSearchParams(options.body as string);
    expect(body.get('legal_name')).toBe('Acme Corp');
    expect(body.get('owner_first_name')).toBe('John');
    expect(body.get('owner_last_name')).toBe('Doe');
    expect(body.get('bank_account_type')).toBe('checking');
  });

  it('handles response_code 100 as APPROVED', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 200,
      ok: true,
      text: async () =>
        'response_code=100&merchant_id=MID500&security_key=sk_500&tokenization_key=tk_500',
    });

    const result = await submitBoardingApplication(TEST_APP);

    expect(result.success).toBe(true);
    expect(result.status).toBe('APPROVED');
    expect(result.nmiMerchantId).toBe('MID500');
  });
});

// ---------------------------------------------------------------------------
// checkBoardingStatus
// ---------------------------------------------------------------------------

describe('checkBoardingStatus', () => {
  it('returns null when NMI credentials are not configured', async () => {
    nmiState.NMI_PARTNER_ID = '';
    nmiState.NMI_PARTNER_KEY = '';

    const result = await checkBoardingStatus('B001');
    expect(result).toBeNull();
  });

  it('returns null for mock boarding IDs', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    const result = await checkBoardingStatus('mock_boarding_1234567890');
    expect(result).toBeNull();
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it('throws when API URL is not HTTPS', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';
    nmiState.NMI_BOARDING_API_URL = 'http://insecure.example.com/api/boarding/';

    await expect(checkBoardingStatus('B001')).rejects.toThrow('must use HTTPS');
  });

  it('returns APPROVED with credentials on approval', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 200,
      ok: true,
      text: async () =>
        '<status>APPROVED</status><merchant_id>MID888</merchant_id><security_key>sk_live_888</security_key><tokenization_key>tk_live_888</tokenization_key>',
    });

    const result = await checkBoardingStatus('B888');

    expect(result).not.toBeNull();
    expect(result!.boardingId).toBe('B888');
    expect(result!.status).toBe('APPROVED');
    expect(result!.nmiMerchantId).toBe('MID888');
    expect(result!.securityKey).toBe('sk_live_888');
    expect(result!.tokenizationKey).toBe('tk_live_888');
  });

  it('returns PENDING when still under review', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 200,
      ok: true,
      text: async () => 'status=PENDING&boarding_id=B777',
    });

    const result = await checkBoardingStatus('B777');

    expect(result).not.toBeNull();
    expect(result!.status).toBe('PENDING');
    expect(result!.securityKey).toBeUndefined();
  });

  it('returns DECLINED with reason', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 200,
      ok: true,
      text: async () =>
        JSON.stringify({ status: 'DECLINED', decline_reason: 'Prohibited business type' }),
    });

    const result = await checkBoardingStatus('B666');

    expect(result).not.toBeNull();
    expect(result!.status).toBe('DECLINED');
    expect(result!.declineReason).toBe('Prohibited business type');
  });

  it('returns null on HTTP 4xx error', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 403,
      ok: false,
      text: async () => 'Forbidden',
    });

    const result = await checkBoardingStatus('B555');
    expect(result).toBeNull();
  });

  it('returns null on fetch network failure', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockRejectedValueOnce(new Error('Connection refused'));

    const result = await checkBoardingStatus('B444');
    expect(result).toBeNull();
  });

  it('sends boarding_id in status check request', async () => {
    nmiState.NMI_PARTNER_ID = 'pid';
    nmiState.NMI_PARTNER_KEY = 'pkey';

    (global.fetch as any).mockResolvedValueOnce({
      status: 200,
      ok: true,
      text: async () => 'status=PENDING',
    });

    await checkBoardingStatus('B_SPECIFIC_123');

    const [, options] = (global.fetch as any).mock.calls[0] as [string, RequestInit];
    const body = new URLSearchParams(options.body as string);
    expect(body.get('boarding_id')).toBe('B_SPECIFIC_123');
  });
});
