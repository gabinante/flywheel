/**
 * Acceptance Tests — Email Notification System
 *
 * Validates all acceptance criteria for the email notification system:
 * 1. Email service sends via Postmark API
 * 2. All 7 templates render correctly with variables
 * 3. Emails queued via pg-boss, never sent synchronously
 * 4. Email failures logged but don't break payment flows
 * 5. NotificationSchedule records track delivery status
 * 6. Transaction receipts queueable for customers
 * 7. Chargeback alerts queueable for merchants
 * 8. Residual statements queueable for agencies
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { createEmailService } from "../services/email.service.js";
import { renderTemplate, isValidTemplate, TEMPLATE_IDS } from "../templates/index.js";
import { enqueueEmail, enqueueEmailSimple } from "../jobs/queue.js";
import { EMAIL_SEND_JOB } from "../jobs/handlers.js";

// ─── Mock Factories ───────────────────────────────────────────────────

function createMockBoss() {
  return {
    send: vi.fn().mockResolvedValue("job-id-acceptance"),
  };
}

function createMockPrisma() {
  return {
    notificationSchedule: {
      create: vi.fn().mockResolvedValue({
        id: "notif-acceptance-1",
        to: "test@example.com",
        templateId: "merchant-welcome",
        variables: {},
        status: "QUEUED",
        attempts: 0,
      }),
      update: vi.fn().mockResolvedValue({}),
    },
  };
}

const mockFetch = vi.fn();
beforeEach(() => {
  vi.stubGlobal("fetch", mockFetch);
  mockFetch.mockReset();
});
afterEach(() => {
  vi.unstubAllGlobals();
});

// ─── Acceptance Test 1: Email service sends via Postmark ──────────────

describe("Acceptance: Email service sends via Postmark API", () => {
  it("sends email and returns messageId on success", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        To: "customer@example.com",
        SubmittedAt: "2026-04-28T00:00:00Z",
        MessageID: "acc-msg-001",
        ErrorCode: 0,
        Message: "OK",
      }),
    });

    const service = createEmailService({
      apiKey: "test-key",
      baseUrl: "https://api.postmarkapp.test",
    });

    const result = await service.sendEmail({
      to: "customer@example.com",
      templateId: "transaction-receipt",
      variables: {
        merchantName: "Test Store",
        amount: "$50.00",
        last4: "4242",
        transactionId: "txn_acc_001",
        date: "2026-04-28",
      },
    });

    expect(result.success).toBe(true);
    expect(result.messageId).toBe("acc-msg-001");
  });
});

// ─── Acceptance Test 2: All 7 templates render correctly ──────────────

describe("Acceptance: All 7 email templates render correctly", () => {
  it("has exactly 7 templates registered", () => {
    expect(TEMPLATE_IDS).toHaveLength(7);
  });

  it("merchant-welcome renders with variables", () => {
    const html = renderTemplate("merchant-welcome", {
      merchantName: "Acceptance Test Merchant",
      dashboardUrl: "https://dashboard.example.com",
    });
    expect(html).toContain("Acceptance Test Merchant");
    expect(html).toContain("https://dashboard.example.com");
  });

  it("transaction-receipt renders with variables", () => {
    const html = renderTemplate("transaction-receipt", {
      merchantName: "Test Store",
      amount: "$50.00",
      last4: "4242",
      transactionId: "txn_001",
      date: "2026-04-28",
    });
    expect(html).toContain("$50.00");
    expect(html).toContain("txn_001");
  });

  it("chargeback-alert renders with variables", () => {
    const html = renderTemplate("chargeback-alert", {
      merchantName: "Test Merchant",
      amount: "$150.00",
      transactionId: "txn_002",
      reason: "Fraudulent",
    });
    expect(html).toContain("$150.00");
    expect(html).toContain("Fraudulent");
  });

  it("payment-failed-dunning renders with variables", () => {
    const html = renderTemplate("payment-failed-dunning", {
      merchantName: "Sub Service",
      amount: "$29.99",
      updatePaymentUrl: "https://update.example.com",
      retryDate: "May 1, 2026",
    });
    expect(html).toContain("$29.99");
    expect(html).toContain("https://update.example.com");
  });

  it("invoice-sent renders with variables", () => {
    const html = renderTemplate("invoice-sent", {
      merchantName: "Invoice Corp",
      invoiceNumber: "INV-001",
      amount: "$500.00",
      dueDate: "May 15, 2026",
      payUrl: "https://pay.example.com/001",
    });
    expect(html).toContain("INV-001");
    expect(html).toContain("$500.00");
  });

  it("residual-statement renders with breakdown", () => {
    const html = renderTemplate("residual-statement", {
      agencyName: "Top Agency",
      period: "March 2026",
      totalEarned: "$1,000.00",
      merchantBreakdown: [
        { merchantName: "Store A", volume: "$100,000", share: "$1,000.00" },
      ],
      payoutDate: "April 15, 2026",
    });
    expect(html).toContain("$1,000.00");
    expect(html).toContain("Store A");
  });

  it("tier-upgrade renders with variables", () => {
    const html = renderTemplate("tier-upgrade", {
      agencyName: "Growth Agency",
      oldTier: "Tier 1",
      newTier: "Tier 2",
      newBpsRate: "12",
    });
    expect(html).toContain("Tier 2");
    expect(html).toContain("12 bps");
  });
});

// ─── Acceptance Test 3: Emails queued via pg-boss ─────────────────────

describe("Acceptance: Emails queued via pg-boss, never sync", () => {
  it("enqueueEmail creates NotificationSchedule and sends to pg-boss", async () => {
    const boss = createMockBoss();
    const prisma = createMockPrisma();

    const notifId = await enqueueEmail(boss as any, prisma as any, {
      to: "customer@example.com",
      templateId: "transaction-receipt",
      variables: { merchantName: "Test", amount: "$50" },
      merchantId: "merch-acc-1",
    });

    expect(notifId).toBe("notif-acceptance-1");
    expect(prisma.notificationSchedule.create).toHaveBeenCalledOnce();
    expect(boss.send).toHaveBeenCalledWith(
      EMAIL_SEND_JOB,
      expect.objectContaining({
        to: "customer@example.com",
        templateId: "transaction-receipt",
        notificationId: "notif-acceptance-1",
      }),
      expect.objectContaining({
        retryLimit: 3,
        retryBackoff: true,
      })
    );
  });
});

// ─── Acceptance Test 4: Email failures don't break payments ───────────

describe("Acceptance: Email failures don't break payment flows", () => {
  it("sendEmail never throws on network failure", async () => {
    mockFetch.mockRejectedValueOnce(new Error("Connection refused"));

    const service = createEmailService({
      apiKey: "test-key",
      baseUrl: "https://api.postmarkapp.test",
    });

    // This should NOT throw
    const result = await service.sendEmail({
      to: "user@example.com",
      templateId: "merchant-welcome",
      variables: { merchantName: "Test" },
    });

    expect(result.success).toBe(false);
    expect(result.error).toContain("Connection refused");
  });

  it("enqueueEmail returns null on failure instead of throwing", async () => {
    const boss = createMockBoss();
    const prisma = createMockPrisma();
    prisma.notificationSchedule.create.mockRejectedValueOnce(
      new Error("DB error")
    );

    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    const result = await enqueueEmail(boss as any, prisma as any, {
      to: "user@example.com",
      templateId: "merchant-welcome",
      variables: {},
    });

    expect(result).toBeNull(); // does not throw
    consoleSpy.mockRestore();
  });
});

// ─── Acceptance Test 5: NotificationSchedule tracks delivery ──────────

describe("Acceptance: NotificationSchedule records track delivery", () => {
  it("creates record with QUEUED status on enqueue", async () => {
    const boss = createMockBoss();
    const prisma = createMockPrisma();

    await enqueueEmail(boss as any, prisma as any, {
      to: "test@example.com",
      templateId: "chargeback-alert",
      variables: { merchantName: "Test" },
      merchantId: "merch-1",
    });

    const createCall = prisma.notificationSchedule.create.mock.calls[0][0];
    expect(createCall.data.status).toBe("QUEUED");
    expect(createCall.data.to).toBe("test@example.com");
    expect(createCall.data.templateId).toBe("chargeback-alert");
    expect(createCall.data.merchantId).toBe("merch-1");
  });
});

// ─── Acceptance Test 6-8: Integration points ─────────────────────────

describe("Acceptance: Integration points", () => {
  it("transaction-receipt can be queued for customers on payment", async () => {
    const boss = createMockBoss();
    const prisma = createMockPrisma();

    const notifId = await enqueueEmail(boss as any, prisma as any, {
      to: "customer@example.com",
      templateId: "transaction-receipt",
      variables: {
        merchantName: "Shop",
        amount: "$100.00",
        last4: "4242",
        transactionId: "txn_123",
        date: "2026-04-28",
      },
      merchantId: "merch-1",
    });

    expect(notifId).not.toBeNull();
    expect(boss.send).toHaveBeenCalledWith(
      "email-send",
      expect.objectContaining({ templateId: "transaction-receipt" }),
      expect.any(Object)
    );
  });

  it("chargeback-alert can be queued for merchants on chargeback", async () => {
    const boss = createMockBoss();
    const prisma = createMockPrisma();

    const notifId = await enqueueEmail(boss as any, prisma as any, {
      to: "merchant@example.com",
      templateId: "chargeback-alert",
      variables: {
        merchantName: "My Store",
        amount: "$200.00",
        transactionId: "txn_456",
        reason: "Customer dispute",
      },
      merchantId: "merch-2",
    });

    expect(notifId).not.toBeNull();
  });

  it("residual-statement can be queued for agencies monthly", async () => {
    const boss = createMockBoss();
    const prisma = createMockPrisma();

    const notifId = await enqueueEmail(boss as any, prisma as any, {
      to: "agency@example.com",
      templateId: "residual-statement",
      variables: {
        agencyName: "Top Agency",
        period: "March 2026",
        totalEarned: "$5,000.00",
        merchantBreakdown: [],
        payoutDate: "April 15, 2026",
      },
      agencyId: "agency-1",
    });

    expect(notifId).not.toBeNull();
  });
});
