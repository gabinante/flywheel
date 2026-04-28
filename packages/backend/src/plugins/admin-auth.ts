import { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { prisma } from '../utils/prisma.js';

export interface AdminTokenPayload {
  id: string;
  email: string;
  role: string;
  type: 'admin';
}

export async function verifyAdmin(request: FastifyRequest, reply: FastifyReply) {
  try {
    const payload = await request.jwtVerify<AdminTokenPayload>();
    if (payload.type !== 'admin') {
      return reply.status(401).send({ error: 'Invalid token type' });
    }

    const admin = await prisma.adminUser.findUnique({
      where: { id: payload.id },
    });

    if (!admin || !admin.active) {
      return reply.status(401).send({ error: 'Admin not found or inactive' });
    }

    request.admin = { id: admin.id, email: admin.email, role: admin.role, name: admin.name };
  } catch {
    return reply.status(401).send({ error: 'Unauthorized' });
  }
}

export async function verifySuperAdmin(request: FastifyRequest, reply: FastifyReply) {
  await verifyAdmin(request, reply);
  if (reply.sent) return;
  if (request.admin?.role !== 'SUPER_ADMIN') {
    return reply.status(403).send({ error: 'Super admin required' });
  }
}

// Extend Fastify types
declare module 'fastify' {
  interface FastifyRequest {
    admin?: {
      id: string;
      email: string;
      role: string;
      name: string;
    };
  }
}
