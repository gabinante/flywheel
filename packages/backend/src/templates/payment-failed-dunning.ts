/**
 * payment-failed-dunning email template
 *
 * Sent to customer on subscription payment failure.
 * Variables: merchantName, amount, updatePaymentUrl, retryDate
 */

import { wrapInBaseLayout } from "./base.js";

export interface PaymentFailedDunningVars {
  merchantName: string;
  amount: string;
  updatePaymentUrl: string;
  retryDate: string;
}

export function render(vars: Record<string, unknown>): string {
  const merchantName = String(vars.merchantName ?? "Merchant");
  const amount = String(vars.amount ?? "$0.00");
  const updatePaymentUrl = String(vars.updatePaymentUrl ?? "#");
  const retryDate = String(vars.retryDate ?? "soon");

  const body = `
    <div style="padding: 16px; background-color: #fffbeb; border: 1px solid #fde68a; border-radius: 8px; margin-bottom: 24px;">
      <p style="margin: 0; font-size: 16px; font-weight: 600; color: #92400e;">
        Your payment could not be processed
      </p>
    </div>
    <h1 style="margin: 0 0 16px; font-size: 24px; font-weight: 700; color: #111827;">
      Payment Failed
    </h1>
    <p style="margin: 0 0 16px; font-size: 16px; line-height: 1.5; color: #374151;">
      We were unable to process your payment of <strong>${escapeHtml(amount)}</strong>
      to <strong>${escapeHtml(merchantName)}</strong>.
    </p>
    <p style="margin: 0 0 16px; font-size: 16px; line-height: 1.5; color: #374151;">
      To avoid service interruption, please update your payment method.
      We will automatically retry the payment on <strong>${escapeHtml(retryDate)}</strong>.
    </p>
    <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 0 0 24px;">
      <tr>
        <td style="background-color: #2563eb; border-radius: 6px;">
          <a href="${escapeHtml(updatePaymentUrl)}" style="display: inline-block; padding: 12px 24px; font-size: 16px; font-weight: 600; color: #ffffff; text-decoration: none;">
            Update Payment Method
          </a>
        </td>
      </tr>
    </table>
    <p style="margin: 0; font-size: 14px; color: #6b7280;">
      If you believe this is an error, please contact ${escapeHtml(merchantName)} for assistance.
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
