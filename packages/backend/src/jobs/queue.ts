/**
 * Job Queue — pg-boss setup and monthly scheduling
 *
 * Schedules the residual-calculation job to run on the 1st of each
 * month at 00:00 UTC. The job computes residuals for the previous month.
 *
 * Also provides email queueing functions for the email-send job.
 * All operations use structured logging via an injected logger.
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

/** Logger interface — accepts any pino-compatible logger */
interface QueueLogger {
  info(obj: Record<string, unknown>, msg: string): void;
  warn(obj: Record<string, unknown>, msg: string): void;
  error(obj: Record<string, unknown>, msg: string): void;
}

// ─── Schedule ─────────────────────────────────────────────────────────

/**
 * Schedule the monthly residual calculation job via pg-boss cron.
 *
 * Cron: "0 0 1 * *" — 00:00 UTC on the 1st of every month.
 */
export async function scheduleMonthlyResidualJob(
  boss: PgBoss,
  logger?: QueueLogger
): Promise<void> {
  const { periodStart, periodEnd } = getPreviousMonthRange();

  const jobData: ResidualJobData = {
    periodStart: periodStart.toISOString(),
    periodEnd: periodEnd.toISOString(),
  };

  await boss.schedule(RESIDUAL_CALCULATION_JOB, "0 0 1 * *", jobData, {
    tz: "UTC",
  });

  logger?.info(
    {
      action: 'residual_job_scheduled',
      cron: '0 0 1 * *',
      periodStart: periodStart.toISOString(),
      periodEnd: periodEnd.toISOString(),
    },
    `Scheduled monthly residual calculation at "0 0 1 * *" UTC`
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
  periodEnd: Date,
  logger?: QueueLogger
): Promise<string | null> {
  const jobData: ResidualJobData = {
    periodStart: periodStart.toISOString(),
    periodEnd: periodEnd.toISOString(),
  };

  const jobId = await boss.send(RESIDUAL_CALCULATION_JOB, jobData, {
    singletonKey: `residual-${periodStart.toISOString()}-${periodEnd.toISOString()}`,
  });

  logger?.info(
    {
      action: 'residual_job_enqueued',
      jobId,
      periodStart: periodStart.toISOString(),
      periodEnd: periodEnd.toISOString(),
    },
    'Residual calculation job enqueued'
  );

  return jobId;
}

// ─── Email queue ──────────────────────────────────────────────────────

export interface EnqueueEmailParams {
  to: string;
  templateId: string;
  variables: Record<string, unknown>;
  merchantId?: string;
  agencyId?: string;
  correlationId?: string;
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
  params: EnqueueEmailParams,
  logger?: QueueLogger
): Promise<string | null> {
  const { to, templateId, variables, merchantId, agencyId, correlationId } = params;

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
      correlationId,
    };

    await boss.send(EMAIL_SEND_JOB, jobData, {
      retryLimit: 3,
      retryDelay: 30, // 30 seconds between retries
      retryBackoff: true, // exponential backoff
      expireInMinutes: 60, // expire after 1 hour
    });

    logger?.info(
      {
        action: 'email_enqueued',
        templateId,
        to,
        notificationId: notification.id,
        merchantId,
        correlationId,
      },
      `Enqueued email ${templateId} to ${to}`
    );

    return notification.id;
  } catch (err: unknown) {
    // Email queueing failures must NEVER break calling code
    logger?.error(
      {
        action: 'email_enqueue_failed',
        templateId,
        to,
        merchantId,
        correlationId,
        err,
      },
      `Failed to enqueue email ${templateId} to ${to}`
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
  params: EnqueueEmailParams,
  logger?: QueueLogger
): Promise<string | null> {
  const { to, templateId, variables, merchantId, agencyId, correlationId } = params;

  try {
    const jobData: EmailSendJobData = {
      to,
      templateId,
      variables,
      merchantId,
      agencyId,
      correlationId,
    };

    const jobId = await boss.send(EMAIL_SEND_JOB, jobData, {
      retryLimit: 3,
      retryDelay: 30,
      retryBackoff: true,
      expireInMinutes: 60,
    });

    logger?.info(
      {
        action: 'email_enqueued_simple',
        templateId,
        to,
        jobId,
        correlationId,
      },
      `Enqueued email ${templateId} to ${to} (simple)`
    );

    return jobId;
  } catch (err: unknown) {
    logger?.error(
      {
        action: 'email_enqueue_simple_failed',
        templateId,
        to,
        correlationId,
        err,
      },
      `Failed to enqueue email ${templateId} to ${to}`
    );
    return null;
  }
}
