import PgBoss from 'pg-boss';
import { prisma } from '../utils/prisma.js';

export async function registerJobHandlers(boss: PgBoss) {
  // Daily metrics aggregation
  await boss.work('metrics:aggregate-daily', async ([job]) => {
    console.log('Aggregating daily metrics:', job.data);
  });

  // Webhook delivery
  await boss.work('webhook:deliver', async ([job]) => {
    const { webhookEventId } = job.data as { webhookEventId: string };
    await deliverWebhook(webhookEventId);
  });

  // GHL token refresh
  await boss.work('ghl:refresh-tokens', async () => {
    console.log('Refreshing GHL tokens');
  });

  // Create queues for scheduled jobs, then schedule
  await boss.createQueue('metrics:aggregate-daily');
  await boss.createQueue('ghl:refresh-tokens');

  await boss.schedule('metrics:aggregate-daily', '0 2 * * *', {}); // 2am daily
  await boss.schedule('ghl:refresh-tokens', '*/30 * * * *', {}); // every 30 min
}

async function deliverWebhook(webhookEventId: string) {
  const event = await prisma.webhookEvent.findUnique({
    where: { id: webhookEventId },
    include: { merchant: true },
  });

  if (!event || !event.merchant.webhookUrl) return;

  const url = event.merchant.webhookUrl;
  const attempt = event.attempts + 1;

  try {
    const response = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Webhook-Signature': event.merchant.webhookSecret || '',
        'X-Webhook-Event': event.eventType,
      },
      body: JSON.stringify(event.payload),
      signal: AbortSignal.timeout(10000),
    });

    await prisma.merchantWebhookDelivery.create({
      data: {
        merchantId: event.merchantId,
        webhookEventId: event.id,
        url,
        statusCode: response.status,
        success: response.ok,
        attemptNumber: attempt,
      },
    });

    if (response.ok) {
      await prisma.webhookEvent.update({
        where: { id: webhookEventId },
        data: { delivered: true, deliveredAt: new Date(), attempts: attempt },
      });
    } else {
      await prisma.webhookEvent.update({
        where: { id: webhookEventId },
        data: { attempts: attempt },
      });
    }
  } catch (error) {
    await prisma.merchantWebhookDelivery.create({
      data: {
        merchantId: event.merchantId,
        webhookEventId: event.id,
        url,
        responseBody: error instanceof Error ? error.message : 'Unknown error',
        success: false,
        attemptNumber: attempt,
      },
    });

    await prisma.webhookEvent.update({
      where: { id: webhookEventId },
      data: { attempts: attempt },
    });
  }
}
