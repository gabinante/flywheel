/**
 * Email Template Registry
 *
 * Central registry for all email templates. Each template module exports
 * a `render(vars)` function that returns HTML.
 *
 * Supported templates:
 * - merchant-welcome
 * - transaction-receipt
 * - chargeback-alert
 * - payment-failed-dunning
 * - invoice-sent
 * - residual-statement
 * - tier-upgrade
 */

import { render as renderMerchantWelcome } from "./merchant-welcome.js";
import { render as renderTransactionReceipt } from "./transaction-receipt.js";
import { render as renderChargebackAlert } from "./chargeback-alert.js";
import { render as renderPaymentFailedDunning } from "./payment-failed-dunning.js";
import { render as renderInvoiceSent } from "./invoice-sent.js";
import { render as renderResidualStatement } from "./residual-statement.js";
import { render as renderTierUpgrade } from "./tier-upgrade.js";

// ─── Template Registry ────────────────────────────────────────────────

type TemplateRenderer = (vars: Record<string, unknown>) => string;

const TEMPLATES: Record<string, TemplateRenderer> = {
  "merchant-welcome": renderMerchantWelcome,
  "transaction-receipt": renderTransactionReceipt,
  "chargeback-alert": renderChargebackAlert,
  "payment-failed-dunning": renderPaymentFailedDunning,
  "invoice-sent": renderInvoiceSent,
  "residual-statement": renderResidualStatement,
  "tier-upgrade": renderTierUpgrade,
};

/**
 * List of all supported template IDs.
 */
export const TEMPLATE_IDS = Object.keys(TEMPLATES) as readonly string[];

/**
 * Render an email template with the given variables.
 *
 * @throws Error if templateId is not found in registry
 */
export function renderTemplate(
  templateId: string,
  variables: Record<string, unknown>
): string {
  const renderer = TEMPLATES[templateId];
  if (!renderer) {
    throw new Error(
      `Unknown email template: "${templateId}". ` +
        `Available templates: ${TEMPLATE_IDS.join(", ")}`
    );
  }
  return renderer(variables);
}

/**
 * Check whether a template ID is valid/registered.
 */
export function isValidTemplate(templateId: string): boolean {
  return templateId in TEMPLATES;
}
