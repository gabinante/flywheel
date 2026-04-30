/**
 * Merchant Auth Routes — Registration with Affiliate Attribution
 *
 * POST /api/v1/merchant/auth/register
 *   Registers a new merchant. Accepts an optional `ref` field (body or
 *   query param) containing an agency referral code.  If the code is
 *   valid the merchant is permanently attributed to that agency.
 *   Falls back to the `ghp_ref` cookie when no explicit `ref` is given.
 *   Invalid / missing codes are silently ignored (never block signup).
 *
 * PATCH /api/v1/merchant/:id
 *   Partial update for merchant profile fields.
 *   agencyId and attributedAt are immutable — any attempt to change
 *   them after initial attribution is rejected with 409.
 */

import { Router, type Request, type Response } from "express";
import { z } from "zod";
import type { PrismaClient } from "@prisma/client";
import {
  enforceAttributionImmutability,
  AttributionImmutableError,
} from "../utils/attribution.js";
import {
  parseReferralCookie,
  buildReferralCookieHeader,
} from "../utils/cookies.js";

// ─── Validation Schemas ───────────────────────────────────────────────

const registerSchema = z.object({
  name: z.string().min(1, "Name is required").max(255),
  email: z.string().email("Invalid email address"),
  phone: z.string().optional(),
  /** Referral code — agency's unique referral code, e.g. "agency-abc123" */
  ref: z.string().optional(),
});

const patchMerchantSchema = z.object({
  name: z.string().min(1).max(255).optional(),
  phone: z.string().optional(),
  status: z.enum(["ACTIVE", "INACTIVE", "SUSPENDED"]).optional(),
  /** Blocked: agencyId is immutable after initial attribution */
  agencyId: z.string().optional(),
  /** Blocked: attributedAt is immutable after initial attribution */
  attributedAt: z.string().datetime().optional(),
});

// ─── Router Deps ──────────────────────────────────────────────────────

export interface MerchantAuthRouterDeps {
  prisma: PrismaClient;
}

// ─── Helpers ──────────────────────────────────────────────────────────

/**
 * Resolve the referral code from (in priority order):
 *   1. Explicit `ref` in request body
 *   2. `ref` query parameter
 *   3. `ghp_ref` cookie
 */
function resolveReferralCode(req: Request, bodyRef?: string): string | undefined {
  if (bodyRef) return bodyRef;
  if (typeof req.query.ref === "string" && req.query.ref) return req.query.ref;
  return parseReferralCookie(req.headers.cookie);
}

/**
 * Look up an Agency by its referral code.
 * Returns the agency id if found, null if the code is unknown.
 * Never throws — invalid codes are silently ignored.
 */
async function lookupAgencyByReferralCode(
  prismaClient: PrismaClient,
  referralCode: string
): Promise<string | null> {
  try {
    const agency = await prismaClient.agency.findUnique({
      where: { referralCode },
      select: { id: true },
    });
    return agency?.id ?? null;
  } catch {
    return null;
  }
}

// ─── Router Factory ───────────────────────────────────────────────────

export function createMerchantAuthRouter(deps: MerchantAuthRouterDeps): Router {
  const router = Router();

  /**
   * POST /register
   *
   * Registers a new merchant.  If a valid referral code is resolved
   * (body, query param, or cookie), the merchant is attributed to that
   * agency.  Attribution is set once and cannot be changed later.
   *
   * If the referral code is provided but invalid the merchant is still
   * created — attribution is simply left null.  This ensures invalid
   * codes never block sign-up.
   *
   * On success, if a referral code was present in the request, a
   * Set-Cookie header is returned so the browser retains the code for
   * subsequent sign-up attempts (90-day, SameSite=Lax).
   */
  router.post(
    "/register",
    async (req: Request, res: Response): Promise<void> => {
      const parsed = registerSchema.safeParse(req.body);
      if (!parsed.success) {
        res.status(400).json({
          error: "Invalid request body",
          details: parsed.error.issues,
        });
        return;
      }

      const { name, email, phone, ref: bodyRef } = parsed.data;
      const refCode = resolveReferralCode(req, bodyRef);

      // Resolve attribution
      let agencyId: string | null = null;
      let attributedAt: Date | null = null;

      if (refCode) {
        const resolvedAgencyId = await lookupAgencyByReferralCode(deps.prisma, refCode);
        if (resolvedAgencyId) {
          agencyId = resolvedAgencyId;
          attributedAt = new Date();
        }
        // If code unknown: silently ignore, proceed with agencyId=null
      }

      // Create merchant
      let merchant;
      try {
        merchant = await deps.prisma.merchant.create({
          data: {
            name,
            email,
            phone: phone ?? null,
            agencyId,
            attributedAt,
          },
          select: {
            id: true,
            email: true,
            name: true,
            agencyId: true,
            attributedAt: true,
            createdAt: true,
          },
        });
      } catch (err: unknown) {
        // Unique constraint violation — e.g. email already registered
        const message = err instanceof Error ? err.message : String(err);
        if (message.includes("Unique constraint") || message.includes("unique")) {
          res.status(409).json({ error: "Email already registered" });
          return;
        }
        throw err;
      }

      // Set referral cookie so the browser retains it across page loads
      if (refCode) {
        res.setHeader("Set-Cookie", buildReferralCookieHeader(refCode));
      }

      res.status(201).json(merchant);
    }
  );

  /**
   * PATCH /:id
   *
   * Partial update for a merchant's mutable profile fields.
   * Attempting to change `agencyId` or `attributedAt` after they have
   * been set returns 409 Conflict.
   */
  router.patch(
    "/:id",
    async (req: Request, res: Response): Promise<void> => {
      // Express v5 types ParamsDictionary as string | string[]; named params are always strings
      const id = String(req.params.id);

      const parsed = patchMerchantSchema.safeParse(req.body);
      if (!parsed.success) {
        res.status(400).json({
          error: "Invalid request body",
          details: parsed.error.issues,
        });
        return;
      }

      // Guard: reject any attempt to overwrite attribution
      if (parsed.data.agencyId !== undefined || parsed.data.attributedAt !== undefined) {
        try {
          await enforceAttributionImmutability(
            id,
            parsed.data.agencyId,
            deps.prisma
          );
        } catch (err) {
          if (err instanceof AttributionImmutableError) {
            res.status(409).json({
              error: err.message,
            });
            return;
          }
          throw err;
        }
      }

      // Strip immutable fields from the update payload
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      const { agencyId: _agencyId, attributedAt: _attributedAt, ...mutableFields } = parsed.data;

      try {
        const merchant = await deps.prisma.merchant.update({
          where: { id },
          data: mutableFields,
          select: {
            id: true,
            email: true,
            name: true,
            phone: true,
            status: true,
            agencyId: true,
            attributedAt: true,
            updatedAt: true,
          },
        });
        res.status(200).json(merchant);
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        if (message.includes("Record to update not found")) {
          res.status(404).json({ error: "Merchant not found" });
          return;
        }
        throw err;
      }
    }
  );

  return router;
}
