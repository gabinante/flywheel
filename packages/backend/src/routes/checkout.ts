/**
 * Checkout Routes
 *
 * Customer-facing endpoints for checkout and subscription enrollment.
 */

import type { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import type { PrismaClient } from '@prisma/client';
import { NmiService, intervalToNmiFrequency } from '../services/nmi.service.js';

interface SubscribeBody {
  planId: string;
  sessionId: string;
  paymentToken: string;
  email?: string;
  firstName?: string;
  lastName?: string;
}

function validateSubscribeBody(body: unknown): {
  valid: boolean;
  error?: string;
  data?: SubscribeBody;
} {
  if (!body || typeof body !== 'object') {
    return { valid: false, error: 'Request body is required' };
  }

  const b = body as Record<string, unknown>;

  if (!b.planId || typeof b.planId !== 'string') {
    return { valid: false, error: 'planId is required' };
  }

  if (!b.sessionId || typeof b.sessionId !== 'string') {
    return { valid: false, error: 'sessionId is required' };
  }

  if (!b.paymentToken || typeof b.paymentToken !== 'string') {
    return { valid: false, error: 'paymentToken is required' };
  }

  return {
    valid: true,
    data: {
      planId: b.planId,
      sessionId: b.sessionId,
      paymentToken: b.paymentToken,
      email: b.email as string | undefined,
      firstName: b.firstName as string | undefined,
      lastName: b.lastName as string | undefined,
    },
  };
}

/**
 * Format date as YYYYMMDD for NMI API.
 */
function formatNmiDate(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  return `${y}${m}${d}`;
}

/**
 * Calculate the next billing period end date from a start date and interval.
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
 * Register checkout routes.
 */
export async function registerCheckoutRoutes(
  app: FastifyInstance,
  opts: { prisma: PrismaClient }
): Promise<void> {
  const { prisma } = opts;

  /**
   * POST /checkout/subscribe — Subscribe a customer to a plan
   *
   * Flow:
   * 1. Validate session + plan
   * 2. $0 auth if trial, or sale for first charge
   * 3. Add customer to NMI Customer Vault
   * 4. Create NMI recurring plan (idempotent by plan ID)
   * 5. Add NMI subscription with customer_vault_id
   * 6. Create local Subscription + Transaction records
   * 7. Return subscription confirmation
   */
  app.post<{ Body: SubscribeBody }>(
    '/subscribe',
    async (request: FastifyRequest<{ Body: SubscribeBody }>, reply: FastifyReply) => {
      const validation = validateSubscribeBody(request.body);
      if (!validation.valid || !validation.data) {
        return reply.code(400).send({ error: validation.error });
      }

      const { planId, sessionId, paymentToken, email, firstName, lastName } =
        validation.data;

      try {
        // 1. Validate session + plan
        const session = await prisma.checkoutSession.findUnique({
          where: { id: sessionId },
          include: { merchant: true },
        });

        if (!session) {
          return reply.code(404).send({ error: 'Checkout session not found' });
        }

        if (session.status !== 'pending') {
          return reply.code(400).send({
            error: 'Checkout session is no longer active',
          });
        }

        const plan = await prisma.subscriptionPlan.findFirst({
          where: { id: planId, merchantId: session.merchantId, active: true },
        });

        if (!plan) {
          return reply
            .code(404)
            .send({ error: 'Subscription plan not found or inactive' });
        }

        const merchant = session.merchant;
        if (!merchant.nmiSecurityKey) {
          return reply
            .code(500)
            .send({ error: 'Merchant payment gateway not configured' });
        }

        const nmi = new NmiService({ securityKey: merchant.nmiSecurityKey });
        const hasTrial = plan.trialDays > 0;
        const customerId = `cust_${session.id}_${Date.now()}`;

        // 2. Tokenize card via Collect.js token → NMI sale or $0 auth
        let initialTransaction: Awaited<ReturnType<NmiService['sale']>>;

        if (hasTrial) {
          // $0 auth to validate card
          initialTransaction = await nmi.authorize({
            amount: 0,
            paymentToken,
            currency: plan.currency,
            orderId: `trial_${plan.id}_${session.id}`,
          });
        } else {
          // First charge
          initialTransaction = await nmi.sale({
            amount: plan.amount,
            paymentToken,
            currency: plan.currency,
            orderId: `sub_${plan.id}_${session.id}`,
            description: `Subscription: ${plan.name}`,
          });
        }

        if (initialTransaction.response !== '1') {
          return reply.code(402).send({
            error: 'Payment failed',
            detail: initialTransaction.responsetext,
          });
        }

        // 3. Add customer to NMI Customer Vault
        const vaultResult = await nmi.addToCustomerVault({
          paymentToken,
          customerId,
          firstName,
          lastName,
          email,
        });

        if (vaultResult.response !== '1') {
          return reply.code(500).send({
            error: 'Failed to store payment method',
            detail: vaultResult.responsetext,
          });
        }

        const customerVaultId = vaultResult.customer_vault_id ?? customerId;

        // 4. Create NMI recurring plan (use plan.id as plan_id for idempotency)
        const frequency = intervalToNmiFrequency(plan.interval);
        const planResult = await nmi.createRecurringPlan({
          planId: plan.id,
          planName: plan.name,
          amount: plan.amount,
          dayFrequency: frequency.dayFrequency,
          monthFrequency: frequency.monthFrequency,
        });

        // Plan may already exist — that's OK (NMI returns error for duplicate)
        if (planResult.response !== '1' && !planResult.responsetext?.includes('Plan ID already exists')) {
          request.log.warn({ planResult }, 'NMI create plan returned non-success');
        }

        // 5. Add NMI subscription
        const now = new Date();
        let startDate: Date;

        if (hasTrial) {
          // Subscription starts after trial period
          startDate = new Date(now);
          startDate.setDate(startDate.getDate() + plan.trialDays);
        } else {
          // Start date is the next billing period
          startDate = calculatePeriodEnd(now, plan.interval);
        }

        const subscriptionResult = await nmi.addSubscription({
          planId: plan.id,
          customerVaultId,
          startDate: formatNmiDate(startDate),
        });

        if (subscriptionResult.response !== '1') {
          return reply.code(500).send({
            error: 'Failed to create recurring subscription',
            detail: subscriptionResult.responsetext,
          });
        }

        const nmiSubscriptionId = subscriptionResult.subscription_id ?? null;

        // 6. Create local Subscription + Transaction records
        const trialEnd = hasTrial
          ? new Date(now.getTime() + plan.trialDays * 24 * 60 * 60 * 1000)
          : null;

        const currentPeriodStart = now;
        const currentPeriodEnd = hasTrial ? trialEnd! : calculatePeriodEnd(now, plan.interval);

        const [subscription, transaction] = await prisma.$transaction([
          prisma.subscription.create({
            data: {
              merchantId: session.merchantId,
              planId: plan.id,
              customerId,
              nmiSubscriptionId,
              nmiCustomerVaultId: customerVaultId,
              status: hasTrial ? 'TRIALING' : 'ACTIVE',
              currentPeriodStart,
              currentPeriodEnd,
              trialEnd,
            },
          }),
          prisma.transaction.create({
            data: {
              merchantId: session.merchantId,
              amount: hasTrial ? 0 : plan.amount,
              currency: plan.currency,
              status: 'completed',
              nmiTransactionId: initialTransaction.transactionid ?? null,
              paymentMethod: 'card',
            },
          }),
        ]);

        // Update session status
        await prisma.checkoutSession.update({
          where: { id: sessionId },
          data: { status: 'completed' },
        });

        // Link transaction to subscription
        await prisma.transaction.update({
          where: { id: transaction.id },
          data: { subscriptionId: subscription.id },
        });

        // 7. Return subscription confirmation
        return reply.code(201).send({
          subscription: {
            id: subscription.id,
            status: subscription.status,
            planName: plan.name,
            amount: plan.amount,
            currency: plan.currency,
            interval: plan.interval,
            currentPeriodStart: subscription.currentPeriodStart,
            currentPeriodEnd: subscription.currentPeriodEnd,
            trialEnd: subscription.trialEnd,
          },
          transaction: {
            id: transaction.id,
            amount: transaction.amount,
            status: transaction.status,
          },
        });
      } catch (err) {
        request.log.error({ err }, 'Failed to create subscription');
        return reply.code(500).send({ error: 'Failed to process subscription' });
      }
    }
  );
}

export default registerCheckoutRoutes;
