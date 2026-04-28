/**
 * Tests for boarding-status-check job handler and related functions.
 *
 * Tests cover:
 * - Polling UNDER_REVIEW applications
 * - Handling approval: credential storage, status update, welcome email
 * - Handling rejection: status update, notification email
 * - Handling NMI errors during polling
 * - Instant approval handling
 * - Pending review handling
 * - Job registration
 */

import {
  handleBoardingStatusCheck,
  handleInstantApproval,
  handlePendingReview,
  registerBoardingStatusCheckJob,
  BOARDING_STATUS_CHECK_JOB,
  BOARDING_STATUS_CHECK_CRON,
  PgBossLike,
  PrismaLike,
  ApplicationRecord,
} from "../handlers";

// Mock the NMI boarding service
jest.mock("../../services/nmi-boarding.service", () => ({
  checkBoardingStatus: jest.fn(),
  encryptNmiCredentials: jest.fn(),
  isNmiConfigured: jest.fn(),
}));

// Mock encryption for encryptNmiCredentials calls
jest.mock("../../utils/encryption", () => ({
  encrypt: jest.fn((v: string) => `encrypted:${v}`),
  decrypt: jest.fn((v: string) =>
    v.startsWith("encrypted:") ? v.slice(10) : v
  ),
  isEncrypted: jest.fn((v: string) => v.startsWith("encrypted:")),
  validateEncryptionKey: jest.fn(),
  _resetKeys: jest.fn(),
}));

import {
  checkBoardingStatus,
  encryptNmiCredentials,
  isNmiConfigured,
} from "../../services/nmi-boarding.service";

const mockedCheckBoardingStatus = checkBoardingStatus as jest.MockedFunction<
  typeof checkBoardingStatus
>;
const mockedEncryptNmiCredentials =
  encryptNmiCredentials as jest.MockedFunction<typeof encryptNmiCredentials>;
const mockedIsNmiConfigured = isNmiConfigured as jest.MockedFunction<
  typeof isNmiConfigured
>;

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

const TEST_ENCRYPTION_KEY =
  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

function makeMockPrisma(): PrismaLike {
  return {
    application: {
      findMany: jest.fn().mockResolvedValue([]),
      update: jest.fn().mockResolvedValue({}),
    },
    merchant: {
      update: jest.fn().mockResolvedValue({}),
    },
  };
}

function makeMockBoss(): PgBossLike {
  return {
    schedule: jest.fn().mockResolvedValue(undefined),
    work: jest.fn().mockResolvedValue("worker-id"),
    send: jest.fn().mockResolvedValue("job-id"),
  };
}

function makeApp(overrides: Partial<ApplicationRecord> = {}): ApplicationRecord {
  return {
    id: "app-001",
    merchantId: "merch-001",
    nmiApplicationId: "nmi-brd-001",
    boardingStatus: "UNDER_REVIEW",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("Boarding Status Check Job", () => {
  const originalEnv = process.env;

  beforeEach(() => {
    jest.clearAllMocks();
    process.env = { ...originalEnv };
    process.env.ENCRYPTION_KEY = TEST_ENCRYPTION_KEY;
    process.env.NMI_PARTNER_ID = "test-partner-id";
    process.env.NMI_PARTNER_KEY = "test-partner-key";
    mockedIsNmiConfigured.mockReturnValue(true);
    mockedEncryptNmiCredentials.mockImplementation((result) => ({
      nmiMerchantId: result.nmiMerchantId || null,
      nmiSecurityKey: result.securityKey
        ? `encrypted:${result.securityKey}`
        : null,
      nmiTokenizationKey: result.tokenizationKey
        ? `encrypted:${result.tokenizationKey}`
        : null,
    }));
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  // -----------------------------------------------------------------------
  // handleBoardingStatusCheck
  // -----------------------------------------------------------------------

  describe("handleBoardingStatusCheck", () => {
    it("skips when NMI is not configured", async () => {
      mockedIsNmiConfigured.mockReturnValue(false);
      const prisma = makeMockPrisma();
      const boss = makeMockBoss();

      const stats = await handleBoardingStatusCheck(prisma, boss);

      expect(stats.checked).toBe(0);
      expect(prisma.application.findMany).not.toHaveBeenCalled();
    });

    it("returns zero stats when no applications under review", async () => {
      const prisma = makeMockPrisma();
      const boss = makeMockBoss();

      const stats = await handleBoardingStatusCheck(prisma, boss);

      expect(stats.checked).toBe(0);
      expect(stats.approved).toBe(0);
      expect(stats.rejected).toBe(0);
      expect(stats.stillPending).toBe(0);
      expect(stats.errors).toBe(0);
    });

    it("checks all UNDER_REVIEW applications", async () => {
      const prisma = makeMockPrisma();
      const boss = makeMockBoss();

      const apps = [
        makeApp({ id: "app-1", nmiApplicationId: "nmi-1" }),
        makeApp({ id: "app-2", nmiApplicationId: "nmi-2" }),
        makeApp({ id: "app-3", nmiApplicationId: "nmi-3" }),
      ];
      (prisma.application.findMany as jest.Mock).mockResolvedValue(apps);

      // All still under review
      mockedCheckBoardingStatus.mockResolvedValue({
        nmiApplicationId: "nmi-x",
        status: "UNDER_REVIEW",
        message: "Still pending",
      });

      const stats = await handleBoardingStatusCheck(prisma, boss);

      expect(stats.checked).toBe(3);
      expect(stats.stillPending).toBe(3);
      expect(mockedCheckBoardingStatus).toHaveBeenCalledTimes(3);
    });

    it("handles approval: stores credentials, updates status, queues email", async () => {
      const prisma = makeMockPrisma();
      const boss = makeMockBoss();

      const app = makeApp();
      (prisma.application.findMany as jest.Mock).mockResolvedValue([app]);

      mockedCheckBoardingStatus.mockResolvedValue({
        nmiApplicationId: "nmi-brd-001",
        status: "APPROVED",
        nmiMerchantId: "MID_APPROVED",
        securityKey: "SK_APPROVED",
        tokenizationKey: "TK_APPROVED",
      });

      const stats = await handleBoardingStatusCheck(prisma, boss);

      expect(stats.approved).toBe(1);

      // Verify merchant was updated with encrypted credentials
      expect(prisma.merchant.update).toHaveBeenCalledWith(
        expect.objectContaining({
          where: { id: "merch-001" },
          data: expect.objectContaining({
            nmiMerchantId: "MID_APPROVED",
            nmiSecurityKey: "encrypted:SK_APPROVED",
            nmiTokenizationKey: "encrypted:TK_APPROVED",
            onboardingStatus: "ACTIVE",
          }),
        })
      );

      // Verify application status updated
      expect(prisma.application.update).toHaveBeenCalledWith(
        expect.objectContaining({
          where: { id: "app-001" },
          data: expect.objectContaining({
            boardingStatus: "APPROVED",
          }),
        })
      );

      // Verify welcome email queued
      expect(boss.send).toHaveBeenCalledWith(
        "send-email",
        expect.objectContaining({
          type: "welcome",
          merchantId: "merch-001",
          template: "merchant-boarding-approved",
        })
      );
    });

    it("handles rejection: updates status, queues notification", async () => {
      const prisma = makeMockPrisma();
      const boss = makeMockBoss();

      const app = makeApp({ id: "app-rej", merchantId: "merch-rej" });
      (prisma.application.findMany as jest.Mock).mockResolvedValue([app]);

      mockedCheckBoardingStatus.mockResolvedValue({
        nmiApplicationId: "nmi-brd-001",
        status: "DECLINED",
        declineReason: "High risk industry",
      });

      const stats = await handleBoardingStatusCheck(prisma, boss);

      expect(stats.rejected).toBe(1);

      // Verify application status updated
      expect(prisma.application.update).toHaveBeenCalledWith(
        expect.objectContaining({
          where: { id: "app-rej" },
          data: expect.objectContaining({
            boardingStatus: "REJECTED",
            boardingRejectionReason: "High risk industry",
          }),
        })
      );

      // Verify merchant updated
      expect(prisma.merchant.update).toHaveBeenCalledWith(
        expect.objectContaining({
          where: { id: "merch-rej" },
          data: expect.objectContaining({
            onboardingStatus: "REJECTED",
          }),
        })
      );

      // Verify rejection email queued
      expect(boss.send).toHaveBeenCalledWith(
        "send-email",
        expect.objectContaining({
          type: "boarding-rejected",
          merchantId: "merch-rej",
          reason: "High risk industry",
          template: "merchant-boarding-rejected",
        })
      );
    });

    it("handles NMI API errors gracefully per-application", async () => {
      const prisma = makeMockPrisma();
      const boss = makeMockBoss();

      const apps = [
        makeApp({ id: "app-ok", nmiApplicationId: "nmi-ok" }),
        makeApp({ id: "app-err", nmiApplicationId: "nmi-err" }),
      ];
      (prisma.application.findMany as jest.Mock).mockResolvedValue(apps);

      mockedCheckBoardingStatus
        .mockResolvedValueOnce({
          nmiApplicationId: "nmi-ok",
          status: "UNDER_REVIEW",
        })
        .mockRejectedValueOnce(new Error("Connection timeout"));

      const errorSpy = jest.spyOn(console, "error").mockImplementation();
      const stats = await handleBoardingStatusCheck(prisma, boss);
      errorSpy.mockRestore();

      expect(stats.checked).toBe(2);
      expect(stats.stillPending).toBe(1);
      expect(stats.errors).toBe(1);
    });

    it("skips applications with null nmiApplicationId", async () => {
      const prisma = makeMockPrisma();
      const boss = makeMockBoss();

      const apps = [
        makeApp({ id: "app-null", nmiApplicationId: null }),
      ];
      (prisma.application.findMany as jest.Mock).mockResolvedValue(apps);

      const stats = await handleBoardingStatusCheck(prisma, boss);

      expect(stats.checked).toBe(0);
      expect(mockedCheckBoardingStatus).not.toHaveBeenCalled();
    });

    it("handles mixed approval/rejection/pending results", async () => {
      const prisma = makeMockPrisma();
      const boss = makeMockBoss();

      const apps = [
        makeApp({ id: "app-a", merchantId: "m-a", nmiApplicationId: "nmi-a" }),
        makeApp({ id: "app-b", merchantId: "m-b", nmiApplicationId: "nmi-b" }),
        makeApp({ id: "app-c", merchantId: "m-c", nmiApplicationId: "nmi-c" }),
      ];
      (prisma.application.findMany as jest.Mock).mockResolvedValue(apps);

      mockedCheckBoardingStatus
        .mockResolvedValueOnce({
          nmiApplicationId: "nmi-a",
          status: "APPROVED",
          nmiMerchantId: "MID-A",
          securityKey: "SK-A",
          tokenizationKey: "TK-A",
        })
        .mockResolvedValueOnce({
          nmiApplicationId: "nmi-b",
          status: "DECLINED",
          declineReason: "Bad credit",
        })
        .mockResolvedValueOnce({
          nmiApplicationId: "nmi-c",
          status: "UNDER_REVIEW",
        });

      const stats = await handleBoardingStatusCheck(prisma, boss);

      expect(stats.checked).toBe(3);
      expect(stats.approved).toBe(1);
      expect(stats.rejected).toBe(1);
      expect(stats.stillPending).toBe(1);
    });

    it("continues checking other applications if welcome email fails", async () => {
      const prisma = makeMockPrisma();
      const boss = makeMockBoss();

      const app = makeApp();
      (prisma.application.findMany as jest.Mock).mockResolvedValue([app]);

      mockedCheckBoardingStatus.mockResolvedValue({
        nmiApplicationId: "nmi-brd-001",
        status: "APPROVED",
        nmiMerchantId: "MID1",
        securityKey: "SK1",
        tokenizationKey: "TK1",
      });

      // Email sending fails
      (boss.send as jest.Mock).mockRejectedValue(
        new Error("Email service down")
      );

      const errorSpy = jest.spyOn(console, "error").mockImplementation();
      const stats = await handleBoardingStatusCheck(prisma, boss);
      errorSpy.mockRestore();

      // Approval should still be counted
      expect(stats.approved).toBe(1);
      // Merchant and application should still be updated
      expect(prisma.merchant.update).toHaveBeenCalled();
      expect(prisma.application.update).toHaveBeenCalled();
    });
  });

  // -----------------------------------------------------------------------
  // handleInstantApproval
  // -----------------------------------------------------------------------

  describe("handleInstantApproval", () => {
    it("stores encrypted credentials and updates statuses", async () => {
      const prisma = makeMockPrisma();
      const boss = makeMockBoss();

      await handleInstantApproval(prisma, boss, "app-inst", "merch-inst", {
        nmiApplicationId: "BRD_INST",
        nmiMerchantId: "MID_INST",
        securityKey: "SK_INST",
        tokenizationKey: "TK_INST",
      });

      // Verify merchant updated
      expect(prisma.merchant.update).toHaveBeenCalledWith(
        expect.objectContaining({
          where: { id: "merch-inst" },
          data: expect.objectContaining({
            nmiMerchantId: "MID_INST",
            nmiSecurityKey: "encrypted:SK_INST",
            nmiTokenizationKey: "encrypted:TK_INST",
            onboardingStatus: "ACTIVE",
          }),
        })
      );

      // Verify application updated
      expect(prisma.application.update).toHaveBeenCalledWith(
        expect.objectContaining({
          where: { id: "app-inst" },
          data: expect.objectContaining({
            boardingStatus: "APPROVED",
            nmiApplicationId: "BRD_INST",
          }),
        })
      );

      // Verify welcome email queued
      expect(boss.send).toHaveBeenCalledWith(
        "send-email",
        expect.objectContaining({
          type: "welcome",
          merchantId: "merch-inst",
        })
      );
    });

    it("handles email failure gracefully", async () => {
      const prisma = makeMockPrisma();
      const boss = makeMockBoss();

      (boss.send as jest.Mock).mockRejectedValue(new Error("Email error"));

      const errorSpy = jest.spyOn(console, "error").mockImplementation();

      // Should not throw
      await handleInstantApproval(prisma, boss, "app-1", "merch-1", {
        nmiMerchantId: "MID1",
        securityKey: "SK1",
        tokenizationKey: "TK1",
      });

      errorSpy.mockRestore();

      // Merchant and application should still be updated
      expect(prisma.merchant.update).toHaveBeenCalled();
      expect(prisma.application.update).toHaveBeenCalled();
    });
  });

  // -----------------------------------------------------------------------
  // handlePendingReview
  // -----------------------------------------------------------------------

  describe("handlePendingReview", () => {
    it("updates application with NMI application ID and UNDER_REVIEW status", async () => {
      const prisma = makeMockPrisma();

      await handlePendingReview(prisma, "app-pend", "BRD_PEND_001");

      expect(prisma.application.update).toHaveBeenCalledWith({
        where: { id: "app-pend" },
        data: {
          boardingStatus: "UNDER_REVIEW",
          nmiApplicationId: "BRD_PEND_001",
        },
      });
    });
  });

  // -----------------------------------------------------------------------
  // registerBoardingStatusCheckJob
  // -----------------------------------------------------------------------

  describe("registerBoardingStatusCheckJob", () => {
    it("registers a scheduled job with pg-boss", async () => {
      const boss = makeMockBoss();
      const prisma = makeMockPrisma();

      await registerBoardingStatusCheckJob(boss, prisma);

      expect(boss.schedule).toHaveBeenCalledWith(
        BOARDING_STATUS_CHECK_JOB,
        BOARDING_STATUS_CHECK_CRON
      );
      expect(boss.work).toHaveBeenCalledWith(
        BOARDING_STATUS_CHECK_JOB,
        expect.any(Function)
      );
    });

    it("uses correct cron for every 15 minutes", () => {
      expect(BOARDING_STATUS_CHECK_CRON).toBe("*/15 * * * *");
    });

    it("uses correct job name", () => {
      expect(BOARDING_STATUS_CHECK_JOB).toBe("boarding-status-check");
    });
  });
});
