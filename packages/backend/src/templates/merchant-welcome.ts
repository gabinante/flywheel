/**
 * merchant-welcome email template
 *
 * Sent on merchant onboarding completion.
 * Variables: merchantName, dashboardUrl
 */

import { wrapInBaseLayout } from "./base.js";

export interface MerchantWelcomeVars {
  merchantName: string;
  dashboardUrl: string;
}

export function render(vars: Record<string, unknown>): string {
  const merchantName = String(vars.merchantName ?? "Merchant");
  const dashboardUrl = String(vars.dashboardUrl ?? "#");

  const body = `
    <h1 style="margin: 0 0 16px; font-size: 24px; font-weight: 700; color: #111827;">
      Welcome to GoHighPayment!
    </h1>
    <p style="margin: 0 0 16px; font-size: 16px; line-height: 1.5; color: #374151;">
      Hi ${escapeHtml(merchantName)},
    </p>
    <p style="margin: 0 0 16px; font-size: 16px; line-height: 1.5; color: #374151;">
      Your merchant account has been successfully set up. You can now start accepting payments
      through GoHighPayment.
    </p>
    <p style="margin: 0 0 24px; font-size: 16px; line-height: 1.5; color: #374151;">
      Access your dashboard to configure payment settings, view transactions, and manage your account.
    </p>
    <table role="presentation" cellpadding="0" cellspacing="0">
      <tr>
        <td style="background-color: #2563eb; border-radius: 6px;">
          <a href="${escapeHtml(dashboardUrl)}" style="display: inline-block; padding: 12px 24px; font-size: 16px; font-weight: 600; color: #ffffff; text-decoration: none;">
            Go to Dashboard
          </a>
        </td>
      </tr>
    </table>
    <p style="margin: 24px 0 0; font-size: 14px; color: #6b7280;">
      If you have any questions, our support team is here to help.
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
