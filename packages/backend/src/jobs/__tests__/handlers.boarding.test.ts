/**
 * Tests for boarding-related job handlers.
 *
 * Uses jest.unstable_mockModule (ESM-compatible) + dynamic imports.
 *
 * Tests cover:
 * - handleBoardingStatusCheck: polling loop, no-op when no pending apps
 * - onBoardingApproved: credential encryption, merchant activation, email queue
 * - onBoardingDeclined: application rejected, email queue
 * - handleInstantApproval: delegates to onBoardingApproved
 * - handlePendingBoarding: stores boarding ID, sets UNDER_REVIEW status
 * - submitAndHandleBoarding: routes to instant or pending handler
 */

// In ESM mode with --experimental-vm-modules, jest must be explicitly imported
import { jest } from '@jest/globals';

// ---------------------------------------------------------------------------
// Module-level variables for mock functions (created in beforeAll)
// and imported handler functions (populated after dynamic import)
// ---------------------------------------------------------------------------

// Mock function placeholders — assigned jest.fn() inside beforeAll
// (jest global is available inside lifecycle callbacks but not at module top level in ESM)
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let mockFindMany: any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let mockApplicationUpdate: any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let mockMerchantUpdate: any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let mockMerchantFindUnique: any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let mockNotificationScheduleCreate: any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let mockEncrypt: any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let mockCheckBoardingStatus: any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let mockSubmitBoardingApplication: any;

// Imported handler function signatures
type AnyFn = (...args: unknown[]) => unknown;
let handleBoardingStatusCheck: AnyFn;
let onBoardingApproved: AnyFn;
let onBoardingDeclined: AnyFn;
let handleInstantApproval: AnyFn;
let handlePendingBoarding: AnyFn;
let submitAndHandleBoarding: AnyFn;
let BOARDING_STATUS_CHECK_JOB: string;

// ---------------------------------------------------------------------------
// Setup — register mocks then import module under test
// ---------------------------------------------------------------------------

beforeAll(async () => {
  // Initialize mock functions (jest IS available inside callbacks)
  mockFindMany = jest.fn();
  mockApplicationUpdate = jest.fn();
  mockMerchantUpdate = jest.fn();
  mockMerchantFindUnique = jest.fn();
  mockNotificationScheduleCreate = jest.fn();
  mockEncrypt = jest.fn((v: string) => `enc(${v})`);
  mockCheckBoardingStatus = jest.fn();
  mockSubmitBoardingApplication = jest.fn();

  // Register ESM-compatible module mocks BEFORE dynamic import
  // Cast jest to any because unstable_mockModule is experimental and not in @types/jest
  await (jest as any).unstable_mockModule('../../utils/prisma', () => ({
    prisma: {
      application: {
        findMany: (...args: unknown[]) => mockFindMany(...args),
        update: (...args: unknown[]) => mockApplicationUpdate(...args),
      },
      merchant: {
        update: (...args: unknown[]) => mockMerchantUpdate(...args),
        findUnique: (...args: unknown[]) => mockMerchantFindUnique(...args),
      },
      notificationSchedule: {
        create: (...args: unknown[]) => mockNotificationScheduleCreate(...args),
      },
    },
  }));

  await (jest as any).unstable_mockModule('../../utils/encryption', () => ({
    encrypt: (v: string) => mockEncrypt(v),
  }));

  await (jest as any).unstable_mockModule('../../services/nmi-boarding.service', () => ({
    checkBoardingStatus: (...args: unknown[]) => mockCheckBoardingStatus(...args),
    submitBoardingApplication: (...args: unknown[]) => mockSubmitBoardingApplication(...args),
  }));

  // Dynamic import AFTER mock registration
  const handlers = await import('../handlers.js');
  handleBoardingStatusCheck = handlers.handleBoardingStatusCheck as AnyFn;
  onBoardingApproved = handlers.onBoardingApproved as AnyFn;
  onBoardingDeclined = handlers.onBoardingDeclined as AnyFn;
  handleInstantApproval = handlers.handleInstantApproval as AnyFn;
  handlePendingBoarding = handlers.handlePendingBoarding as AnyFn;
  submitAndHandleBoarding = handlers.submitAndHandleBoarding as AnyFn;
  BOARDING_STATUS_CHECK_JOB = handlers.BOARDING_STATUS_CHECK_JOB;
});

// ---------------------------------------------------------------------------
// Per-test setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  jest.clearAllMocks();
  mockEncrypt.mockImplementation((v: string) => `enc(${v})`);
  mockMerchantFindUnique.mockResolvedValue({
    contactEmail: 'owner@acme.com',
    businessName: 'Acme Corp',
  });
  mockApplicationUpdate.mockResolvedValue({});
  mockMerchantUpdate.mockResolvedValue({});
  mockNotificationScheduleCreate.mockResolvedValue({});
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeApp(overrides: Record<string, unknown> = {}) {
  return {
    id: 'app-1',
    merchantId: 'merch-1',
    nmiBoardingId: 'B001',
    nmiBoardingStatus: 'PENDING',
    merchant: { id: 'merch-1', contactEmail: 'test@example.com', businessName: 'Test Corp' },
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// BOARDING_STATUS_CHECK_JOB constant
// ---------------------------------------------------------------------------

describe('BOARDING_STATUS_CHECK_JOB', () => {
  it('has the correct job name', () => {
    expect(BOARDING_STATUS_CHECK_JOB).toBe('boarding-status-check');
  });
});

// ---------------------------------------------------------------------------
// handleBoardingStatusCheck
// ---------------------------------------------------------------------------

describe('handleBoardingStatusCheck', () => {
  it('does nothing when there are no pending applications', async () => {
    mockFindMany.mockResolvedValue([]);

    await handleBoardingStatusCheck();

    expect(mockCheckBoardingStatus).not.toHaveBeenCalled();
  });

  it('polls each pending application', async () => {
    mockFindMany.mockResolvedValue([
      makeApp({ nmiBoardingId: 'B001' }),
      makeApp({ id: 'app-2', merchantId: 'merch-2', nmiBoardingId: 'B002' }),
    ]);
    mockCheckBoardingStatus.mockResolvedValue({ boardingId: 'B001', status: 'PENDING' });

    await handleBoardingStatusCheck();

    expect(mockCheckBoardingStatus).toHaveBeenCalledTimes(2);
    expect(mockCheckBoardingStatus).toHaveBeenCalledWith('B001');
    expect(mockCheckBoardingStatus).toHaveBeenCalledWith('B002');
  });

  it('calls onBoardingApproved when status is APPROVED', async () => {
    mockFindMany.mockResolvedValue([makeApp({ nmiBoardingId: 'B001' })]);
    mockCheckBoardingStatus.mockResolvedValue({
      boardingId: 'B001',
      status: 'APPROVED',
      securityKey: 'sk_live_abc',
      tokenizationKey: 'tk_live_xyz',
      nmiMerchantId: 'MID123',
    });

    await handleBoardingStatusCheck();

    expect(mockMerchantUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        where: { id: 'merch-1' },
        data: expect.objectContaining({
          status: 'ACTIVE',
          nmiSecurityKey: 'enc(sk_live_abc)',
          nmiTokenizationKey: 'enc(tk_live_xyz)',
          nmiMerchantId: 'MID123',
        }),
      })
    );
    expect(mockApplicationUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        where: { id: 'app-1' },
        data: expect.objectContaining({ status: 'APPROVED', nmiBoardingStatus: 'APPROVED' }),
      })
    );
    expect(mockNotificationScheduleCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ type: 'merchant_approved' }),
      })
    );
  });

  it('calls onBoardingDeclined when status is DECLINED', async () => {
    mockFindMany.mockResolvedValue([makeApp({ nmiBoardingId: 'B002' })]);
    mockCheckBoardingStatus.mockResolvedValue({
      boardingId: 'B002',
      status: 'DECLINED',
      declineReason: 'High chargeback ratio',
    });

    await handleBoardingStatusCheck();

    expect(mockApplicationUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        where: { id: 'app-1' },
        data: expect.objectContaining({
          status: 'REJECTED',
          nmiBoardingStatus: 'DECLINED',
          rejectionReason: 'High chargeback ratio',
        }),
      })
    );
    expect(mockNotificationScheduleCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ type: 'merchant_rejected' }),
      })
    );
  });

  it('skips applications with null nmiBoardingId', async () => {
    mockFindMany.mockResolvedValue([makeApp({ nmiBoardingId: null })]);

    await handleBoardingStatusCheck();

    expect(mockCheckBoardingStatus).not.toHaveBeenCalled();
  });

  it('skips when checkBoardingStatus returns null (not configured)', async () => {
    mockFindMany.mockResolvedValue([makeApp({ nmiBoardingId: 'B003' })]);
    mockCheckBoardingStatus.mockResolvedValue(null);

    await handleBoardingStatusCheck();

    expect(mockMerchantUpdate).not.toHaveBeenCalled();
    expect(mockApplicationUpdate).not.toHaveBeenCalled();
  });

  it('continues processing other apps when one throws', async () => {
    mockFindMany.mockResolvedValue([
      makeApp({ id: 'app-1', nmiBoardingId: 'B001' }),
      makeApp({ id: 'app-2', merchantId: 'merch-2', nmiBoardingId: 'B002' }),
    ]);
    mockCheckBoardingStatus
      .mockRejectedValueOnce(new Error('NMI error'))
      .mockResolvedValueOnce({ boardingId: 'B002', status: 'PENDING' });

    await expect(handleBoardingStatusCheck()).resolves.not.toThrow();
    expect(mockCheckBoardingStatus).toHaveBeenCalledTimes(2);
  });

  it('queries applications with nmiBoardingStatus PENDING and nmiBoardingId not null', async () => {
    mockFindMany.mockResolvedValue([]);

    await handleBoardingStatusCheck();

    expect(mockFindMany).toHaveBeenCalledWith(
      expect.objectContaining({
        where: {
          nmiBoardingStatus: 'PENDING',
          nmiBoardingId: { not: null },
        },
      })
    );
  });
});

// ---------------------------------------------------------------------------
// onBoardingApproved
// ---------------------------------------------------------------------------

describe('onBoardingApproved', () => {
  it('encrypts security key and tokenization key', async () => {
    await onBoardingApproved('merch-1', 'app-1', 'sk_live_abc', 'tk_live_xyz', 'MID001');

    expect(mockEncrypt).toHaveBeenCalledWith('sk_live_abc');
    expect(mockEncrypt).toHaveBeenCalledWith('tk_live_xyz');

    expect(mockMerchantUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          nmiSecurityKey: 'enc(sk_live_abc)',
          nmiTokenizationKey: 'enc(tk_live_xyz)',
          nmiMerchantId: 'MID001',
          status: 'ACTIVE',
        }),
      })
    );
  });

  it('sets merchant status to ACTIVE', async () => {
    await onBoardingApproved('merch-1', 'app-1', 'sk', 'tk', undefined);

    expect(mockMerchantUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ status: 'ACTIVE' }),
      })
    );
  });

  it('updates application status to APPROVED', async () => {
    await onBoardingApproved('merch-1', 'app-1', 'sk', 'tk', 'MID001');

    expect(mockApplicationUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        where: { id: 'app-1' },
        data: expect.objectContaining({ status: 'APPROVED', nmiBoardingStatus: 'APPROVED' }),
      })
    );
  });

  it('queues welcome email notification', async () => {
    await onBoardingApproved('merch-1', 'app-1', 'sk', 'tk', 'MID001');

    expect(mockNotificationScheduleCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          type: 'merchant_approved',
          recipientId: 'merch-1',
          channel: 'email',
        }),
      })
    );
  });

  it('works when securityKey and tokenizationKey are undefined', async () => {
    await onBoardingApproved('merch-1', 'app-1', undefined, undefined, 'MID001');

    expect(mockEncrypt).not.toHaveBeenCalled();
    expect(mockMerchantUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ status: 'ACTIVE', nmiMerchantId: 'MID001' }),
      })
    );
  });
});

// ---------------------------------------------------------------------------
// onBoardingDeclined
// ---------------------------------------------------------------------------

describe('onBoardingDeclined', () => {
  it('updates application status to REJECTED', async () => {
    await onBoardingDeclined('merch-1', 'app-1', 'Prohibited business type');

    expect(mockApplicationUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        where: { id: 'app-1' },
        data: expect.objectContaining({
          status: 'REJECTED',
          nmiBoardingStatus: 'DECLINED',
          rejectionReason: 'Prohibited business type',
        }),
      })
    );
  });

  it('uses default reason when none provided', async () => {
    await onBoardingDeclined('merch-1', 'app-1', undefined);

    expect(mockApplicationUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          rejectionReason: 'Application declined by NMI',
        }),
      })
    );
  });

  it('queues rejection email notification', async () => {
    await onBoardingDeclined('merch-1', 'app-1', 'Risk threshold exceeded');

    expect(mockNotificationScheduleCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          type: 'merchant_rejected',
          recipientId: 'merch-1',
          channel: 'email',
        }),
      })
    );
  });

  it('does not update merchant record', async () => {
    await onBoardingDeclined('merch-1', 'app-1', 'Declined');

    expect(mockMerchantUpdate).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// handleInstantApproval
// ---------------------------------------------------------------------------

describe('handleInstantApproval', () => {
  it('delegates to onBoardingApproved with boarding result fields', async () => {
    const boardingResult = {
      success: true,
      status: 'APPROVED',
      boardingId: 'B001',
      securityKey: 'sk_live_test',
      tokenizationKey: 'tk_live_test',
      nmiMerchantId: 'MID001',
    };

    await handleInstantApproval('merch-1', 'app-1', boardingResult);

    expect(mockEncrypt).toHaveBeenCalledWith('sk_live_test');
    expect(mockEncrypt).toHaveBeenCalledWith('tk_live_test');
    expect(mockMerchantUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ status: 'ACTIVE', nmiMerchantId: 'MID001' }),
      })
    );
  });
});

// ---------------------------------------------------------------------------
// handlePendingBoarding
// ---------------------------------------------------------------------------

describe('handlePendingBoarding', () => {
  it('stores boarding ID and sets status to UNDER_REVIEW', async () => {
    const boardingResult = {
      success: true,
      status: 'PENDING',
      boardingId: 'B999',
    };

    await handlePendingBoarding('app-1', boardingResult);

    expect(mockApplicationUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        where: { id: 'app-1' },
        data: expect.objectContaining({
          status: 'UNDER_REVIEW',
          nmiBoardingId: 'B999',
          nmiBoardingStatus: 'PENDING',
        }),
      })
    );
  });

  it('handles missing boardingId gracefully', async () => {
    const boardingResult = {
      success: true,
      status: 'PENDING',
      boardingId: undefined,
    };

    await handlePendingBoarding('app-1', boardingResult);

    expect(mockApplicationUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          nmiBoardingId: undefined,
          nmiBoardingStatus: 'PENDING',
        }),
      })
    );
  });
});

// ---------------------------------------------------------------------------
// submitAndHandleBoarding
// ---------------------------------------------------------------------------

describe('submitAndHandleBoarding', () => {
  const testApp = {
    businessName: 'Test Biz',
    businessType: 'retail',
    businessAddress: '1 Test St',
    businessCity: 'City',
    businessState: 'CA',
    businessZip: '90210',
    ownerFirstName: 'Jane',
    ownerLastName: 'Smith',
    ownerEmail: 'jane@test.com',
    ownerDob: '1985-06-15',
    ownerSsnLast4: '5678',
    ownerAddress: '2 Home Rd',
    ownerCity: 'City',
    ownerState: 'CA',
    ownerZip: '90211',
    bankName: 'Chase',
    bankRoutingNumber: '021000021',
    bankAccountNumber: '987654321',
    bankAccountType: 'checking',
  };

  it('returns error result without updating DB on submission failure', async () => {
    mockSubmitBoardingApplication.mockResolvedValue({
      success: false,
      error: 'NMI server unavailable',
    });

    const result = await submitAndHandleBoarding('merch-1', 'app-1', testApp) as Record<string, unknown>;

    expect(result.success).toBe(false);
    expect(mockApplicationUpdate).not.toHaveBeenCalled();
    expect(mockMerchantUpdate).not.toHaveBeenCalled();
  });

  it('routes APPROVED result to handleInstantApproval', async () => {
    mockSubmitBoardingApplication.mockResolvedValue({
      success: true,
      status: 'APPROVED',
      boardingId: 'B001',
      securityKey: 'sk_live',
      tokenizationKey: 'tk_live',
      nmiMerchantId: 'MID001',
    });

    const result = await submitAndHandleBoarding('merch-1', 'app-1', testApp) as Record<string, unknown>;

    expect(result.success).toBe(true);
    expect(result.status).toBe('APPROVED');
    expect(mockMerchantUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ status: 'ACTIVE' }),
      })
    );
  });

  it('routes PENDING result to handlePendingBoarding', async () => {
    mockSubmitBoardingApplication.mockResolvedValue({
      success: true,
      status: 'PENDING',
      boardingId: 'B002',
    });

    const result = await submitAndHandleBoarding('merch-1', 'app-1', testApp) as Record<string, unknown>;

    expect(result.success).toBe(true);
    expect(result.status).toBe('PENDING');
    expect(mockApplicationUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          status: 'UNDER_REVIEW',
          nmiBoardingId: 'B002',
        }),
      })
    );
  });
});
