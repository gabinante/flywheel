/**
 * Subscription & Recurring Billing Routes
 *
 * Merchant-facing endpoints for managing subscription plans and subscriptions.
 * All routes require merchant authentication.
 */

import type { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import type { PrismaClient } from '@prisma/client';

/** Billing interval — defined locally until Prisma client is generated */
type BillingInterval = 'MONTHLY' | 'QUARTERLY' | 'YEARLY';
/** Subscription status — defined locally until Prisma client is generated */
type SubscriptionStatus = 'ACTIVE' | 'CANCELLED' | 'PAST_DUE' | 'PAUSED';
import { NmiService } from '../services/nmi.service.js';

// ---- Zod-like validation helpers ----

interface CreatePlanBody {
  name: string;
  amount: number;
  currency?: string;
  interval: string;
  trialDays?: number;
}

interface UpdatePlanBody {
  name?: string;
  amount?: number;
  active?: boolean;
}

interface CancelSubscriptionBody {
  reason?: string;
}

interface ListSubscriptionsQuery {
  status?: string;
  page?: string;
  limit?: string;
}

const VALID_INTERVALS = ['WEEKLY', 'MONTHLY', 'QUARTERLY', 'ANNUAL'] as const;

function validateCreatePlan(body: unknown): {
  valid: boolean;
  error?: string;
  data?: CreatePlanBody;
} {
  if (!body || typeof body !== 'object') {
    return { valid: false, error: 'Request body is required' };
  }

  const b = body as Record<string, unknown>;

  if (!b.name || typeof b.name !== 'string' || b.name.trim().length === 0) {
    return { valid: false, error: 'name is required and must be a non-empty string' };
  }

  if (typeof b.amount !== 'number' || !Number.isInteger(b.amount) || b.amount < 1) {
    return { valid: false, error: 'amount is required and must be a positive integer (cents)' };
  }

  if (b.currency !== undefined && typeof b.currency !== 'string') {
    return { valid: false, error: 'currency must be a string' };
  }

  if (!b.interval || typeof b.interval !== 'string') {
    return { valid: false, error: 'interval is required' };
  }

  const interval = b.interval.toUpperCase();
  if (!VALID_INTERVALS.includes(interval as typeof VALID_INTERVALS[number])) {
    return {
      valid: false,
      error: `interval must be one of: ${VALID_INTERVALS.join(', ')}`,
    };
  }

  if (b.trialDays !== undefined) {
    if (typeof b.trialDays !== 'number' || !Number.isInteger(b.trialDays) || b.trialDays < 0) {
      return { valid: false, error: 'trialDays must be a non-negative integer' };
    }
  }

  return {
    valid: true,
    data: {
      name: b.name.trim(),
      amount: b.amount,
      currency: (b.currency as string) ?? 'USD',
      interval: interval,
      trialDays: (b.trialDays as number) ?? 0,
    },
  };
}

function validateUpdatePlan(body: unknown): {
  valid: boolean;
  error?: string;
  data?: UpdatePlanBody;
} {
  if (!body || typeof body !== 'object') {
    return { valid: false, error: 'Request body is required' };
  }

  const b = body as Record<string, unknown>;
  const data: UpdatePlanBody = {};

  if (b.name !== undefined) {
    if (typeof b.name !== 'string' || b.name.trim().length === 0) {
      return { valid: false, error: 'name must be a non-empty string' };
    }
    data.name = b.name.trim();
  }

  if (b.amount !== undefined) {
    if (typeof b.amount !== 'number' || !Number.isInteger(b.amount) || b.amount < 1) {
      return { valid: false, error: 'amount must be a positive integer (cents)' };
    }
    data.amount = b.amount;
  }

  if (b.active !== undefined) {
    if (typeof b.active !== 'boolean') {
      return { valid: false, error: 'active must be a boolean' };
    }
    data.active = b.active;
  }

  if (Object.keys(data).length === 0) {
    return { valid: false, error: 'At least one field must be provided' };
  }

  return { valid: true, data };
}

/**
 * Register subscription management routes under the merchant namespace.
 *
 * Expects:
 * - `request.merchantId` to be set by authentication middleware
 * - `app.prisma` to be available (PrismaClient)
 */
export async function registerSubscriptionRoutes(
  app: FastifyInstance,
  opts: { prisma: PrismaClient }
): Promise<void> {
  const { prisma } = opts;

  // ---- Subscription Plans ----

  /**
   * POST /subscriptions/plans — Create a new subscription plan
   */
  app.post<{ Body: CreatePlanBody }>(
    '/subscriptions/plans',
    async (request: FastifyRequest<{ Body: CreatePlanBody }>, reply: FastifyReply) => {
      const merchantId = (request as any).merchantId as string;
      if (!merchantId) {
        return reply.code(401).send({ error: 'Unauthorized' });
      }

      const validation = validateCreatePlan(request.body);
      if (!validation.valid || !validation.data) {
        return reply.code(400).send({ error: validation.error });
      }

      const { name, amount, currency, interval, trialDays } = validation.data;

      try {
        const plan = await prisma.subscriptionPlan.create({
          data: {
            merchantId,
            name,
            amount,
            currency: currency!,
            interval: interval as BillingInterval,
            trialDays: trialDays!,
          },
        });

        return reply.code(201).send(plan);
      } catch (err) {
        request.log.error({ err }, 'Failed to create subscription plan');
        return reply.code(500).send({ error: 'Failed to create plan' });
      }
    }
  );

  /**
   * GET /subscriptions/plans — List subscription plans for the merchant
   */
  app.get(
    '/subscriptions/plans',
    async (request: FastifyRequest, reply: FastifyReply) => {
      const merchantId = (request as any).merchantId as string;
      if (!merchantId) {
        return reply.code(401).send({ error: 'Unauthorized' });
      }

      try {
        const plans = await prisma.subscriptionPlan.findMany({
          where: { merchantId },
          orderBy: { createdAt: 'desc' },
        });

        return reply.code(200).send(plans);
      } catch (err) {
        request.log.error({ err }, 'Failed to list subscription plans');
        return reply.code(500).send({ error: 'Failed to list plans' });
      }
    }
  );

  /**
   * PATCH /subscriptions/plans/:id — Update a subscription plan
   */
  app.patch<{ Params: { id: string }; Body: UpdatePlanBody }>(
    '/subscriptions/plans/:id',
    async (
      request: FastifyRequest<{ Params: { id: string }; Body: UpdatePlanBody }>,
      reply: FastifyReply
    ) => {
      const merchantId = (request as any).merchantId as string;
      if (!merchantId) {
        return reply.code(401).send({ error: 'Unauthorized' });
      }

      const { id } = request.params;
      const validation = validateUpdatePlan(request.body);
      if (!validation.valid || !validation.data) {
        return reply.code(400).send({ error: validation.error });
      }

      try {
        // Verify plan belongs to merchant
        const existing = await prisma.subscriptionPlan.findFirst({
          where: { id, merchantId },
        });

        if (!existing) {
          return reply.code(404).send({ error: 'Plan not found' });
        }

        const updated = await prisma.subscriptionPlan.update({
          where: { id },
          data: validation.data,
        });

        return reply.code(200).send(updated);
      } catch (err) {
        request.log.error({ err }, 'Failed to update subscription plan');
        return reply.code(500).send({ error: 'Failed to update plan' });
      }
    }
  );

  // ---- Subscriptions ----

  /**
   * GET /subscriptions — List subscriptions with pagination and optional status filter
   */
  app.get<{ Querystring: ListSubscriptionsQuery }>(
    '/subscriptions',
    async (
      request: FastifyRequest<{ Querystring: ListSubscriptionsQuery }>,
      reply: FastifyReply
    ) => {
      const merchantId = (request as any).merchantId as string;
      if (!merchantId) {
        return reply.code(401).send({ error: 'Unauthorized' });
      }

      const { status, page: pageStr, limit: limitStr } = request.query;
      const page = Math.max(1, parseInt(pageStr ?? '1', 10) || 1);
      const limit = Math.min(100, Math.max(1, parseInt(limitStr ?? '20', 10) || 20));
      const skip = (page - 1) * limit;

      const where: Record<string, unknown> = { merchantId };
      if (status) {
        const upperStatus = status.toUpperCase();
        const validStatuses = ['TRIALING', 'ACTIVE', 'PAST_DUE', 'PAUSED', 'CANCELED'];
        if (validStatuses.includes(upperStatus)) {
          where.status = upperStatus as SubscriptionStatus;
        }
      }

      try {
        const [subscriptions, total] = await Promise.all([
          prisma.subscription.findMany({
            where: where as any,
            include: { plan: true },
            skip,
            take: limit,
            orderBy: { createdAt: 'desc' },
          }),
          prisma.subscription.count({ where: where as any }),
        ]);

        return reply.code(200).send({
          data: subscriptions,
          pagination: {
            page,
            limit,
            total,
            totalPages: Math.ceil(total / limit),
          },
        });
      } catch (err) {
        request.log.error({ err }, 'Failed to list subscriptions');
        return reply.code(500).send({ error: 'Failed to list subscriptions' });
      }
    }
  );

  /**
   * POST /subscriptions/:id/cancel — Cancel a subscription
   */
  app.post<{ Params: { id: string }; Body: CancelSubscriptionBody }>(
    '/subscriptions/:id/cancel',
    async (
      request: FastifyRequest<{
        Params: { id: string };
        Body: CancelSubscriptionBody;
      }>,
      reply: FastifyReply
    ) => {
      const merchantId = (request as any).merchantId as string;
      if (!merchantId) {
        return reply.code(401).send({ error: 'Unauthorized' });
      }

      const { id } = request.params;
      const { reason } = (request.body ?? {}) as CancelSubscriptionBody;

      try {
        // Verify subscription belongs to merchant
        const subscription = await prisma.subscription.findFirst({
          where: { id, merchantId },
          include: { merchant: true },
        });

        if (!subscription) {
          return reply.code(404).send({ error: 'Subscription not found' });
        }

        if (subscription.status === 'CANCELED') {
          return reply.code(400).send({ error: 'Subscription is already canceled' });
        }

        // Cancel in NMI if we have a subscription ID
        if (subscription.nmiSubscriptionId && subscription.merchant.nmiSecurityKey) {
          const nmi = new NmiService({
            securityKey: subscription.merchant.nmiSecurityKey,
          });

          const nmiResult = await nmi.cancelSubscription(
            subscription.nmiSubscriptionId
          );

          if (nmiResult.response !== '1') {
            request.log.warn(
              { nmiResult },
              'NMI subscription cancellation returned non-success'
            );
            // Continue with local cancellation even if NMI fails
          }
        }

        // Update local subscription status
        const updated = await prisma.subscription.update({
          where: { id },
          data: {
            status: 'CANCELED',
            canceledAt: new Date(),
            cancelReason: reason ?? null,
          },
        });

        return reply.code(200).send(updated);
      } catch (err) {
        request.log.error({ err }, 'Failed to cancel subscription');
        return reply.code(500).send({ error: 'Failed to cancel subscription' });
      }
    }
  );
}

export default registerSubscriptionRoutes;
