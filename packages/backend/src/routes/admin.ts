/**
 * Admin Routes — manual residual calculation trigger
 *
 * POST /api/v1/admin/residuals/calculate
 *   Body: { periodStart: string, periodEnd: string }
 *   Returns: { entriesCreated, entriesSkipped, merchantsProcessed, errors }
 */

import { Router, type Request, type Response } from "express";
import { z } from "zod";
import type { PrismaClient } from "@prisma/client";
import {
  createResidualService,
  type NmiReportingClient,
} from "../services/residual.service.js";

// ─── Validation ───────────────────────────────────────────────────────

const calculateResidualsSchema = z.object({
  periodStart: z.string().datetime({ message: "periodStart must be ISO 8601" }),
  periodEnd: z.string().datetime({ message: "periodEnd must be ISO 8601" }),
});

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

  return router;
}
