/**
 * transaction-receipt email template
 *
 * Sent to customer on successful payment.
 * Variables: merchantName, amount, last4, transactionId, date
 */

import { wrapInBaseLayout } from "./base.js";

export interface TransactionReceiptVars {
  merchantName: string;
  amount: string;
  last4: string;
  transactionId: string;
  date: string;
}

export function render(vars: Record<string, unknown>): string {
  const merchantName = String(vars.merchantName ?? "Merchant");
  const amount = String(vars.amount ?? "$0.00");
  const last4 = String(vars.last4 ?? "****");
  const transactionId = String(vars.transactionId ?? "N/A");
  const date = String(vars.date ?? new Date().toLocaleDateString());

  const body = `
    <h1 style="margin: 0 0 16px; font-size: 24px; font-weight: 700; color: #111827;">
      Payment Receipt
    </h1>
    <p style="margin: 0 0 16px; font-size: 16px; line-height: 1.5; color: #374151;">
      Your payment to <strong>${escapeHtml(merchantName)}</strong> was successful.
    </p>
    <table role="presentation" cellpadding="0" cellspacing="0" width="100%" style="margin: 0 0 24px; border: 1px solid #e5e7eb; border-radius: 8px; overflow: hidden;">
      <tr>
        <td style="padding: 12px 16px; background-color: #f9fafb; font-size: 14px; color: #6b7280; border-bottom: 1px solid #e5e7eb;">
          Amount
        </td>
        <td style="padding: 12px 16px; background-color: #f9fafb; font-size: 16px; font-weight: 700; color: #111827; text-align: right; border-bottom: 1px solid #e5e7eb;">
          ${escapeHtml(amount)}
        </td>
      </tr>
      <tr>
        <td style="padding: 12px 16px; font-size: 14px; color: #6b7280; border-bottom: 1px solid #e5e7eb;">
          Card
        </td>
        <td style="padding: 12px 16px; font-size: 14px; color: #374151; text-align: right; border-bottom: 1px solid #e5e7eb;">
          **** **** **** ${escapeHtml(last4)}
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
          Date
        </td>
        <td style="padding: 12px 16px; font-size: 14px; color: #374151; text-align: right;">
          ${escapeHtml(date)}
        </td>
      </tr>
    </table>
    <p style="margin: 0; font-size: 14px; color: #6b7280;">
      If you have questions about this charge, please contact ${escapeHtml(merchantName)} directly.
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
