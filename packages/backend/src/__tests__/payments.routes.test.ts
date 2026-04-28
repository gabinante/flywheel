/**
 * Payment Routes — Unit Tests
 *
 * Tests the POST /api/v1/payments endpoint including:
 * - API key authentication
 * - Request validation
 * - NMI transaction submission
 * - Error handling
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import express from "express";
import {
  createPaymentRouter,
  type NmiTransactionClient,
  type MerchantKeyLookup,
  type NmiTransactionResult,
} from "../routes/payments.js";

// ─── Lightweight supertest-like helper ──────────────────────────────

async function request(
  app: express.Express,
  method: string,
  path: string,
  options: { body?: unknown; headers?: Record<string, string> } = {}
) {
  return new Promise<{
    status: number;
    body: Record<string, unknown>;
  }>((resolve, reject) => {
    const server = app.listen(0, () => {
      const addr = server.address();
      if (!addr || typeof addr === "string") {
        server.close();
        return reject(new Error("Failed to get server address"));
      }
      const url = `http://127.0.0.1:${addr.port}${path}`;

      const fetchHeaders: Record<string, string> = {
        "Content-Type": "application/json",
        ...options.headers,
      };

      fetch(url, {
        method,
        headers: fetchHeaders,
        body: options.body ? JSON.stringify(options.body) : undefined,
      })
        .then(async (res) => {
          const json = (await res.json()) as Record<string, unknown>;
          server.close();
          resolve({ status: res.status, body: json });
        })
        .catch((err) => {
          server.close();
          reject(err);
        });
    });
  });
}

// ─── Test Helpers ────────────────────────────────────────────────────

function createMockNmiClient(
  overrides: Partial<NmiTransactionClient> = {}
): NmiTransactionClient {
  return {
    sale: vi.fn(async (): Promise<NmiTransactionResult> => ({
      success: true,
      transactionId: "txn_12345",
      responseCode: "1",
      responseText: "SUCCESS",
    })),
    ...overrides,
  };
}

function createMockMerchantLookup(
  overrides: Partial<MerchantKeyLookup> = {}
): MerchantKeyLookup {
  return {
    getSecurityKey: vi.fn(async (apiKey: string) => {
      if (apiKey === "sec_live_valid") {
        return {
          merchantId: "merchant_001",
          nmiSecurityKey: "nmi_secret_key_123",
        };
      }
      return null;
    }),
    ...overrides,
  };
}

function createApp(
  nmiClient?: NmiTransactionClient,
  merchantLookup?: MerchantKeyLookup
) {
  const app = express();
  app.use(express.json());
  const router = createPaymentRouter({
    nmiClient: nmiClient ?? createMockNmiClient(),
    merchantKeyLookup: merchantLookup ?? createMockMerchantLookup(),
  });
  app.use("/api/v1/payments", router);
  return app;
}

// ─── Tests ───────────────────────────────────────────────────────────

const validPayload = {
  paymentToken: "tok_abc123",
  amountCents: 2500,
  currency: "USD",
  customerEmail: "buyer@example.com",
  description: "Order #1234",
};

describe("POST /api/v1/payments", () => {
  // ── Authentication ──

  describe("authentication", () => {
    it("returns 401 if X-API-Key header is missing", async () => {
      const app = createApp();
      const res = await request(app, "POST", "/api/v1/payments", {
        body: validPayload,
      });

      expect(res.status).toBe(401);
      expect(res.body.error).toContain("Missing X-API-Key");
    });

    it("returns 401 if API key is invalid", async () => {
      const app = createApp();
      const res = await request(app, "POST", "/api/v1/payments", {
        body: validPayload,
        headers: { "X-API-Key": "sec_live_invalid" },
      });

      expect(res.status).toBe(401);
      expect(res.body.error).toContain("Invalid API key");
    });

    it("accepts a valid API key", async () => {
      const app = createApp();
      const res = await request(app, "POST", "/api/v1/payments", {
        body: validPayload,
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(res.status).toBe(200);
    });
  });

  // ── Validation ──

  describe("validation", () => {
    it("returns 400 if paymentToken is missing", async () => {
      const app = createApp();
      const res = await request(app, "POST", "/api/v1/payments", {
        body: { amountCents: 2500 },
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(res.status).toBe(400);
      expect(res.body.error).toContain("Invalid request body");
    });

    it("returns 400 if amountCents is missing", async () => {
      const app = createApp();
      const res = await request(app, "POST", "/api/v1/payments", {
        body: { paymentToken: "tok_abc" },
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(res.status).toBe(400);
    });

    it("returns 400 if amountCents is not positive", async () => {
      const app = createApp();
      const res = await request(app, "POST", "/api/v1/payments", {
        body: { paymentToken: "tok_abc", amountCents: -100 },
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(res.status).toBe(400);
    });

    it("returns 400 if amountCents is not an integer", async () => {
      const app = createApp();
      const res = await request(app, "POST", "/api/v1/payments", {
        body: { paymentToken: "tok_abc", amountCents: 25.5 },
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(res.status).toBe(400);
    });

    it("returns 400 if currency is not 3 letters", async () => {
      const app = createApp();
      const res = await request(app, "POST", "/api/v1/payments", {
        body: { paymentToken: "tok_abc", amountCents: 2500, currency: "US" },
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(res.status).toBe(400);
    });

    it("returns 400 if customerEmail is invalid", async () => {
      const app = createApp();
      const res = await request(app, "POST", "/api/v1/payments", {
        body: {
          paymentToken: "tok_abc",
          amountCents: 2500,
          customerEmail: "not-an-email",
        },
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(res.status).toBe(400);
    });

    it("defaults currency to USD", async () => {
      const mockNmi = createMockNmiClient();
      const app = createApp(mockNmi);

      await request(app, "POST", "/api/v1/payments", {
        body: { paymentToken: "tok_abc123", amountCents: 1000 },
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(mockNmi.sale).toHaveBeenCalledWith(
        expect.objectContaining({ currency: "USD" })
      );
    });
  });

  // ── Successful Payment ──

  describe("successful payment", () => {
    it("returns 200 with transaction details", async () => {
      const app = createApp();
      const res = await request(app, "POST", "/api/v1/payments", {
        body: validPayload,
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(res.status).toBe(200);
      expect(res.body).toMatchObject({
        success: true,
        transactionId: "txn_12345",
        status: "approved",
        amountCents: 2500,
        currency: "USD",
      });
    });

    it("converts amountCents to dollars for NMI", async () => {
      const mockNmi = createMockNmiClient();
      const app = createApp(mockNmi);

      await request(app, "POST", "/api/v1/payments", {
        body: validPayload,
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(mockNmi.sale).toHaveBeenCalledWith(
        expect.objectContaining({
          amountDollars: "25.00",
          token: "tok_abc123",
          securityKey: "nmi_secret_key_123",
        })
      );
    });

    it("passes optional fields to NMI", async () => {
      const mockNmi = createMockNmiClient();
      const app = createApp(mockNmi);

      await request(app, "POST", "/api/v1/payments", {
        body: { ...validPayload, orderId: "order_xyz" },
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(mockNmi.sale).toHaveBeenCalledWith(
        expect.objectContaining({
          email: "buyer@example.com",
          description: "Order #1234",
          orderId: "order_xyz",
        })
      );
    });
  });

  // ── Declined Payment ──

  describe("declined payment", () => {
    it("returns 402 when transaction is declined", async () => {
      const mockNmi = createMockNmiClient({
        sale: vi.fn(async () => ({
          success: false,
          transactionId: "txn_67890",
          responseCode: "2",
          responseText: "DECLINE",
        })),
      });
      const app = createApp(mockNmi);

      const res = await request(app, "POST", "/api/v1/payments", {
        body: validPayload,
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(res.status).toBe(402);
      expect(res.body).toMatchObject({
        success: false,
        error: "DECLINE",
        transactionId: "txn_67890",
        status: "declined",
      });
    });
  });

  // ── Server Errors ──

  describe("server errors", () => {
    it("returns 500 when NMI client throws", async () => {
      const mockNmi = createMockNmiClient({
        sale: vi.fn(async () => {
          throw new Error("NMI connection timeout");
        }),
      });
      const app = createApp(mockNmi);

      const res = await request(app, "POST", "/api/v1/payments", {
        body: validPayload,
        headers: { "X-API-Key": "sec_live_valid" },
      });

      expect(res.status).toBe(500);
      expect(res.body.error).toContain("Payment processing failed");
    });
  });
});
