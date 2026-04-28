import Fastify from 'fastify';
import cors from '@fastify/cors';
import helmet from '@fastify/helmet';
import rateLimit from '@fastify/rate-limit';
import jwt from '@fastify/jwt';
import { prisma } from './utils/prisma.js';
import { redis } from './utils/redis.js';
import { createJobQueue } from './jobs/queue.js';
import { adminAuthRoutes } from './routes/admin-auth.js';
import { adminRoutes } from './routes/admin.js';
import { merchantAuthRoutes } from './routes/merchant-auth.js';
import { merchantRoutes } from './routes/merchant.js';
import { checkoutRoutes } from './routes/checkout.js';
import { ghlRoutes } from './routes/ghl.js';
import { webhookRoutes } from './routes/webhooks.js';
import { env } from './utils/env.js';

const app = Fastify({
  logger: {
    level: env.NODE_ENV === 'production' ? 'info' : 'debug',
  },
});

// Plugins
await app.register(helmet, {
  contentSecurityPolicy: env.NODE_ENV === 'production' ? {
    directives: {
      defaultSrc: ["'self'"],
      scriptSrc: ["'self'", "'unsafe-inline'", 'https://secure.nmi.com'],
      styleSrc: ["'self'", "'unsafe-inline'", 'https://fonts.googleapis.com'],
      fontSrc: ["'self'", 'https://fonts.gstatic.com'],
      imgSrc: ["'self'", 'data:', 'https:'],
      connectSrc: ["'self'", 'https://secure.nmi.com', 'https://services.leadconnectorhq.com'],
      // Allow GHL to embed our merchant dashboard + checkout in iframes
      frameAncestors: [
        "'self'",
        'https://*.gohighlevel.com',
        'https://*.leadconnectorhq.com',
        'https://*.msgsndr.com',
      ],
      frameSrc: ["'self'", 'https://secure.nmi.com'],
    },
  } : false,
  // Allow iframe embedding in dev too
  crossOriginEmbedderPolicy: false,
});

await app.register(cors, {
  origin: [
    env.FRONTEND_URL,
    env.ADMIN_URL,
    env.MERCHANT_URL,
    env.CHECKOUT_URL,
    // GHL domains that may make API calls or embed our frontend
    'https://app.gohighlevel.com',
    'https://app.msgsndr.com',
  ],
  credentials: true,
});

await app.register(rateLimit, {
  max: 100,
  timeWindow: '1 minute',
});

await app.register(jwt, {
  secret: env.JWT_SECRET,
  sign: { expiresIn: env.JWT_EXPIRES_IN },
});

// Decorators
app.decorate('prisma', prisma);
app.decorate('redis', redis);

// Health check
app.get('/health', async () => {
  await prisma.$queryRaw`SELECT 1`;
  return { status: 'ok', timestamp: new Date().toISOString() };
});

// Routes
await app.register(adminAuthRoutes, { prefix: '/api/v1/admin/auth' });
await app.register(adminRoutes, { prefix: '/api/v1/admin' });
await app.register(merchantAuthRoutes, { prefix: '/api/v1/merchant/auth' });
await app.register(merchantRoutes, { prefix: '/api/v1/merchant' });
await app.register(checkoutRoutes, { prefix: '/api/v1/checkout' });
await app.register(ghlRoutes, { prefix: '/api/v1/ghl' });
await app.register(webhookRoutes, { prefix: '/api/v1/webhooks' });

// Start
const start = async () => {
  try {
    const jobQueue = await createJobQueue();
    app.decorate('jobQueue', jobQueue);

    await app.listen({ port: Number(env.PORT), host: env.HOST });
    app.log.info(`Server running on ${env.HOST}:${env.PORT}`);
  } catch (err) {
    app.log.error(err);
    process.exit(1);
  }
};

// Graceful shutdown
const shutdown = async () => {
  app.log.info('Shutting down...');
  await app.close();
  await prisma.$disconnect();
  redis.disconnect();
  process.exit(0);
};

process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);

start();
