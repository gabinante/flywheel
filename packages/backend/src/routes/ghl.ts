import { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { z } from 'zod';
import { prisma } from '../utils/prisma.js';
import { env } from '../utils/env.js';
import { generateApiKey, hashApiKey } from '../utils/api-keys.js';
import crypto from 'crypto';

export async function ghlRoutes(app: FastifyInstance) {
  // ── OAuth Install ──────────────────────────────────────

  app.get('/install', async (request: FastifyRequest, reply: FastifyReply) => {
    const state = crypto.randomBytes(32).toString('hex');

    await prisma.ghlOAuthState.create({
      data: {
        state,
        expiresAt: new Date(Date.now() + 10 * 60 * 1000), // 10 min
      },
    });

    const params = new URLSearchParams({
      response_type: 'code',
      client_id: env.GHL_CLIENT_ID,
      redirect_uri: env.GHL_REDIRECT_URI,
      scope: 'payments/orders.readonly payments/orders.write payments/transactions.readonly contacts.readonly contacts.write locations.readonly',
      state,
    });

    return reply.redirect(`https://marketplace.gohighlevel.com/oauth/chooselocation?${params.toString()}`);
  });

  // ── OAuth Callback ─────────────────────────────────────

  app.get('/callback', async (request: FastifyRequest, reply: FastifyReply) => {
    const { code, state } = request.query as { code: string; state: string };

    if (!code || !state) {
      return reply.status(400).send({ error: 'Missing code or state' });
    }

    // Validate CSRF state
    const oauthState = await prisma.ghlOAuthState.findUnique({ where: { state } });
    if (!oauthState || oauthState.expiresAt < new Date()) {
      return reply.status(400).send({ error: 'Invalid or expired state' });
    }

    await prisma.ghlOAuthState.delete({ where: { state } });

    // Exchange code for tokens
    const tokenResponse = await fetch('https://services.leadconnectorhq.com/oauth/token', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        client_id: env.GHL_CLIENT_ID,
        client_secret: env.GHL_CLIENT_SECRET,
        grant_type: 'authorization_code',
        code,
        redirect_uri: env.GHL_REDIRECT_URI,
      }).toString(),
    });

    if (!tokenResponse.ok) {
      const error = await tokenResponse.text();
      app.log.error({ error }, 'GHL token exchange failed');
      return reply.status(400).send({ error: 'Token exchange failed' });
    }

    const tokens = await tokenResponse.json() as {
      access_token: string;
      refresh_token: string;
      expires_in: number;
      locationId: string;
      companyId: string;
      userType: string;
    };

    // Find or create merchant
    let merchant = await prisma.merchant.findUnique({
      where: { ghlLocationId: tokens.locationId },
    });

    if (merchant) {
      merchant = await prisma.merchant.update({
        where: { id: merchant.id },
        data: {
          ghlAccessToken: tokens.access_token,
          ghlRefreshToken: tokens.refresh_token,
          ghlTokenExpiresAt: new Date(Date.now() + tokens.expires_in * 1000),
          ghlCompanyId: tokens.companyId,
        },
      });
    } else {
      const liveKey = generateApiKey('sk_live');
      const testKey = generateApiKey('sk_test');

      merchant = await prisma.merchant.create({
        data: {
          businessName: `GHL Location ${tokens.locationId}`,
          contactEmail: `${tokens.locationId}@ghl.placeholder`,
          ghlLocationId: tokens.locationId,
          ghlCompanyId: tokens.companyId,
          ghlAccessToken: tokens.access_token,
          ghlRefreshToken: tokens.refresh_token,
          ghlTokenExpiresAt: new Date(Date.now() + tokens.expires_in * 1000),
          status: 'PENDING',
          apiKeyLive: liveKey,
          apiKeyTest: testKey,
          apiKeyLiveHash: hashApiKey(liveKey),
          apiKeyTestHash: hashApiKey(testKey),
        },
      });

      // Create onboarding application
      await prisma.application.create({
        data: {
          merchantId: merchant.id,
          status: 'DRAFT',
        },
      });
    }

    // Issue local JWT and redirect to merchant dashboard
    const token = app.jwt.sign(
      { id: merchant.id, email: merchant.contactEmail, type: 'merchant' },
      { expiresIn: '15m' }
    );

    return reply.redirect(`${env.MERCHANT_URL}/auth/callback?token=${token}`);
  });

  // ── GHL SSO (POST - API call from frontend) ───────────
  // The merchant dashboard's GhlSsoEntry component calls this
  // to exchange a GHL SSO token for a local JWT.

  app.post('/sso', async (request: FastifyRequest, reply: FastifyReply) => {
    const schema = z.object({
      ssoToken: z.string(),
    });

    const body = schema.parse(request.body);

    try {
      const response = await fetch('https://services.leadconnectorhq.com/oauth/sso/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          token: body.ssoToken,
          clientId: env.GHL_CLIENT_ID,
          clientSecret: env.GHL_SSO_KEY,
        }),
      });

      if (!response.ok) {
        return reply.status(401).send({ error: 'Invalid SSO token' });
      }

      const ssoData = await response.json() as {
        locationId: string;
        userId: string;
        email: string;
        companyId: string;
      };

      const merchant = await prisma.merchant.findUnique({
        where: { ghlLocationId: ssoData.locationId },
      });

      if (!merchant) {
        return reply.status(404).send({ error: 'Merchant not found for this location. Please install GoHighPayment first.' });
      }

      const token = app.jwt.sign(
        { id: merchant.id, email: merchant.contactEmail, type: 'merchant' },
        { expiresIn: '4h' } // Longer expiry for iframe sessions
      );

      const refreshToken = app.jwt.sign(
        { id: merchant.id, type: 'merchant-refresh' },
        { expiresIn: '7d' }
      );

      return {
        token,
        refreshToken,
        merchant: {
          id: merchant.id,
          businessName: merchant.businessName,
          status: merchant.status,
        },
      };
    } catch (error) {
      app.log.error({ error }, 'SSO verification failed');
      return reply.status(500).send({ error: 'SSO verification failed' });
    }
  });

  // ── GHL SSO (GET - direct redirect from GHL sidebar) ──
  // GHL loads: https://merchant.gohighpayment.com/ghl/sso?ssoToken=xxx
  // This backend route can optionally handle it server-side, validating
  // the token and redirecting with a JWT. But since our frontend handles
  // it at /ghl/sso, this is a fallback for server-side SSO.

  app.get('/sso', async (request: FastifyRequest, reply: FastifyReply) => {
    const { ssoToken, token } = request.query as { ssoToken?: string; token?: string };
    const sso = ssoToken || token;

    if (!sso) {
      // No token in URL - redirect to merchant dashboard's SSO page
      // (which will show an error since there's no token)
      return reply.redirect(`${env.MERCHANT_URL}/ghl/sso`);
    }

    // Redirect to the merchant dashboard's client-side SSO handler
    // The frontend will call POST /api/v1/ghl/sso to validate
    return reply.redirect(`${env.MERCHANT_URL}/ghl/sso?ssoToken=${encodeURIComponent(sso)}`);
  });

  // ── GHL Webhook ────────────────────────────────────────

  app.post('/webhook', async (request: FastifyRequest, reply: FastifyReply) => {
    const body = request.body as { type: string; locationId?: string; companyId?: string };

    switch (body.type) {
      case 'INSTALL': {
        app.log.info({ locationId: body.locationId }, 'GHL app installed');
        break;
      }
      case 'UNINSTALL': {
        if (body.locationId) {
          const merchant = await prisma.merchant.findUnique({
            where: { ghlLocationId: body.locationId },
          });

          if (merchant) {
            await prisma.merchant.update({
              where: { id: merchant.id },
              data: {
                status: 'DEACTIVATED',
                ghlAccessToken: null,
                ghlRefreshToken: null,
              },
            });
            app.log.info({ merchantId: merchant.id }, 'GHL app uninstalled, merchant deactivated');
          }
        }
        break;
      }
    }

    return { received: true };
  });
}
