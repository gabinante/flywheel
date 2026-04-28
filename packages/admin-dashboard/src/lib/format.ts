/**
 * Formatting utilities for revenue dashboard
 */

/**
 * Format cents as a dollar string with commas.
 * e.g., 1500000 → "$15,000.00"
 */
export function formatCents(cents: number): string {
  const dollars = cents / 100;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(dollars);
}

/**
 * Format cents as a compact dollar string.
 * e.g., 1500000000 → "$15M"
 */
export function formatCentsCompact(cents: number): string {
  const dollars = cents / 100;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(dollars);
}

/**
 * Format a decimal ratio as a percentage.
 * e.g., 0.004 → "0.40%"
 */
export function formatPercent(ratio: number): string {
  return `${(ratio * 100).toFixed(2)}%`;
}

/**
 * Calculate month-over-month delta percentage.
 * Returns null if previous value is 0 or undefined.
 */
export function calcMoMDelta(
  current: number,
  previous: number | undefined
): number | null {
  if (previous === undefined || previous === 0) return null;
  return ((current - previous) / Math.abs(previous)) * 100;
}

/**
 * Format a delta as a signed percentage string.
 * e.g., 12.5 → "+12.5%", -3.2 → "-3.2%"
 */
export function formatDelta(delta: number | null): string {
  if (delta === null) return "N/A";
  const sign = delta >= 0 ? "+" : "";
  return `${sign}${delta.toFixed(1)}%`;
}
