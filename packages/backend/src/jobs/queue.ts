/**
 * Job Queue — pg-boss setup and monthly scheduling
 *
 * Schedules the residual-calculation job to run on the 1st of each
 * month at 00:00 UTC. The job computes residuals for the previous month.
 *
 * Also provides email queueing functions for the email-send job.
 */

import type PgBoss from "pg-boss";
import type { PrismaClient } from "@prisma/client";
import {
  RESIDUAL_CALCULATION_JOB,
  EMAIL_SEND_JOB,
  getPreviousMonthRange,
  type ResidualJobData,
  type EmailSendJobData,
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

// ─── Email queue ──────────────────────────────────────────────────────

export interface EnqueueEmailParams {
  to: string;
  templateId: string;
  variables: Record<string, unknown>;
  merchantId?: string;
  agencyId?: string;
}

/**
 * Enqueue a transactional email for delivery via pg-boss.
 *
 * Creates a NotificationSchedule record first, then enqueues the job.
 * Email failures NEVER block the calling code — errors are caught and logged.
 *
 * Returns the notification record ID (or null if queueing failed).
 */
export async function enqueueEmail(
  boss: PgBoss,
  prisma: PrismaClient,
  params: EnqueueEmailParams
): Promise<string | null> {
  const { to, templateId, variables, merchantId, agencyId } = params;

  try {
    // 1. Create NotificationSchedule record
    const notification = await prisma.notificationSchedule.create({
      data: {
        to,
        templateId,
        variables: variables as any,
        merchantId: merchantId ?? null,
        agencyId: agencyId ?? null,
        status: "QUEUED",
      },
    });

    // 2. Enqueue the email-send job
    const jobData: EmailSendJobData = {
      to,
      templateId,
      variables,
      merchantId,
      agencyId,
      notificationId: notification.id,
    };

    await boss.send(EMAIL_SEND_JOB, jobData, {
      retryLimit: 3,
      retryDelay: 30, // 30 seconds between retries
      retryBackoff: true, // exponential backoff
      expireInMinutes: 60, // expire after 1 hour
    });

    console.log(
      `[Queue] Enqueued email ${templateId} to ${to} (notification: ${notification.id})`
    );

    return notification.id;
  } catch (err: unknown) {
    // Email queueing failures must NEVER break calling code
    const message = err instanceof Error ? err.message : String(err);
    console.error(
      `[Queue] Failed to enqueue email ${templateId} to ${to}: ${message}`
    );
    return null;
  }
}

/**
 * Enqueue a transactional email without Prisma tracking.
 *
 * Use this when you don't have Prisma available or don't need
 * NotificationSchedule tracking (e.g., in lightweight jobs).
 */
export async function enqueueEmailSimple(
  boss: PgBoss,
  params: EnqueueEmailParams
): Promise<string | null> {
  const { to, templateId, variables, merchantId, agencyId } = params;

  try {
    const jobData: EmailSendJobData = {
      to,
      templateId,
      variables,
      merchantId,
      agencyId,
    };

    const jobId = await boss.send(EMAIL_SEND_JOB, jobData, {
      retryLimit: 3,
      retryDelay: 30,
      retryBackoff: true,
      expireInMinutes: 60,
    });

    return jobId;
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    console.error(
      `[Queue] Failed to enqueue email ${templateId} to ${to}: ${message}`
    );
    return null;
  }
}
