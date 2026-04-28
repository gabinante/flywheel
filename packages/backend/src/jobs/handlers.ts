/**
 * Job Handlers
 *
 * Registers pg-boss job handlers for background processing.
 * - residual-calculation: runs monthly (1st of month, 00:00 UTC)
 * - email-send: sends transactional emails via Postmark
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
}

export interface JobHandlerDeps {
  prisma: PrismaClient;
  nmiClient?: NmiReportingClient;
  emailConfig?: EmailServiceConfig;
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
    async (job) => {
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

      return result;
    }
  );

  // ─── Email Send Handler ───────────────────────────────────────────

  // Only register email handler if email config is provided
  if (deps.emailConfig) {
    const emailService = createEmailService(deps.emailConfig);

    await boss.work<EmailSendJobData>(
      EMAIL_SEND_JOB,
      {
        teamSize: 5, // process up to 5 emails concurrently
        teamConcurrency: 5,
      },
      async (job) => {
        const { to, templateId, variables, notificationId } = job.data;

        console.log(
          `[EmailJob] Sending ${templateId} to ${to} (notification: ${notificationId ?? "none"})`
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
            // Log but don't fail the job for tracking errors
            console.warn(
              `[EmailJob] Failed to update NotificationSchedule ${notificationId}`
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

    console.log("[Handlers] Email send handler registered");
  } else {
    console.log(
      "[Handlers] Email send handler skipped (no POSTMARK_API_KEY configured)"
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
