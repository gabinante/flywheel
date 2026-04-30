/**
 * Job Handlers
 *
 * Registers pg-boss job handlers for background processing.
 * - residual-calculation: runs monthly (1st of month, 00:00 UTC)
 * - email-send: sends transactional emails via Postmark
 * - chargeback-monitor: runs daily at 06:00 UTC, computes CB ratios
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
import {
  computeChargebackRatio,
  getRiskLevel,
} from "./chargeback-thresholds.js";

// Re-export threshold utilities for admin routes
export { getRiskLevel, computeChargebackRatio } from "./chargeback-thresholds.js";
export { CB_WARNING_THRESHOLD, CB_CRITICAL_THRESHOLD, CB_HIGH_THRESHOLD } from "./chargeback-thresholds.js";

// ─── Job Names ────────────────────────────────────────────────────────

export const RESIDUAL_CALCULATION_JOB = "residual-calculation";
export const EMAIL_SEND_JOB = "email-send";
export const CHARGEBACK_MONITOR_JOB = "chargeback-monitor";

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

export interface ChargebackMonitorResult {
  merchantId: string;
  merchantName: string;
  transactionCount: number;
  chargebackCount: number;
  ratio: number;
  riskLevel: string | null;
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

  // ─── Chargeback Monitor Handler ──────────────────────────────────

  await boss.work<Record<string, never>>(
    CHARGEBACK_MONITOR_JOB,
    async (_job) => {
      console.log("[ChargebackMonitor] Starting daily chargeback ratio scan");
      const results = await runChargebackMonitor(deps.prisma);
      const flagged = results.filter((r) => r.riskLevel !== null);
      console.log(
        `[ChargebackMonitor] Complete: ${results.length} merchants scanned, ${flagged.length} flagged`
      );
      return results;
    }
  );

  console.log("[Handlers] Chargeback monitor handler registered");

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

// ─── Chargeback Monitor Logic ──────────────────────────────────────

/**
 * For each ACTIVE merchant:
 *  - Count transactions in last 30 days with status CAPTURED or SETTLED
 *  - Count chargebacks in last 30 days
 *  - Compute ratio; assign WARNING / CRITICAL / HIGH risk level
 *  - Persist risk level on the merchant record
 *  - Queue NotificationSchedule records for operators
 */
export async function runChargebackMonitor(
  prisma: PrismaClient
): Promise<ChargebackMonitorResult[]> {
  const thirtyDaysAgo = new Date();
  thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30);

  const merchants = await prisma.merchant.findMany({
    where: { status: "ACTIVE" },
    select: { id: true, name: true },
  });

  const results: ChargebackMonitorResult[] = [];

  for (const merchant of merchants) {
    const [transactionCount, chargebackCount] = await Promise.all([
      prisma.transaction.count({
        where: {
          merchantId: merchant.id,
          status: { in: ["CAPTURED", "SETTLED"] },
          createdAt: { gte: thirtyDaysAgo },
        },
      }),
      prisma.chargeback.count({
        where: {
          merchantId: merchant.id,
          createdAt: { gte: thirtyDaysAgo },
        },
      }),
    ]);

    const ratio = computeChargebackRatio(transactionCount, chargebackCount);
    const riskLevel = getRiskLevel(ratio);

    // Persist risk level on merchant
    await prisma.merchant.update({
      where: { id: merchant.id },
      data: {
        chargebackRiskLevel: riskLevel,
        chargebackRiskUpdatedAt: new Date(),
      },
    });

    // Queue operator notifications for elevated risk
    if (riskLevel !== null) {
      const ratioPercent = (ratio * 100).toFixed(2);
      const urgency = riskLevel === "HIGH" ? "urgent" : riskLevel === "CRITICAL" ? "high" : "normal";

      await prisma.notificationSchedule.create({
        data: {
          templateId: "chargeback-risk-alert",
          to: "ops@shamroq.com",
          variables: {
            merchantId: merchant.id,
            merchantName: merchant.name,
            riskLevel,
            ratioPercent,
            transactionCount,
            chargebackCount,
            urgency,
            message: `Merchant "${merchant.name}" has a chargeback ratio of ${ratioPercent}% (${chargebackCount}/${transactionCount}) — ${riskLevel}`,
          },
          status: "QUEUED",
          merchantId: merchant.id,
        },
      });
    }

    results.push({
      merchantId: merchant.id,
      merchantName: merchant.name,
      transactionCount,
      chargebackCount,
      ratio,
      riskLevel,
    });
  }

  return results;
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
