/**
 * Email Service Tests
 *
 * Tests for the Postmark email service integration.
 * Uses fetch mocking to avoid real API calls.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { createEmailService } from "../services/email.service.js";

// ─── Mock Setup ───────────────────────────────────────────────────────

const mockFetch = vi.fn();

beforeEach(() => {
  vi.stubGlobal("fetch", mockFetch);
  mockFetch.mockReset();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// ─── Tests ────────────────────────────────────────────────────────────

describe("createEmailService", () => {
  const config = {
    apiKey: "test-postmark-key",
    fromAddress: "test@gohighpayment.com",
    baseUrl: "https://api.postmarkapp.test",
  };

  describe("sendEmail", () => {
    it("sends email successfully via Postmark API", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          To: "user@example.com",
          SubmittedAt: "2026-04-28T00:00:00Z",
          MessageID: "msg-123",
          ErrorCode: 0,
          Message: "OK",
        }),
      });

      const service = createEmailService(config);
      const result = await service.sendEmail({
        to: "user@example.com",
        templateId: "merchant-welcome",
        variables: { merchantName: "Acme Corp", dashboardUrl: "https://app.example.com" },
      });

      expect(result.success).toBe(true);
      expect(result.messageId).toBe("msg-123");
      expect(result.error).toBeUndefined();

      // Verify fetch was called correctly
      expect(mockFetch).toHaveBeenCalledOnce();
      const [url, options] = mockFetch.mock.calls[0];
      expect(url).toBe("https://api.postmarkapp.test/email");
      expect(options.method).toBe("POST");
      expect(options.headers["X-Postmark-Server-Token"]).toBe("test-postmark-key");

      const body = JSON.parse(options.body);
      expect(body.From).toBe("test@gohighpayment.com");
      expect(body.To).toBe("user@example.com");
      expect(body.Subject).toBe("Welcome to GoHighPayment!");
      expect(body.HtmlBody).toContain("Acme Corp");
      expect(body.HtmlBody).toContain("https://app.example.com");
      expect(body.MessageStream).toBe("outbound");
    });

    it("returns failure on Postmark API HTTP error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 422,
        text: async () => '{"ErrorCode":300,"Message":"Invalid email address"}',
      });

      const service = createEmailService(config);
      const result = await service.sendEmail({
        to: "invalid",
        templateId: "merchant-welcome",
        variables: { merchantName: "Test" },
      });

      expect(result.success).toBe(false);
      expect(result.error).toContain("422");
    });

    it("returns failure on Postmark error code in response", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          To: "user@example.com",
          SubmittedAt: "2026-04-28T00:00:00Z",
          MessageID: "",
          ErrorCode: 406,
          Message: "Inactive recipient",
        }),
      });

      const service = createEmailService(config);
      const result = await service.sendEmail({
        to: "bounced@example.com",
        templateId: "merchant-welcome",
        variables: { merchantName: "Test" },
      });

      expect(result.success).toBe(false);
      expect(result.error).toContain("406");
      expect(result.error).toContain("Inactive recipient");
    });

    it("catches fetch exceptions and returns failure (never throws)", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network timeout"));

      const service = createEmailService(config);
      const result = await service.sendEmail({
        to: "user@example.com",
        templateId: "merchant-welcome",
        variables: { merchantName: "Test" },
      });

      expect(result.success).toBe(false);
      expect(result.error).toBe("Network timeout");
    });

    it("uses default from address and base URL", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          To: "user@example.com",
          SubmittedAt: "2026-04-28T00:00:00Z",
          MessageID: "msg-456",
          ErrorCode: 0,
          Message: "OK",
        }),
      });

      const service = createEmailService({ apiKey: "key-only" });
      await service.sendEmail({
        to: "user@example.com",
        templateId: "transaction-receipt",
        variables: {
          merchantName: "Shop",
          amount: "$50.00",
          last4: "4242",
          transactionId: "txn_123",
          date: "2026-04-28",
        },
      });

      const [url, options] = mockFetch.mock.calls[0];
      expect(url).toBe("https://api.postmarkapp.com/email");
      const body = JSON.parse(options.body);
      expect(body.From).toBe("noreply@gohighpayment.com");
    });

    it("allows subject override", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          To: "user@example.com",
          SubmittedAt: "2026-04-28T00:00:00Z",
          MessageID: "msg-789",
          ErrorCode: 0,
          Message: "OK",
        }),
      });

      const service = createEmailService(config);
      await service.sendEmail({
        to: "user@example.com",
        templateId: "merchant-welcome",
        variables: { merchantName: "Test" },
        subject: "Custom Subject",
      });

      const body = JSON.parse(mockFetch.mock.calls[0][1].body);
      expect(body.Subject).toBe("Custom Subject");
    });
  });
});
