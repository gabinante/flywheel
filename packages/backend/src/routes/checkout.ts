import { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { z } from 'zod';
import { prisma } from '../utils/prisma.js';
import { verifyApiKey } from '../plugins/merchant-auth.js';
import { chargeCard } from '../services/nmi.service.js';
import { createCheck } from '../services/seamlesschex.service.js';
import { runPostPaymentPipeline } from '../services/post-payment-pipeline.service.js';
import { env } from '../utils/env.js';

const createSessionSchema = z.object({
  amountCents: z.number().int().positive(),
  currency: z.string().default('USD'),
  description: z.string().optional(),
  customerEmail: z.string().email().optional(),
  customerName: z.string().optional(),
  metadata: z.record(z.unknown()).optional(),
  successUrl: z.string().url().optional(),
  cancelUrl: z.string().url().optional(),
});

const processCardSchema = z.object({
  sessionId: z.string(),
  paymentToken: z.string(),
  paymentMethod: z.enum(['CARD', 'GOOGLE_PAY', 'APPLE_PAY']).default('CARD'),
  customerEmail: z.string().email().optional(),
  customerFirstName: z.string().optional(),
  customerLastName: z.string().optional(),
});

const processAchSchema = z.object({
  sessionId: z.string(),
  routingNumber: z.string().length(9),
  accountNumber: z.string().min(4).max(17),
  accountType: z.enum(['checking', 'savings']),
  nameOnAccount: z.string().min(1),
  customerEmail: z.string().email().optional(),
});

export async function checkoutRoutes(app: FastifyInstance) {
  // Get merchant's tokenization key for Collect.js (per-session)
  app.get('/config/:sessionId', async (request: FastifyRequest<{ Params: { sessionId: string } }>, reply: FastifyReply) => {
    const session = await prisma.checkoutSession.findUnique({
      where: { id: request.params.sessionId },
    });

    if (!session) {
      return reply.status(404).send({ error: 'Session not found' });
    }

    const merchant = await prisma.merchant.findUnique({
      where: { id: session.merchantId },
      select: { nmiTokenizationKey: true },
    });

    if (!merchant?.nmiTokenizationKey) {
      return reply.status(400).send({ error: 'Merchant payment processing not configured' });
    }

    return {
      tokenizationKey: merchant.nmiTokenizationKey,
      collectJsUrl: 'https://secure.nmi.com/token/Collect.js',
      googlePay: env.GOOGLE_PAY_MERCHANT_ID ? {
        enabled: true,
        merchantId: env.GOOGLE_PAY_MERCHANT_ID,
      } : { enabled: false },
      applePay: env.APPLE_PAY_MERCHANT_ID ? {
        enabled: true,
        merchantId: env.APPLE_PAY_MERCHANT_ID,
      } : { enabled: false },
    };
  });

  // Create checkout session (API key auth)
  app.post(
    '/sessions',
    { preHandler: [verifyApiKey] },
    async (request: FastifyRequest, reply: FastifyReply) => {
      const body = createSessionSchema.parse(request.body);
      const merchantId = request.merchant!.id;

      const session = await prisma.checkoutSession.create({
        data: {
          merchantId,
          amountCents: body.amountCents,
          currency: body.currency,
          description: body.description,
          customerEmail: body.customerEmail,
          customerName: body.customerName,
          metadata: body.metadata as any ?? undefined,
          successUrl: body.successUrl,
          cancelUrl: body.cancelUrl,
          expiresAt: new Date(Date.now() + 30 * 60 * 1000), // 30 min
        },
      });

      return {
        sessionId: session.id,
        checkoutUrl: `${env.CHECKOUT_URL}/${session.id}`,
        expiresAt: session.expiresAt,
      };
    }
  );

  // Get session status
  app.get('/session/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
    const session = await prisma.checkoutSession.findUnique({
      where: { id: request.params.id },
    });

    if (!session) {
      return reply.status(404).send({ error: 'Session not found' });
    }

    if (session.expiresAt < new Date() && session.status === 'PENDING') {
      return reply.status(410).send({ error: 'Session expired' });
    }

    return {
      id: session.id,
      amountCents: session.amountCents,
      currency: session.currency,
      description: session.description,
      customerEmail: session.customerEmail,
      customerName: session.customerName,
      status: session.status,
      transactionId: session.transactionId,
    };
  });

  // Process card/wallet payment via Collect.js token
  app.post('/process', async (request: FastifyRequest, reply: FastifyReply) => {
    const body = processCardSchema.parse(request.body);

    const session = await prisma.checkoutSession.findUnique({
      where: { id: body.sessionId },
    });

    if (!session || session.status !== 'PENDING') {
      return reply.status(400).send({ error: 'Invalid or already processed session' });
    }

    if (session.expiresAt < new Date()) {
      return reply.status(410).send({ error: 'Session expired' });
    }

    const merchant = await prisma.merchant.findUnique({
      where: { id: session.merchantId },
    });

    if (!merchant || merchant.status !== 'ACTIVE') {
      return reply.status(400).send({ error: 'Merchant not available' });
    }

    if (!merchant.nmiSecurityKey) {
      return reply.status(400).send({ error: 'Merchant payment processing not configured' });
    }

    // Upsert customer
    const email = body.customerEmail || session.customerEmail || '';
    let customer = null;
    if (email) {
      customer = await prisma.customer.upsert({
        where: { merchantId_email: { merchantId: merchant.id, email } },
        update: {
          firstName: body.customerFirstName || undefined,
          lastName: body.customerLastName || undefined,
        },
        create: {
          merchantId: merchant.id,
          email,
          firstName: body.customerFirstName,
          lastName: body.customerLastName,
        },
      });
    }

    // Create pending transaction
    const transaction = await prisma.transaction.create({
      data: {
        merchantId: merchant.id,
        customerId: customer?.id,
        amountCents: session.amountCents,
        currency: session.currency,
        status: 'PENDING',
        paymentMethod: body.paymentMethod,
        description: session.description,
        checkoutSessionId: session.id,
        ipAddress: request.ip,
        userAgent: request.headers['user-agent'] || null,
      },
    });

    // Charge via merchant's own NMI account
    const chargeResult = await chargeCard({
      paymentToken: body.paymentToken,
      amountCents: session.amountCents,
      orderId: transaction.id,
      customerEmail: email || undefined,
      customerFirstName: body.customerFirstName,
      customerLastName: body.customerLastName,
      ipAddress: request.ip,
      securityKey: merchant.nmiSecurityKey,
    });

    if (!chargeResult.success) {
      await prisma.transaction.update({
        where: { id: transaction.id },
        data: {
          status: 'DECLINED',
          nmiTransactionId: chargeResult.transactionId,
          nmiResponseCode: chargeResult.responseCode,
          nmiResponseText: chargeResult.responseText,
        },
      });

      await prisma.checkoutSession.update({
        where: { id: session.id },
        data: { status: 'FAILED' },
      });

      return reply.status(402).send({
        error: 'Payment declined',
        responseText: chargeResult.responseText,
      });
    }

    // Update transaction with NMI details
    await prisma.transaction.update({
      where: { id: transaction.id },
      data: {
        nmiTransactionId: chargeResult.transactionId,
        nmiResponseCode: chargeResult.responseCode,
        nmiResponseText: chargeResult.responseText,
        nmiAuthCode: chargeResult.authCode,
        cardBrand: chargeResult.cardBrand,
        cardLast4: chargeResult.cardLast4,
        cardExpMonth: chargeResult.cardExpMonth,
        cardExpYear: chargeResult.cardExpYear,
      },
    });

    // Run post-payment pipeline (status update + webhook)
    await runPostPaymentPipeline(transaction.id);

    // Update session
    await prisma.checkoutSession.update({
      where: { id: session.id },
      data: {
        status: 'COMPLETED',
        transactionId: transaction.id,
      },
    });

    return {
      success: true,
      transactionId: transaction.id,
      status: 'CAPTURED',
      amountCents: session.amountCents,
      paymentMethod: body.paymentMethod,
      cardBrand: chargeResult.cardBrand,
      cardLast4: chargeResult.cardLast4,
      redirectUrl: session.successUrl || `${env.CHECKOUT_URL}/confirmation/${session.id}`,
    };
  });

  // Process ACH payment via merchant's own Seamlesschex account
  app.post('/process-ach', async (request: FastifyRequest, reply: FastifyReply) => {
    const body = processAchSchema.parse(request.body);

    const session = await prisma.checkoutSession.findUnique({
      where: { id: body.sessionId },
    });

    if (!session || session.status !== 'PENDING') {
      return reply.status(400).send({ error: 'Invalid or already processed session' });
    }

    if (session.expiresAt < new Date()) {
      return reply.status(410).send({ error: 'Session expired' });
    }

    const merchant = await prisma.merchant.findUnique({
      where: { id: session.merchantId },
    });

    if (!merchant || merchant.status !== 'ACTIVE') {
      return reply.status(400).send({ error: 'Merchant not available' });
    }

    if (!merchant.seamlesschexApiKey) {
      return reply.status(400).send({ error: 'Merchant ACH processing not configured' });
    }

    // Upsert customer
    const email = body.customerEmail || session.customerEmail || '';
    let customer = null;
    if (email) {
      customer = await prisma.customer.upsert({
        where: { merchantId_email: { merchantId: merchant.id, email } },
        update: {},
        create: { merchantId: merchant.id, email },
      });
    }

    // Create ACH check via merchant's own Seamlesschex account
    const checkResult = await createCheck({
      amountCents: session.amountCents,
      routingNumber: body.routingNumber,
      accountNumber: body.accountNumber,
      accountType: body.accountType,
      nameOnAccount: body.nameOnAccount,
      email: email || undefined,
      merchantReference: session.id,
      apiKey: merchant.seamlesschexApiKey,
    });

    if (!checkResult.success) {
      return reply.status(400).send({ error: checkResult.error || 'ACH processing failed' });
    }

    // Create transaction
    const transaction = await prisma.transaction.create({
      data: {
        merchantId: merchant.id,
        customerId: customer?.id,
        amountCents: session.amountCents,
        currency: session.currency,
        status: 'PENDING',
        paymentMethod: 'ACH',
        description: session.description,
        checkoutSessionId: session.id,
        seamlesschexTransactionId: checkResult.checkId,
        ipAddress: request.ip,
        userAgent: request.headers['user-agent'] || null,
      },
    });

    // Create Seamlesschex transaction record
    await prisma.seamlesschexTransaction.create({
      data: {
        transactionId: transaction.id,
        checkId: checkResult.checkId,
        routingLast4: body.routingNumber.slice(-4),
        accountLast4: body.accountNumber.slice(-4),
        accountType: body.accountType,
        nameOnAccount: body.nameOnAccount,
        checkStatus: 'CREATED',
      },
    });

    // Run post-payment pipeline (status update + webhook)
    await runPostPaymentPipeline(transaction.id);

    // Update session
    await prisma.checkoutSession.update({
      where: { id: session.id },
      data: {
        status: 'COMPLETED',
        transactionId: transaction.id,
      },
    });

    return {
      success: true,
      transactionId: transaction.id,
      status: 'PENDING',
      amountCents: session.amountCents,
      paymentMethod: 'ACH',
      message: 'ACH payment initiated. Settlement typically takes 2-5 business days.',
      redirectUrl: session.successUrl || `${env.CHECKOUT_URL}/confirmation/${session.id}`,
    };
  });
}
