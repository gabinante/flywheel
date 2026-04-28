import { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { z } from 'zod';
import bcrypt from 'bcryptjs';
import { prisma } from '../utils/prisma.js';
import { generateApiKey, hashApiKey } from '../utils/api-keys.js';

const registerSchema = z.object({
  businessName: z.string().min(1),
  contactEmail: z.string().email(),
  password: z.string().min(8),
  contactPhone: z.string().optional(),
  website: z.string().url().optional(),
});

const loginSchema = z.object({
  email: z.string().email(),
  password: z.string(),
});

export async function merchantAuthRoutes(app: FastifyInstance) {
  // Register
  app.post('/register', async (request: FastifyRequest, reply: FastifyReply) => {
    const body = registerSchema.parse(request.body);

    const existing = await prisma.merchant.findUnique({
      where: { contactEmail: body.contactEmail },
    });

    if (existing) {
      return reply.status(409).send({ error: 'Email already registered' });
    }

    const passwordHash = await bcrypt.hash(body.password, 12);

    // Generate API keys
    const liveKey = generateApiKey('sk_live');
    const testKey = generateApiKey('sk_test');

    const merchant = await prisma.merchant.create({
      data: {
        businessName: body.businessName,
        contactEmail: body.contactEmail,
        passwordHash,
        contactPhone: body.contactPhone,
        website: body.website,
        status: 'PENDING',
        apiKeyLive: liveKey,
        apiKeyTest: testKey,
        apiKeyLiveHash: hashApiKey(liveKey),
        apiKeyTestHash: hashApiKey(testKey),
      },
    });

    // Create application for onboarding
    await prisma.application.create({
      data: {
        merchantId: merchant.id,
        status: 'DRAFT',
      },
    });

    const token = app.jwt.sign(
      { id: merchant.id, email: merchant.contactEmail, type: 'merchant' },
      { expiresIn: '15m' }
    );

    const refreshToken = app.jwt.sign(
      { id: merchant.id, type: 'merchant-refresh' },
      { expiresIn: '7d' }
    );

    return {
      token,
      refreshToken,
      merchant: {
        id: merchant.id,
        businessName: merchant.businessName,
        email: merchant.contactEmail,
        status: merchant.status,
      },
      apiKeys: {
        live: liveKey,
        test: testKey,
        note: 'Store these securely. They will not be shown again.',
      },
    };
  });

  // Login
  app.post('/login', async (request: FastifyRequest, reply: FastifyReply) => {
    const body = loginSchema.parse(request.body);

    const merchant = await prisma.merchant.findUnique({
      where: { contactEmail: body.email },
    });

    if (!merchant || !merchant.passwordHash) {
      return reply.status(401).send({ error: 'Invalid credentials' });
    }

    const valid = await bcrypt.compare(body.password, merchant.passwordHash);
    if (!valid) {
      return reply.status(401).send({ error: 'Invalid credentials' });
    }

    const token = app.jwt.sign(
      { id: merchant.id, email: merchant.contactEmail, type: 'merchant' },
      { expiresIn: '15m' }
    );

    const refreshToken = app.jwt.sign(
      { id: merchant.id, type: 'merchant-refresh' },
      { expiresIn: '7d' }
    );

    return {
      token,
      refreshToken,
      merchant: {
        id: merchant.id,
        businessName: merchant.businessName,
        email: merchant.contactEmail,
        status: merchant.status,
      },
    };
  });

  // Refresh token
  app.post('/refresh', async (request: FastifyRequest, reply: FastifyReply) => {
    try {
      const payload = await request.jwtVerify<{ id: string; type: string }>();
      if (payload.type !== 'merchant-refresh') {
        return reply.status(401).send({ error: 'Invalid refresh token' });
      }

      const merchant = await prisma.merchant.findUnique({
        where: { id: payload.id },
      });

      if (!merchant) {
        return reply.status(401).send({ error: 'Merchant not found' });
      }

      const token = app.jwt.sign(
        { id: merchant.id, email: merchant.contactEmail, type: 'merchant' },
        { expiresIn: '15m' }
      );

      return { token };
    } catch {
      return reply.status(401).send({ error: 'Invalid refresh token' });
    }
  });
}
