import { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { z } from 'zod';
import { prisma } from '../utils/prisma.js';
import { env } from '../utils/env.js';
import { generateApiKey, hashApiKey } from '../utils/api-keys.js';
import { chargeCard, refundTransaction } from '../services/nmi.service.js';
import crypto from 'crypto';

// ─── GHL Query Endpoint Types ────────────────────────────────────────────────

interface GhlQueryMeta {
  contactId?: string;
  locationId?: string;
  email?: string;
  name?: string;
  orderId?: string;
  /** NMI transaction ID — used for refund lookups */
  transactionId?: string;
}

interface GhlQueryBody {
  type: 'charge' | 'refund' | 'subscription' | string;
  /** Amount in cents */
  amount: number;
  currency?: string;
  /** Payment token (Collect.js or GHL-provided) */
  source?: string;
  meta?: GhlQueryMeta;
}

// ─── GHL Response Format ─────────────────────────────────────────────────────

interface GhlQueryResponse {
  success: boolean;
  transactionId?: string;
  message: string;
}

// ─── HMAC Verification ───────────────────────────────────────────────────────

/**
 * Verify GHL's HMAC-SHA256 signature.
 *
 * GHL signs the raw request body with the app's client secret and puts the
 * hex digest in the `x-ghl-signature` header (optionally prefixed with
 * "sha256=").  We verify using a timing-safe comparison.
 */
function verifyGhlSignature(rawBody: Buffer, signatureHeader: string, secret: string): boolean {
  // Strip optional "sha256=" prefix
  const signature = signatureHeader.startsWith('sha256=')
    ? signatureHeader.slice(7)
    : signatureHeader;

  const expected = crypto.createHmac('sha256', secret).update(rawBody).digest('hex');

  try {
    const sigBuf = Buffer.from(signature, 'hex');
    const expBuf = Buffer.from(expected, 'hex');
    if (sigBuf.length !== expBuf.length) return false;
    return crypto.timingSafeEqual(sigBuf, expBuf);
  } catch {
    return false;
  }
}

export async function ghlRoutes(app: FastifyInstance) {
  // ── Raw body capture for HMAC verification ─────────────────────────────────
  //
  // We override the JSON content-type parser within this plugin scope so we
  // can capture the raw bytes for signature verification on the /query route.
  // Fastify scopes this parser to routes within this plugin only.
  app.addContentTypeParser(
    'application/json',
    { parseAs: 'buffer' },
    (_req: FastifyRequest, body: Buffer, done: (err: Error | null, result?: unknown) => void) => {
      (_req as FastifyRequest & { rawBody?: Buffer }).rawBody = body;
      try {
        done(null, JSON.parse(body.toString('utf-8')));
      } catch (err) {
        done(err instanceof Error ? err : new Error(String(err)), undefined);
      }
    },
  );

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
        return reply.status(404).send({ error: 'Merchant not found for this location. Please install Shamroq first.' });
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
  // GHL loads: https://merchant.shamroq.com/ghl/sso?ssoToken=xxx
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

  // ── GHL queryUrl Payment Processing ────────────────────────────────────────
  //
  // GHL calls this endpoint for every payment made through order forms,
  // funnels, and invoices that use Shamroq as the payment provider.
  //
  // Flow:
  //   1. Verify HMAC signature (reject unsigned requests with 401)
  //   2. Map GHL locationId → merchant
  //   3. Route to NMI using merchant's credentials
  //   4. Record transaction with ghlOrderId for reconciliation
  //   5. Return GHL-expected response format

  app.post('/query', async (request: FastifyRequest, reply: FastifyReply) => {
    // ── 1. Verify HMAC signature ────────────────────────────────────────────
    const signatureHeader = (
      (request.headers['x-ghl-signature'] as string | undefined) ||
      (request.headers['x-hub-signature-256'] as string | undefined)
    );

    if (!signatureHeader) {
      app.log.warn({ url: request.url }, 'GHL query: missing signature header');
      return reply.status(401).send({ success: false, message: 'Missing signature' } satisfies GhlQueryResponse);
    }

    const rawBody = (request as FastifyRequest & { rawBody?: Buffer }).rawBody;
    if (!rawBody) {
      app.log.error('GHL query: rawBody not available — content type parser misconfigured');
      return reply.status(500).send({ success: false, message: 'Internal error' } satisfies GhlQueryResponse);
    }

    if (!verifyGhlSignature(rawBody, signatureHeader, env.GHL_CLIENT_SECRET)) {
      app.log.warn({ url: request.url }, 'GHL query: invalid HMAC signature');
      return reply.status(401).send({ success: false, message: 'Invalid signature' } satisfies GhlQueryResponse);
    }

    // ── 2. Parse and validate body ──────────────────────────────────────────
    const body = request.body as GhlQueryBody;

    if (!body || typeof body.type !== 'string') {
      return reply.status(400).send({ success: false, message: 'Invalid request body' } satisfies GhlQueryResponse);
    }

    const locationId = body.meta?.locationId;
    if (!locationId) {
      return reply.status(400).send({ success: false, message: 'Missing locationId in meta' } satisfies GhlQueryResponse);
    }

    // ── 3. Look up merchant by GHL locationId ───────────────────────────────
    const merchant = await prisma.merchant.findUnique({
      where: { ghlLocationId: locationId },
    });

    if (!merchant) {
      app.log.warn({ locationId }, 'GHL query: merchant not found for locationId');
      await prisma.auditLog.create({
        data: {
          actorType: 'ghl',
          actorId: locationId,
          action: 'ghl_query_merchant_not_found',
          resource: 'merchant',
          details: { locationId, type: body.type, orderId: body.meta?.orderId },
        },
      }).catch((err: unknown) => {
        app.log.error({ err }, 'Failed to write audit log');
      });
      return { success: false, message: 'Merchant not configured' } satisfies GhlQueryResponse;
    }

    if (!merchant.nmiSecurityKey) {
      app.log.warn({ merchantId: merchant.id }, 'GHL query: merchant NMI credentials not configured');
      return { success: false, message: 'Merchant payment processing not configured' } satisfies GhlQueryResponse;
    }

    // ── 4. Route by payment type ────────────────────────────────────────────
    switch (body.type) {
      // ── Charge ─────────────────────────────────────────────────────────────
      case 'charge': {
        if (!body.source) {
          return reply.status(400).send({ success: false, message: 'Missing payment source/token' } satisfies GhlQueryResponse);
        }

        const ghlOrderId = body.meta?.orderId;
        const amountCents = body.amount; // GHL sends amount in cents

        // Parse customer name into first/last
        const fullName = body.meta?.name || '';
        const nameParts = fullName.trim().split(/\s+/);
        const firstName = nameParts[0] || undefined;
        const lastName = nameParts.length > 1 ? nameParts.slice(1).join(' ') : undefined;

        // Create pending transaction record
        const transaction = await prisma.transaction.create({
          data: {
            merchantId: merchant.id,
            amountCents,
            currency: body.currency || 'USD',
            status: 'PENDING',
            paymentMethod: 'CARD',
            description: `GHL order ${ghlOrderId || 'unknown'}`,
            ghlOrderId: ghlOrderId || null,
            ipAddress: request.ip || null,
          },
        });

        // Charge via merchant's NMI account
        const chargeResult = await chargeCard({
          paymentToken: body.source,
          amountCents,
          orderId: transaction.id,
          customerEmail: body.meta?.email,
          customerFirstName: firstName,
          customerLastName: lastName,
          ipAddress: request.ip || undefined,
          securityKey: merchant.nmiSecurityKey,
        });

        if (!chargeResult.success) {
          // Update transaction as declined
          await prisma.transaction.update({
            where: { id: transaction.id },
            data: {
              status: 'DECLINED',
              nmiTransactionId: chargeResult.transactionId || null,
              nmiResponseCode: chargeResult.responseCode || null,
              nmiResponseText: chargeResult.responseText || null,
            },
          });

          // Audit log
          await prisma.auditLog.create({
            data: {
              merchantId: merchant.id,
              actorType: 'ghl',
              actorId: locationId,
              action: 'ghl_charge_declined',
              resource: 'transaction',
              resourceId: transaction.id,
              details: {
                ghlOrderId,
                responseCode: chargeResult.responseCode,
                responseText: chargeResult.responseText,
              },
            },
          }).catch((err: unknown) => {
            app.log.error({ err }, 'Failed to write audit log');
          });

          return {
            success: false,
            message: chargeResult.responseText || 'Payment declined',
          } satisfies GhlQueryResponse;
        }

        // Update transaction with NMI result
        await prisma.transaction.update({
          where: { id: transaction.id },
          data: {
            status: 'CAPTURED',
            nmiTransactionId: chargeResult.transactionId || null,
            nmiResponseCode: chargeResult.responseCode || null,
            nmiResponseText: chargeResult.responseText || null,
            nmiAuthCode: chargeResult.authCode || null,
            cardBrand: chargeResult.cardBrand || null,
            cardLast4: chargeResult.cardLast4 || null,
            cardExpMonth: chargeResult.cardExpMonth || null,
            cardExpYear: chargeResult.cardExpYear || null,
          },
        });

        // Audit log success
        await prisma.auditLog.create({
          data: {
            merchantId: merchant.id,
            actorType: 'ghl',
            actorId: locationId,
            action: 'ghl_charge_success',
            resource: 'transaction',
            resourceId: transaction.id,
            details: {
              ghlOrderId,
              nmiTransactionId: chargeResult.transactionId,
              amountCents,
            },
          },
        }).catch((err: unknown) => {
          app.log.error({ err }, 'Failed to write audit log');
        });

        app.log.info(
          { merchantId: merchant.id, transactionId: transaction.id, ghlOrderId },
          'GHL charge processed successfully',
        );

        return {
          success: true,
          transactionId: transaction.id,
          message: 'Payment successful',
        } satisfies GhlQueryResponse;
      }

      // ── Refund ─────────────────────────────────────────────────────────────
      case 'refund': {
        const ghlOrderId = body.meta?.orderId;
        const nmiTxnId = body.meta?.transactionId;

        // Find the original transaction — try by NMI transaction ID first,
        // then fall back to ghlOrderId
        let originalTxn = null;

        if (nmiTxnId) {
          originalTxn = await prisma.transaction.findFirst({
            where: { merchantId: merchant.id, nmiTransactionId: nmiTxnId },
          });
        }

        if (!originalTxn && ghlOrderId) {
          originalTxn = await prisma.transaction.findFirst({
            where: { merchantId: merchant.id, ghlOrderId },
          });
        }

        if (!originalTxn) {
          app.log.warn({ merchantId: merchant.id, ghlOrderId, nmiTxnId }, 'GHL refund: original transaction not found');
          return {
            success: false,
            message: 'Original transaction not found',
          } satisfies GhlQueryResponse;
        }

        if (!originalTxn.nmiTransactionId) {
          return {
            success: false,
            message: 'Original transaction has no NMI reference',
          } satisfies GhlQueryResponse;
        }

        const refundAmountCents = body.amount || undefined;

        const refundResult = await refundTransaction({
          transactionId: originalTxn.nmiTransactionId,
          amountCents: refundAmountCents,
          securityKey: merchant.nmiSecurityKey,
        });

        if (!refundResult.success) {
          await prisma.auditLog.create({
            data: {
              merchantId: merchant.id,
              actorType: 'ghl',
              actorId: locationId,
              action: 'ghl_refund_failed',
              resource: 'transaction',
              resourceId: originalTxn.id,
              details: {
                ghlOrderId,
                nmiTxnId: originalTxn.nmiTransactionId,
                responseCode: refundResult.responseCode,
                responseText: refundResult.responseText,
              },
            },
          }).catch((err: unknown) => {
            app.log.error({ err }, 'Failed to write audit log');
          });

          return {
            success: false,
            message: refundResult.responseText || 'Refund failed',
          } satisfies GhlQueryResponse;
        }

        // Update transaction status
        await prisma.transaction.update({
          where: { id: originalTxn.id },
          data: {
            status: 'REFUNDED',
            refundedAmountCents: refundAmountCents ?? originalTxn.amountCents,
          },
        });

        // Audit log
        await prisma.auditLog.create({
          data: {
            merchantId: merchant.id,
            actorType: 'ghl',
            actorId: locationId,
            action: 'ghl_refund_success',
            resource: 'transaction',
            resourceId: originalTxn.id,
            details: {
              ghlOrderId,
              nmiTxnId: originalTxn.nmiTransactionId,
              refundAmountCents: refundAmountCents ?? originalTxn.amountCents,
            },
          },
        }).catch((err: unknown) => {
          app.log.error({ err }, 'Failed to write audit log');
        });

        app.log.info(
          { merchantId: merchant.id, transactionId: originalTxn.id, ghlOrderId },
          'GHL refund processed successfully',
        );

        return {
          success: true,
          transactionId: originalTxn.id,
          message: 'Refund successful',
        } satisfies GhlQueryResponse;
      }

      // ── Subscription ────────────────────────────────────────────────────────
      case 'subscription': {
        // Subscription support is planned but not yet implemented.
        // Log and return a clear message.
        app.log.warn({ merchantId: merchant.id, locationId }, 'GHL query: subscription type not yet supported');
        return {
          success: false,
          message: 'Subscription payments not yet supported',
        } satisfies GhlQueryResponse;
      }

      // ── Unknown ─────────────────────────────────────────────────────────────
      default: {
        app.log.warn({ type: body.type }, 'GHL query: unknown payment type');
        return {
          success: false,
          message: `Unknown payment type: ${body.type}`,
        } satisfies GhlQueryResponse;
      }
    }
  });
}
