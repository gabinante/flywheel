/**
 * Prisma Client Singleton
 *
 * Exports a shared Prisma client instance.  Also re-exports the
 * attribution and cookie utilities for convenience.
 *
 * NOTE: This module instantiates PrismaClient on first import.
 *       Route files that need only the helper functions should import
 *       directly from utils/attribution.ts or utils/cookies.ts instead.
 */

import { PrismaClient } from "@prisma/client";

// ─── Singleton ────────────────────────────────────────────────────────

const globalForPrisma = globalThis as unknown as { _prisma?: PrismaClient };

export const prisma = globalForPrisma._prisma ?? new PrismaClient();

if (process.env.NODE_ENV !== "production") {
  globalForPrisma._prisma = prisma;
}

export default prisma;

// ─── Re-exports from sub-modules ────────────────────────────────────

export {
  AttributionImmutableError,
  enforceAttributionImmutability,
} from "./attribution.js";

export {
  REFERRAL_COOKIE_NAME,
  REFERRAL_COOKIE_MAX_AGE_DAYS,
  parseReferralCookie,
  buildReferralCookieHeader,
} from "./cookies.js";
