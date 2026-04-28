import { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { prisma } from '../utils/prisma.js';
import { env } from '../utils/env.js';
import crypto from 'crypto';

export async function webhookRoutes(app: FastifyInstance) {
  // ── NMI Webhook ────────────────────────────────────────

  app.post('/nmi', async (request: FastifyRequest, reply: FastifyReply) => {
    // Verify webhook signature if configured
    if (env.NMI_WEBHOOK_SECRET) {
      const signature = request.headers['x-nmi-signature'] as string;
      const expected = crypto
        .createHmac('sha256', env.NMI_WEBHOOK_SECRET)
        .update(JSON.stringify(request.body))
        .digest('hex');

      if (signature !== expected) {
        return reply.status(401).send({ error: 'Invalid signature' });
      }
    }

    const body = request.body as Record<string, string>;
    const nmiTxnId = body.transaction_id;
    const eventType = body.event_type || body.action;

    if (!nmiTxnId) {
      return reply.status(400).send({ error: 'Missing transaction_id' });
    }

    const transaction = await prisma.transaction.findFirst({
      where: { nmiTransactionId: nmiTxnId },
    });

    if (!transaction) {
      app.log.warn({ nmiTxnId }, 'NMI webhook for unknown transaction');
      return { received: true };
    }

    switch (eventType) {
      case 'sale.success':
      case 'settle.success':
        await prisma.transaction.update({
          where: { id: transaction.id },
          data: { status: 'SETTLED' },
        });
        break;

      case 'refund.success':
        await prisma.transaction.update({
          where: { id: transaction.id },
          data: { status: 'REFUNDED' },
        });
        break;

      case 'chargeback':
        await prisma.chargeback.create({
          data: {
            merchantId: transaction.merchantId,
            transactionId: transaction.id,
            amountCents: transaction.amountCents,
            reason: body.reason || 'Chargeback initiated',
            reasonCode: body.reason_code,
            status: 'OPENED',
          },
        });
        break;
    }

    return { received: true };
  });

  // ── Seamlesschex Webhook ───────────────────────────────

  app.post('/seamlesschex', async (request: FastifyRequest, reply: FastifyReply) => {
    if (env.SEAMLESSCHEX_WEBHOOK_SECRET) {
      const signature = request.headers['x-seamlesschex-signature'] as string;
      const expected = crypto
        .createHmac('sha256', env.SEAMLESSCHEX_WEBHOOK_SECRET)
        .update(JSON.stringify(request.body))
        .digest('hex');

      if (signature !== expected) {
        return reply.status(401).send({ error: 'Invalid signature' });
      }
    }

    const body = request.body as {
      check_id: string;
      status: string;
      return_code?: string;
      return_reason?: string;
    };

    const achTx = await prisma.seamlesschexTransaction.findUnique({
      where: { checkId: body.check_id },
      include: { transaction: true },
    });

    if (!achTx) {
      app.log.warn({ checkId: body.check_id }, 'Seamlesschex webhook for unknown check');
      return { received: true };
    }

    // Map Seamlesschex status to our status
    const statusMap: Record<string, string> = {
      pending: 'PENDING',
      in_transit: 'IN_TRANSIT',
      cleared: 'CLEARED',
      settled: 'SETTLED',
      returned: 'RETURNED',
      voided: 'VOIDED',
    };

    const newCheckStatus = statusMap[body.status] || body.status.toUpperCase();

    await prisma.seamlesschexTransaction.update({
      where: { id: achTx.id },
      data: {
        checkStatus: newCheckStatus as any,
        returnCode: body.return_code,
        returnReason: body.return_reason,
        ...(body.status === 'settled' && { settledAt: new Date() }),
      },
    });

    // Update parent transaction status
    if (body.status === 'settled' || body.status === 'cleared') {
      await prisma.transaction.update({
        where: { id: achTx.transactionId },
        data: { status: 'SETTLED' },
      });
    } else if (body.status === 'returned') {
      await prisma.transaction.update({
        where: { id: achTx.transactionId },
        data: { status: 'RETURNED' },
      });
    }

    return { received: true };
  });
}
