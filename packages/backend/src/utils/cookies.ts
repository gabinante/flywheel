/**
 * Cookie utilities for the affiliate referral flow.
 *
 * Pure functions — no Prisma or database dependency.
 * Safe to import in any context, including tests.
 */

export const REFERRAL_COOKIE_NAME = "ghp_ref";
export const REFERRAL_COOKIE_MAX_AGE_DAYS = 90;

/**
 * Parse the ghp_ref referral cookie from a raw Cookie header string.
 * Returns the referral code if present, or undefined.
 */
export function parseReferralCookie(
  cookieHeader: string | undefined
): string | undefined {
  if (!cookieHeader) return undefined;
  const pairs = cookieHeader.split(";");
  for (const pair of pairs) {
    const eqIdx = pair.indexOf("=");
    if (eqIdx === -1) continue;
    const key = pair.slice(0, eqIdx).trim();
    if (key === REFERRAL_COOKIE_NAME) {
      const value = pair.slice(eqIdx + 1).trim();
      try {
        return decodeURIComponent(value) || undefined;
      } catch {
        return value || undefined;
      }
    }
  }
  return undefined;
}

/**
 * Build a Set-Cookie header value for the ghp_ref referral cookie.
 * 90-day expiry, SameSite=Lax, no HttpOnly so frontend JS can read it.
 */
export function buildReferralCookieHeader(referralCode: string): string {
  const maxAge = REFERRAL_COOKIE_MAX_AGE_DAYS * 24 * 60 * 60;
  return `${REFERRAL_COOKIE_NAME}=${encodeURIComponent(referralCode)}; Max-Age=${maxAge}; Path=/; SameSite=Lax`;
}
