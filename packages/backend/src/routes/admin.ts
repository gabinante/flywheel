import type { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';

/**
 * Accepted body for PATCH /admin/merchants/:id
 */
interface UpdateMerchantBody {
  name?: string;
  email?: string;
  isActive?: boolean;
  webhookUrl?: string;
  rateLimitOverride?: {
    checkout?: number;
    read?: number;
    write?: number;
  } | null;
}

/**
 * Validate that rateLimitOverride values (if provided) are positive integers.
 */
function validateRateLimitOverride(
  override: UpdateMerchantBody['rateLimitOverride']
): string | null {
  if (override === null || override === undefined) {
    return null; // null clears the override, undefined means no change
  }

  if (typeof override !== 'object') {
    return 'rateLimitOverride must be an object or null';
  }

  for (const [key, value] of Object.entries(override)) {
    if (!['checkout', 'read', 'write'].includes(key)) {
      return `rateLimitOverride contains unknown key: ${key}`;
    }
    if (value !== undefined) {
      if (typeof value !== 'number' || !Number.isInteger(value) || value < 1) {
        return `rateLimitOverride.${key} must be a positive integer`;
      }
    }
  }

  return null;
}

/**
 * Register admin routes.
 *
 * Admin routes are excluded from per-merchant rate limiting
 * (they use separate admin auth, checked by admin auth middleware).
 */
export async function registerAdminRoutes(app: FastifyInstance): Promise<void> {
  /**
   * PATCH /admin/merchants/:id
   *
   * Update merchant settings including rate limit overrides.
   *
   * Body (all fields optional):
   * - name: string
   * - email: string
   * - isActive: boolean
   * - webhookUrl: string
   * - rateLimitOverride: { checkout?: number, read?: number, write?: number } | null
   *
   * Setting rateLimitOverride to null clears the override (defaults apply).
   */
  app.patch<{
    Params: { id: string };
    Body: UpdateMerchantBody;
  }>(
    '/merchants/:id',
    async (
      request: FastifyRequest<{
        Params: { id: string };
        Body: UpdateMerchantBody;
      }>,
      reply: FastifyReply
    ) => {
      const { id } = request.params;
      const body = request.body;

      // Validate rateLimitOverride if present
      if ('rateLimitOverride' in body) {
        const validationError = validateRateLimitOverride(
          body.rateLimitOverride
        );
        if (validationError) {
          return reply.code(400).send({
            statusCode: 400,
            error: 'Bad Request',
            message: validationError,
          });
        }
      }

      try {
        const { PrismaClient } = await import('@prisma/client');
        const prisma = new PrismaClient();

        // Check merchant exists
        const existing = await prisma.merchant.findUnique({
          where: { id },
        });

        if (!existing) {
          await prisma.$disconnect();
          return reply.code(404).send({
            statusCode: 404,
            error: 'Not Found',
            message: `Merchant ${id} not found`,
          });
        }

        // Build update data
        const updateData: Record<string, unknown> = {};
        if (body.name !== undefined) updateData.name = body.name;
        if (body.email !== undefined) updateData.email = body.email;
        if (body.isActive !== undefined) updateData.isActive = body.isActive;
        if (body.webhookUrl !== undefined) updateData.webhookUrl = body.webhookUrl;
        if ('rateLimitOverride' in body) {
          // null clears the override, object sets it
          updateData.rateLimitOverride =
            body.rateLimitOverride === null
              ? null
              : body.rateLimitOverride;
        }

        const updated = await prisma.merchant.update({
          where: { id },
          data: updateData,
          select: {
            id: true,
            name: true,
            email: true,
            isActive: true,
            webhookUrl: true,
            rateLimitOverride: true,
            createdAt: true,
            updatedAt: true,
          },
        });

        await prisma.$disconnect();

        return reply.code(200).send(updated);
      } catch (err) {
        request.log.error({ err }, 'Failed to update merchant');
        return reply.code(500).send({
          statusCode: 500,
          error: 'Internal Server Error',
          message: 'Failed to update merchant',
        });
      }
    }
  );

  /**
   * GET /admin/merchants/:id
   *
   * Get merchant details including rate limit overrides.
   */
  app.get<{
    Params: { id: string };
  }>(
    '/merchants/:id',
    async (
      request: FastifyRequest<{ Params: { id: string } }>,
      reply: FastifyReply
    ) => {
      const { id } = request.params;

      try {
        const { PrismaClient } = await import('@prisma/client');
        const prisma = new PrismaClient();

        const merchant = await prisma.merchant.findUnique({
          where: { id },
          select: {
            id: true,
            name: true,
            email: true,
            isActive: true,
            webhookUrl: true,
            rateLimitOverride: true,
            createdAt: true,
            updatedAt: true,
          },
        });

        await prisma.$disconnect();

        if (!merchant) {
          return reply.code(404).send({
            statusCode: 404,
            error: 'Not Found',
            message: `Merchant ${id} not found`,
          });
        }

        return reply.code(200).send(merchant);
      } catch (err) {
        request.log.error({ err }, 'Failed to get merchant');
        return reply.code(500).send({
          statusCode: 500,
          error: 'Internal Server Error',
          message: 'Failed to get merchant',
        });
      }
    }
  );
}

export default registerAdminRoutes;
