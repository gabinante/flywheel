import { prisma } from '../utils/prisma.js';
import { getJobQueue } from '../jobs/queue.js';
import type { Transaction } from '@prisma/client';

export interface PipelineResult {
  success: boolean;
  transaction?: Transaction;
  error?: string;
}

/**
 * Post-payment pipeline for referral ISO model.
 *
 * We do NOT calculate fees, manage settlements, or track processing caps.
 * NMI handles all of that directly with the merchant.
 *
 * Our pipeline:
 * 1. Update transaction status
 * 2. Fire webhook notification to merchant
 */
export async function runPostPaymentPipeline(
  transactionId: string
): Promise<PipelineResult> {
  const transaction = await prisma.transaction.findUnique({
    where: { id: transactionId },
    include: { merchant: true },
  });

  if (!transaction) {
    return { success: false, error: 'Transaction not found' };
  }

  const merchant = transaction.merchant;

  try {
    // 1. Update transaction status
    const updated = await prisma.transaction.update({
      where: { id: transactionId },
      data: {
        status: transaction.paymentMethod === 'ACH' ? 'PENDING' : 'CAPTURED',
      },
    });

    // 2. Fire webhook notification
    if (merchant.webhookUrl) {
      const event = await prisma.webhookEvent.create({
        data: {
          merchantId: merchant.id,
          transactionId: transaction.id,
          eventType: 'TRANSACTION_COMPLETED',
          payload: {
            transactionId: transaction.id,
            amountCents: transaction.amountCents,
            currency: transaction.currency,
            paymentMethod: transaction.paymentMethod,
            status: updated.status,
          },
        },
      });

      try {
        const boss = getJobQueue();
        await boss.send('webhook:deliver', { webhookEventId: event.id });
      } catch {
        // Job queue may not be available in tests
      }
    }

    return { success: true, transaction: updated };
  } catch (error) {
    console.error('Post-payment pipeline error:', error);

    await prisma.transaction.update({
      where: { id: transactionId },
      data: { status: 'ERROR' },
    });

    return {
      success: false,
      error: error instanceof Error ? error.message : 'Pipeline error',
    };
  }
}
