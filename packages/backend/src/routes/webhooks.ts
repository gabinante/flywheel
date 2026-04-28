/**
 * NMI Webhook Handlers
 *
 * Handles incoming webhooks from NMI for recurring payment events.
 * All webhook processing includes correlationId for end-to-end tracing.
 *
 * Key events:
 * - recurring.success -> Update subscription period, create transaction, reset failedAttempts
 * - recurring.failure -> Increment failedAttempts, handle PAST_DUE / auto-cancel
 */

import type { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import type { PrismaClient } from '@prisma/client';
import { createTimer } from '../utils/logger.js';
import { sentryTrace, setSentryContext, captureException } from '../utils/sentry.js';

/** How many consecutive failures before marking as PAST_DUE */
const PAST_DUE_THRESHOLD = 3;

/** Days past due after which auto-cancel fires (7 days) */
const AUTO_CANCEL_DAYS = 7;

interface NmiWebhookPayload {
  event_type?: string;
  subscription_id?: string;
  transaction_id?: string;
  amount?: string;
  condition?: string;
  order_id?: string;
  [key: string]: string | undefined;
}

/**
 * Calculate next period end from current period start and interval.
 */
function calculatePeriodEnd(start: Date, interval: string): Date {
  const end = new Date(start);
  switch (interval) {
    case 'WEEKLY':
      end.setUTCDate(end.getUTCDate() + 7);
      break;
    case 'MONTHLY':
      end.setUTCMonth(end.getUTCMonth() + 1);
      break;
    case 'QUARTERLY':
      end.setUTCMonth(end.getUTCMonth() + 3);
      break;
    case 'ANNUAL':
      end.setUTCFullYear(end.getUTCFullYear() + 1);
      break;
    default:
      end.setUTCMonth(end.getUTCMonth() + 1);
  }
  return end;
}

/**
 * Register NMI webhook routes.
 */
export async function registerWebhookRoutes(
  app: FastifyInstance,
  opts: { prisma: PrismaClient }
): Promise<void> {
  const { prisma } = opts;

  /**
   * POST /webhooks/nmi — Handle NMI recurring billing webhooks
   */
  app.post(
    '/nmi',
    async (request: FastifyRequest, reply: FastifyReply) => {
      const webhookTimer = createTimer();
      const payload = request.body as NmiWebhookPayload;

      if (!payload || typeof payload !== 'object') {
        return reply.code(400).send({ error: 'Invalid webhook payload' });
      }

      const eventType = payload.event_type ?? payload.condition;
      const nmiSubscriptionId = payload.subscription_id;

      request.log.info(
        { action: 'webhook_received', eventType, nmiSubscriptionId },
        'NMI webhook received'
      );

      if (!nmiSubscriptionId) {
        request.log.warn(
          { action: 'webhook_missing_subscription_id', eventType },
          'Webhook missing subscription_id'
        );
        return reply.code(200).send({ received: true, skipped: true });
      }

      // Find the local subscription by NMI subscription ID
      const subscription = await prisma.subscription.findFirst({
        where: { nmiSubscriptionId },
        include: { plan: true, merchant: true },
      });

      if (!subscription) {
        request.log.warn(
          { action: 'webhook_unknown_subscription', nmiSubscriptionId },
          'Webhook for unknown subscription'
        );
        return reply.code(200).send({ received: true, skipped: true });
      }

      const merchantId = subscription.merchantId;

      // Look up the correlation ID from recent transactions for this subscription
      const recentTx = await prisma.transaction.findFirst({
        where: { subscriptionId: subscription.id, correlationId: { not: null } },
        orderBy: { createdAt: 'desc' },
        select: { correlationId: true },
      });
      const correlationId = recentTx?.correlationId ?? undefined;

      // Set Sentry context for this webhook
      setSentryContext({
        correlationId: correlationId ?? '',
        merchantId,
        subscriptionId: subscription.id,
        eventType: eventType ?? 'unknown',
        requestId: request.id,
      });

      request.log.info(
        {
          action: 'webhook_processing',
          eventType,
          merchantId,
          subscriptionId: subscription.id,
          correlationId,
        },
        'Processing NMI webhook'
      );

      // Log the webhook event
      await prisma.webhookEvent.create({
        data: {
          merchantId,
          eventType: eventType ?? 'unknown',
          payload: payload as any,
        },
      });

      try {
        await sentryTrace(
          'webhook.nmi',
          { eventType: eventType ?? 'unknown', merchantId, correlationId: correlationId ?? '' },
          async () => {
            if (
              eventType === 'recurring.success' ||
              eventType === 'recurring_success' ||
              payload.condition === 'complete'
            ) {
              await handleRecurringSuccess(prisma, subscription, payload, request, correlationId);
            } else if (
              eventType === 'recurring.failure' ||
              eventType === 'recurring_failure' ||
              payload.condition === 'failed'
            ) {
              await handleRecurringFailure(prisma, subscription, payload, request, correlationId);
            } else {
              request.log.info(
                { action: 'webhook_unhandled_event', eventType, merchantId, correlationId },
                'Unhandled webhook event type'
              );
            }
          }
        );

        const duration_ms = webhookTimer.elapsed();
        request.log.info(
          { action: 'webhook_processed', eventType, merchantId, correlationId, duration_ms },
          'NMI webhook processed successfully'
        );

        return reply.code(200).send({ received: true });
      } catch (err) {
        const duration_ms = webhookTimer.elapsed();
        captureException(err, { merchantId, correlationId, eventType: eventType ?? 'unknown' });
        request.log.error(
          { err, action: 'webhook_processing_failed', eventType, merchantId, correlationId, duration_ms },
          'Failed to process webhook'
        );
        return reply.code(500).send({ error: 'Failed to process webhook' });
      }
    }
  );
}

/**
 * Handle a successful recurring charge.
 */
async function handleRecurringSuccess(
  prisma: PrismaClient,
  subscription: any,
  payload: NmiWebhookPayload,
  request: FastifyRequest,
  correlationId?: string
): Promise<void> {
  const now = new Date();
  const periodEnd = calculatePeriodEnd(now, subscription.plan.interval);
  const amount = payload.amount
    ? Math.round(parseFloat(payload.amount) * 100)
    : subscription.plan.amount;

  request.log.info(
    {
      action: 'recurring_success',
      merchantId: subscription.merchantId,
      subscriptionId: subscription.id,
      amount,
      correlationId,
    },
    'Processing recurring payment success'
  );

  await prisma.$transaction([
    prisma.subscription.update({
      where: { id: subscription.id },
      data: {
        status: 'ACTIVE',
        currentPeriodStart: now,
        currentPeriodEnd: periodEnd,
        failedAttempts: 0,
        pastDueAt: null,
      },
    }),
    prisma.transaction.create({
      data: {
        merchantId: subscription.merchantId,
        subscriptionId: subscription.id,
        nmiTransactionId: payload.transaction_id ?? null,
        amount,
        currency: subscription.plan.currency,
        status: 'completed',
        paymentMethod: 'card',
        correlationId,
        metadata: { source: 'recurring', nmi_event: payload.event_type },
      },
    }),
  ]);
}

/**
 * Handle a failed recurring charge.
 */
async function handleRecurringFailure(
  prisma: PrismaClient,
  subscription: any,
  payload: NmiWebhookPayload,
  request: FastifyRequest,
  correlationId?: string
): Promise<void> {
  const newFailedAttempts = subscription.failedAttempts + 1;
  const now = new Date();
  const merchantId = subscription.merchantId;

  // Check if subscription should be auto-canceled
  if (
    subscription.status === 'PAST_DUE' &&
    subscription.pastDueAt
  ) {
    const pastDueDate = new Date(subscription.pastDueAt);
    const daysPastDue =
      (now.getTime() - pastDueDate.getTime()) / (1000 * 60 * 60 * 24);

    if (daysPastDue >= AUTO_CANCEL_DAYS) {
      await prisma.subscription.update({
        where: { id: subscription.id },
        data: {
          status: 'CANCELED',
          failedAttempts: newFailedAttempts,
          canceledAt: now,
          cancelReason: 'Auto-canceled after 7 days past due',
        },
      });

      request.log.info(
        {
          action: 'subscription_auto_canceled',
          merchantId,
          subscriptionId: subscription.id,
          daysPastDue: Math.round(daysPastDue),
          correlationId,
        },
        'Subscription auto-canceled after 7 days past due'
      );
      return;
    }
  }

  // Determine new status
  const shouldBePastDue = newFailedAttempts >= PAST_DUE_THRESHOLD;

  const updateData: Record<string, unknown> = {
    failedAttempts: newFailedAttempts,
  };

  if (shouldBePastDue && subscription.status !== 'PAST_DUE') {
    updateData.status = 'PAST_DUE';
    updateData.pastDueAt = now;

    request.log.warn(
      {
        action: 'subscription_past_due',
        merchantId,
        subscriptionId: subscription.id,
        failedAttempts: newFailedAttempts,
        correlationId,
      },
      'Subscription marked as PAST_DUE'
    );
  } else {
    request.log.info(
      {
        action: 'recurring_failure',
        merchantId,
        subscriptionId: subscription.id,
        failedAttempts: newFailedAttempts,
        correlationId,
      },
      'Recurring payment failed'
    );
  }

  await prisma.subscription.update({
    where: { id: subscription.id },
    data: updateData,
  });

  // Create a failed transaction record for tracking
  await prisma.transaction.create({
    data: {
      merchantId,
      subscriptionId: subscription.id,
      nmiTransactionId: payload.transaction_id ?? null,
      amount: subscription.plan.amount,
      currency: subscription.plan.currency,
      status: 'failed',
      paymentMethod: 'card',
      correlationId,
      metadata: {
        source: 'recurring',
        nmi_event: payload.event_type,
        failedAttempts: newFailedAttempts,
      },
    },
  });
}

export default registerWebhookRoutes;
