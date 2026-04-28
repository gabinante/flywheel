import { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { z } from 'zod';
import prisma from '../utils/prisma';

// ─── Zod Schemas ──────────────────────────────────────────────────────

/**
 * Only these fields are updatable via PATCH /profile.
 * Immutable fields (contactEmail, referralCode, referredByAgencyId, tier) are rejected.
 */
const updateProfileSchema = z
  .object({
    contactPhone: z.string().nullable().optional(),
    payoutEmail: z.string().email('Invalid payout email').nullable().optional(),
    payoutBankLast4: z
      .string()
      .regex(/^\d{4}$/, 'Must be exactly 4 digits')
      .nullable()
      .optional(),
  })
  .strict(); // .strict() rejects any extra/unknown keys

// Fields that CANNOT be changed via the profile endpoint
const IMMUTABLE_FIELDS = [
  'contactEmail',
  'referralCode',
  'referredByAgencyId',
  'tier',
  'tierOverride',
  'status',
  'id',
  'name',
  'passwordHash',
] as const;

// ─── Route Registration ───────────────────────────────────────────────

export async function agencyRoutes(app: FastifyInstance) {
  // All routes here require agency authentication
  app.addHook('onRequest', app.authenticateAgency);

  /**
   * GET /profile — Return agency details
   */
  app.get('/profile', async (request: FastifyRequest, reply: FastifyReply) => {
    const agencyId = request.agencyId!;

    const agency = await prisma.agency.findUnique({
      where: { id: agencyId },
      include: {
        referredByAgency: {
          select: { name: true },
        },
      },
    });

    if (!agency) {
      return reply.status(404).send({
        error: 'Not Found',
        message: 'Agency not found',
      });
    }

    return reply.send({
      id: agency.id,
      name: agency.name,
      contactEmail: agency.contactEmail,
      contactPhone: agency.contactPhone,
      referralCode: agency.referralCode,
      tier: agency.tier,
      status: agency.status,
      payoutEmail: agency.payoutEmail,
      payoutBankLast4: agency.payoutBankLast4,
      referredByAgency: agency.referredByAgency
        ? { name: agency.referredByAgency.name }
        : null,
      createdAt: agency.createdAt,
    });
  });

  /**
   * PATCH /profile — Update mutable agency fields
   */
  app.patch('/profile', async (request: FastifyRequest, reply: FastifyReply) => {
    const agencyId = request.agencyId!;

    // Check for attempts to change immutable fields
    const body = request.body as Record<string, unknown> | undefined;
    if (body) {
      const attemptedImmutable = IMMUTABLE_FIELDS.filter(
        (field) => field in body
      );
      if (attemptedImmutable.length > 0) {
        return reply.status(400).send({
          error: 'Validation Error',
          message: `Cannot modify immutable fields: ${attemptedImmutable.join(', ')}`,
        });
      }
    }

    // Validate input with strict schema (rejects unknown keys)
    const parseResult = updateProfileSchema.safeParse(request.body);
    if (!parseResult.success) {
      return reply.status(400).send({
        error: 'Validation Error',
        message: parseResult.error.issues.map((i) => i.message).join(', '),
        details: parseResult.error.issues,
      });
    }

    const updateData = parseResult.data;

    // Don't update if no fields provided
    if (Object.keys(updateData).length === 0) {
      return reply.status(400).send({
        error: 'Validation Error',
        message: 'No fields to update',
      });
    }

    // Build update payload (only include defined fields)
    const data: Record<string, unknown> = {};
    if (updateData.contactPhone !== undefined) data.contactPhone = updateData.contactPhone;
    if (updateData.payoutEmail !== undefined) data.payoutEmail = updateData.payoutEmail;
    if (updateData.payoutBankLast4 !== undefined) data.payoutBankLast4 = updateData.payoutBankLast4;

    const agency = await prisma.agency.update({
      where: { id: agencyId },
      data,
      include: {
        referredByAgency: {
          select: { name: true },
        },
      },
    });

    return reply.send({
      id: agency.id,
      name: agency.name,
      contactEmail: agency.contactEmail,
      contactPhone: agency.contactPhone,
      referralCode: agency.referralCode,
      tier: agency.tier,
      status: agency.status,
      payoutEmail: agency.payoutEmail,
      payoutBankLast4: agency.payoutBankLast4,
      referredByAgency: agency.referredByAgency
        ? { name: agency.referredByAgency.name }
        : null,
      createdAt: agency.createdAt,
    });
  });
}
