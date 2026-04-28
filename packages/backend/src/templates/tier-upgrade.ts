/**
 * tier-upgrade email template
 *
 * Sent to agency on tier change.
 * Variables: agencyName, oldTier, newTier, newBpsRate
 */

import { wrapInBaseLayout } from "./base.js";

export interface TierUpgradeVars {
  agencyName: string;
  oldTier: string;
  newTier: string;
  newBpsRate: string;
}

export function render(vars: Record<string, unknown>): string {
  const agencyName = String(vars.agencyName ?? "Agency");
  const oldTier = String(vars.oldTier ?? "N/A");
  const newTier = String(vars.newTier ?? "N/A");
  const newBpsRate = String(vars.newBpsRate ?? "N/A");

  const body = `
    <div style="padding: 16px; background-color: #eff6ff; border: 1px solid #bfdbfe; border-radius: 8px; margin-bottom: 24px; text-align: center;">
      <p style="margin: 0; font-size: 32px;">&#127881;</p>
      <p style="margin: 8px 0 0; font-size: 18px; font-weight: 700; color: #1e40af;">
        You've been upgraded!
      </p>
    </div>
    <h1 style="margin: 0 0 16px; font-size: 24px; font-weight: 700; color: #111827;">
      Tier Upgrade
    </h1>
    <p style="margin: 0 0 16px; font-size: 16px; line-height: 1.5; color: #374151;">
      Congratulations ${escapeHtml(agencyName)}! Your agency tier has been upgraded based on your
      merchant volume performance.
    </p>
    <table role="presentation" cellpadding="0" cellspacing="0" width="100%" style="margin: 0 0 24px; border: 1px solid #e5e7eb; border-radius: 8px; overflow: hidden;">
      <tr>
        <td style="padding: 12px 16px; background-color: #f9fafb; font-size: 14px; color: #6b7280; border-bottom: 1px solid #e5e7eb;">
          Previous Tier
        </td>
        <td style="padding: 12px 16px; background-color: #f9fafb; font-size: 14px; color: #374151; text-align: right; border-bottom: 1px solid #e5e7eb;">
          ${escapeHtml(oldTier)}
        </td>
      </tr>
      <tr>
        <td style="padding: 12px 16px; font-size: 14px; color: #6b7280; border-bottom: 1px solid #e5e7eb;">
          New Tier
        </td>
        <td style="padding: 12px 16px; font-size: 16px; font-weight: 700; color: #059669; text-align: right; border-bottom: 1px solid #e5e7eb;">
          ${escapeHtml(newTier)}
        </td>
      </tr>
      <tr>
        <td style="padding: 12px 16px; font-size: 14px; color: #6b7280;">
          New Rate
        </td>
        <td style="padding: 12px 16px; font-size: 14px; color: #374151; text-align: right;">
          ${escapeHtml(newBpsRate)} bps
        </td>
      </tr>
    </table>
    <p style="margin: 0 0 16px; font-size: 16px; line-height: 1.5; color: #374151;">
      Your new rate will be applied to all residual calculations starting next month.
      Keep growing your merchant portfolio to unlock even higher tiers!
    </p>
    <p style="margin: 0; font-size: 14px; color: #6b7280;">
      Thank you for your partnership with GoHighPayment.
    </p>
  `;

  return wrapInBaseLayout(body);
}

function escapeHtml(str: string): string {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}
