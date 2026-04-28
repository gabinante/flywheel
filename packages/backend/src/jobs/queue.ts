/**
 * Job Queue — pg-boss setup and monthly scheduling
 *
 * Schedules the residual-calculation job to run on the 1st of each
 * month at 00:00 UTC. The job computes residuals for the previous month.
 */

import type PgBoss from "pg-boss";
import {
  RESIDUAL_CALCULATION_JOB,
  getPreviousMonthRange,
  type ResidualJobData,
} from "./handlers.js";

// ─── Schedule ─────────────────────────────────────────────────────────

/**
 * Schedule the monthly residual calculation job via pg-boss cron.
 *
 * Cron: "0 0 1 * *" — 00:00 UTC on the 1st of every month.
 *
 * When the cron fires, it enqueues a residual-calculation job
 * with the previous month's date range.
 */
export async function scheduleMonthlyResidualJob(
  boss: PgBoss
): Promise<void> {
  const { periodStart, periodEnd } = getPreviousMonthRange();

  const jobData: ResidualJobData = {
    periodStart: periodStart.toISOString(),
    periodEnd: periodEnd.toISOString(),
  };

  await boss.schedule(RESIDUAL_CALCULATION_JOB, "0 0 1 * *", jobData, {
    tz: "UTC",
  });

  console.log(
    `[Queue] Scheduled monthly residual calculation: ` +
      `${RESIDUAL_CALCULATION_JOB} at "0 0 1 * *" UTC`
  );
}

// ─── Manual trigger ───────────────────────────────────────────────────

/**
 * Manually enqueue a residual calculation job for a given period.
 * Used by the admin endpoint for backfills and recalculations.
 *
 * Returns the pg-boss job ID.
 */
export async function enqueueResidualCalculation(
  boss: PgBoss,
  periodStart: Date,
  periodEnd: Date
): Promise<string | null> {
  const jobData: ResidualJobData = {
    periodStart: periodStart.toISOString(),
    periodEnd: periodEnd.toISOString(),
  };

  const jobId = await boss.send(RESIDUAL_CALCULATION_JOB, jobData, {
    singletonKey: `residual-${periodStart.toISOString()}-${periodEnd.toISOString()}`,
  });

  return jobId;
}
