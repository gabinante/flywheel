/**
 * Revenue Analytics Service — Unit Tests
 *
 * Tests the revenue analytics service logic including
 * aggregation, date range resolution, and monthly breakdown.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { createRevenueAnalyticsService } from "../services/revenue-analytics.service.js";

// ─── Mock Prisma ─────────────────────────────────────────────────────

function createMockPrisma() {
  return {
    dailyMetrics: {
      aggregate: vi.fn().mockResolvedValue({
        _sum: { totalVolume: null, totalTransactions: null, totalChargebacks: null },
      }),
    },
    residualEntry: {
      aggregate: vi.fn().mockResolvedValue({
        _sum: { nmiResidualEarned: null },
      }),
    },
    residualPayout: {
      aggregate: vi.fn().mockResolvedValue({
        _sum: { totalAmount: null },
      }),
    },
    merchant: {
      count: vi.fn().mockResolvedValue(0),
    },
    agency: {
      count: vi.fn().mockResolvedValue(0),
    },
    $queryRaw: vi.fn().mockResolvedValue([{ total: BigInt(0) }]),
  };
}

type MockPrisma = ReturnType<typeof createMockPrisma>;

describe("Revenue Analytics Service", () => {
  let mockPrisma: MockPrisma;
  let service: ReturnType<typeof createRevenueAnalyticsService>;

  beforeEach(() => {
    mockPrisma = createMockPrisma();
    service = createRevenueAnalyticsService({
      prisma: mockPrisma as unknown as import("@prisma/client").PrismaClient,
    });
  });

  // ─── Date Range Resolution ──────────────────────────────────────

  describe("resolveDateRange", () => {
    it("resolves trailing_30d to 30 days before now", () => {
      const { start, end } = service.resolveDateRange({ period: "trailing_30d" });
      const diff = end.getTime() - start.getTime();
      const thirtyDaysMs = 30 * 24 * 60 * 60 * 1000;
      expect(diff).toBeCloseTo(thirtyDaysMs, -3); // within 1 second
    });

    it("resolves trailing_12m to ~12 months before now", () => {
      const { start, end } = service.resolveDateRange({ period: "trailing_12m" });
      // Start should be 12 months ago, first of month
      expect(start.getDate()).toBe(1);
      expect(start.getHours()).toBe(0);
      expect(end.getTime()).toBeLessThanOrEqual(Date.now() + 1000);
    });

    it("resolves custom with provided dates", () => {
      const { start, end } = service.resolveDateRange({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-03-31T23:59:59.999Z",
      });
      expect(start.toISOString()).toBe("2026-01-01T00:00:00.000Z");
      expect(end.toISOString()).toBe("2026-03-31T23:59:59.999Z");
    });

    it("throws for custom period without dates", () => {
      expect(() => service.resolveDateRange({ period: "custom" })).toThrow(
        "startDate and endDate are required"
      );
    });
  });

  // ─── getRevenueAnalytics ────────────────────────────────────────

  describe("getRevenueAnalytics", () => {
    it("returns zeroes when no data exists", async () => {
      const result = await service.getRevenueAnalytics({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-01-31T23:59:59.999Z",
      });

      expect(result.totalPlatformVolume).toBe(0);
      expect(result.nmiResidualReceived).toBe(0);
      expect(result.totalAgencyPayouts).toBe(0);
      expect(result.netRetainedRevenue).toBe(0);
      expect(result.activeMerchants).toBe(0);
      expect(result.activeAgencies).toBe(0);
      expect(result.chargebackRatio).toBe(0);
      // Should contain January
      expect(result.monthlyBreakdown.some(m => m.month === "2026-01")).toBe(true);
    });

    it("calculates net retained as nmiResidual - agencyPayouts", async () => {
      // Setup: NMI residual = 45000 cents
      mockPrisma.residualEntry.aggregate.mockResolvedValue({
        _sum: { nmiResidualEarned: 45000 },
      });
      // Setup: agency payouts = 28000 cents
      mockPrisma.residualPayout.aggregate.mockResolvedValue({
        _sum: { totalAmount: 28000 },
      });

      const result = await service.getRevenueAnalytics({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-01-31T23:59:59.999Z",
      });

      expect(result.nmiResidualReceived).toBe(45000);
      expect(result.totalAgencyPayouts).toBe(28000);
      expect(result.netRetainedRevenue).toBe(17000);
    });

    it("uses DailyMetrics for platform volume when available", async () => {
      mockPrisma.dailyMetrics.aggregate.mockResolvedValue({
        _sum: { totalVolume: 15000000 },
      });

      const result = await service.getRevenueAnalytics({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-01-31T23:59:59.999Z",
      });

      expect(result.totalPlatformVolume).toBe(15000000);
    });

    it("falls back to ResidualEntry volume when DailyMetrics is empty", async () => {
      // DailyMetrics returns 0
      mockPrisma.dailyMetrics.aggregate.mockResolvedValue({
        _sum: { totalVolume: 0 },
      });
      // ResidualEntry fallback returns volume
      mockPrisma.$queryRaw.mockResolvedValue([{ total: BigInt(5000000) }]);

      const result = await service.getRevenueAnalytics({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-01-31T23:59:59.999Z",
      });

      expect(result.totalPlatformVolume).toBe(5000000);
    });

    it("returns active merchant and agency counts", async () => {
      mockPrisma.merchant.count.mockResolvedValue(142);
      mockPrisma.agency.count.mockResolvedValue(23);

      const result = await service.getRevenueAnalytics({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-01-31T23:59:59.999Z",
      });

      expect(result.activeMerchants).toBe(142);
      expect(result.activeAgencies).toBe(23);
    });

    it("calculates chargeback ratio from DailyMetrics", async () => {
      // Use mockImplementation to handle multiple parallel calls correctly
      mockPrisma.dailyMetrics.aggregate.mockImplementation(async (args: { _sum: Record<string, boolean> }) => {
        // When querying for transactions/chargebacks (chargeback ratio query)
        if (args?._sum?.totalTransactions) {
          return {
            _sum: { totalTransactions: 10000, totalChargebacks: 40 },
          };
        }
        // For volume queries
        return { _sum: { totalVolume: null } };
      });

      const result = await service.getRevenueAnalytics({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-01-31T23:59:59.999Z",
      });

      expect(result.chargebackRatio).toBe(0.004);
    });
  });

  // ─── Monthly Breakdown ──────────────────────────────────────────

  describe("getMonthlyBreakdown", () => {
    it("returns breakdown for each month in the range", async () => {
      const breakdown = await service.getMonthlyBreakdown(
        new Date("2026-01-01T00:00:00.000Z"),
        new Date("2026-03-31T23:59:59.999Z")
      );

      expect(breakdown).toHaveLength(3);
      expect(breakdown[0].month).toBe("2026-01");
      expect(breakdown[1].month).toBe("2026-02");
      expect(breakdown[2].month).toBe("2026-03");
    });

    it("calculates retained = nmiResidual - agencyPayout per month", async () => {
      // Per-month residual
      mockPrisma.residualEntry.aggregate.mockResolvedValue({
        _sum: { nmiResidualEarned: 36000 },
      });
      // Per-month payout
      mockPrisma.residualPayout.aggregate.mockResolvedValue({
        _sum: { totalAmount: 22000 },
      });
      // DailyMetrics volume
      mockPrisma.dailyMetrics.aggregate.mockResolvedValue({
        _sum: { totalVolume: 12000000 },
      });

      const breakdown = await service.getMonthlyBreakdown(
        new Date("2026-03-01T00:00:00.000Z"),
        new Date("2026-03-31T23:59:59.999Z")
      );

      expect(breakdown).toHaveLength(1);
      expect(breakdown[0]).toEqual({
        month: "2026-03",
        volume: 12000000,
        nmiResidual: 36000,
        agencyPayout: 22000,
        retained: 14000,
      });
    });
  });

  // ─── Chargeback Ratio ──────────────────────────────────────────

  describe("getChargebackRatio", () => {
    it("returns 0 when no transactions", async () => {
      mockPrisma.dailyMetrics.aggregate.mockResolvedValue({
        _sum: { totalTransactions: 0, totalChargebacks: 0 },
      });

      const ratio = await service.getChargebackRatio();
      expect(ratio).toBe(0);
    });

    it("calculates correct ratio", async () => {
      mockPrisma.dailyMetrics.aggregate.mockResolvedValue({
        _sum: { totalTransactions: 25000, totalChargebacks: 100 },
      });

      const ratio = await service.getChargebackRatio();
      expect(ratio).toBe(0.004);
    });

    it("handles DailyMetrics errors gracefully", async () => {
      mockPrisma.dailyMetrics.aggregate.mockRejectedValue(new Error("Table not found"));

      const ratio = await service.getChargebackRatio();
      expect(ratio).toBe(0);
    });
  });

  // ─── Seeded Data Integration Test ──────────────────────────────

  describe("acceptance test scenario — 3 months of seeded data", () => {
    beforeEach(() => {
      // Simulate 3 months of DailyMetrics
      const monthlyVolumes: Record<string, number> = {
        "2026-01": 4000000,
        "2026-02": 5000000,
        "2026-03": 6000000,
      };

      // DailyMetrics aggregate will return volume based on date range
      mockPrisma.dailyMetrics.aggregate.mockImplementation(async (args: { where?: { date?: { gte?: Date; lt?: Date; lte?: Date } }; _sum: Record<string, boolean> }) => {
        const where = args?.where;
        const sum = args?._sum;

        // For chargeback ratio query
        if (sum?.totalTransactions) {
          return {
            _sum: { totalTransactions: 30000, totalChargebacks: 120 },
          };
        }

        // For volume queries
        if (where?.date?.gte && (where?.date?.lt || where?.date?.lte)) {
          const start = new Date(where.date.gte);
          const end = new Date(where.date.lt || where.date.lte);
          let total = 0;
          for (const [month, vol] of Object.entries(monthlyVolumes)) {
            const monthDate = new Date(`${month}-01T00:00:00.000Z`);
            if (monthDate >= start && monthDate < end) {
              total += vol;
            }
          }
          return { _sum: { totalVolume: total } };
        }

        return { _sum: { totalVolume: 15000000 } };
      });

      // ResidualEntry aggregate — NMI residual earned per month
      const monthlyResiduals: Record<string, number> = {
        "2026-01": 12000,
        "2026-02": 15000,
        "2026-03": 18000,
      };

      mockPrisma.residualEntry.aggregate.mockImplementation(async (args: { where?: { periodStart?: { gte?: Date; lt?: Date } } }) => {
        const where = args?.where;
        if (where?.periodStart?.gte && where?.periodStart?.lt) {
          const start = new Date(where.periodStart.gte);
          const end = new Date(where.periodStart.lt);
          let total = 0;
          for (const [month, res] of Object.entries(monthlyResiduals)) {
            const monthDate = new Date(`${month}-01T00:00:00.000Z`);
            if (monthDate >= start && monthDate < end) {
              total += res;
            }
          }
          return { _sum: { nmiResidualEarned: total } };
        }
        return { _sum: { nmiResidualEarned: 45000 } };
      });

      // ResidualPayout aggregate — payouts per month
      const monthlyPayouts: Record<string, number> = {
        "2026-01": 8000,
        "2026-02": 10000,
        "2026-03": 10000,
      };

      mockPrisma.residualPayout.aggregate.mockImplementation(async (args: { where?: { periodStart?: { gte?: Date; lt?: Date } } }) => {
        const where = args?.where;
        if (where?.periodStart?.gte && where?.periodStart?.lt) {
          const start = new Date(where.periodStart.gte);
          const end = new Date(where.periodStart.lt);
          let total = 0;
          for (const [month, pay] of Object.entries(monthlyPayouts)) {
            const monthDate = new Date(`${month}-01T00:00:00.000Z`);
            if (monthDate >= start && monthDate < end) {
              total += pay;
            }
          }
          return { _sum: { totalAmount: total } };
        }
        return { _sum: { totalAmount: 28000 } };
      });

      // Active counts
      mockPrisma.merchant.count.mockResolvedValue(142);
      mockPrisma.agency.count.mockResolvedValue(23);
    });

    it("totalPlatformVolume matches sum of DailyMetrics", async () => {
      const result = await service.getRevenueAnalytics({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-03-31T23:59:59.999Z",
      });

      expect(result.totalPlatformVolume).toBe(15000000);
    });

    it("nmiResidualReceived matches sum of ResidualEntry.nmiResidualEarned", async () => {
      const result = await service.getRevenueAnalytics({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-03-31T23:59:59.999Z",
      });

      expect(result.nmiResidualReceived).toBe(45000);
    });

    it("totalAgencyPayouts matches sum of ResidualPayout.totalAmount (PAID)", async () => {
      const result = await service.getRevenueAnalytics({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-03-31T23:59:59.999Z",
      });

      expect(result.totalAgencyPayouts).toBe(28000);
    });

    it("netRetainedRevenue = nmiResidualReceived - totalAgencyPayouts", async () => {
      const result = await service.getRevenueAnalytics({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-03-31T23:59:59.999Z",
      });

      expect(result.netRetainedRevenue).toBe(45000 - 28000);
      expect(result.netRetainedRevenue).toBe(17000);
    });

    it("monthlyBreakdown has correct per-month figures", async () => {
      const result = await service.getRevenueAnalytics({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-03-31T23:59:59.999Z",
      });

      expect(result.monthlyBreakdown).toHaveLength(3);

      expect(result.monthlyBreakdown[0]).toEqual({
        month: "2026-01",
        volume: 4000000,
        nmiResidual: 12000,
        agencyPayout: 8000,
        retained: 4000,
      });

      expect(result.monthlyBreakdown[1]).toEqual({
        month: "2026-02",
        volume: 5000000,
        nmiResidual: 15000,
        agencyPayout: 10000,
        retained: 5000,
      });

      expect(result.monthlyBreakdown[2]).toEqual({
        month: "2026-03",
        volume: 6000000,
        nmiResidual: 18000,
        agencyPayout: 10000,
        retained: 8000,
      });
    });

    it("chargeback ratio is correct from rolling 30-day window", async () => {
      const result = await service.getRevenueAnalytics({
        period: "custom",
        startDate: "2026-01-01T00:00:00.000Z",
        endDate: "2026-03-31T23:59:59.999Z",
      });

      // 120 chargebacks / 30000 transactions = 0.004
      expect(result.chargebackRatio).toBe(0.004);
    });
  });
});
