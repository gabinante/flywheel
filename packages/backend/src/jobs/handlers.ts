/**
 * Job Handlers
 *
 * Registers pg-boss job handlers for background processing.
 * The residual-calculation job runs monthly (1st of month, 00:00 UTC)
 * or can be triggered manually via admin endpoint.
 */

import type PgBoss from "pg-boss";
import type { PrismaClient } from "@prisma/client";
import {
  createResidualService,
  type NmiReportingClient,
} from "../services/residual.service.js";

// ─── Job Names ────────────────────────────────────────────────────────

export const RESIDUAL_CALCULATION_JOB = "residual-calculation";

// ─── Types ────────────────────────────────────────────────────────────

export interface ResidualJobData {
  periodStart: string; // ISO 8601
  periodEnd: string; // ISO 8601
}

export interface JobHandlerDeps {
  prisma: PrismaClient;
  nmiClient?: NmiReportingClient;
}

// ─── Registration ─────────────────────────────────────────────────────

/**
 * Register all job handlers with pg-boss.
 */
export async function registerJobHandlers(
  boss: PgBoss,
  deps: JobHandlerDeps
): Promise<void> {
  const residualService = createResidualService({
    prisma: deps.prisma,
    nmiClient: deps.nmiClient,
  });

  await boss.work<ResidualJobData>(
    RESIDUAL_CALCULATION_JOB,
    async (jobs) => {
      // pg-boss v10 delivers an array of jobs to the handler
      for (const job of jobs) {
        const { periodStart, periodEnd } = job.data;

        console.log(
          `[ResidualJob] Starting calculation for period ${periodStart} — ${periodEnd}`
        );

        const result = await residualService.calculateResiduals(
          new Date(periodStart),
          new Date(periodEnd)
        );

        console.log(
          `[ResidualJob] Completed: ${result.entriesCreated} entries created, ` +
            `${result.entriesSkipped} skipped, ` +
            `${result.merchantsProcessed} merchants processed, ` +
            `${result.errors.length} errors`
        );

        if (result.errors.length > 0) {
          console.warn(
            "[ResidualJob] Errors:",
            JSON.stringify(result.errors)
          );
        }
      }
    }
  );
}

// ─── Job helpers ──────────────────────────────────────────────────────

/**
 * Compute the first and last moments of the previous month.
 * Used by the scheduled job to determine the calculation period.
 */
export function getPreviousMonthRange(): {
  periodStart: Date;
  periodEnd: Date;
} {
  const now = new Date();
  const year = now.getUTCFullYear();
  const month = now.getUTCMonth(); // 0-indexed, so this is already "previous" month for getUTCMonth()

  // First day of previous month at 00:00:00 UTC
  const periodStart = new Date(Date.UTC(year, month - 1, 1, 0, 0, 0, 0));

  // First day of current month at 00:00:00 UTC (exclusive end)
  const periodEnd = new Date(Date.UTC(year, month, 1, 0, 0, 0, 0));

  return { periodStart, periodEnd };
}
