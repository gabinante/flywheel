import { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { z } from 'zod';
import { prisma } from '../utils/prisma.js';
import { verifyMerchant } from '../plugins/merchant-auth.js';
import { refundTransaction } from '../services/nmi.service.js';
import { generateApiKey, hashApiKey } from '../utils/api-keys.js';

export async function merchantRoutes(app: FastifyInstance) {
  app.addHook('preHandler', verifyMerchant);

  // ── Dashboard ──────────────────────────────────────────

  app.get('/dashboard', async (request: FastifyRequest, reply: FastifyReply) => {
    const merchantId = request.merchant!.id;
    const today = new Date();
    today.setHours(0, 0, 0, 0);

    const thirtyDaysAgo = new Date();
    thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30);

    const [
      todayTx,
      todayVolume,
      monthTx,
      monthVolume,
      recentTransactions,
    ] = await Promise.all([
      prisma.transaction.count({
        where: { merchantId, createdAt: { gte: today } },
      }),
      prisma.transaction.aggregate({
        where: { merchantId, createdAt: { gte: today }, status: { in: ['CAPTURED', 'SETTLED'] } },
        _sum: { amountCents: true },
      }),
      prisma.transaction.count({
        where: { merchantId, createdAt: { gte: thirtyDaysAgo } },
      }),
      prisma.transaction.aggregate({
        where: { merchantId, createdAt: { gte: thirtyDaysAgo }, status: { in: ['CAPTURED', 'SETTLED'] } },
        _sum: { amountCents: true },
      }),
      prisma.transaction.findMany({
        where: { merchantId },
        orderBy: { createdAt: 'desc' },
        take: 10,
        select: {
          id: true,
          amountCents: true,
          status: true,
          paymentMethod: true,
          cardBrand: true,
          cardLast4: true,
          createdAt: true,
          customer: { select: { email: true } },
        },
      }),
    ]);

    return {
      today: {
        transactions: todayTx,
        volumeCents: todayVolume._sum.amountCents || 0,
      },
      month: {
        transactions: monthTx,
        volumeCents: monthVolume._sum.amountCents || 0,
      },
      recentTransactions,
    };
  });

  // ── Transactions ───────────────────────────────────────

  app.get('/transactions', async (request: FastifyRequest, reply: FastifyReply) => {
    const merchantId = request.merchant!.id;
    const { page = '1', limit = '20', status, paymentMethod, from, to } = request.query as Record<string, string>;
    const skip = (Number(page) - 1) * Number(limit);

    const where: Record<string, unknown> = { merchantId };
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
          customer: { select: { email: true, firstName: true, lastName: true } },
        },
      }),
      prisma.transaction.count({ where }),
    ]);

    return { transactions, total, page: Number(page), limit: Number(limit) };
  });

  // ── Refunds ────────────────────────────────────────────

  app.post('/transactions/:id/refund', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
    const merchantId = request.merchant!.id;
    const schema = z.object({
      amountCents: z.number().int().positive().optional(),
      reason: z.string().optional(),
    });

    const body = schema.parse(request.body);

    const transaction = await prisma.transaction.findFirst({
      where: { id: request.params.id, merchantId },
      include: { merchant: true },
    });

    if (!transaction) return reply.status(404).send({ error: 'Transaction not found' });
    if (!['CAPTURED', 'SETTLED'].includes(transaction.status)) {
      return reply.status(400).send({ error: 'Transaction cannot be refunded' });
    }

    const refundAmount = body.amountCents || transaction.amountCents;
    if (refundAmount > transaction.amountCents - transaction.refundedAmountCents) {
      return reply.status(400).send({ error: 'Refund amount exceeds available amount' });
    }

    // Refund via merchant's own NMI account
    if (transaction.paymentMethod !== 'ACH' && transaction.nmiTransactionId && transaction.merchant.nmiSecurityKey) {
      const result = await refundTransaction({
        transactionId: transaction.nmiTransactionId,
        amountCents: body.amountCents,
        securityKey: transaction.merchant.nmiSecurityKey,
      });

      if (!result.success) {
        return reply.status(400).send({ error: 'Refund failed', details: result.responseText });
      }
    }

    const isFullRefund = refundAmount === transaction.amountCents;
    const updated = await prisma.transaction.update({
      where: { id: transaction.id },
      data: {
        status: isFullRefund ? 'REFUNDED' : 'PARTIALLY_REFUNDED',
        refundedAmountCents: { increment: refundAmount },
        refundReason: body.reason,
      },
    });

    return { success: true, transaction: updated };
  });

  // ── Webhook Config ─────────────────────────────────────

  app.get('/webhooks/config', async (request: FastifyRequest, reply: FastifyReply) => {
    const merchant = await prisma.merchant.findUnique({
      where: { id: request.merchant!.id },
      select: { webhookUrl: true },
    });
    return { webhookUrl: merchant?.webhookUrl };
  });

  app.put('/webhooks/config', async (request: FastifyRequest, reply: FastifyReply) => {
    const schema = z.object({
      webhookUrl: z.string().url().nullable(),
    });

    const body = schema.parse(request.body);
    await prisma.merchant.update({
      where: { id: request.merchant!.id },
      data: { webhookUrl: body.webhookUrl },
    });

    return { success: true };
  });

  app.get('/webhooks/deliveries', async (request: FastifyRequest, reply: FastifyReply) => {
    const { page = '1', limit = '20' } = request.query as Record<string, string>;
    const skip = (Number(page) - 1) * Number(limit);

    const [deliveries, total] = await Promise.all([
      prisma.merchantWebhookDelivery.findMany({
        where: { merchantId: request.merchant!.id },
        skip,
        take: Number(limit),
        orderBy: { createdAt: 'desc' },
        include: {
          webhookEvent: { select: { eventType: true } },
        },
      }),
      prisma.merchantWebhookDelivery.count({ where: { merchantId: request.merchant!.id } }),
    ]);

    return { deliveries, total, page: Number(page), limit: Number(limit) };
  });

  // ── API Key Rotation ───────────────────────────────────

  app.post('/api-keys/rotate', async (request: FastifyRequest, reply: FastifyReply) => {
    const schema = z.object({
      keyType: z.enum(['live', 'test']),
    });

    const body = schema.parse(request.body);
    const newKey = generateApiKey(body.keyType === 'live' ? 'sk_live' : 'sk_test');
    const newHash = hashApiKey(newKey);

    const data = body.keyType === 'live'
      ? { apiKeyLive: newKey, apiKeyLiveHash: newHash }
      : { apiKeyTest: newKey, apiKeyTestHash: newHash };

    await prisma.merchant.update({
      where: { id: request.merchant!.id },
      data,
    });

    return { key: newKey, note: 'Store this securely. It will not be shown again.' };
  });

  // ── Account Settings ───────────────────────────────────

  app.get('/settings', async (request: FastifyRequest, reply: FastifyReply) => {
    const merchant = await prisma.merchant.findUnique({
      where: { id: request.merchant!.id },
      select: {
        id: true,
        businessName: true,
        contactEmail: true,
        contactPhone: true,
        website: true,
        status: true,
        ghlLocationId: true,
        webhookUrl: true,
        nmiTokenizationKey: true,
        createdAt: true,
      },
    });

    return {
      ...merchant,
      processingConfigured: !!merchant?.nmiTokenizationKey,
    };
  });

  app.patch('/settings', async (request: FastifyRequest, reply: FastifyReply) => {
    const schema = z.object({
      businessName: z.string().min(1).optional(),
      contactPhone: z.string().optional(),
      website: z.string().optional().transform(v => v && !/^https?:\/\//i.test(v) ? `https://${v}` : v),
    });

    const body = schema.parse(request.body);
    await prisma.merchant.update({
      where: { id: request.merchant!.id },
      data: body,
    });

    return { success: true };
  });

  // ── Onboarding ──────────────────────────────────────────

  app.get('/onboarding', async (request: FastifyRequest, reply: FastifyReply) => {
    const merchantId = request.merchant!.id;

    const merchant = await prisma.merchant.findUnique({
      where: { id: merchantId },
      select: {
        id: true,
        status: true,
        businessName: true,
        contactEmail: true,
        contactPhone: true,
        website: true,
        ghlLocationId: true,
        nmiTokenizationKey: true,
        seamlesschexApiKey: true,
      },
    });

    const application = await prisma.application.findUnique({
      where: { merchantId },
    });

    // Determine onboarding step
    let step = 'business_info';
    if (application?.businessType && application?.businessAddress) {
      step = 'owner_banking';
    }
    if (application?.ownerFirstName && application?.bankAccountNumber) {
      step = 'review';
    }
    if (application?.nmiBoardingStatus === 'PENDING') {
      step = 'pending_approval';
    }
    if (application?.nmiBoardingStatus === 'APPROVED' || merchant?.nmiTokenizationKey) {
      step = 'complete';
    }
    if (application?.nmiBoardingStatus === 'DECLINED') {
      step = 'declined';
    }

    return {
      merchant: {
        ...merchant,
        processingConfigured: !!merchant?.nmiTokenizationKey,
        achConfigured: !!merchant?.seamlesschexApiKey,
      },
      application: application ? {
        ...application,
        // Strip sensitive fields from response
        bankAccountNumber: application.bankAccountNumber ? '****' + application.bankAccountNumber.slice(-4) : null,
        bankRoutingNumber: application.bankRoutingNumber ? '****' + application.bankRoutingNumber.slice(-4) : null,
        ownerSsnLast4: application.ownerSsnLast4 ? '****' : null,
      } : null,
      step,
    };
  });

  // Step 1: Business info
  app.patch('/onboarding/business', async (request: FastifyRequest, reply: FastifyReply) => {
    const merchantId = request.merchant!.id;
    const schema = z.object({
      businessName: z.string().min(1),
      contactPhone: z.string().optional(),
      website: z.string().optional().transform(v => v && !/^https?:\/\//i.test(v) ? `https://${v}` : v),
      businessType: z.string().min(1),
      businessDescription: z.string().optional(),
      ein: z.string().optional(),
      averageTicketCents: z.number().int().positive().optional(),
      monthlyVolumeCents: z.number().int().positive().optional(),
      businessAddress: z.string().min(1),
      businessCity: z.string().min(1),
      businessState: z.string().min(1),
      businessZip: z.string().min(1),
    });

    const body = schema.parse(request.body);

    // Update merchant profile
    await prisma.merchant.update({
      where: { id: merchantId },
      data: {
        businessName: body.businessName,
        contactPhone: body.contactPhone,
        website: body.website || null,
      },
    });

    // Upsert application
    await prisma.application.upsert({
      where: { merchantId },
      update: {
        businessType: body.businessType,
        businessDescription: body.businessDescription,
        ein: body.ein,
        averageTicketCents: body.averageTicketCents,
        monthlyVolumeCents: body.monthlyVolumeCents,
        businessAddress: body.businessAddress,
        businessCity: body.businessCity,
        businessState: body.businessState,
        businessZip: body.businessZip,
      },
      create: {
        merchantId,
        status: 'DRAFT',
        businessType: body.businessType,
        businessDescription: body.businessDescription,
        ein: body.ein,
        averageTicketCents: body.averageTicketCents,
        monthlyVolumeCents: body.monthlyVolumeCents,
        businessAddress: body.businessAddress,
        businessCity: body.businessCity,
        businessState: body.businessState,
        businessZip: body.businessZip,
      },
    });

    return { success: true };
  });

  // Step 2: Owner & banking details
  app.patch('/onboarding/owner-banking', async (request: FastifyRequest, reply: FastifyReply) => {
    const merchantId = request.merchant!.id;
    const schema = z.object({
      ownerFirstName: z.string().min(1),
      ownerLastName: z.string().min(1),
      ownerEmail: z.string().email(),
      ownerPhone: z.string().optional(),
      ownerDob: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, 'Date must be YYYY-MM-DD'),
      ownerSsnLast4: z.string().length(4).regex(/^\d{4}$/),
      ownerAddress: z.string().min(1),
      ownerCity: z.string().min(1),
      ownerState: z.string().min(1),
      ownerZip: z.string().min(1),
      bankName: z.string().min(1),
      bankRoutingNumber: z.string().min(9).max(9),
      bankAccountNumber: z.string().min(4).max(17),
      bankAccountType: z.enum(['checking', 'savings']),
    });

    const body = schema.parse(request.body);

    await prisma.application.update({
      where: { merchantId },
      data: {
        ownerFirstName: body.ownerFirstName,
        ownerLastName: body.ownerLastName,
        ownerEmail: body.ownerEmail,
        ownerPhone: body.ownerPhone,
        ownerDob: body.ownerDob,
        ownerSsnLast4: body.ownerSsnLast4,
        ownerAddress: body.ownerAddress,
        ownerCity: body.ownerCity,
        ownerState: body.ownerState,
        ownerZip: body.ownerZip,
        bankName: body.bankName,
        bankRoutingNumber: body.bankRoutingNumber,
        bankAccountNumber: body.bankAccountNumber,
        bankAccountType: body.bankAccountType,
      },
    });

    return { success: true };
  });

  // Step 3: Submit to NMI boarding
  app.post('/onboarding/submit-boarding', async (request: FastifyRequest, reply: FastifyReply) => {
    const merchantId = request.merchant!.id;

    const application = await prisma.application.findUnique({
      where: { merchantId },
    });

    if (!application) {
      return reply.status(400).send({ error: 'Application not found' });
    }

    if (!application.ownerFirstName || !application.bankAccountNumber) {
      return reply.status(400).send({ error: 'Please complete all onboarding steps first' });
    }

    const { submitBoardingApplication } = await import('../services/nmi-boarding.service.js');

    const merchant = await prisma.merchant.findUnique({ where: { id: merchantId } });

    const result = await submitBoardingApplication({
      businessName: merchant!.businessName,
      businessType: application.businessType || 'LLC',
      ein: application.ein || undefined,
      website: merchant!.website || undefined,
      businessPhone: merchant!.contactPhone || undefined,
      businessDescription: application.businessDescription || undefined,
      averageTicketCents: application.averageTicketCents || undefined,
      monthlyVolumeCents: application.monthlyVolumeCents || undefined,
      businessAddress: application.businessAddress!,
      businessCity: application.businessCity!,
      businessState: application.businessState!,
      businessZip: application.businessZip!,
      ownerFirstName: application.ownerFirstName!,
      ownerLastName: application.ownerLastName!,
      ownerEmail: application.ownerEmail || merchant!.contactEmail,
      ownerPhone: application.ownerPhone || undefined,
      ownerDob: application.ownerDob!,
      ownerSsnLast4: application.ownerSsnLast4!,
      ownerAddress: application.ownerAddress!,
      ownerCity: application.ownerCity!,
      ownerState: application.ownerState!,
      ownerZip: application.ownerZip!,
      bankName: application.bankName!,
      bankRoutingNumber: application.bankRoutingNumber!,
      bankAccountNumber: application.bankAccountNumber!,
      bankAccountType: (application.bankAccountType as 'checking' | 'savings') || 'checking',
    });

    if (!result.success) {
      return reply.status(400).send({ error: result.error || 'Boarding submission failed' });
    }

    // Update application with boarding info
    await prisma.application.update({
      where: { merchantId },
      data: {
        status: 'UNDER_REVIEW',
        nmiBoardingId: result.boardingId,
        nmiBoardingStatus: result.status || 'PENDING',
        boardingSubmittedAt: new Date(),
      },
    });

    // If instantly approved (low-risk), activate immediately
    if (result.status === 'APPROVED' && result.securityKey && result.tokenizationKey) {
      await prisma.merchant.update({
        where: { id: merchantId },
        data: {
          nmiSecurityKey: result.securityKey,
          nmiTokenizationKey: result.tokenizationKey,
          status: 'ACTIVE',
        },
      });

      await prisma.application.update({
        where: { merchantId },
        data: { status: 'APPROVED', nmiBoardingStatus: 'APPROVED', reviewedAt: new Date() },
      });

      await prisma.auditLog.create({
        data: {
          merchantId,
          actorType: 'system',
          actorId: 'nmi-boarding',
          action: 'onboarding_complete',
          resource: 'merchant',
          resourceId: merchantId,
          details: { method: 'nmi_boarding', instant: true },
        },
      });

      return { success: true, status: 'APPROVED', activated: true };
    }

    // Pending review
    await prisma.auditLog.create({
      data: {
        merchantId,
        actorType: 'merchant',
        actorId: merchantId,
        action: 'boarding_submitted',
        resource: 'application',
        resourceId: application.id,
        details: { boardingId: result.boardingId },
      },
    });

    return { success: true, status: 'PENDING', boardingId: result.boardingId };
  });

  // Manual credential entry (fallback)
  app.post('/onboarding/credentials', async (request: FastifyRequest, reply: FastifyReply) => {
    const merchantId = request.merchant!.id;
    const schema = z.object({
      nmiSecurityKey: z.string().min(1),
      nmiTokenizationKey: z.string().min(1),
      seamlesschexApiKey: z.string().optional(),
    });

    const body = schema.parse(request.body);

    // Validate NMI credentials by making a test API call
    try {
      const testResponse = await fetch('https://secure.nmi.com/api/transact.php', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({
          security_key: body.nmiSecurityKey,
          type: 'validate',
        }).toString(),
      });
      const responseText = await testResponse.text();
      if (!responseText.includes('response=1') && !responseText.includes('response_code=100')) {
        const { env } = await import('../utils/env.js');
        if (!env.NMI_MOCK_MODE) {
          return reply.status(400).send({ error: 'NMI credentials appear invalid. Please double-check your Security Key.' });
        }
      }
    } catch {
      app.log.warn('Could not validate NMI credentials - NMI API unreachable');
    }

    await prisma.merchant.update({
      where: { id: merchantId },
      data: {
        nmiSecurityKey: body.nmiSecurityKey,
        nmiTokenizationKey: body.nmiTokenizationKey,
        seamlesschexApiKey: body.seamlesschexApiKey || null,
        status: 'ACTIVE',
      },
    });

    await prisma.application.updateMany({
      where: { merchantId },
      data: { status: 'APPROVED', reviewedAt: new Date() },
    });

    await prisma.auditLog.create({
      data: {
        merchantId,
        actorType: 'merchant',
        actorId: merchantId,
        action: 'onboarding_complete',
        resource: 'merchant',
        resourceId: merchantId,
        details: { method: 'manual_credentials', achConfigured: !!body.seamlesschexApiKey },
      },
    });

    return { success: true, status: 'ACTIVE' };
  });

  // Check boarding status (polling endpoint for frontend)
  app.get('/onboarding/boarding-status', async (request: FastifyRequest, reply: FastifyReply) => {
    const merchantId = request.merchant!.id;

    const application = await prisma.application.findUnique({
      where: { merchantId },
      select: { nmiBoardingId: true, nmiBoardingStatus: true, boardingSubmittedAt: true },
    });

    if (!application?.nmiBoardingId) {
      return reply.status(404).send({ error: 'No boarding application found' });
    }

    // If already resolved, just return current status
    if (application.nmiBoardingStatus === 'APPROVED' || application.nmiBoardingStatus === 'DECLINED') {
      return { status: application.nmiBoardingStatus };
    }

    // Try to check with NMI
    const { checkBoardingStatus } = await import('../services/nmi-boarding.service.js');
    const result = await checkBoardingStatus(application.nmiBoardingId);

    if (result && result.status !== 'PENDING') {
      // Update our records
      await prisma.application.update({
        where: { merchantId },
        data: {
          nmiBoardingStatus: result.status,
          reviewedAt: new Date(),
          status: result.status === 'APPROVED' ? 'APPROVED' : 'REJECTED',
          rejectionReason: result.declineReason || null,
        },
      });

      if (result.status === 'APPROVED' && result.securityKey && result.tokenizationKey) {
        await prisma.merchant.update({
          where: { id: merchantId },
          data: {
            nmiSecurityKey: result.securityKey,
            nmiTokenizationKey: result.tokenizationKey,
            status: 'ACTIVE',
          },
        });
      }

      return { status: result.status };
    }

    return { status: 'PENDING' };
  });
}
