/**
 * Job Handlers
 *
 * Registers pg-boss job handlers for background processing.
 * - residual-calculation: runs monthly (1st of month, 00:00 UTC)
 * - email-send: sends transactional emails via Postmark
 *
 * All handlers use structured logging via an injected logger.
 */

import type PgBoss from "pg-boss";
import type { PrismaClient } from "@prisma/client";
import {
  createResidualService,
  type NmiReportingClient,
} from "../services/residual.service.js";
import {
  createEmailService,
  type EmailServiceConfig,
} from "../services/email.service.js";
import { createTimer } from "../utils/logger.js";

// ─── Job Names ────────────────────────────────────────────────────────

export const RESIDUAL_CALCULATION_JOB = "residual-calculation";
export const EMAIL_SEND_JOB = "email-send";

// ─── Types ────────────────────────────────────────────────────────────

export interface ResidualJobData {
  periodStart: string; // ISO 8601
  periodEnd: string; // ISO 8601
}

export interface EmailSendJobData {
  to: string;
  templateId: string;
  variables: Record<string, unknown>;
  merchantId?: string;
  agencyId?: string;
  notificationId?: string; // ID of the NotificationSchedule record
  correlationId?: string;
}

/** Logger interface — accepts any pino-compatible logger */
interface JobLogger {
  info(obj: Record<string, unknown>, msg: string): void;
  warn(obj: Record<string, unknown>, msg: string): void;
  error(obj: Record<string, unknown>, msg: string): void;
  child(bindings: Record<string, unknown>): JobLogger;
}

export interface JobHandlerDeps {
  prisma: PrismaClient;
  nmiClient?: NmiReportingClient;
  emailConfig?: EmailServiceConfig;
  logger: JobLogger;
}

// ─── Registration ─────────────────────────────────────────────────────

/**
 * Register all job handlers with pg-boss.
 */
export async function registerJobHandlers(
  boss: PgBoss,
  deps: JobHandlerDeps
): Promise<void> {
  const { logger } = deps;

  const residualService = createResidualService({
    prisma: deps.prisma,
    nmiClient: deps.nmiClient,
    logger,
  });

  await boss.work<ResidualJobData>(
    RESIDUAL_CALCULATION_JOB,
    async (job) => {
      const { periodStart, periodEnd } = job.data;
      const timer = createTimer();

      logger.info(
        {
          action: 'residual_calculation_started',
          jobId: job.id,
          periodStart,
          periodEnd,
        },
        `Residual calculation started for period ${periodStart} - ${periodEnd}`
      );

      const result = await residualService.calculateResiduals(
        new Date(periodStart),
        new Date(periodEnd)
      );

      const duration_ms = timer.elapsed();
      logger.info(
        {
          action: 'residual_calculation_completed',
          jobId: job.id,
          periodStart,
          periodEnd,
          entriesCreated: result.entriesCreated,
          entriesSkipped: result.entriesSkipped,
          merchantsProcessed: result.merchantsProcessed,
          errorCount: result.errors.length,
          duration_ms,
        },
        `Residual calculation completed: ${result.entriesCreated} entries created, ${result.entriesSkipped} skipped`
      );

      if (result.errors.length > 0) {
        logger.warn(
          {
            action: 'residual_calculation_errors',
            jobId: job.id,
            errors: result.errors,
          },
          `Residual calculation had ${result.errors.length} errors`
        );
      }

      return result;
    }
  );

  // ─── Email Send Handler ───────────────────────────────────────────

  // Only register email handler if email config is provided
  if (deps.emailConfig) {
    const emailService = createEmailService({
      ...deps.emailConfig,
      logger,
    });

    await boss.work<EmailSendJobData>(
      EMAIL_SEND_JOB,
      {
        teamSize: 5, // process up to 5 emails concurrently
        teamConcurrency: 5,
      },
      async (job) => {
        const { to, templateId, variables, notificationId, correlationId } = job.data;

        logger.info(
          {
            action: 'email_job_started',
            jobId: job.id,
            templateId,
            to,
            notificationId: notificationId ?? null,
            correlationId,
          },
          `Sending ${templateId} to ${to}`
        );

        // Update NotificationSchedule attempt count
        if (notificationId) {
          try {
            await deps.prisma.notificationSchedule.update({
              where: { id: notificationId },
              data: {
                attempts: { increment: 1 },
              },
            });
          } catch {
            // NotificationSchedule record may not exist — proceed anyway
          }
        }

        const result = await emailService.sendEmail({
          to,
          templateId,
          variables,
          correlationId,
        });

        // Update NotificationSchedule with result
        if (notificationId) {
          try {
            if (result.success) {
              await deps.prisma.notificationSchedule.update({
                where: { id: notificationId },
                data: {
                  status: "SENT",
                  sentAt: new Date(),
                  lastError: null,
                },
              });
            } else {
              await deps.prisma.notificationSchedule.update({
                where: { id: notificationId },
                data: {
                  status: "FAILED",
                  lastError: result.error ?? "Unknown error",
                },
              });
            }
          } catch {
            logger.warn(
              {
                action: 'notification_schedule_update_failed',
                notificationId,
                correlationId,
              },
              `Failed to update NotificationSchedule ${notificationId}`
            );
          }
        }

        if (!result.success) {
          // Throw to trigger pg-boss retry (up to 3 attempts)
          throw new Error(
            `Email send failed: ${result.error}`
          );
        }

        return result;
      }
    );

    logger.info(
      { action: 'handler_registered', handler: 'email_send' },
      'Email send handler registered'
    );
  } else {
    logger.info(
      { action: 'handler_skipped', handler: 'email_send', reason: 'no_postmark_api_key' },
      'Email send handler skipped (no POSTMARK_API_KEY configured)'
    );
  }
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
