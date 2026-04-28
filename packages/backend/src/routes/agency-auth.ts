import { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { z } from 'zod';
import bcrypt from 'bcrypt';
import { nanoid } from 'nanoid';
import prisma from '../utils/prisma';
import { generateTokenPair } from '../utils/jwt';

// ─── Zod Schemas ──────────────────────────────────────────────────────

const registerSchema = z.object({
  name: z.string().min(1, 'Name is required').max(255),
  contactEmail: z.string().email('Invalid email address'),
  password: z.string().min(8, 'Password must be at least 8 characters'),
  contactPhone: z.string().optional(),
  ref: z.string().optional(), // referral code from another agency
});

const loginSchema = z.object({
  email: z.string().email('Invalid email address'),
  password: z.string().min(1, 'Password is required'),
});

const refreshSchema = z.object({
  refreshToken: z.string().min(1, 'Refresh token is required'),
});

// ─── Constants ────────────────────────────────────────────────────────

const BCRYPT_ROUNDS = 12;
const MAX_REFERRAL_CODE_RETRIES = 5;

// ─── Helpers ──────────────────────────────────────────────────────────

/**
 * Generate a unique referral code with collision retry.
 * Format: agency-{nanoid(8)}
 */
async function generateUniqueReferralCode(): Promise<string> {
  for (let i = 0; i < MAX_REFERRAL_CODE_RETRIES; i++) {
    const code = `agency-${nanoid(8)}`;
    const existing = await prisma.agency.findUnique({
      where: { referralCode: code },
    });
    if (!existing) return code;
  }
  // Extremely unlikely to reach here
  throw new Error('Failed to generate unique referral code');
}

// ─── Route Registration ───────────────────────────────────────────────

export async function agencyAuthRoutes(app: FastifyInstance) {
  /**
   * POST /register — Agency registration
   */
  app.post('/register', async (request: FastifyRequest, reply: FastifyReply) => {
    // 1. Validate input
    const parseResult = registerSchema.safeParse(request.body);
    if (!parseResult.success) {
      return reply.status(400).send({
        error: 'Validation Error',
        message: parseResult.error.issues.map((i) => i.message).join(', '),
        details: parseResult.error.issues,
      });
    }
    const { name, contactEmail, password, contactPhone, ref } = parseResult.data;

    // 2. Check email uniqueness
    const existingAgency = await prisma.agency.findUnique({
      where: { contactEmail },
    });
    if (existingAgency) {
      return reply.status(409).send({
        error: 'Conflict',
        message: 'An agency with this email already exists',
      });
    }

    // 3. Resolve referral code (if provided)
    let referredByAgencyId: string | null = null;
    if (ref) {
      const referrer = await prisma.agency.findUnique({
        where: { referralCode: ref },
      });
      // If invalid ref, register anyway with null referrer — don't block registration
      if (referrer) {
        referredByAgencyId = referrer.id;
      }
    }

    // 4. Generate unique referral code
    const referralCode = await generateUniqueReferralCode();

    // 5. Hash password with bcrypt (12 rounds)
    const passwordHash = await bcrypt.hash(password, BCRYPT_ROUNDS);

    // 6. Create Agency record
    const agency = await prisma.agency.create({
      data: {
        name,
        contactEmail,
        passwordHash,
        contactPhone: contactPhone || null,
        referralCode,
        referredByAgencyId,
      },
    });

    // 7. Return JWT + refresh token
    const tokens = generateTokenPair(app, agency.id);

    return reply.status(201).send({
      agency: {
        id: agency.id,
        name: agency.name,
        contactEmail: agency.contactEmail,
        referralCode: agency.referralCode,
      },
      ...tokens,
    });
  });

  /**
   * POST /login — Agency login
   */
  app.post('/login', async (request: FastifyRequest, reply: FastifyReply) => {
    // Validate input
    const parseResult = loginSchema.safeParse(request.body);
    if (!parseResult.success) {
      return reply.status(400).send({
        error: 'Validation Error',
        message: parseResult.error.issues.map((i) => i.message).join(', '),
        details: parseResult.error.issues,
      });
    }
    const { email, password } = parseResult.data;

    // Find agency by email
    const agency = await prisma.agency.findUnique({
      where: { contactEmail: email },
    });
    if (!agency) {
      return reply.status(401).send({
        error: 'Unauthorized',
        message: 'Invalid email or password',
      });
    }

    // Check agency status
    if (agency.status === 'SUSPENDED') {
      return reply.status(403).send({
        error: 'Forbidden',
        message: 'Account is suspended. Please contact support.',
      });
    }
    if (agency.status === 'CHURNED') {
      return reply.status(403).send({
        error: 'Forbidden',
        message: 'Account is no longer active. Please contact support.',
      });
    }

    // Verify password
    const passwordValid = await bcrypt.compare(password, agency.passwordHash);
    if (!passwordValid) {
      return reply.status(401).send({
        error: 'Unauthorized',
        message: 'Invalid email or password',
      });
    }

    // Return JWT + refresh token
    const tokens = generateTokenPair(app, agency.id);

    return reply.send({
      agency: {
        id: agency.id,
        name: agency.name,
        contactEmail: agency.contactEmail,
        referralCode: agency.referralCode,
      },
      ...tokens,
    });
  });

  /**
   * POST /refresh — Refresh access token
   */
  app.post('/refresh', async (request: FastifyRequest, reply: FastifyReply) => {
    // Validate input
    const parseResult = refreshSchema.safeParse(request.body);
    if (!parseResult.success) {
      return reply.status(400).send({
        error: 'Validation Error',
        message: parseResult.error.issues.map((i) => i.message).join(', '),
        details: parseResult.error.issues,
      });
    }
    const { refreshToken } = parseResult.data;

    try {
      // Verify the refresh token
      const decoded = app.jwt.verify<{
        sub: string;
        type: string;
        tokenType?: string;
      }>(refreshToken);

      // Must be an agency refresh token
      if (decoded.type !== 'agency' || decoded.tokenType !== 'refresh') {
        return reply.status(401).send({
          error: 'Unauthorized',
          message: 'Invalid refresh token',
        });
      }

      // Verify agency still exists and is active
      const agency = await prisma.agency.findUnique({
        where: { id: decoded.sub },
      });
      if (!agency) {
        return reply.status(401).send({
          error: 'Unauthorized',
          message: 'Agency not found',
        });
      }
      if (agency.status === 'SUSPENDED' || agency.status === 'CHURNED') {
        return reply.status(403).send({
          error: 'Forbidden',
          message: 'Account is not active',
        });
      }

      // Generate new token pair
      const tokens = generateTokenPair(app, agency.id);
      return reply.send(tokens);
    } catch (err) {
      return reply.status(401).send({
        error: 'Unauthorized',
        message: 'Invalid or expired refresh token',
      });
    }
  });
}
