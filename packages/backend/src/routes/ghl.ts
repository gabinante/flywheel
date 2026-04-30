/**
 * GHL (GoHighLevel) OAuth Routes
 *
 * Implements the GHL OAuth 2.0 install flow for the Shamroq marketplace
 * integration.  Referral codes are threaded through the flow so that
 * merchants who install via an agency's referral link are correctly
 * attributed.
 *
 * Flow:
 *   1. Agency shares link: https://app.shamroq.com/install?ref=agency-abc123
 *   2. GET /api/v1/ghl/install?ref=agency-abc123
 *      - Generates a CSRF state token
 *      - Persists state + referralCode in GhlOAuthState
 *      - Sets ghp_ref cookie (90-day) for cookie-based fallback
 *      - Redirects user to GHL OAuth authorization URL
 *   3. GHL redirects back to GET /api/v1/ghl/callback?code=xxx&state=yyy
 *      - Validates CSRF state
 *      - Retrieves referralCode from GhlOAuthState
 *      - Exchanges code for GHL access token
 *      - Creates or finds Merchant keyed on ghlLocationId
 *      - Attributes merchant to agency (immutability rule: only set if not already set)
 *      - Cleans up the used state record
 */

import { Router, type Request, type Response } from "express";
import { randomBytes } from "crypto";
import { z } from "zod";
import type { PrismaClient } from "@prisma/client";
import { buildReferralCookieHeader, parseReferralCookie } from "../utils/cookies.js";

// ─── Types ────────────────────────────────────────────────────────────

export interface GhlRouterDeps {
  prisma: PrismaClient;
  /** GHL app client ID from marketplace config */
  ghlClientId?: string;
  /** GHL app client secret */
  ghlClientSecret?: string;
  /** Base URL for OAuth redirect (e.g. https://app.shamroq.com) */
  appBaseUrl?: string;
  /**
   * Optional: inject a custom GHL token exchange function for testing.
   * In production this calls the real GHL OAuth token endpoint.
   */
  exchangeGhlCode?: (code: string, redirectUri: string) => Promise<GhlTokenResponse>;
  /**
   * Optional: inject a custom function to fetch the GHL location info.
   * In production this calls GET /locations/:locationId on the GHL API.
   */
  fetchGhlLocation?: (
    accessToken: string,
    locationId: string
  ) => Promise<GhlLocationInfo>;
}

export interface GhlTokenResponse {
  access_token: string;
  refresh_token?: string;
  locationId: string;
  companyId?: string;
  userId?: string;
}

export interface GhlLocationInfo {
  id: string;
  name: string;
  email?: string;
  phone?: string;
}

// ─── Constants ────────────────────────────────────────────────────────

const GHL_AUTH_BASE = "https://marketplace.gohighlevel.com/oauth/chooselocation";
const GHL_TOKEN_URL = "https://services.leadconnectorhq.com/oauth/token";
const GHL_API_BASE = "https://services.leadconnectorhq.com";

/** How long the CSRF state is valid (15 minutes) */
const STATE_TTL_MS = 15 * 60 * 1000;

const callbackQuerySchema = z.object({
  code: z.string().min(1, "code is required"),
  state: z.string().min(1, "state is required"),
});

// ─── Router Factory ───────────────────────────────────────────────────

export function createGhlRouter(deps: GhlRouterDeps): Router {
  const router = Router();

  const clientId = deps.ghlClientId ?? process.env.GHL_CLIENT_ID ?? "";
  const clientSecret = deps.ghlClientSecret ?? process.env.GHL_CLIENT_SECRET ?? "";
  const appBaseUrl = deps.appBaseUrl ?? process.env.APP_BASE_URL ?? "http://localhost:3000";
  const redirectUri = `${appBaseUrl}/api/v1/ghl/callback`;

  /**
   * Default GHL token exchange — calls the real GHL OAuth endpoint.
   * Replaced in tests via deps.exchangeGhlCode.
   */
  const exchangeGhlCode =
    deps.exchangeGhlCode ??
    (async (code: string, uri: string): Promise<GhlTokenResponse> => {
      const body = new URLSearchParams({
        client_id: clientId,
        client_secret: clientSecret,
        grant_type: "authorization_code",
        code,
        redirect_uri: uri,
      });
      const resp = await fetch(GHL_TOKEN_URL, {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: body.toString(),
      });
      if (!resp.ok) {
        const text = await resp.text();
        throw new Error(`GHL token exchange failed (${resp.status}): ${text}`);
      }
      return resp.json() as Promise<GhlTokenResponse>;
    });

  /**
   * Default GHL location fetch — calls the real GHL API.
   * Replaced in tests via deps.fetchGhlLocation.
   */
  const fetchGhlLocation =
    deps.fetchGhlLocation ??
    (async (accessToken: string, locationId: string): Promise<GhlLocationInfo> => {
      const resp = await fetch(`${GHL_API_BASE}/locations/${locationId}`, {
        headers: { Authorization: `Bearer ${accessToken}` },
      });
      if (!resp.ok) {
        const text = await resp.text();
        throw new Error(`GHL location fetch failed (${resp.status}): ${text}`);
      }
      const data = (await resp.json()) as { location?: GhlLocationInfo };
      return data.location ?? (data as unknown as GhlLocationInfo);
    });

  // ─── GET /install ──────────────────────────────────────────────────

  /**
   * Initiates the GHL OAuth flow.
   *
   * Query params:
   *   ref  — optional agency referral code (e.g. "agency-abc123")
   *
   * Behaviour:
   *   - Generates a cryptographically random CSRF state token
   *   - Stores {state, referralCode} in GhlOAuthState with a 15-min TTL
   *   - If `ref` is present, sets a ghp_ref cookie (90-day, SameSite=Lax)
   *   - Redirects to the GHL OAuth authorization URL
   */
  router.get("/install", async (req: Request, res: Response): Promise<void> => {
    const refCode =
      (typeof req.query.ref === "string" && req.query.ref) ||
      parseReferralCookie(req.headers.cookie) ||
      undefined;

    // Generate CSRF state token
    const state = randomBytes(32).toString("hex");
    const expiresAt = new Date(Date.now() + STATE_TTL_MS);

    // Persist state + referralCode for the callback
    await deps.prisma.ghlOAuthState.create({
      data: {
        state,
        referralCode: refCode ?? null,
        expiresAt,
      },
    });

    // Set referral cookie so it survives redirects
    const cookies: string[] = [];
    if (refCode) {
      cookies.push(buildReferralCookieHeader(refCode));
    }
    if (cookies.length > 0) {
      res.setHeader("Set-Cookie", cookies);
    }

    // Redirect to GHL OAuth
    const authUrl = new URL(GHL_AUTH_BASE);
    authUrl.searchParams.set("client_id", clientId);
    authUrl.searchParams.set("redirect_uri", redirectUri);
    authUrl.searchParams.set("response_type", "code");
    authUrl.searchParams.set("state", state);

    res.redirect(302, authUrl.toString());
  });

  // ─── GET /callback ─────────────────────────────────────────────────

  /**
   * GHL OAuth callback.
   *
   * GHL redirects here with:
   *   code  — authorization code to exchange for tokens
   *   state — CSRF token we generated in /install
   *
   * Behaviour:
   *   - Validates the CSRF state token (rejects if expired or unknown)
   *   - Retrieves the stored referralCode from GhlOAuthState
   *   - Exchanges code for GHL access token
   *   - Fetches location info from GHL API
   *   - Creates or finds the Merchant keyed on ghlLocationId
   *   - If merchant has no agencyId and referralCode is valid: attributes
   *     merchant to that agency (immutability: never overrides existing)
   *   - Cleans up the used GhlOAuthState record
   */
  router.get("/callback", async (req: Request, res: Response): Promise<void> => {
    const parsed = callbackQuerySchema.safeParse(req.query);
    if (!parsed.success) {
      res.status(400).json({
        error: "Invalid callback parameters",
        details: parsed.error.issues,
      });
      return;
    }

    const { code, state } = parsed.data;

    // Validate CSRF state
    const oauthState = await deps.prisma.ghlOAuthState.findUnique({
      where: { state },
    });

    if (!oauthState) {
      res.status(400).json({ error: "Invalid or expired OAuth state" });
      return;
    }

    if (oauthState.expiresAt < new Date()) {
      // Clean up expired state
      await deps.prisma.ghlOAuthState.delete({ where: { state } }).catch(() => {});
      res.status(400).json({ error: "OAuth state has expired" });
      return;
    }

    // Extract stored referral code (may be null if no ref was provided)
    const storedReferralCode = oauthState.referralCode ?? undefined;

    // Also check cookie as a fallback (belt + suspenders)
    const cookieReferralCode = parseReferralCookie(req.headers.cookie);
    const referralCode = storedReferralCode ?? cookieReferralCode;

    // Exchange code for tokens
    let tokenData: GhlTokenResponse;
    try {
      tokenData = await exchangeGhlCode(code, redirectUri);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      await deps.prisma.ghlOAuthState.delete({ where: { state } }).catch(() => {});
      res.status(502).json({ error: "Failed to exchange OAuth code", message });
      return;
    }

    // Fetch location details
    let locationInfo: GhlLocationInfo;
    try {
      locationInfo = await fetchGhlLocation(tokenData.access_token, tokenData.locationId);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      await deps.prisma.ghlOAuthState.delete({ where: { state } }).catch(() => {});
      res.status(502).json({ error: "Failed to fetch GHL location", message });
      return;
    }

    // Resolve attribution (never overrides existing agencyId)
    let agencyId: string | null = null;
    if (referralCode) {
      const agency = await deps.prisma.agency
        .findUnique({
          where: { referralCode },
          select: { id: true },
        })
        .catch(() => null);
      if (agency) {
        agencyId = agency.id;
      }
    }

    // Upsert merchant keyed on GHL location ID
    const existingMerchant = await deps.prisma.merchant.findUnique({
      where: { ghlLocationId: tokenData.locationId },
      select: { id: true, agencyId: true },
    });

    let merchantId: string;

    if (existingMerchant) {
      merchantId = existingMerchant.id;

      // Immutability rule: only apply attribution if not already set
      if (!existingMerchant.agencyId && agencyId) {
        await deps.prisma.merchant.update({
          where: { id: existingMerchant.id },
          data: { agencyId, attributedAt: new Date() },
        });
      }
    } else {
      // Create new merchant from GHL location data
      const newMerchant = await deps.prisma.merchant.create({
        data: {
          name: locationInfo.name,
          email: locationInfo.email ?? `ghl-${tokenData.locationId}@placeholder.local`,
          phone: locationInfo.phone ?? null,
          ghlLocationId: tokenData.locationId,
          agencyId: agencyId ?? null,
          attributedAt: agencyId ? new Date() : null,
        },
        select: { id: true },
      });
      merchantId = newMerchant.id;
    }

    // Clean up used state record
    await deps.prisma.ghlOAuthState.delete({ where: { state } }).catch(() => {});

    // Set referral cookie on the response so the browser retains it
    if (referralCode) {
      res.setHeader("Set-Cookie", buildReferralCookieHeader(referralCode));
    }

    res.status(200).json({
      success: true,
      merchantId,
    });
  });

  return router;
}
