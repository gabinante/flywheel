/**
 * invoice-sent email template
 *
 * Sent to customer with invoice link.
 * Variables: merchantName, invoiceNumber, amount, dueDate, payUrl
 */

import { wrapInBaseLayout } from "./base.js";

export interface InvoiceSentVars {
  merchantName: string;
  invoiceNumber: string;
  amount: string;
  dueDate: string;
  payUrl: string;
}

export function render(vars: Record<string, unknown>): string {
  const merchantName = String(vars.merchantName ?? "Merchant");
  const invoiceNumber = String(vars.invoiceNumber ?? "N/A");
  const amount = String(vars.amount ?? "$0.00");
  const dueDate = String(vars.dueDate ?? "N/A");
  const payUrl = String(vars.payUrl ?? "#");

  const body = `
    <h1 style="margin: 0 0 16px; font-size: 24px; font-weight: 700; color: #111827;">
      Invoice from ${escapeHtml(merchantName)}
    </h1>
    <p style="margin: 0 0 16px; font-size: 16px; line-height: 1.5; color: #374151;">
      You have received a new invoice. Please review the details below.
    </p>
    <table role="presentation" cellpadding="0" cellspacing="0" width="100%" style="margin: 0 0 24px; border: 1px solid #e5e7eb; border-radius: 8px; overflow: hidden;">
      <tr>
        <td style="padding: 12px 16px; background-color: #f9fafb; font-size: 14px; color: #6b7280; border-bottom: 1px solid #e5e7eb;">
          Invoice Number
        </td>
        <td style="padding: 12px 16px; background-color: #f9fafb; font-size: 14px; color: #374151; text-align: right; border-bottom: 1px solid #e5e7eb;">
          ${escapeHtml(invoiceNumber)}
        </td>
      </tr>
      <tr>
        <td style="padding: 12px 16px; font-size: 14px; color: #6b7280; border-bottom: 1px solid #e5e7eb;">
          Amount Due
        </td>
        <td style="padding: 12px 16px; font-size: 16px; font-weight: 700; color: #111827; text-align: right; border-bottom: 1px solid #e5e7eb;">
          ${escapeHtml(amount)}
        </td>
      </tr>
      <tr>
        <td style="padding: 12px 16px; font-size: 14px; color: #6b7280;">
          Due Date
        </td>
        <td style="padding: 12px 16px; font-size: 14px; color: #374151; text-align: right;">
          ${escapeHtml(dueDate)}
        </td>
      </tr>
    </table>
    <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 0 0 24px;">
      <tr>
        <td style="background-color: #2563eb; border-radius: 6px;">
          <a href="${escapeHtml(payUrl)}" style="display: inline-block; padding: 12px 24px; font-size: 16px; font-weight: 600; color: #ffffff; text-decoration: none;">
            Pay Invoice
          </a>
        </td>
      </tr>
    </table>
    <p style="margin: 0; font-size: 14px; color: #6b7280;">
      If you have any questions about this invoice, please contact ${escapeHtml(merchantName)}.
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
