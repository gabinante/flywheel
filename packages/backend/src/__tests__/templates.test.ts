/**
 * Email Template Tests
 *
 * Validates that all 7 email templates render correctly with their
 * expected variables, produce valid HTML, and the registry works.
 */

import { describe, it, expect } from "vitest";
import {
  renderTemplate,
  isValidTemplate,
  TEMPLATE_IDS,
} from "../templates/index.js";

// ─── Registry Tests ───────────────────────────────────────────────────

describe("Template Registry", () => {
  it("has all 7 expected templates registered", () => {
    expect(TEMPLATE_IDS).toHaveLength(7);
    expect(TEMPLATE_IDS).toContain("merchant-welcome");
    expect(TEMPLATE_IDS).toContain("transaction-receipt");
    expect(TEMPLATE_IDS).toContain("chargeback-alert");
    expect(TEMPLATE_IDS).toContain("payment-failed-dunning");
    expect(TEMPLATE_IDS).toContain("invoice-sent");
    expect(TEMPLATE_IDS).toContain("residual-statement");
    expect(TEMPLATE_IDS).toContain("tier-upgrade");
  });

  it("isValidTemplate returns true for registered templates", () => {
    for (const id of TEMPLATE_IDS) {
      expect(isValidTemplate(id)).toBe(true);
    }
  });

  it("isValidTemplate returns false for unknown template", () => {
    expect(isValidTemplate("unknown-template")).toBe(false);
    expect(isValidTemplate("")).toBe(false);
  });

  it("renderTemplate throws for unknown template", () => {
    expect(() => renderTemplate("nonexistent", {})).toThrow(
      /Unknown email template.*nonexistent/
    );
  });
});

// ─── Individual Template Tests ────────────────────────────────────────

describe("merchant-welcome template", () => {
  it("renders with all variables", () => {
    const html = renderTemplate("merchant-welcome", {
      merchantName: "Acme Corp",
      dashboardUrl: "https://dashboard.example.com",
    });

    expect(html).toContain("<!DOCTYPE html>");
    expect(html).toContain("Acme Corp");
    expect(html).toContain("https://dashboard.example.com");
    expect(html).toContain("Welcome to GoHighPayment");
    expect(html).toContain("Go to Dashboard");
  });

  it("renders with default values when variables missing", () => {
    const html = renderTemplate("merchant-welcome", {});
    expect(html).toContain("Merchant");
    expect(html).toContain("<!DOCTYPE html>");
  });

  it("escapes HTML in merchant name", () => {
    const html = renderTemplate("merchant-welcome", {
      merchantName: '<script>alert("xss")</script>',
    });
    expect(html).not.toContain("<script>");
    expect(html).toContain("&lt;script&gt;");
  });
});

describe("transaction-receipt template", () => {
  it("renders with all variables", () => {
    const html = renderTemplate("transaction-receipt", {
      merchantName: "Coffee Shop",
      amount: "$25.50",
      last4: "4242",
      transactionId: "txn_abc123",
      date: "April 28, 2026",
    });

    expect(html).toContain("Coffee Shop");
    expect(html).toContain("$25.50");
    expect(html).toContain("4242");
    expect(html).toContain("txn_abc123");
    expect(html).toContain("April 28, 2026");
    expect(html).toContain("Payment Receipt");
  });
});

describe("chargeback-alert template", () => {
  it("renders with all variables", () => {
    const html = renderTemplate("chargeback-alert", {
      merchantName: "Widget Store",
      amount: "$150.00",
      transactionId: "txn_xyz789",
      reason: "Fraudulent charge",
    });

    expect(html).toContain("Widget Store");
    expect(html).toContain("$150.00");
    expect(html).toContain("txn_xyz789");
    expect(html).toContain("Fraudulent charge");
    expect(html).toContain("Chargeback Alert");
    expect(html).toContain("Action required");
  });
});

describe("payment-failed-dunning template", () => {
  it("renders with all variables", () => {
    const html = renderTemplate("payment-failed-dunning", {
      merchantName: "Gym Membership",
      amount: "$49.99",
      updatePaymentUrl: "https://pay.example.com/update",
      retryDate: "May 1, 2026",
    });

    expect(html).toContain("Gym Membership");
    expect(html).toContain("$49.99");
    expect(html).toContain("https://pay.example.com/update");
    expect(html).toContain("May 1, 2026");
    expect(html).toContain("Payment Failed");
    expect(html).toContain("Update Payment Method");
  });

  it("includes unsubscribe link", () => {
    const html = renderTemplate("payment-failed-dunning", {
      merchantName: "Test",
      amount: "$10",
      updatePaymentUrl: "#",
      retryDate: "soon",
    });
    expect(html).toContain("Unsubscribe");
  });
});

describe("invoice-sent template", () => {
  it("renders with all variables", () => {
    const html = renderTemplate("invoice-sent", {
      merchantName: "Design Agency",
      invoiceNumber: "INV-2026-001",
      amount: "$2,500.00",
      dueDate: "May 15, 2026",
      payUrl: "https://pay.example.com/inv/123",
    });

    expect(html).toContain("Design Agency");
    expect(html).toContain("INV-2026-001");
    expect(html).toContain("$2,500.00");
    expect(html).toContain("May 15, 2026");
    expect(html).toContain("https://pay.example.com/inv/123");
    expect(html).toContain("Pay Invoice");
  });
});

describe("residual-statement template", () => {
  it("renders with merchant breakdown", () => {
    const html = renderTemplate("residual-statement", {
      agencyName: "Top Agency",
      period: "March 2026",
      totalEarned: "$1,250.00",
      merchantBreakdown: [
        {
          merchantName: "Store A",
          volume: "$100,000",
          share: "$750.00",
        },
        {
          merchantName: "Store B",
          volume: "$50,000",
          share: "$500.00",
        },
      ],
      payoutDate: "April 15, 2026",
    });

    expect(html).toContain("Top Agency");
    expect(html).toContain("March 2026");
    expect(html).toContain("$1,250.00");
    expect(html).toContain("Store A");
    expect(html).toContain("$100,000");
    expect(html).toContain("$750.00");
    expect(html).toContain("Store B");
    expect(html).toContain("April 15, 2026");
    expect(html).toContain("Monthly Residual Statement");
  });

  it("renders without merchant breakdown", () => {
    const html = renderTemplate("residual-statement", {
      agencyName: "New Agency",
      period: "March 2026",
      totalEarned: "$0.00",
      merchantBreakdown: [],
      payoutDate: "N/A",
    });

    expect(html).toContain("No merchant activity this period");
  });

  it("includes unsubscribe link", () => {
    const html = renderTemplate("residual-statement", {
      agencyName: "Test",
      period: "March 2026",
      totalEarned: "$0",
      payoutDate: "N/A",
    });
    expect(html).toContain("Unsubscribe");
  });
});

describe("tier-upgrade template", () => {
  it("renders with all variables", () => {
    const html = renderTemplate("tier-upgrade", {
      agencyName: "Growth Agency",
      oldTier: "Tier 1",
      newTier: "Tier 2",
      newBpsRate: "12",
    });

    expect(html).toContain("Growth Agency");
    expect(html).toContain("Tier 1");
    expect(html).toContain("Tier 2");
    expect(html).toContain("12 bps");
    expect(html).toContain("Tier Upgrade");
    expect(html).toContain("upgraded");
  });
});

// ─── Cross-cutting Template Tests ─────────────────────────────────────

describe("All templates", () => {
  const templateVars: Record<string, Record<string, unknown>> = {
    "merchant-welcome": {
      merchantName: "Test",
      dashboardUrl: "https://example.com",
    },
    "transaction-receipt": {
      merchantName: "Test",
      amount: "$10.00",
      last4: "1234",
      transactionId: "txn_1",
      date: "2026-04-28",
    },
    "chargeback-alert": {
      merchantName: "Test",
      amount: "$10.00",
      transactionId: "txn_1",
      reason: "Fraud",
    },
    "payment-failed-dunning": {
      merchantName: "Test",
      amount: "$10.00",
      updatePaymentUrl: "https://example.com",
      retryDate: "2026-05-01",
    },
    "invoice-sent": {
      merchantName: "Test",
      invoiceNumber: "INV-1",
      amount: "$10.00",
      dueDate: "2026-05-15",
      payUrl: "https://example.com/pay",
    },
    "residual-statement": {
      agencyName: "Test",
      period: "March 2026",
      totalEarned: "$100.00",
      merchantBreakdown: [],
      payoutDate: "2026-04-15",
    },
    "tier-upgrade": {
      agencyName: "Test",
      oldTier: "Tier 1",
      newTier: "Tier 2",
      newBpsRate: "12",
    },
  };

  for (const templateId of TEMPLATE_IDS) {
    it(`${templateId}: renders valid HTML document`, () => {
      const html = renderTemplate(templateId, templateVars[templateId] ?? {});

      expect(html).toContain("<!DOCTYPE html>");
      expect(html).toContain("<html");
      expect(html).toContain("</html>");
      expect(html).toContain("GoHighPayment");
    });

    it(`${templateId}: renders without throwing on empty variables`, () => {
      expect(() => renderTemplate(templateId, {})).not.toThrow();
    });
  }
});
