/**
 * chargeback-alert email template
 *
 * Sent to merchant on new chargeback.
 * Variables: merchantName, amount, transactionId, reason
 */

import { wrapInBaseLayout } from "./base.js";

export interface ChargebackAlertVars {
  merchantName: string;
  amount: string;
  transactionId: string;
  reason: string;
}

export function render(vars: Record<string, unknown>): string {
  const merchantName = String(vars.merchantName ?? "Merchant");
  const amount = String(vars.amount ?? "$0.00");
  const transactionId = String(vars.transactionId ?? "N/A");
  const reason = String(vars.reason ?? "Not specified");

  const body = `
    <div style="padding: 16px; background-color: #fef2f2; border: 1px solid #fecaca; border-radius: 8px; margin-bottom: 24px;">
      <p style="margin: 0; font-size: 16px; font-weight: 600; color: #991b1b;">
        A chargeback has been filed against your account
      </p>
    </div>
    <h1 style="margin: 0 0 16px; font-size: 24px; font-weight: 700; color: #111827;">
      Chargeback Alert
    </h1>
    <p style="margin: 0 0 16px; font-size: 16px; line-height: 1.5; color: #374151;">
      Hi ${escapeHtml(merchantName)}, a customer has filed a chargeback for a recent transaction.
      Please review the details below and take action.
    </p>
    <table role="presentation" cellpadding="0" cellspacing="0" width="100%" style="margin: 0 0 24px; border: 1px solid #e5e7eb; border-radius: 8px; overflow: hidden;">
      <tr>
        <td style="padding: 12px 16px; background-color: #f9fafb; font-size: 14px; color: #6b7280; border-bottom: 1px solid #e5e7eb;">
          Amount
        </td>
        <td style="padding: 12px 16px; background-color: #f9fafb; font-size: 16px; font-weight: 700; color: #dc2626; text-align: right; border-bottom: 1px solid #e5e7eb;">
          ${escapeHtml(amount)}
        </td>
      </tr>
      <tr>
        <td style="padding: 12px 16px; font-size: 14px; color: #6b7280; border-bottom: 1px solid #e5e7eb;">
          Transaction ID
        </td>
        <td style="padding: 12px 16px; font-size: 14px; color: #374151; text-align: right; border-bottom: 1px solid #e5e7eb;">
          ${escapeHtml(transactionId)}
        </td>
      </tr>
      <tr>
        <td style="padding: 12px 16px; font-size: 14px; color: #6b7280;">
          Reason
        </td>
        <td style="padding: 12px 16px; font-size: 14px; color: #374151; text-align: right;">
          ${escapeHtml(reason)}
        </td>
      </tr>
    </table>
    <p style="margin: 0 0 16px; font-size: 16px; line-height: 1.5; color: #374151;">
      <strong>Action required:</strong> You typically have 7–14 days to respond to a chargeback.
      Gather relevant documentation (receipts, delivery proof, correspondence) and submit your response.
    </p>
    <p style="margin: 0; font-size: 14px; color: #6b7280;">
      Unresolved chargebacks may result in additional fees and can impact your merchant account standing.
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
