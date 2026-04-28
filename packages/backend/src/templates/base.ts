/**
 * Base Email Template
 *
 * Shared layout wrapper for all transactional emails.
 * Uses inline CSS for maximum email client compatibility.
 */

export interface BaseTemplateOptions {
  /** Show unsubscribe link (required for marketing-adjacent emails) */
  showUnsubscribe?: boolean;
}

/**
 * Wrap email body content in the base layout.
 */
export function wrapInBaseLayout(
  bodyContent: string,
  options: BaseTemplateOptions = {}
): string {
  const { showUnsubscribe = false } = options;

  const unsubscribeBlock = showUnsubscribe
    ? `
      <tr>
        <td style="padding: 16px 24px; text-align: center; font-size: 12px; color: #9ca3af;">
          <a href="{{unsubscribeUrl}}" style="color: #9ca3af; text-decoration: underline;">Unsubscribe</a>
          from these emails
        </td>
      </tr>`
    : "";

  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>GoHighPayment</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f3f4f6; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;">
  <table role="presentation" cellpadding="0" cellspacing="0" width="100%" style="background-color: #f3f4f6;">
    <tr>
      <td style="padding: 24px 0;">
        <table role="presentation" cellpadding="0" cellspacing="0" width="600" style="margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 1px 3px rgba(0,0,0,0.1);">
          <!-- Header -->
          <tr>
            <td style="padding: 24px; background-color: #1f2937; text-align: center;">
              <span style="font-size: 20px; font-weight: 700; color: #ffffff; letter-spacing: -0.025em;">GoHighPayment</span>
            </td>
          </tr>
          <!-- Body -->
          <tr>
            <td style="padding: 32px 24px;">
              ${bodyContent}
            </td>
          </tr>
          <!-- Footer -->
          <tr>
            <td style="padding: 16px 24px; border-top: 1px solid #e5e7eb; text-align: center; font-size: 12px; color: #9ca3af;">
              &copy; ${new Date().getFullYear()} GoHighPayment. All rights reserved.
            </td>
          </tr>
          ${unsubscribeBlock}
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`;
}
