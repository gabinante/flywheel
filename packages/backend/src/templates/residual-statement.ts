/**
 * residual-statement email template
 *
 * Monthly statement sent to agency.
 * Variables: agencyName, period, totalEarned, merchantBreakdown[], payoutDate
 */

import { wrapInBaseLayout } from "./base.js";

export interface MerchantBreakdownItem {
  merchantName: string;
  volume: string;
  share: string;
}

export interface ResidualStatementVars {
  agencyName: string;
  period: string;
  totalEarned: string;
  merchantBreakdown: MerchantBreakdownItem[];
  payoutDate: string;
}

export function render(vars: Record<string, unknown>): string {
  const agencyName = String(vars.agencyName ?? "Agency");
  const period = String(vars.period ?? "N/A");
  const totalEarned = String(vars.totalEarned ?? "$0.00");
  const payoutDate = String(vars.payoutDate ?? "N/A");

  // Parse merchantBreakdown array
  let merchantBreakdown: MerchantBreakdownItem[] = [];
  if (Array.isArray(vars.merchantBreakdown)) {
    merchantBreakdown = vars.merchantBreakdown.map((item: unknown) => {
      const i = item as Record<string, unknown>;
      return {
        merchantName: String(i.merchantName ?? "N/A"),
        volume: String(i.volume ?? "$0.00"),
        share: String(i.share ?? "$0.00"),
      };
    });
  }

  const breakdownRows = merchantBreakdown
    .map(
      (item) => `
      <tr>
        <td style="padding: 8px 16px; font-size: 14px; color: #374151; border-bottom: 1px solid #e5e7eb;">
          ${escapeHtml(item.merchantName)}
        </td>
        <td style="padding: 8px 16px; font-size: 14px; color: #374151; text-align: right; border-bottom: 1px solid #e5e7eb;">
          ${escapeHtml(item.volume)}
        </td>
        <td style="padding: 8px 16px; font-size: 14px; color: #374151; text-align: right; border-bottom: 1px solid #e5e7eb;">
          ${escapeHtml(item.share)}
        </td>
      </tr>`
    )
    .join("");

  const breakdownTable =
    merchantBreakdown.length > 0
      ? `
    <table role="presentation" cellpadding="0" cellspacing="0" width="100%" style="margin: 0 0 24px; border: 1px solid #e5e7eb; border-radius: 8px; overflow: hidden;">
      <tr>
        <th style="padding: 10px 16px; background-color: #f9fafb; font-size: 12px; font-weight: 600; color: #6b7280; text-align: left; border-bottom: 1px solid #e5e7eb;">
          Merchant
        </th>
        <th style="padding: 10px 16px; background-color: #f9fafb; font-size: 12px; font-weight: 600; color: #6b7280; text-align: right; border-bottom: 1px solid #e5e7eb;">
          Volume
        </th>
        <th style="padding: 10px 16px; background-color: #f9fafb; font-size: 12px; font-weight: 600; color: #6b7280; text-align: right; border-bottom: 1px solid #e5e7eb;">
          Your Share
        </th>
      </tr>
      ${breakdownRows}
      <tr>
        <td style="padding: 10px 16px; font-size: 14px; font-weight: 700; color: #111827;">
          Total
        </td>
        <td style="padding: 10px 16px;"></td>
        <td style="padding: 10px 16px; font-size: 16px; font-weight: 700; color: #059669; text-align: right;">
          ${escapeHtml(totalEarned)}
        </td>
      </tr>
    </table>`
      : `
    <p style="margin: 0 0 24px; font-size: 14px; color: #6b7280;">
      No merchant activity this period.
    </p>`;

  const body = `
    <h1 style="margin: 0 0 16px; font-size: 24px; font-weight: 700; color: #111827;">
      Monthly Residual Statement
    </h1>
    <p style="margin: 0 0 16px; font-size: 16px; line-height: 1.5; color: #374151;">
      Hi ${escapeHtml(agencyName)}, here is your residual statement for <strong>${escapeHtml(period)}</strong>.
    </p>
    <div style="padding: 16px; background-color: #ecfdf5; border: 1px solid #a7f3d0; border-radius: 8px; margin-bottom: 24px; text-align: center;">
      <p style="margin: 0 0 4px; font-size: 14px; color: #065f46;">Total Earned</p>
      <p style="margin: 0; font-size: 32px; font-weight: 700; color: #059669;">
        ${escapeHtml(totalEarned)}
      </p>
    </div>
    <h2 style="margin: 0 0 12px; font-size: 18px; font-weight: 600; color: #111827;">
      Merchant Breakdown
    </h2>
    ${breakdownTable}
    <p style="margin: 0; font-size: 14px; color: #6b7280;">
      Payout scheduled for: <strong>${escapeHtml(payoutDate)}</strong>
    </p>
  `;

  return wrapInBaseLayout(body, { showUnsubscribe: true });
}

function escapeHtml(str: string): string {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}
