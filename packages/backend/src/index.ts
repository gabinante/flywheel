import Fastify from 'fastify';
import fastifyJwt from '@fastify/jwt';
import agencyAuthPlugin from './plugins/agency-auth';
import { agencyAuthRoutes } from './routes/agency-auth';
import { agencyRoutes } from './routes/agency';

/**
 * Build and configure the Fastify application.
 * Exported for testing; the server is started at the bottom of this file.
 */
export async function buildApp(opts?: {
  logger?: boolean;
  jwtSecret?: string;
}) {
  const app = Fastify({
    logger: opts?.logger ?? true,
  });

  // ─── JWT Plugin ───────────────────────────────────────────────────
  const jwtSecret = opts?.jwtSecret ?? process.env.JWT_SECRET;
  if (!jwtSecret) {
    throw new Error('JWT_SECRET environment variable is required');
  }
  await app.register(fastifyJwt, {
    secret: jwtSecret,
  });

  // ─── Agency Auth Plugin (JWT verification decorator) ──────────────
  await app.register(agencyAuthPlugin);

  // ─── Routes ───────────────────────────────────────────────────────
  await app.register(agencyAuthRoutes, { prefix: '/api/v1/agency/auth' });
  await app.register(agencyRoutes, { prefix: '/api/v1/agency' });

  // ─── Health Check ─────────────────────────────────────────────────
  app.get('/health', async () => ({ status: 'ok' }));

  return app;
}

// ─── Start server (only when run directly) ──────────────────────────
const isMainModule =
  typeof process !== 'undefined' &&
  process.argv[1] &&
  (process.argv[1].endsWith('/index.ts') ||
    process.argv[1].endsWith('/index.js'));

if (isMainModule) {
  buildApp({ logger: true })
    .then((app) => {
      const port = parseInt(process.env.PORT ?? '3000', 10);
      const host = process.env.HOST ?? '0.0.0.0';
      return app.listen({ port, host });
    })
    .then((address) => {
      console.log(`Server listening on ${address}`);
    })
    .catch((err) => {
      console.error('Failed to start server:', err);
      process.exit(1);
    });
}
