/**
 * Email Service — Postmark transactional email integration
 *
 * Sends emails via the Postmark API. All failures are caught and logged
 * (email failure must NEVER break payment flows).
 *
 * Usage:
 *   const emailService = createEmailService({ apiKey: process.env.POSTMARK_API_KEY });
 *   const result = await emailService.sendEmail({
 *     to: 'user@example.com',
 *     templateId: 'merchant-welcome',
 *     variables: { merchantName: 'Acme' },
 *   });
 */

// ─── Types ────────────────────────────────────────────────────────────

export interface EmailServiceConfig {
  /** Postmark server API token */
  apiKey: string;
  /** From address (default: noreply@gohighpayment.com) */
  fromAddress?: string;
  /** Postmark API base URL (override for testing) */
  baseUrl?: string;
}

export interface SendEmailParams {
  to: string;
  templateId: string;
  variables: Record<string, unknown>;
  /** Optional subject override (defaults to template-specific subject) */
  subject?: string;
}

export interface SendEmailResult {
  success: boolean;
  messageId?: string;
  error?: string;
}

export interface PostmarkResponse {
  To: string;
  SubmittedAt: string;
  MessageID: string;
  ErrorCode: number;
  Message: string;
}

// ─── Template subjects ────────────────────────────────────────────────

const TEMPLATE_SUBJECTS: Record<string, string> = {
  "merchant-welcome": "Welcome to GoHighPayment!",
  "transaction-receipt": "Payment Receipt",
  "chargeback-alert": "Chargeback Alert — Action Required",
  "payment-failed-dunning": "Payment Failed — Update Your Payment Method",
  "invoice-sent": "You Have a New Invoice",
  "residual-statement": "Monthly Residual Statement",
  "tier-upgrade": "Congratulations — Tier Upgrade!",
};

// ─── Service ──────────────────────────────────────────────────────────

export function createEmailService(config: EmailServiceConfig) {
  const {
    apiKey,
    fromAddress = "noreply@gohighpayment.com",
    baseUrl = "https://api.postmarkapp.com",
  } = config;

  /**
   * Send a transactional email via Postmark.
   *
   * This function NEVER throws — all errors are caught, logged,
   * and returned in the result object.
   */
  async function sendEmail(params: SendEmailParams): Promise<SendEmailResult> {
    const { to, templateId, variables, subject } = params;

    try {
      // Import template renderer
      const { renderTemplate } = await import("../templates/index.js");

      const htmlBody = renderTemplate(templateId, variables);
      const emailSubject =
        subject ?? TEMPLATE_SUBJECTS[templateId] ?? `GoHighPayment — ${templateId}`;

      const response = await fetch(`${baseUrl}/email`, {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          "X-Postmark-Server-Token": apiKey,
        },
        body: JSON.stringify({
          From: fromAddress,
          To: to,
          Subject: emailSubject,
          HtmlBody: htmlBody,
          MessageStream: "outbound",
        }),
      });

      if (!response.ok) {
        const errorBody = await response.text();
        console.error(
          `[EmailService] Postmark API error (${response.status}): ${errorBody}`
        );
        return {
          success: false,
          error: `Postmark API error: ${response.status} — ${errorBody}`,
        };
      }

      const data = (await response.json()) as PostmarkResponse;

      if (data.ErrorCode !== 0) {
        console.error(
          `[EmailService] Postmark returned error code ${data.ErrorCode}: ${data.Message}`
        );
        return {
          success: false,
          error: `Postmark error: ${data.ErrorCode} — ${data.Message}`,
        };
      }

      console.log(
        `[EmailService] Sent ${templateId} to ${to} — MessageID: ${data.MessageID}`
      );

      return {
        success: true,
        messageId: data.MessageID,
      };
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      console.error(
        `[EmailService] Failed to send ${templateId} to ${to}: ${message}`
      );
      return {
        success: false,
        error: message,
      };
    }
  }

  return {
    sendEmail,
  };
}

export type EmailService = ReturnType<typeof createEmailService>;
