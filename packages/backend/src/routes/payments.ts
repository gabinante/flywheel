/**
 * Direct Payment Routes — server-to-server charge endpoint
 *
 * POST /api/v1/payments
 *   Headers: { X-API-Key: sec_live_xyz789 }
 *   Body: {
 *     paymentToken: "tok_abc123",
 *     amountCents: 2500,
 *     currency: "USD",
 *     customerEmail: "buyer@example.com",
 *     description: "Order #1234"
 *   }
 *   Returns: { success, transactionId, status, amountCents, currency }
 *
 * This endpoint allows merchants to charge tokens collected via NoStripeTax Elements.
 * The merchant's server sends the token + amount, and we submit an NMI sale transaction.
 */

import { Router, type Request, type Response } from "express";
import { z } from "zod";

// ─── Validation ───────────────────────────────────────────────────────

const paymentSchema = z.object({
  paymentToken: z
    .string()
    .min(1, { message: "paymentToken is required" }),
  amountCents: z
    .number()
    .int()
    .positive({ message: "amountCents must be a positive integer" }),
  currency: z
    .string()
    .length(3, { message: "currency must be a 3-letter ISO code" })
    .default("USD"),
  customerEmail: z.string().email().optional(),
  description: z.string().max(255).optional(),
  orderId: z.string().max(64).optional(),
});

export type PaymentRequest = z.infer<typeof paymentSchema>;

// ─── NMI Client Interface ────────────────────────────────────────────

export interface NmiTransactionClient {
  /**
   * Submit a sale transaction to NMI using a payment token.
   */
  sale(params: {
    token: string;
    amountDollars: string;
    currency: string;
    email?: string;
    description?: string;
    orderId?: string;
    securityKey: string;
  }): Promise<NmiTransactionResult>;
}

export interface NmiTransactionResult {
  success: boolean;
  transactionId: string;
  responseCode: string;
  responseText: string;
}

/**
 * Default NMI transaction client — submits sale via NMI Direct Post API.
 */
export function createNmiTransactionClient(): NmiTransactionClient {
  return {
    async sale(params): Promise<NmiTransactionResult> {
      const body = new URLSearchParams({
        security_key: params.securityKey,
        type: "sale",
        payment_token: params.token,
        amount: params.amountDollars,
        currency: params.currency,
      });

      if (params.email) body.append("email", params.email);
      if (params.description) body.append("order_description", params.description);
      if (params.orderId) body.append("orderid", params.orderId);

      const response = await fetch(
        "https://secure.nmi.com/api/transact.php",
        {
          method: "POST",
          headers: { "Content-Type": "application/x-www-form-urlencoded" },
          body: body.toString(),
        }
      );

      const text = await response.text();
      const parsed = new URLSearchParams(text);

      const responseCode = parsed.get("response") ?? "";
      const transactionId = parsed.get("transactionid") ?? "";
      const responseText = parsed.get("responsetext") ?? "";

      return {
        success: responseCode === "1",
        transactionId,
        responseCode,
        responseText,
      };
    },
  };
}

// ─── API Key Lookup ──────────────────────────────────────────────────

export interface MerchantKeyLookup {
  /**
   * Look up a merchant's NMI security key by their API key.
   * Returns null if the API key is invalid.
   */
  getSecurityKey(apiKey: string): Promise<{
    merchantId: string;
    nmiSecurityKey: string;
  } | null>;
}

// ─── Router Factory ──────────────────────────────────────────────────

export interface PaymentRouterDeps {
  nmiClient?: NmiTransactionClient;
  merchantKeyLookup: MerchantKeyLookup;
}

export function createPaymentRouter(deps: PaymentRouterDeps): Router {
  const router = Router();
  const nmiClient = deps.nmiClient ?? createNmiTransactionClient();

  /**
   * POST /api/v1/payments
   *
   * Accept a payment token from NoStripeTax Elements and charge via NMI.
   * Requires X-API-Key header for merchant authentication.
   */
  router.post("/", async (req: Request, res: Response): Promise<void> => {
    // ── Authenticate via API key ──
    const apiKey = req.headers["x-api-key"] as string | undefined;
    if (!apiKey) {
      res.status(401).json({
        error: "Missing X-API-Key header",
      });
      return;
    }

    const merchant = await deps.merchantKeyLookup.getSecurityKey(apiKey);
    if (!merchant) {
      res.status(401).json({
        error: "Invalid API key",
      });
      return;
    }

    // ── Validate request body ──
    const parsed = paymentSchema.safeParse(req.body);
    if (!parsed.success) {
      res.status(400).json({
        error: "Invalid request body",
        details: parsed.error.issues,
      });
      return;
    }

    const { paymentToken, amountCents, currency, customerEmail, description, orderId } =
      parsed.data;

    // Convert cents to dollars string for NMI
    const amountDollars = (amountCents / 100).toFixed(2);

    try {
      const result = await nmiClient.sale({
        token: paymentToken,
        amountDollars,
        currency,
        email: customerEmail,
        description,
        orderId,
        securityKey: merchant.nmiSecurityKey,
      });

      if (result.success) {
        res.status(200).json({
          success: true,
          transactionId: result.transactionId,
          status: "approved",
          amountCents,
          currency,
        });
      } else {
        res.status(402).json({
          success: false,
          error: result.responseText || "Transaction declined",
          transactionId: result.transactionId,
          status: "declined",
        });
      }
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      console.error("[Payments] Transaction failed:", message);
      res.status(500).json({
        error: "Payment processing failed",
        message,
      });
    }
  });

  return router;
}
