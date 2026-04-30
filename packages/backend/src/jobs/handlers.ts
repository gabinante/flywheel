import PgBoss from 'pg-boss';
import { prisma } from '../utils/prisma.js';
import { encrypt } from '../utils/encryption.js';
import {
  checkBoardingStatus,
  submitBoardingApplication,
  type BoardingApplication,
  type BoardingResult,
} from '../services/nmi-boarding.service.js';

// ---------------------------------------------------------------------------
// Job names & schedules
// ---------------------------------------------------------------------------

export const BOARDING_STATUS_CHECK_JOB = 'boarding-status-check';
const BOARDING_STATUS_CHECK_CRON = '*/15 * * * *'; // every 15 minutes

// ---------------------------------------------------------------------------
// Job registration
// ---------------------------------------------------------------------------

export async function registerJobHandlers(boss: PgBoss) {
  // Daily metrics aggregation
  await boss.work('metrics:aggregate-daily', async ([job]) => {
    console.log('Aggregating daily metrics:', job.data);
  });

  // Webhook delivery
  await boss.work('webhook:deliver', async ([job]) => {
    const { webhookEventId } = job.data as { webhookEventId: string };
    await deliverWebhook(webhookEventId);
  });

  // GHL token refresh
  await boss.work('ghl:refresh-tokens', async () => {
    console.log('Refreshing GHL tokens');
  });

  // NMI boarding status polling — runs every 15 minutes
  await boss.work(BOARDING_STATUS_CHECK_JOB, async () => {
    await handleBoardingStatusCheck();
  });

  // Create queues for scheduled jobs, then schedule
  await boss.createQueue('metrics:aggregate-daily');
  await boss.createQueue('ghl:refresh-tokens');
  await boss.createQueue(BOARDING_STATUS_CHECK_JOB);

  await boss.schedule('metrics:aggregate-daily', '0 2 * * *', {}); // 2am daily
  await boss.schedule('ghl:refresh-tokens', '*/30 * * * *', {}); // every 30 min
  await boss.schedule(BOARDING_STATUS_CHECK_JOB, BOARDING_STATUS_CHECK_CRON, {}); // every 15 min
}

// ---------------------------------------------------------------------------
// Boarding status polling
// ---------------------------------------------------------------------------

/**
 * Poll NMI for the status of all PENDING boarding applications.
 * Called by the boarding-status-check scheduled job every 15 minutes.
 */
export async function handleBoardingStatusCheck(): Promise<void> {
  // Find applications awaiting NMI underwriting decision
  const pendingApps = await prisma.application.findMany({
    where: {
      nmiBoardingStatus: 'PENDING',
      nmiBoardingId: { not: null },
    },
    include: { merchant: true },
  });

  if (pendingApps.length === 0) {
    return;
  }

  console.log(`[Boarding Poll] Checking ${pendingApps.length} pending application(s)`);

  for (const app of pendingApps) {
    if (!app.nmiBoardingId) continue;

    try {
      const result = await checkBoardingStatus(app.nmiBoardingId);

      if (!result) {
        // Credentials not configured or mock ID — skip
        continue;
      }

      if (result.status === 'APPROVED') {
        await onBoardingApproved(app.merchantId, app.id, result.securityKey, result.tokenizationKey, result.nmiMerchantId);
      } else if (result.status === 'DECLINED') {
        await onBoardingDeclined(app.merchantId, app.id, result.declineReason);
      }
      // PENDING — no action needed, will poll again next cycle
    } catch (error) {
      console.error(
        `[Boarding Poll] Error checking status for application ${app.id}:`,
        error instanceof Error ? error.message : 'Unknown error'
      );
    }
  }
}

// ---------------------------------------------------------------------------
// Approval / rejection handlers
// ---------------------------------------------------------------------------

/**
 * Handle NMI approval: encrypt and store credentials, activate merchant.
 * Called both from polling job and directly after instant approval.
 *
 * Security: credentials are encrypted immediately and never logged.
 */
export async function onBoardingApproved(
  merchantId: string,
  applicationId: string,
  securityKey: string | undefined,
  tokenizationKey: string | undefined,
  nmiMerchantId: string | undefined
): Promise<void> {
  console.log(`[Boarding] Application ${applicationId} APPROVED for merchant ${merchantId}`);

  // Encrypt credentials immediately — never store plaintext
  const encryptedSecurityKey = securityKey ? encrypt(securityKey) : undefined;
  const encryptedTokenizationKey = tokenizationKey ? encrypt(tokenizationKey) : undefined;

  // Update merchant with encrypted NMI credentials and activate
  await prisma.merchant.update({
    where: { id: merchantId },
    data: {
      status: 'ACTIVE',
      ...(encryptedSecurityKey && { nmiSecurityKey: encryptedSecurityKey }),
      ...(encryptedTokenizationKey && { nmiTokenizationKey: encryptedTokenizationKey }),
      ...(nmiMerchantId && { nmiMerchantId }),
    },
  });

  // Update application status
  await prisma.application.update({
    where: { id: applicationId },
    data: {
      status: 'APPROVED',
      nmiBoardingStatus: 'APPROVED',
      reviewedAt: new Date(),
    },
  });

  // Queue welcome email notification
  await queueWelcomeEmail(merchantId);
}

/**
 * Handle NMI rejection: update application status, queue notification.
 */
export async function onBoardingDeclined(
  merchantId: string,
  applicationId: string,
  reason: string | undefined
): Promise<void> {
  console.log(`[Boarding] Application ${applicationId} DECLINED for merchant ${merchantId}`);

  await prisma.application.update({
    where: { id: applicationId },
    data: {
      status: 'REJECTED',
      nmiBoardingStatus: 'DECLINED',
      rejectionReason: reason || 'Application declined by NMI',
      reviewedAt: new Date(),
    },
  });

  // Queue rejection notification email
  await queueRejectionEmail(merchantId, reason);
}

// ---------------------------------------------------------------------------
// Onboarding submission helpers
// (Called from onboarding routes after application is saved)
// ---------------------------------------------------------------------------

/**
 * Handle the result of an instant approval during initial boarding submission.
 * Credentials returned immediately in the boarding API response.
 */
export async function handleInstantApproval(
  merchantId: string,
  applicationId: string,
  boardingResult: BoardingResult
): Promise<void> {
  await onBoardingApproved(
    merchantId,
    applicationId,
    boardingResult.securityKey,
    boardingResult.tokenizationKey,
    boardingResult.nmiMerchantId
  );
}

/**
 * Handle a PENDING boarding result — store the NMI boarding ID for polling.
 */
export async function handlePendingBoarding(
  applicationId: string,
  boardingResult: BoardingResult
): Promise<void> {
  console.log(`[Boarding] Application ${applicationId} pending NMI review. Boarding ID: ${boardingResult.boardingId || 'unknown'}`);

  await prisma.application.update({
    where: { id: applicationId },
    data: {
      status: 'UNDER_REVIEW',
      nmiBoardingId: boardingResult.boardingId,
      nmiBoardingStatus: 'PENDING',
      boardingSubmittedAt: new Date(),
    },
  });
}

/**
 * Submit a boarding application and dispatch to the appropriate handler.
 * Convenience wrapper used by onboarding routes.
 */
export async function submitAndHandleBoarding(
  merchantId: string,
  applicationId: string,
  boardingApplication: BoardingApplication
): Promise<BoardingResult> {
  const result = await submitBoardingApplication(boardingApplication);

  if (!result.success) {
    console.error(`[Boarding] Submission failed for application ${applicationId}: ${result.error}`);
    return result;
  }

  if (result.status === 'APPROVED') {
    await handleInstantApproval(merchantId, applicationId, result);
  } else if (result.status === 'PENDING') {
    await handlePendingBoarding(applicationId, result);
  }

  return result;
}

// ---------------------------------------------------------------------------
// Email queue helpers
// ---------------------------------------------------------------------------

/**
 * Queue a welcome email for a newly approved merchant.
 * Uses notification_schedules table — actual sending handled by email worker.
 */
async function queueWelcomeEmail(merchantId: string): Promise<void> {
  try {
    const merchant = await prisma.merchant.findUnique({
      where: { id: merchantId },
      select: { contactEmail: true, businessName: true },
    });

    if (!merchant) return;

    await prisma.notificationSchedule.create({
      data: {
        type: 'merchant_approved',
        recipientType: 'merchant',
        recipientId: merchantId,
        channel: 'email',
        scheduledFor: new Date(),
        payload: {
          email: merchant.contactEmail,
          businessName: merchant.businessName,
          template: 'merchant_welcome',
        },
      },
    });

    console.log(`[Boarding] Welcome email queued for merchant ${merchantId}`);
  } catch (error) {
    // Non-fatal — log and continue
    console.error(
      `[Boarding] Failed to queue welcome email for merchant ${merchantId}:`,
      error instanceof Error ? error.message : 'Unknown error'
    );
  }
}

/**
 * Queue a rejection notification email.
 */
async function queueRejectionEmail(merchantId: string, reason: string | undefined): Promise<void> {
  try {
    const merchant = await prisma.merchant.findUnique({
      where: { id: merchantId },
      select: { contactEmail: true, businessName: true },
    });

    if (!merchant) return;

    await prisma.notificationSchedule.create({
      data: {
        type: 'merchant_rejected',
        recipientType: 'merchant',
        recipientId: merchantId,
        channel: 'email',
        scheduledFor: new Date(),
        payload: {
          email: merchant.contactEmail,
          businessName: merchant.businessName,
          template: 'merchant_rejected',
          reason: reason || 'Application declined',
        },
      },
    });

    console.log(`[Boarding] Rejection email queued for merchant ${merchantId}`);
  } catch (error) {
    // Non-fatal — log and continue
    console.error(
      `[Boarding] Failed to queue rejection email for merchant ${merchantId}:`,
      error instanceof Error ? error.message : 'Unknown error'
    );
  }
}

// ---------------------------------------------------------------------------
// Webhook delivery
// ---------------------------------------------------------------------------

async function deliverWebhook(webhookEventId: string) {
  const event = await prisma.webhookEvent.findUnique({
    where: { id: webhookEventId },
    include: { merchant: true },
  });

  if (!event || !event.merchant.webhookUrl) return;

  const url = event.merchant.webhookUrl;
  const attempt = event.attempts + 1;

  try {
    const response = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Webhook-Signature': event.merchant.webhookSecret || '',
        'X-Webhook-Event': event.eventType,
      },
      body: JSON.stringify(event.payload),
      signal: AbortSignal.timeout(10000),
    });

    await prisma.merchantWebhookDelivery.create({
      data: {
        merchantId: event.merchantId,
        webhookEventId: event.id,
        url,
        statusCode: response.status,
        success: response.ok,
        attemptNumber: attempt,
      },
    });

    if (response.ok) {
      await prisma.webhookEvent.update({
        where: { id: webhookEventId },
        data: { delivered: true, deliveredAt: new Date(), attempts: attempt },
      });
    } else {
      await prisma.webhookEvent.update({
        where: { id: webhookEventId },
        data: { attempts: attempt },
      });
    }
  } catch (error) {
    await prisma.merchantWebhookDelivery.create({
      data: {
        merchantId: event.merchantId,
        webhookEventId: event.id,
        url,
        responseBody: error instanceof Error ? error.message : 'Unknown error',
        success: false,
        attemptNumber: attempt,
      },
    });

    await prisma.webhookEvent.update({
      where: { id: webhookEventId },
      data: { attempts: attempt },
    });
  }
}
