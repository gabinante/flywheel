/**
 * NMI Webhook Handlers
 *
 * Handles incoming webhooks from NMI for recurring payment events.
 *
 * Key events:
 * - recurring.success → Update subscription period, create transaction, reset failedAttempts
 * - recurring.failure → Increment failedAttempts, handle PAST_DUE / auto-cancel
 */

import type { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import type { PrismaClient } from '@prisma/client';

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
      const payload = request.body as NmiWebhookPayload;

      if (!payload || typeof payload !== 'object') {
        return reply.code(400).send({ error: 'Invalid webhook payload' });
      }

      const eventType = payload.event_type ?? payload.condition;
      const nmiSubscriptionId = payload.subscription_id;

      if (!nmiSubscriptionId) {
        request.log.warn({ payload }, 'Webhook missing subscription_id');
        return reply.code(200).send({ received: true, skipped: true });
      }

      // Find the local subscription by NMI subscription ID
      const subscription = await prisma.subscription.findFirst({
        where: { nmiSubscriptionId },
        include: { plan: true, merchant: true },
      });

      if (!subscription) {
        request.log.warn(
          { nmiSubscriptionId },
          'Webhook for unknown subscription'
        );
        return reply.code(200).send({ received: true, skipped: true });
      }

      // Log the webhook event
      await prisma.webhookEvent.create({
        data: {
          merchantId: subscription.merchantId,
          eventType: eventType ?? 'unknown',
          payload: payload as any,
        },
      });

      try {
        if (
          eventType === 'recurring.success' ||
          eventType === 'recurring_success' ||
          payload.condition === 'complete'
        ) {
          await handleRecurringSuccess(prisma, subscription, payload);
        } else if (
          eventType === 'recurring.failure' ||
          eventType === 'recurring_failure' ||
          payload.condition === 'failed'
        ) {
          await handleRecurringFailure(prisma, subscription, payload, request);
        } else {
          request.log.info({ eventType }, 'Unhandled webhook event type');
        }

        return reply.code(200).send({ received: true });
      } catch (err) {
        request.log.error({ err }, 'Failed to process webhook');
        return reply.code(500).send({ error: 'Failed to process webhook' });
      }
    }
  );
}

/**
 * Handle a successful recurring charge.
 *
 * - Update subscription currentPeriodStart/End
 * - Create Transaction record
 * - Reset failedAttempts
 * - Set status to ACTIVE (if was TRIALING or PAST_DUE)
 */
async function handleRecurringSuccess(
  prisma: PrismaClient,
  subscription: any,
  payload: NmiWebhookPayload
): Promise<void> {
  const now = new Date();
  const periodEnd = calculatePeriodEnd(now, subscription.plan.interval);
  const amount = payload.amount
    ? Math.round(parseFloat(payload.amount) * 100)
    : subscription.plan.amount;

  await prisma.$transaction([
    // Update subscription
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
    // Create transaction
    prisma.transaction.create({
      data: {
        merchantId: subscription.merchantId,
        subscriptionId: subscription.id,
        nmiTransactionId: payload.transaction_id ?? null,
        amount,
        currency: subscription.plan.currency,
        status: 'completed',
        paymentMethod: 'card',
        metadata: { source: 'recurring', nmi_event: payload.event_type },
      },
    }),
  ]);
}

/**
 * Handle a failed recurring charge.
 *
 * - Increment failedAttempts
 * - If failedAttempts >= 3 → set status to PAST_DUE, record pastDueAt
 * - If 7 days past due with no success → set status to CANCELED
 */
async function handleRecurringFailure(
  prisma: PrismaClient,
  subscription: any,
  payload: NmiWebhookPayload,
  request: FastifyRequest
): Promise<void> {
  const newFailedAttempts = subscription.failedAttempts + 1;
  const now = new Date();

  // Check if subscription should be auto-canceled
  // (7 days past due with no successful charge)
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
        { subscriptionId: subscription.id },
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
      { subscriptionId: subscription.id, failedAttempts: newFailedAttempts },
      'Subscription marked as PAST_DUE — dunning email should be queued'
    );
  }

  await prisma.subscription.update({
    where: { id: subscription.id },
    data: updateData,
  });

  // Create a failed transaction record for tracking
  await prisma.transaction.create({
    data: {
      merchantId: subscription.merchantId,
      subscriptionId: subscription.id,
      nmiTransactionId: payload.transaction_id ?? null,
      amount: subscription.plan.amount,
      currency: subscription.plan.currency,
      status: 'failed',
      paymentMethod: 'card',
      metadata: {
        source: 'recurring',
        nmi_event: payload.event_type,
        failedAttempts: newFailedAttempts,
      },
    },
  });
}

export default registerWebhookRoutes;
