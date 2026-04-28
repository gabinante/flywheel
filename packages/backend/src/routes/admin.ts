import { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { z } from 'zod';
import bcrypt from 'bcryptjs';
import { prisma } from '../utils/prisma.js';
import { verifyAdmin, verifySuperAdmin } from '../plugins/admin-auth.js';

export async function adminRoutes(app: FastifyInstance) {
  // All admin routes require auth
  app.addHook('preHandler', verifyAdmin);

  // ── Merchants ──────────────────────────────────────────

  app.get('/merchants', async (request: FastifyRequest, reply: FastifyReply) => {
    const { page = '1', limit = '20', status, search } = request.query as Record<string, string>;
    const skip = (Number(page) - 1) * Number(limit);

    const where: Record<string, unknown> = {};
    if (status) where.status = status;
    if (search) {
      where.OR = [
        { businessName: { contains: search, mode: 'insensitive' } },
        { contactEmail: { contains: search, mode: 'insensitive' } },
      ];
    }

    const [merchants, total] = await Promise.all([
      prisma.merchant.findMany({
        where,
        skip,
        take: Number(limit),
        orderBy: { createdAt: 'desc' },
        select: {
          id: true,
          businessName: true,
          contactEmail: true,
          status: true,
          createdAt: true,
          ghlLocationId: true,
          nmiTokenizationKey: true, // show if NMI is configured
        },
      }),
      prisma.merchant.count({ where }),
    ]);

    // Add a flag to indicate if payment processing is configured
    const enriched = merchants.map(({ nmiTokenizationKey, ...m }) => ({
      ...m,
      processingConfigured: !!nmiTokenizationKey,
    }));

    return { merchants: enriched, total, page: Number(page), limit: Number(limit) };
  });

  app.get('/merchants/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
    const merchant = await prisma.merchant.findUnique({
      where: { id: request.params.id },
      include: {
        application: true,
      },
    });

    if (!merchant) return reply.status(404).send({ error: 'Merchant not found' });

    // Strip sensitive fields
    const { passwordHash, apiKeyLive, apiKeyTest, ghlAccessToken, ghlRefreshToken, nmiSecurityKey, seamlesschexApiKey, ...safe } = merchant;
    return {
      ...safe,
      processingConfigured: !!merchant.nmiSecurityKey,
      achConfigured: !!merchant.seamlesschexApiKey,
    };
  });

  app.patch('/merchants/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
    const schema = z.object({
      status: z.enum(['PENDING', 'ACTIVE', 'SUSPENDED', 'DEACTIVATED']).optional(),
      businessName: z.string().optional(),
      // Merchant's own NMI credentials
      nmiSecurityKey: z.string().optional(),
      nmiTokenizationKey: z.string().optional(),
      // Merchant's own Seamlesschex credentials
      seamlesschexApiKey: z.string().optional(),
    });

    const body = schema.parse(request.body);
    const merchant = await prisma.merchant.findUnique({ where: { id: request.params.id } });
    if (!merchant) return reply.status(404).send({ error: 'Merchant not found' });

    const updated = await prisma.merchant.update({
      where: { id: request.params.id },
      data: body,
    });

    await prisma.auditLog.create({
      data: {
        merchantId: merchant.id,
        actorType: 'admin',
        actorId: request.admin!.id,
        action: 'UPDATE_MERCHANT',
        resource: 'merchant',
        resourceId: merchant.id,
        details: {
          ...body,
          // Don't log raw keys in audit
          nmiSecurityKey: body.nmiSecurityKey ? '***updated***' : undefined,
          seamlesschexApiKey: body.seamlesschexApiKey ? '***updated***' : undefined,
        },
        ipAddress: request.ip,
      },
    });

    return { id: updated.id, status: updated.status };
  });

  // ── Transactions ───────────────────────────────────────

  app.get('/transactions', async (request: FastifyRequest, reply: FastifyReply) => {
    const { page = '1', limit = '20', merchantId, status, paymentMethod, from, to } = request.query as Record<string, string>;
    const skip = (Number(page) - 1) * Number(limit);

    const where: Record<string, unknown> = {};
    if (merchantId) where.merchantId = merchantId;
    if (status) where.status = status;
    if (paymentMethod) where.paymentMethod = paymentMethod;
    if (from || to) {
      where.createdAt = {};
      if (from) (where.createdAt as Record<string, unknown>).gte = new Date(from);
      if (to) (where.createdAt as Record<string, unknown>).lte = new Date(to);
    }

    const [transactions, total] = await Promise.all([
      prisma.transaction.findMany({
        where,
        skip,
        take: Number(limit),
        orderBy: { createdAt: 'desc' },
        include: {
          merchant: { select: { businessName: true } },
          customer: { select: { email: true, firstName: true, lastName: true } },
        },
      }),
      prisma.transaction.count({ where }),
    ]);

    return { transactions, total, page: Number(page), limit: Number(limit) };
  });

  app.get('/transactions/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
    const transaction = await prisma.transaction.findUnique({
      where: { id: request.params.id },
      include: {
        merchant: { select: { businessName: true, contactEmail: true } },
        customer: true,
        chargebacks: true,
        seamlesschexTransaction: true,
      },
    });

    if (!transaction) return reply.status(404).send({ error: 'Transaction not found' });
    return transaction;
  });

  // ── Chargebacks (read-only view — NMI handles disputes) ──

  app.get('/chargebacks', async (request: FastifyRequest, reply: FastifyReply) => {
    const { page = '1', limit = '20', status } = request.query as Record<string, string>;
    const skip = (Number(page) - 1) * Number(limit);

    const where: Record<string, unknown> = {};
    if (status) where.status = status;

    const [chargebacks, total] = await Promise.all([
      prisma.chargeback.findMany({
        where,
        skip,
        take: Number(limit),
        orderBy: { createdAt: 'desc' },
        include: {
          merchant: { select: { businessName: true } },
          transaction: { select: { amountCents: true, paymentMethod: true } },
        },
      }),
      prisma.chargeback.count({ where }),
    ]);

    return { chargebacks, total, page: Number(page), limit: Number(limit) };
  });

  // ── Staff Management ───────────────────────────────────

  app.get('/staff', { preHandler: [verifySuperAdmin] }, async (request: FastifyRequest, reply: FastifyReply) => {
    const admins = await prisma.adminUser.findMany({
      select: { id: true, email: true, name: true, role: true, active: true, lastLoginAt: true, createdAt: true },
      orderBy: { createdAt: 'desc' },
    });
    return { staff: admins };
  });

  app.post('/staff', { preHandler: [verifySuperAdmin] }, async (request: FastifyRequest, reply: FastifyReply) => {
    const schema = z.object({
      email: z.string().email(),
      name: z.string().min(1),
      password: z.string().min(8),
      role: z.enum(['SUPER_ADMIN', 'OPS_ADMIN']).default('OPS_ADMIN'),
    });

    const body = schema.parse(request.body);
    const passwordHash = await bcrypt.hash(body.password, 12);

    const admin = await prisma.adminUser.create({
      data: { email: body.email, name: body.name, passwordHash, role: body.role },
    });

    return { id: admin.id, email: admin.email, name: admin.name, role: admin.role };
  });

  // ── Analytics ──────────────────────────────────────────

  app.get('/analytics/daily', async (request: FastifyRequest, reply: FastifyReply) => {
    const { from, to } = request.query as Record<string, string>;

    const where: Record<string, unknown> = {};
    if (from || to) {
      where.date = {};
      if (from) (where.date as Record<string, unknown>).gte = new Date(from);
      if (to) (where.date as Record<string, unknown>).lte = new Date(to);
    }

    const metrics = await prisma.dailyMetrics.findMany({
      where,
      orderBy: { date: 'desc' },
      take: 90,
    });

    return { metrics };
  });

  app.get('/analytics/summary', async (request: FastifyRequest, reply: FastifyReply) => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);

    const [totalMerchants, activeMerchants, todayTx, todayVolume, totalChargebacks] = await Promise.all([
      prisma.merchant.count(),
      prisma.merchant.count({ where: { status: 'ACTIVE' } }),
      prisma.transaction.count({ where: { createdAt: { gte: today } } }),
      prisma.transaction.aggregate({
        where: { createdAt: { gte: today }, status: { in: ['CAPTURED', 'SETTLED'] } },
        _sum: { amountCents: true },
      }),
      prisma.chargeback.count({ where: { status: 'OPENED' } }),
    ]);

    return {
      totalMerchants,
      activeMerchants,
      todayTransactions: todayTx,
      todayVolumeCents: todayVolume._sum.amountCents || 0,
      openChargebacks: totalChargebacks,
    };
  });

  // ── Audit Log ──────────────────────────────────────────

  app.get('/audit-log', async (request: FastifyRequest, reply: FastifyReply) => {
    const { page = '1', limit = '50', merchantId, action } = request.query as Record<string, string>;
    const skip = (Number(page) - 1) * Number(limit);

    const where: Record<string, unknown> = {};
    if (merchantId) where.merchantId = merchantId;
    if (action) where.action = action;

    const [logs, total] = await Promise.all([
      prisma.auditLog.findMany({
        where,
        skip,
        take: Number(limit),
        orderBy: { createdAt: 'desc' },
      }),
      prisma.auditLog.count({ where }),
    ]);

    return { logs, total, page: Number(page), limit: Number(limit) };
  });
}
