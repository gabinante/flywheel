/**
 * Pure chargeback threshold logic — no external dependencies.
 * Extracted so it can be unit-tested without loading Prisma or pg-boss.
 */

export const CB_WARNING_THRESHOLD = 0.005;  // 0.5%
export const CB_CRITICAL_THRESHOLD = 0.007; // 0.7%
export const CB_HIGH_THRESHOLD = 0.010;     // 1.0%

/**
 * Compute chargeback ratio (handle divide-by-zero).
 * Returns 0 when transactionCount is 0.
 */
export function computeChargebackRatio(transactionCount: number, chargebackCount: number): number {
  return transactionCount === 0 ? 0 : chargebackCount / transactionCount;
}

/**
 * Determine chargeback risk level from ratio.
 * Returns null when below WARNING threshold.
 *
 * Thresholds (strictly greater than):
 *   > 1.0% → HIGH (auto-flag merchant)
 *   > 0.7% → CRITICAL (urgent notification)
 *   > 0.5% → WARNING (operator notification)
 */
export function getRiskLevel(ratio: number): string | null {
  if (ratio > CB_HIGH_THRESHOLD) return 'HIGH';
  if (ratio > CB_CRITICAL_THRESHOLD) return 'CRITICAL';
  if (ratio > CB_WARNING_THRESHOLD) return 'WARNING';
  return null;
}
