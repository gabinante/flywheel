import { FastifyRequest, FastifyReply } from 'fastify';
import { prisma } from '../utils/prisma.js';
import { hashApiKey } from '../utils/api-keys.js';

export interface MerchantTokenPayload {
  id: string;
  email: string;
  type: 'merchant';
}

export async function verifyMerchant(request: FastifyRequest, reply: FastifyReply) {
  try {
    const payload = await request.jwtVerify<MerchantTokenPayload>();
    if (payload.type !== 'merchant') {
      return reply.status(401).send({ error: 'Invalid token type' });
    }

    const merchant = await prisma.merchant.findUnique({
      where: { id: payload.id },
    });

    if (!merchant || merchant.status === 'DEACTIVATED') {
      return reply.status(401).send({ error: 'Merchant not found or deactivated' });
    }

    request.merchant = { id: merchant.id, email: merchant.contactEmail, businessName: merchant.businessName };
  } catch {
    return reply.status(401).send({ error: 'Unauthorized' });
  }
}

export async function verifyApiKey(request: FastifyRequest, reply: FastifyReply) {
  const apiKey = request.headers['x-api-key'] as string;
  if (!apiKey) {
    return reply.status(401).send({ error: 'API key required' });
  }

  const keyHash = hashApiKey(apiKey);
  const isLive = apiKey.startsWith('sk_live_');
  const isTest = apiKey.startsWith('sk_test_');

  if (!isLive && !isTest) {
    return reply.status(401).send({ error: 'Invalid API key format' });
  }

  const merchant = await prisma.merchant.findFirst({
    where: isLive
      ? { apiKeyLiveHash: keyHash }
      : { apiKeyTestHash: keyHash },
  });

  if (!merchant || merchant.status !== 'ACTIVE') {
    return reply.status(401).send({ error: 'Invalid API key' });
  }

  request.merchant = { id: merchant.id, email: merchant.contactEmail, businessName: merchant.businessName };
  request.isTestMode = isTest;
}

declare module 'fastify' {
  interface FastifyRequest {
    merchant?: {
      id: string;
      email: string;
      businessName: string;
    };
    isTestMode?: boolean;
  }
}
