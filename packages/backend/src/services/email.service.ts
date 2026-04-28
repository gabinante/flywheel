/**
 * Email Service — Postmark transactional email integration
 *
 * Sends emails via the Postmark API. All failures are caught and logged
 * (email failure must NEVER break payment flows).
 *
 * Usage:
 *   const emailService = createEmailService({
 *     apiKey: process.env.POSTMARK_API_KEY,
 *     logger: fastify.log,
 *   });
 *   const result = await emailService.sendEmail({
 *     to: 'user@example.com',
 *     templateId: 'merchant-welcome',
 *     variables: { merchantName: 'Acme' },
 *   });
 */

import { createTimer } from '../utils/logger.js';

// ─── Types ────────────────────────────────────────────────────────────

/** Logger interface — accepts any pino-compatible logger */
interface EmailLogger {
  info(obj: Record<string, unknown>, msg: string): void;
  warn(obj: Record<string, unknown>, msg: string): void;
  error(obj: Record<string, unknown>, msg: string): void;
}

export interface EmailServiceConfig {
  /** Postmark server API token */
  apiKey: string;
  /** From address (default: noreply@gohighpayment.com) */
  fromAddress?: string;
  /** Postmark API base URL (override for testing) */
  baseUrl?: string;
  /** Structured logger */
  logger?: EmailLogger;
}

export interface SendEmailParams {
  to: string;
  templateId: string;
  variables: Record<string, unknown>;
  /** Optional subject override (defaults to template-specific subject) */
  subject?: string;
  /** Correlation ID for end-to-end tracing */
  correlationId?: string;
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
    logger,
  } = config;

  /**
   * Send a transactional email via Postmark.
   *
   * This function NEVER throws — all errors are caught, logged,
   * and returned in the result object.
   */
  async function sendEmail(params: SendEmailParams): Promise<SendEmailResult> {
    const { to, templateId, variables, subject, correlationId } = params;
    const timer = createTimer();

    logger?.info(
      { action: 'email_send_initiated', templateId, to, correlationId },
      `Sending ${templateId} email`
    );

    try {
      // Import template renderer (may not exist yet — fallback to plain text)
      let htmlBody: string;
      try {
        const { renderTemplate } = await import("../templates/index.js");
        htmlBody = renderTemplate(templateId, variables);
      } catch {
        // Template module not yet created — use simple fallback
        htmlBody = `<p>${templateId}: ${JSON.stringify(variables)}</p>`;
      }
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
        const duration_ms = timer.elapsed();
        logger?.error(
          {
            action: 'email_postmark_api_error',
            templateId,
            to,
            correlationId,
            statusCode: response.status,
            duration_ms,
          },
          `Postmark API error (${response.status})`
        );
        return {
          success: false,
          error: `Postmark API error: ${response.status} — ${errorBody}`,
        };
      }

      const data = (await response.json()) as PostmarkResponse;

      if (data.ErrorCode !== 0) {
        const duration_ms = timer.elapsed();
        logger?.error(
          {
            action: 'email_postmark_error_code',
            templateId,
            to,
            correlationId,
            errorCode: data.ErrorCode,
            duration_ms,
          },
          `Postmark returned error code ${data.ErrorCode}: ${data.Message}`
        );
        return {
          success: false,
          error: `Postmark error: ${data.ErrorCode} — ${data.Message}`,
        };
      }

      const duration_ms = timer.elapsed();
      logger?.info(
        {
          action: 'email_sent',
          templateId,
          to,
          correlationId,
          messageId: data.MessageID,
          duration_ms,
        },
        `Email ${templateId} sent successfully`
      );

      return {
        success: true,
        messageId: data.MessageID,
      };
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      const duration_ms = timer.elapsed();
      logger?.error(
        {
          action: 'email_send_failed',
          templateId,
          to,
          correlationId,
          err,
          duration_ms,
        },
        `Failed to send ${templateId} to ${to}`
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
