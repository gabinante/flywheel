/**
 * Revenue Analytics Service
 *
 * Platform-level financial overview: NMI residual received, agency payouts,
 * and net retained revenue. Aggregates from DailyMetrics, ResidualEntry,
 * and ResidualPayout.
 */

import type { PrismaClient } from "@prisma/client";

// ─── Types ───────────────────────────────────────────────────────────

export type RevenuePeriod = "trailing_30d" | "trailing_12m" | "custom";

export interface RevenueAnalyticsParams {
  period: RevenuePeriod;
  startDate?: string; // ISO 8601 — required when period = "custom"
  endDate?: string;   // ISO 8601 — required when period = "custom"
}

export interface MonthlyBreakdown {
  month: string;    // "YYYY-MM"
  volume: number;   // cents
  nmiResidual: number; // cents
  agencyPayout: number; // cents
  retained: number; // cents
}

export interface RevenueAnalyticsResult {
  totalPlatformVolume: number;
  nmiResidualReceived: number;
  totalAgencyPayouts: number;
  netRetainedRevenue: number;
  activeMerchants: number;
  activeAgencies: number;
  newMerchantsThisMonth: number;
  newAgenciesThisMonth: number;
  chargebackRatio: number;
  monthlyBreakdown: MonthlyBreakdown[];
}

// ─── Service ──────────────────────────────────────────────────────────

export interface RevenueAnalyticsServiceDeps {
  prisma: PrismaClient;
}

export function createRevenueAnalyticsService(deps: RevenueAnalyticsServiceDeps) {
  const { prisma } = deps;

  /**
   * Resolve the date range from the period parameter.
   */
  function resolveDateRange(params: RevenueAnalyticsParams): { start: Date; end: Date } {
    const now = new Date();

    switch (params.period) {
      case "trailing_30d": {
        const end = new Date(now);
        const start = new Date(now);
        start.setDate(start.getDate() - 30);
        return { start, end };
      }
      case "trailing_12m": {
        const end = new Date(now);
        const start = new Date(now);
        start.setMonth(start.getMonth() - 12);
        start.setDate(1); // Start of month
        start.setHours(0, 0, 0, 0);
        return { start, end };
      }
      case "custom": {
        if (!params.startDate || !params.endDate) {
          throw new Error("startDate and endDate are required for custom period");
        }
        return {
          start: new Date(params.startDate),
          end: new Date(params.endDate),
        };
      }
      default:
        throw new Error(`Invalid period: ${params.period}`);
    }
  }

  /**
   * Aggregate platform volume from DailyMetrics.
   * Falls back to summing ResidualEntry.merchantVolume if DailyMetrics is empty.
   */
  async function getPlatformVolume(start: Date, end: Date): Promise<number> {
    try {
      const result = await prisma.dailyMetrics.aggregate({
        where: {
          date: { gte: start, lte: end },
        },
        _sum: { totalVolume: true },
      });
      const volume = result._sum.totalVolume ?? 0;

      // Fall back to ResidualEntry.merchantVolume if no DailyMetrics data
      if (volume === 0) {
        return await getVolumeFromResidualEntries(start, end);
      }
      return volume;
    } catch {
      // DailyMetrics table might not exist yet
      return await getVolumeFromResidualEntries(start, end);
    }
  }

  /**
   * Fallback: sum merchantVolume from ResidualEntry records.
   * Uses DISTINCT merchantId+periodStart to avoid double-counting
   * when there are two-tier entries for the same merchant.
   */
  async function getVolumeFromResidualEntries(start: Date, end: Date): Promise<number> {
    try {
      const result = await prisma.$queryRaw<[{ total: bigint | null }]>`
        SELECT COALESCE(SUM(sub."merchantVolume"), 0) AS total
        FROM (
          SELECT DISTINCT ON ("merchantId", "periodStart")
            "merchantVolume"
          FROM "ResidualEntry"
          WHERE "periodStart" >= ${start}
            AND "periodStart" < ${end}
          ORDER BY "merchantId", "periodStart", "createdAt" ASC
        ) sub
      `;
      return Number(result[0]?.total ?? 0);
    } catch {
      return 0;
    }
  }

  /**
   * Sum NMI residual earned from ResidualEntry records.
   */
  async function getNmiResidualReceived(start: Date, end: Date): Promise<number> {
    try {
      const result = await prisma.residualEntry.aggregate({
        where: {
          periodStart: { gte: start, lt: end },
        },
        _sum: { nmiResidualEarned: true },
      });
      return result._sum.nmiResidualEarned ?? 0;
    } catch {
      return 0;
    }
  }

  /**
   * Sum agency payouts (PAID status only) from ResidualPayout.
   */
  async function getTotalAgencyPayouts(start: Date, end: Date): Promise<number> {
    try {
      const result = await prisma.residualPayout.aggregate({
        where: {
          periodStart: { gte: start, lt: end },
          status: "PAID",
        },
        _sum: { totalAmount: true },
      });
      return result._sum.totalAmount ?? 0;
    } catch {
      return 0;
    }
  }

  /**
   * Get active merchant and agency counts.
   */
  async function getActiveCounts(): Promise<{
    activeMerchants: number;
    activeAgencies: number;
  }> {
    try {
      const [activeMerchants, activeAgencies] = await Promise.all([
        prisma.merchant.count({ where: { status: "ACTIVE" } }),
        prisma.agency.count({ where: { status: "ACTIVE" } }),
      ]);
      return { activeMerchants, activeAgencies };
    } catch {
      return { activeMerchants: 0, activeAgencies: 0 };
    }
  }

  /**
   * Get new merchants and agencies this month.
   */
  async function getNewThisMonth(): Promise<{
    newMerchantsThisMonth: number;
    newAgenciesThisMonth: number;
  }> {
    const now = new Date();
    const monthStart = new Date(now.getFullYear(), now.getMonth(), 1);
    try {
      const [newMerchants, newAgencies] = await Promise.all([
        prisma.merchant.count({
          where: { createdAt: { gte: monthStart } },
        }),
        prisma.agency.count({
          where: { createdAt: { gte: monthStart } },
        }),
      ]);
      return {
        newMerchantsThisMonth: newMerchants,
        newAgenciesThisMonth: newAgencies,
      };
    } catch {
      return { newMerchantsThisMonth: 0, newAgenciesThisMonth: 0 };
    }
  }

  /**
   * Calculate chargeback ratio from DailyMetrics (rolling 30 days).
   * chargebackRatio = total chargebacks / total transactions
   */
  async function getChargebackRatio(): Promise<number> {
    const now = new Date();
    const thirtyDaysAgo = new Date(now);
    thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30);

    try {
      const result = await prisma.dailyMetrics.aggregate({
        where: {
          date: { gte: thirtyDaysAgo, lte: now },
        },
        _sum: {
          totalTransactions: true,
          totalChargebacks: true,
        },
      });

      const transactions = result._sum.totalTransactions ?? 0;
      const chargebacks = result._sum.totalChargebacks ?? 0;

      if (transactions === 0) return 0;
      return Math.round((chargebacks / transactions) * 10000) / 10000; // 4 decimal places
    } catch {
      return 0;
    }
  }

  /**
   * Build monthly breakdown of volume, residual, payouts, and retained.
   * Uses UTC dates to avoid timezone issues.
   */
  async function getMonthlyBreakdown(start: Date, end: Date): Promise<MonthlyBreakdown[]> {
    const months: MonthlyBreakdown[] = [];

    // Generate list of months in the range using UTC
    const currentYear = start.getUTCFullYear();
    const currentMonth = start.getUTCMonth();
    const current = new Date(Date.UTC(currentYear, currentMonth, 1));
    const endMonth = new Date(Date.UTC(end.getUTCFullYear(), end.getUTCMonth(), 1));

    while (current <= endMonth) {
      const monthStr = `${current.getUTCFullYear()}-${String(current.getUTCMonth() + 1).padStart(2, "0")}`;
      const monthStart = new Date(current);
      const monthEnd = new Date(Date.UTC(current.getUTCFullYear(), current.getUTCMonth() + 1, 1));

      // Get volume for this month (from DailyMetrics or ResidualEntry fallback)
      let volume = 0;
      try {
        const dailyResult = await prisma.dailyMetrics.aggregate({
          where: {
            date: { gte: monthStart, lt: monthEnd },
          },
          _sum: { totalVolume: true },
        });
        volume = dailyResult._sum.totalVolume ?? 0;
      } catch {
        // ignore
      }

      if (volume === 0) {
        volume = await getVolumeFromResidualEntries(monthStart, monthEnd);
      }

      // Get NMI residual for this month
      let nmiResidual = 0;
      try {
        const residualResult = await prisma.residualEntry.aggregate({
          where: {
            periodStart: { gte: monthStart, lt: monthEnd },
          },
          _sum: { nmiResidualEarned: true },
        });
        nmiResidual = residualResult._sum.nmiResidualEarned ?? 0;
      } catch {
        // ignore
      }

      // Get agency payouts for this month
      let agencyPayout = 0;
      try {
        const payoutResult = await prisma.residualPayout.aggregate({
          where: {
            periodStart: { gte: monthStart, lt: monthEnd },
            status: "PAID",
          },
          _sum: { totalAmount: true },
        });
        agencyPayout = payoutResult._sum.totalAmount ?? 0;
      } catch {
        // ignore
      }

      months.push({
        month: monthStr,
        volume,
        nmiResidual,
        agencyPayout,
        retained: nmiResidual - agencyPayout,
      });

      current.setUTCMonth(current.getUTCMonth() + 1);
    }

    return months;
  }

  /**
   * Main entrypoint: get revenue analytics for a given period.
   */
  async function getRevenueAnalytics(
    params: RevenueAnalyticsParams
  ): Promise<RevenueAnalyticsResult> {
    const { start, end } = resolveDateRange(params);

    const [
      totalPlatformVolume,
      nmiResidualReceived,
      totalAgencyPayouts,
      activeCounts,
      newCounts,
      chargebackRatio,
      monthlyBreakdown,
    ] = await Promise.all([
      getPlatformVolume(start, end),
      getNmiResidualReceived(start, end),
      getTotalAgencyPayouts(start, end),
      getActiveCounts(),
      getNewThisMonth(),
      getChargebackRatio(),
      getMonthlyBreakdown(start, end),
    ]);

    return {
      totalPlatformVolume,
      nmiResidualReceived,
      totalAgencyPayouts,
      netRetainedRevenue: nmiResidualReceived - totalAgencyPayouts,
      activeMerchants: activeCounts.activeMerchants,
      activeAgencies: activeCounts.activeAgencies,
      newMerchantsThisMonth: newCounts.newMerchantsThisMonth,
      newAgenciesThisMonth: newCounts.newAgenciesThisMonth,
      chargebackRatio,
      monthlyBreakdown,
    };
  }

  return {
    getRevenueAnalytics,
    // Expose internals for testing
    resolveDateRange,
    getPlatformVolume,
    getNmiResidualReceived,
    getTotalAgencyPayouts,
    getActiveCounts,
    getNewThisMonth,
    getChargebackRatio,
    getMonthlyBreakdown,
  };
}
