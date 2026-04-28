/**
 * Admin Routes — residual calculation trigger and revenue analytics
 *
 * POST /api/v1/admin/residuals/calculate
 *   Body: { periodStart: string, periodEnd: string }
 *   Returns: { entriesCreated, entriesSkipped, merchantsProcessed, errors }
 *
 * GET /api/v1/admin/analytics/revenue
 *   Query: { period: trailing_30d|trailing_12m|custom, startDate?, endDate? }
 *   Returns: platform volume, NMI residual, agency payouts, net retained, etc.
 */

import { Router, type Request, type Response } from "express";
import { z } from "zod";
import type { PrismaClient } from "@prisma/client";
import {
  createResidualService,
  type NmiReportingClient,
} from "../services/residual.service.js";
import {
  createRevenueAnalyticsService,
  type RevenuePeriod,
} from "../services/revenue-analytics.service.js";

// ─── Validation ───────────────────────────────────────────────────────

const calculateResidualsSchema = z.object({
  periodStart: z.string().datetime({ message: "periodStart must be ISO 8601" }),
  periodEnd: z.string().datetime({ message: "periodEnd must be ISO 8601" }),
});

const revenueAnalyticsSchema = z.object({
  period: z.enum(["trailing_30d", "trailing_12m", "custom"]),
  startDate: z.string().optional(),
  endDate: z.string().optional(),
}).refine(
  (data) => {
    if (data.period === "custom") {
      return !!data.startDate && !!data.endDate;
    }
    return true;
  },
  { message: "startDate and endDate are required when period is 'custom'" }
);

// ─── Router factory ───────────────────────────────────────────────────

export interface AdminRouterDeps {
  prisma: PrismaClient;
  nmiClient?: NmiReportingClient;
}

export function createAdminRouter(deps: AdminRouterDeps): Router {
  const router = Router();
  const residualService = createResidualService({
    prisma: deps.prisma,
    nmiClient: deps.nmiClient,
  });
  const revenueAnalyticsService = createRevenueAnalyticsService({
    prisma: deps.prisma,
  });

  /**
   * POST /api/v1/admin/residuals/calculate
   *
   * Trigger a manual residual calculation for a given period.
   * Useful for backfills and recalculations.
   *
   * Body:
   *   periodStart — ISO 8601 date string (inclusive)
   *   periodEnd   — ISO 8601 date string (exclusive)
   *
   * Returns the count of entries created and details.
   */
  router.post(
    "/residuals/calculate",
    async (req: Request, res: Response): Promise<void> => {
      // Validate request body
      const parsed = calculateResidualsSchema.safeParse(req.body);
      if (!parsed.success) {
        res.status(400).json({
          error: "Invalid request body",
          details: parsed.error.issues,
        });
        return;
      }

      const periodStart = new Date(parsed.data.periodStart);
      const periodEnd = new Date(parsed.data.periodEnd);

      // Validate date range
      if (periodEnd <= periodStart) {
        res.status(400).json({
          error: "periodEnd must be after periodStart",
        });
        return;
      }

      try {
        const result = await residualService.calculateResiduals(
          periodStart,
          periodEnd
        );

        res.status(200).json({
          success: true,
          ...result,
        });
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] Residual calculation failed:", message);
        res.status(500).json({
          error: "Residual calculation failed",
          message,
        });
      }
    }
  );

  /**
   * GET /api/v1/admin/analytics/revenue
   *
   * Platform-level financial overview: NMI residual received, agency payouts,
   * net retained revenue, active counts, chargeback ratio, monthly breakdown.
   *
   * Query params:
   *   period    — "trailing_30d" | "trailing_12m" | "custom"
   *   startDate — ISO 8601 (required when period=custom)
   *   endDate   — ISO 8601 (required when period=custom)
   */
  router.get(
    "/analytics/revenue",
    async (req: Request, res: Response): Promise<void> => {
      const parsed = revenueAnalyticsSchema.safeParse(req.query);
      if (!parsed.success) {
        res.status(400).json({
          error: "Invalid query parameters",
          details: parsed.error.issues,
        });
        return;
      }

      try {
        const result = await revenueAnalyticsService.getRevenueAnalytics({
          period: parsed.data.period as RevenuePeriod,
          startDate: parsed.data.startDate,
          endDate: parsed.data.endDate,
        });

        res.status(200).json(result);
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] Revenue analytics failed:", message);
        res.status(500).json({
          error: "Revenue analytics query failed",
          message,
        });
      }
    }
  );

  return router;
}
