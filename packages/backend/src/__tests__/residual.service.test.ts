/**
 * Residual Calculation Service — Unit Tests
 *
 * Verifies the acceptance criteria from gohighpayment-9:
 *
 * Seed: Agency A (TIER_1, 10bps) with merchant M1 ($100,000 volume),
 *       Agency B (TIER_2, 12bps, referred by A) with merchant M2 ($50,000 volume).
 *
 * Run calculation for month. Verify:
 * (1) ResidualEntry for A+M1: agencyShare = 1000 cents ($10)
 * (2) ResidualEntry for B+M2: agencyShare = 600 cents ($6)
 * (3) ResidualEntry for A as two-tier referrer of B+M2: twoTierShare = 150 cents ($1.50)
 * (4) Re-running same period creates no duplicates (unique constraint)
 * (5) AuditLog entry exists for the run
 */

import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  createResidualService,
  TIER_BPS,
  TWO_TIER_BPS,
  type NmiReportingClient,
} from "../services/residual.service.js";

// ─── Mock Prisma ─────────���─────────────────────────���──────────────────

/**
 * In-memory store that simulates Prisma operations.
 * Enforces the @@unique([agencyId, merchantId, periodStart]) constraint.
 */
function createMockPrisma(opts: {
  merchants: Array<{
    id: string;
    name: string;
    nmiMerchantId: string | null;
    status: string;
    agencyId: string | null;
    agency: {
      id: string;
      tier: "TIER_1" | "TIER_2" | "TIER_3";
      referredByAgencyId: string | null;
      referredByAgency: { id: string } | null;
    } | null;
  }>;
}) {
  const residualEntries: Array<Record<string, unknown>> = [];
  const auditLogs: Array<Record<string, unknown>> = [];
  const uniqueKeys = new Set<string>();

  return {
    _residualEntries: residualEntries,
    _auditLogs: auditLogs,
    _uniqueKeys: uniqueKeys,

    merchant: {
      findMany: vi.fn().mockResolvedValue(opts.merchants),
    },

    residualEntry: {
      create: vi.fn().mockImplementation(
        async ({ data }: { data: Record<string, unknown> }) => {
          const key = [
            data.agencyId,
            data.merchantId,
            (data.periodStart as Date).toISOString(),
          ].join("|");

          if (uniqueKeys.has(key)) {
            const err = new Error(
              "Unique constraint failed on the fields: (`agencyId`,`merchantId`,`periodStart`)"
            );
            (err as unknown as { code: string }).code = "P2002";
            throw err;
          }

          uniqueKeys.add(key);
          const entry = {
            id: `re_${residualEntries.length + 1}`,
            ...data,
          };
          residualEntries.push(entry);
          return entry;
        }
      ),
    },

    // DB fallback for volume — returns 0 (NMI client handles volume)
    $queryRaw: vi.fn().mockResolvedValue([{ total: BigInt(0) }]),

    // Audit log insert
    $executeRaw: vi.fn().mockImplementation(async () => {
      auditLogs.push({
        action: "RESIDUAL_CALCULATION",
        timestamp: new Date(),
      });
      return 1;
    }),
  };
}

// ─── Test Constants ──────────────���────────────────────────────────────

const AGENCY_A_ID = "agency-a-tier1";
const AGENCY_B_ID = "agency-b-tier2";
const MERCHANT_M1_ID = "merchant-m1";
const MERCHANT_M2_ID = "merchant-m2";

const PERIOD_START = new Date("2026-03-01T00:00:00.000Z");
const PERIOD_END = new Date("2026-04-01T00:00:00.000Z");

// Volumes in cents
const M1_VOLUME_CENTS = 10_000_000; // $100,000
const M2_VOLUME_CENTS = 5_000_000; // $50,000

/**
 * Seed data matching the acceptance test:
 * - Agency A: TIER_1 (rate 10), no referrer, with merchant M1
 * - Agency B: TIER_2 (rate 12), referred by A, with merchant M2
 */
function createTestMerchants() {
  return [
    {
      id: MERCHANT_M1_ID,
      name: "Merchant M1",
      nmiMerchantId: "nmi-m1",
      status: "ACTIVE",
      agencyId: AGENCY_A_ID,
      agency: {
        id: AGENCY_A_ID,
        tier: "TIER_1" as const,
        referredByAgencyId: null,
        referredByAgency: null,
      },
    },
    {
      id: MERCHANT_M2_ID,
      name: "Merchant M2",
      nmiMerchantId: "nmi-m2",
      status: "ACTIVE",
      agencyId: AGENCY_B_ID,
      agency: {
        id: AGENCY_B_ID,
        tier: "TIER_2" as const,
        referredByAgencyId: AGENCY_A_ID,
        referredByAgency: { id: AGENCY_A_ID },
      },
    },
  ];
}

/** NMI mock returning known volumes for each merchant */
function createTestNmiClient(): NmiReportingClient {
  return {
    getMerchantVolume: vi.fn().mockImplementation(
      async (nmiMerchantId: string) => {
        const volumes: Record<string, number> = {
          "nmi-m1": M1_VOLUME_CENTS,
          "nmi-m2": M2_VOLUME_CENTS,
        };
        return volumes[nmiMerchantId] ?? 0;
      }
    ),
  };
}

// ─── Tests ────────────────────────────────────────────��───────────────

describe("Residual Calculation Service", () => {
  describe("constants", () => {
    it("maps tier to correct bps rates", () => {
      expect(TIER_BPS.TIER_1).toBe(10);
      expect(TIER_BPS.TIER_2).toBe(12);
      expect(TIER_BPS.TIER_3).toBe(15);
    });

    it("TWO_TIER_BPS is 3", () => {
      expect(TWO_TIER_BPS).toBe(3);
    });
  });

  describe("calculateResiduals — full acceptance test", () => {
    let mockPrisma: ReturnType<typeof createMockPrisma>;
    let nmiClient: NmiReportingClient;
    let service: ReturnType<typeof createResidualService>;

    beforeEach(() => {
      mockPrisma = createMockPrisma({
        merchants: createTestMerchants(),
      });
      nmiClient = createTestNmiClient();
      service = createResidualService({
        prisma:
          mockPrisma as unknown as import("@prisma/client").PrismaClient,
        nmiClient,
      });
    });

    it("(1) A+M1: agencyShare = 1000 cents ($10) at TIER_1 10bps on $100k", async () => {
      await service.calculateResiduals(PERIOD_START, PERIOD_END);

      const entry = mockPrisma._residualEntries.find(
        (e) =>
          e.agencyId === AGENCY_A_ID &&
          e.merchantId === MERCHANT_M1_ID &&
          e.twoTierAgencyId === null
      );

      expect(entry).toBeDefined();
      expect(entry!.merchantVolume).toBe(M1_VOLUME_CENTS);
      expect(entry!.agencyBps).toBe(10);
      expect(entry!.agencyShare).toBe(1000);
      expect(entry!.status).toBe("PENDING");
    });

    it("(2) B+M2: agencyShare = 600 cents ($6) at TIER_2 12bps on $50k", async () => {
      await service.calculateResiduals(PERIOD_START, PERIOD_END);

      const entry = mockPrisma._residualEntries.find(
        (e) =>
          e.agencyId === AGENCY_B_ID &&
          e.merchantId === MERCHANT_M2_ID
      );

      expect(entry).toBeDefined();
      expect(entry!.merchantVolume).toBe(M2_VOLUME_CENTS);
      expect(entry!.agencyBps).toBe(12);
      expect(entry!.agencyShare).toBe(600);
      expect(entry!.status).toBe("PENDING");
    });

    it("(3) A as two-tier referrer of B+M2: twoTierShare = 150 cents ($1.50)", async () => {
      await service.calculateResiduals(PERIOD_START, PERIOD_END);

      const entry = mockPrisma._residualEntries.find(
        (e) =>
          e.agencyId === AGENCY_A_ID &&
          e.merchantId === MERCHANT_M2_ID &&
          e.twoTierAgencyId === AGENCY_B_ID
      );

      expect(entry).toBeDefined();
      expect(entry!.twoTierShare).toBe(150);
      expect(entry!.agencyBps).toBe(TWO_TIER_BPS);
      expect(entry!.agencyShare).toBe(0); // two-tier entries have agencyShare=0
      expect(entry!.status).toBe("PENDING");
    });

    it("(4) re-running same period creates no duplicates", async () => {
      // First run
      const firstRun = await service.calculateResiduals(
        PERIOD_START,
        PERIOD_END
      );
      expect(firstRun.entriesCreated).toBe(3);
      expect(firstRun.entriesSkipped).toBe(0);

      // Second run — same period → unique constraint catches duplicates
      const secondRun = await service.calculateResiduals(
        PERIOD_START,
        PERIOD_END
      );
      expect(secondRun.entriesCreated).toBe(0);
      expect(secondRun.entriesSkipped).toBe(3);

      // Total entries should still be 3 (no duplicates)
      expect(mockPrisma._residualEntries.length).toBe(3);
    });

    it("(5) AuditLog entry exists for the run", async () => {
      await service.calculateResiduals(PERIOD_START, PERIOD_END);

      expect(mockPrisma._auditLogs.length).toBeGreaterThan(0);
      expect(mockPrisma.$executeRaw).toHaveBeenCalled();
    });

    it("creates exactly 3 entries total (A+M1, B+M2, A-ref+M2)", async () => {
      const result = await service.calculateResiduals(
        PERIOD_START,
        PERIOD_END
      );

      expect(result.entriesCreated).toBe(3);
      expect(result.merchantsProcessed).toBe(2);
      expect(result.errors).toHaveLength(0);
      expect(mockPrisma._residualEntries.length).toBe(3);
    });

    it("all amounts are integer cents (no floating point)", async () => {
      await service.calculateResiduals(PERIOD_START, PERIOD_END);

      for (const entry of mockPrisma._residualEntries) {
        expect(Number.isInteger(entry.merchantVolume)).toBe(true);
        expect(Number.isInteger(entry.agencyShare)).toBe(true);
        expect(Number.isInteger(entry.twoTierShare)).toBe(true);
        expect(Number.isInteger(entry.agencyBps)).toBe(true);
      }
    });
  });

  describe("edge cases", () => {
    it("skips merchants with zero volume", async () => {
      const mockPrisma = createMockPrisma({
        merchants: [
          {
            id: "m-zero",
            name: "Zero Volume",
            nmiMerchantId: "nmi-zero",
            status: "ACTIVE",
            agencyId: "agency-x",
            agency: {
              id: "agency-x",
              tier: "TIER_1",
              referredByAgencyId: null,
              referredByAgency: null,
            },
          },
        ],
      });

      // NMI returns 0 volume
      const nmiClient: NmiReportingClient = {
        getMerchantVolume: vi.fn().mockResolvedValue(0),
      };

      const service = createResidualService({
        prisma:
          mockPrisma as unknown as import("@prisma/client").PrismaClient,
        nmiClient,
      });

      const result = await service.calculateResiduals(
        PERIOD_START,
        PERIOD_END
      );

      expect(result.merchantsProcessed).toBe(1);
      expect(result.entriesCreated).toBe(0);
    });

    it("falls back to DB when NMI client throws", async () => {
      const mockPrisma = createMockPrisma({
        merchants: [
          {
            id: "m-fallback",
            name: "Fallback Merchant",
            nmiMerchantId: "nmi-fail",
            status: "ACTIVE",
            agencyId: "agency-fb",
            agency: {
              id: "agency-fb",
              tier: "TIER_3",
              referredByAgencyId: null,
              referredByAgency: null,
            },
          },
        ],
      });

      // NMI throws; DB returns volume via $queryRaw
      const failingNmiClient: NmiReportingClient = {
        getMerchantVolume: vi
          .fn()
          .mockRejectedValue(new Error("NMI API down")),
      };

      // DB fallback returns 2,000,000 cents ($20k)
      mockPrisma.$queryRaw = vi
        .fn()
        .mockResolvedValue([{ total: BigInt(2_000_000) }]);

      const service = createResidualService({
        prisma:
          mockPrisma as unknown as import("@prisma/client").PrismaClient,
        nmiClient: failingNmiClient,
      });

      const result = await service.calculateResiduals(
        PERIOD_START,
        PERIOD_END
      );

      expect(result.entriesCreated).toBe(1);
      // TIER_3 at 15 rate: floor(2_000_000 * 15 / 100_000) = 300 cents
      const entry = mockPrisma._residualEntries[0];
      expect(entry.agencyShare).toBe(300);
      expect(entry.merchantVolume).toBe(2_000_000);
    });

    it("falls back to DB when NMI client is not provided", async () => {
      const mockPrisma = createMockPrisma({
        merchants: [
          {
            id: "m-no-nmi",
            name: "No NMI",
            nmiMerchantId: null,
            status: "ACTIVE",
            agencyId: "agency-nn",
            agency: {
              id: "agency-nn",
              tier: "TIER_2",
              referredByAgencyId: null,
              referredByAgency: null,
            },
          },
        ],
      });

      // DB returns 1,000,000 cents ($10k)
      mockPrisma.$queryRaw = vi
        .fn()
        .mockResolvedValue([{ total: BigInt(1_000_000) }]);

      // No NMI client
      const service = createResidualService({
        prisma:
          mockPrisma as unknown as import("@prisma/client").PrismaClient,
      });

      const result = await service.calculateResiduals(
        PERIOD_START,
        PERIOD_END
      );

      expect(result.entriesCreated).toBe(1);
      // TIER_2 at 12 rate: floor(1_000_000 * 12 / 100_000) = 120 cents
      expect(mockPrisma._residualEntries[0].agencyShare).toBe(120);
    });

    it("handles errors for individual merchants gracefully", async () => {
      const mockPrisma = createMockPrisma({
        merchants: [
          {
            id: "m-err",
            name: "Error Merchant",
            nmiMerchantId: "nmi-err",
            status: "ACTIVE",
            agencyId: "agency-err",
            agency: {
              id: "agency-err",
              tier: "TIER_1",
              referredByAgencyId: null,
              referredByAgency: null,
            },
          },
        ],
      });

      // NMI throws a non-recoverable error
      const nmiClient: NmiReportingClient = {
        getMerchantVolume: vi
          .fn()
          .mockRejectedValue(new Error("NMI timeout")),
      };

      // DB also throws
      mockPrisma.$queryRaw = vi
        .fn()
        .mockRejectedValue(new Error("DB connection lost"));

      const service = createResidualService({
        prisma:
          mockPrisma as unknown as import("@prisma/client").PrismaClient,
        nmiClient,
      });

      const result = await service.calculateResiduals(
        PERIOD_START,
        PERIOD_END
      );

      expect(result.errors).toHaveLength(1);
      expect(result.errors[0].merchantId).toBe("m-err");
      expect(result.errors[0].error).toContain("DB connection lost");
    });

    it("TIER_3 at 15bps calculates correctly", async () => {
      const mockPrisma = createMockPrisma({
        merchants: [
          {
            id: "m-t3",
            name: "Tier 3 Merchant",
            nmiMerchantId: "nmi-t3",
            status: "ACTIVE",
            agencyId: "agency-t3",
            agency: {
              id: "agency-t3",
              tier: "TIER_3",
              referredByAgencyId: null,
              referredByAgency: null,
            },
          },
        ],
      });

      const nmiClient: NmiReportingClient = {
        getMerchantVolume: vi.fn().mockResolvedValue(20_000_000), // $200k
      };

      const service = createResidualService({
        prisma:
          mockPrisma as unknown as import("@prisma/client").PrismaClient,
        nmiClient,
      });

      await service.calculateResiduals(PERIOD_START, PERIOD_END);

      const entry = mockPrisma._residualEntries[0];
      expect(entry.agencyBps).toBe(15);
      // floor(20_000_000 * 15 / 100_000) = 3000 cents = $30
      expect(entry.agencyShare).toBe(3000);
    });
  });
});
