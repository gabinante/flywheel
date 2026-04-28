import { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { z } from 'zod';
import bcrypt from 'bcryptjs';
import { prisma } from '../utils/prisma.js';
import { verifyAdmin } from '../plugins/admin-auth.js';

const bootstrapSchema = z.object({
  email: z.string().email(),
  password: z.string().min(8),
  name: z.string().min(1),
});

const loginSchema = z.object({
  email: z.string().email(),
  password: z.string(),
});

const changePasswordSchema = z.object({
  currentPassword: z.string(),
  newPassword: z.string().min(8),
});

export async function adminAuthRoutes(app: FastifyInstance) {
  // Bootstrap first super admin
  app.post('/bootstrap', async (request: FastifyRequest, reply: FastifyReply) => {
    const existing = await prisma.adminUser.findFirst();
    if (existing) {
      return reply.status(400).send({ error: 'Admin already exists. Bootstrap disabled.' });
    }

    const body = bootstrapSchema.parse(request.body);
    const passwordHash = await bcrypt.hash(body.password, 12);

    const admin = await prisma.adminUser.create({
      data: {
        email: body.email,
        passwordHash,
        name: body.name,
        role: 'SUPER_ADMIN',
      },
    });

    const token = app.jwt.sign(
      { id: admin.id, email: admin.email, role: admin.role, type: 'admin' },
      { expiresIn: '15m' }
    );

    return { token, admin: { id: admin.id, email: admin.email, name: admin.name, role: admin.role } };
  });

  // Login
  app.post('/login', async (request: FastifyRequest, reply: FastifyReply) => {
    const body = loginSchema.parse(request.body);

    const admin = await prisma.adminUser.findUnique({
      where: { email: body.email },
    });

    if (!admin || !admin.active) {
      return reply.status(401).send({ error: 'Invalid credentials' });
    }

    const valid = await bcrypt.compare(body.password, admin.passwordHash);
    if (!valid) {
      return reply.status(401).send({ error: 'Invalid credentials' });
    }

    await prisma.adminUser.update({
      where: { id: admin.id },
      data: { lastLoginAt: new Date() },
    });

    const token = app.jwt.sign(
      { id: admin.id, email: admin.email, role: admin.role, type: 'admin' },
      { expiresIn: '15m' }
    );

    const refreshToken = app.jwt.sign(
      { id: admin.id, type: 'admin-refresh' },
      { expiresIn: '7d' }
    );

    return {
      token,
      refreshToken,
      admin: { id: admin.id, email: admin.email, name: admin.name, role: admin.role },
    };
  });

  // Change password
  app.post(
    '/change-password',
    { preHandler: [verifyAdmin] },
    async (request: FastifyRequest, reply: FastifyReply) => {
      const body = changePasswordSchema.parse(request.body);

      const admin = await prisma.adminUser.findUnique({
        where: { id: request.admin!.id },
      });

      if (!admin) {
        return reply.status(404).send({ error: 'Admin not found' });
      }

      const valid = await bcrypt.compare(body.currentPassword, admin.passwordHash);
      if (!valid) {
        return reply.status(401).send({ error: 'Current password is incorrect' });
      }

      const passwordHash = await bcrypt.hash(body.newPassword, 12);
      await prisma.adminUser.update({
        where: { id: admin.id },
        data: { passwordHash },
      });

      return { success: true };
    }
  );

  // Refresh token
  app.post('/refresh', async (request: FastifyRequest, reply: FastifyReply) => {
    try {
      const payload = await request.jwtVerify<{ id: string; type: string }>();
      if (payload.type !== 'admin-refresh') {
        return reply.status(401).send({ error: 'Invalid refresh token' });
      }

      const admin = await prisma.adminUser.findUnique({
        where: { id: payload.id },
      });

      if (!admin || !admin.active) {
        return reply.status(401).send({ error: 'Admin not found' });
      }

      const token = app.jwt.sign(
        { id: admin.id, email: admin.email, role: admin.role, type: 'admin' },
        { expiresIn: '15m' }
      );

      return { token };
    } catch {
      return reply.status(401).send({ error: 'Invalid refresh token' });
    }
  });
}
