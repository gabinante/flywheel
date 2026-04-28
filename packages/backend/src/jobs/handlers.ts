/**
 * Job handlers for pg-boss background jobs.
 *
 * Includes:
 * - boarding-status-check: Polls NMI for UNDER_REVIEW applications every 15 minutes
 *
 * All handlers export a register function that takes the pg-boss instance
 * and a Prisma client, and sets up the job schedule + handler.
 */

import {
  checkBoardingStatus,
  encryptNmiCredentials,
  isNmiConfigured,
} from "../services/nmi-boarding.service";

// ---------------------------------------------------------------------------
// Types — pg-boss compatible interfaces
// ---------------------------------------------------------------------------

/** Minimal pg-boss-like interface for the parts we use. */
export interface PgBossLike {
  schedule(
    name: string,
    cron: string,
    data?: Record<string, unknown>,
    options?: Record<string, unknown>
  ): Promise<void>;
  work(
    name: string,
    handler: (job: PgBossJob) => Promise<void>
  ): Promise<string>;
  send(
    name: string,
    data: Record<string, unknown>,
    options?: Record<string, unknown>
  ): Promise<string | null>;
}

export interface PgBossJob {
  id: string;
  name: string;
  data: Record<string, unknown>;
}

/** Minimal Prisma-like interface for the parts we use. */
export interface PrismaLike {
  application: {
    findMany(args: {
      where: Record<string, unknown>;
      select?: Record<string, boolean>;
    }): Promise<ApplicationRecord[]>;
    update(args: {
      where: { id: string };
      data: Record<string, unknown>;
    }): Promise<unknown>;
  };
  merchant: {
    update(args: {
      where: { id: string };
      data: Record<string, unknown>;
    }): Promise<unknown>;
  };
}

export interface ApplicationRecord {
  id: string;
  merchantId: string;
  nmiApplicationId: string | null;
  boardingStatus: string;
}

// ---------------------------------------------------------------------------
// Job: boarding-status-check
// ---------------------------------------------------------------------------

/** Job name constant */
export const BOARDING_STATUS_CHECK_JOB = "boarding-status-check";

/** Default cron schedule: every 15 minutes */
export const BOARDING_STATUS_CHECK_CRON = "*/15 * * * *";

/**
 * Handle a single boarding-status-check job.
 *
 * Queries all Applications with boardingStatus = 'UNDER_REVIEW',
 * checks each with NMI, and handles approvals/rejections.
 */
export async function handleBoardingStatusCheck(
  prisma: PrismaLike,
  boss: PgBossLike
): Promise<{
  checked: number;
  approved: number;
  rejected: number;
  stillPending: number;
  errors: number;
}> {
  const stats = {
    checked: 0,
    approved: 0,
    rejected: 0,
    stillPending: 0,
    errors: 0,
  };

  if (!isNmiConfigured()) {
    console.warn(
      "[Boarding Status Check] NMI not configured, skipping status checks."
    );
    return stats;
  }

  // Find all applications under review
  const applications = await prisma.application.findMany({
    where: {
      boardingStatus: "UNDER_REVIEW",
      nmiApplicationId: { not: null },
    },
    select: {
      id: true,
      merchantId: true,
      nmiApplicationId: true,
      boardingStatus: true,
    },
  });

  if (applications.length === 0) {
    console.log(
      "[Boarding Status Check] No applications under review to check."
    );
    return stats;
  }

  console.log(
    `[Boarding Status Check] Checking ${applications.length} application(s) under review.`
  );

  for (const app of applications) {
    if (!app.nmiApplicationId) continue;

    stats.checked++;

    try {
      const result = await checkBoardingStatus(app.nmiApplicationId);

      if (result.status === "APPROVED") {
        await handleApproval(prisma, boss, app, result);
        stats.approved++;
      } else if (result.status === "DECLINED") {
        await handleRejection(prisma, boss, app, result);
        stats.rejected++;
      } else {
        // Still UNDER_REVIEW or PENDING
        stats.stillPending++;
        console.log(
          `[Boarding Status Check] Application ${app.id} still under review.`
        );
      }
    } catch (err) {
      stats.errors++;
      console.error(
        `[Boarding Status Check] Error checking application ${app.id}:`,
        err instanceof Error ? err.message : "Unknown error"
      );
    }
  }

  console.log(
    `[Boarding Status Check] Complete: ` +
      `checked=${stats.checked}, approved=${stats.approved}, ` +
      `rejected=${stats.rejected}, pending=${stats.stillPending}, ` +
      `errors=${stats.errors}`
  );

  return stats;
}

/**
 * Handle an approved boarding application:
 * - Store encrypted NMI credentials on Merchant
 * - Update Application status to APPROVED
 * - Update Merchant onboarding state
 * - Queue welcome email
 */
async function handleApproval(
  prisma: PrismaLike,
  boss: PgBossLike,
  app: ApplicationRecord,
  result: {
    nmiMerchantId?: string;
    securityKey?: string;
    tokenizationKey?: string;
  }
): Promise<void> {
  console.log(
    `[Boarding Status Check] Application ${app.id} APPROVED for merchant ${app.merchantId}.`
  );

  // Encrypt NMI credentials for storage
  const credentials = encryptNmiCredentials(result);

  // Update Merchant with NMI credentials
  await prisma.merchant.update({
    where: { id: app.merchantId },
    data: {
      nmiMerchantId: credentials.nmiMerchantId,
      nmiSecurityKey: credentials.nmiSecurityKey,
      nmiTokenizationKey: credentials.nmiTokenizationKey,
      onboardingStatus: "ACTIVE",
      onboardingCompletedAt: new Date(),
    },
  });

  // Update Application status
  await prisma.application.update({
    where: { id: app.id },
    data: {
      boardingStatus: "APPROVED",
      boardingApprovedAt: new Date(),
    },
  });

  // Queue welcome email
  try {
    await boss.send("send-email", {
      type: "welcome",
      merchantId: app.merchantId,
      applicationId: app.id,
      template: "merchant-boarding-approved",
    });
    console.log(
      `[Boarding Status Check] Welcome email queued for merchant ${app.merchantId}.`
    );
  } catch (emailErr) {
    // Don't fail the whole approval if email fails
    console.error(
      `[Boarding Status Check] Failed to queue welcome email for merchant ${app.merchantId}:`,
      emailErr instanceof Error ? emailErr.message : "Unknown error"
    );
  }
}

/**
 * Handle a rejected boarding application:
 * - Update Application status to REJECTED
 * - Queue notification email with reason
 */
async function handleRejection(
  prisma: PrismaLike,
  boss: PgBossLike,
  app: ApplicationRecord,
  result: { declineReason?: string; message?: string }
): Promise<void> {
  const reason = result.declineReason || result.message || "No reason provided";

  console.log(
    `[Boarding Status Check] Application ${app.id} REJECTED for merchant ${app.merchantId}: ${reason}`
  );

  // Update Application status
  await prisma.application.update({
    where: { id: app.id },
    data: {
      boardingStatus: "REJECTED",
      boardingRejectedAt: new Date(),
      boardingRejectionReason: reason,
    },
  });

  // Update Merchant onboarding state
  await prisma.merchant.update({
    where: { id: app.merchantId },
    data: {
      onboardingStatus: "REJECTED",
    },
  });

  // Queue rejection notification email
  try {
    await boss.send("send-email", {
      type: "boarding-rejected",
      merchantId: app.merchantId,
      applicationId: app.id,
      reason,
      template: "merchant-boarding-rejected",
    });
    console.log(
      `[Boarding Status Check] Rejection email queued for merchant ${app.merchantId}.`
    );
  } catch (emailErr) {
    console.error(
      `[Boarding Status Check] Failed to queue rejection email for merchant ${app.merchantId}:`,
      emailErr instanceof Error ? emailErr.message : "Unknown error"
    );
  }
}

/**
 * Register the boarding-status-check job with pg-boss.
 *
 * Sets up:
 * - A cron schedule to run every 15 minutes
 * - The job handler
 */
export async function registerBoardingStatusCheckJob(
  boss: PgBossLike,
  prisma: PrismaLike
): Promise<void> {
  // Schedule the recurring job (every 15 minutes)
  await boss.schedule(BOARDING_STATUS_CHECK_JOB, BOARDING_STATUS_CHECK_CRON);

  // Register the worker
  await boss.work(BOARDING_STATUS_CHECK_JOB, async (_job: PgBossJob) => {
    await handleBoardingStatusCheck(prisma, boss);
  });

  console.log(
    `[Jobs] Registered ${BOARDING_STATUS_CHECK_JOB} job (cron: ${BOARDING_STATUS_CHECK_CRON})`
  );
}

/**
 * Handle instant boarding approval during onboarding submission.
 *
 * Called directly from the onboarding route when NMI returns instant approval.
 * Stores encrypted credentials on Merchant and updates Application status.
 */
export async function handleInstantApproval(
  prisma: PrismaLike,
  boss: PgBossLike,
  applicationId: string,
  merchantId: string,
  result: {
    nmiApplicationId?: string;
    nmiMerchantId?: string;
    securityKey?: string;
    tokenizationKey?: string;
  }
): Promise<void> {
  // Encrypt NMI credentials for storage
  const credentials = encryptNmiCredentials(result);

  // Update Merchant with NMI credentials and activate
  await prisma.merchant.update({
    where: { id: merchantId },
    data: {
      nmiMerchantId: credentials.nmiMerchantId,
      nmiSecurityKey: credentials.nmiSecurityKey,
      nmiTokenizationKey: credentials.nmiTokenizationKey,
      onboardingStatus: "ACTIVE",
      onboardingCompletedAt: new Date(),
    },
  });

  // Update Application status
  await prisma.application.update({
    where: { id: applicationId },
    data: {
      boardingStatus: "APPROVED",
      nmiApplicationId: result.nmiApplicationId || null,
      boardingApprovedAt: new Date(),
    },
  });

  // Queue welcome email
  try {
    await boss.send("send-email", {
      type: "welcome",
      merchantId,
      applicationId,
      template: "merchant-boarding-approved",
    });
  } catch {
    console.error(
      `[NMI Boarding] Failed to queue welcome email for instant approval, merchant ${merchantId}`
    );
  }

  console.log(
    `[NMI Boarding] Instant approval processed for merchant ${merchantId}, ` +
      `application ${applicationId}`
  );
}

/**
 * Handle UNDER_REVIEW result during onboarding submission.
 *
 * Stores the NMI application ID for polling and updates Application status.
 */
export async function handlePendingReview(
  prisma: PrismaLike,
  applicationId: string,
  nmiApplicationId: string
): Promise<void> {
  await prisma.application.update({
    where: { id: applicationId },
    data: {
      boardingStatus: "UNDER_REVIEW",
      nmiApplicationId,
    },
  });

  console.log(
    `[NMI Boarding] Application ${applicationId} is under review, ` +
      `NMI application ID: ${nmiApplicationId}`
  );
}
