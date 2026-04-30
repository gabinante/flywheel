/**
 * Chargeback Monitor — Unit Tests
 *
 * Tests the pure threshold logic in chargeback-thresholds.ts.
 *
 * Acceptance test scenario:
 *   Merchant A: 1000 transactions, 6 chargebacks → 0.6% → WARNING
 *   Merchant B: 500 transactions, 8 chargebacks  → 1.6% → HIGH
 *
 * Verifies:
 *   1. Merchant A gets chargebackRiskLevel=WARNING
 *   2. Merchant B gets chargebackRiskLevel=HIGH
 *   3. B has higher ratio than A (would rank first in risk endpoint)
 *   4. Color logic: green <0.5%, yellow 0.5–0.7%, orange 0.7–1.0%, red >1.0%
 */

import { describe, it, expect } from "vitest";
import {
  getRiskLevel,
  computeChargebackRatio,
  CB_WARNING_THRESHOLD,
  CB_CRITICAL_THRESHOLD,
  CB_HIGH_THRESHOLD,
  CHARGEBACK_MONITOR_JOB,
} from "../jobs/handlers.js";

// ─── Threshold constants ─────────────────────────────────────────────

describe("Chargeback threshold constants", () => {
  it("WARNING threshold is 0.5%", () => {
    expect(CB_WARNING_THRESHOLD).toBe(0.005);
  });

  it("CRITICAL threshold is 0.7%", () => {
    expect(CB_CRITICAL_THRESHOLD).toBe(0.007);
  });

  it("HIGH threshold is 1.0%", () => {
    expect(CB_HIGH_THRESHOLD).toBe(0.010);
  });

  it("CHARGEBACK_MONITOR_JOB constant is correct", () => {
    expect(CHARGEBACK_MONITOR_JOB).toBe("chargeback-monitor");
  });
});

// ─── computeChargebackRatio ──────────────────────────────────────────

describe("computeChargebackRatio", () => {
  it("returns 0 when transactionCount is 0 (no div-by-zero)", () => {
    expect(computeChargebackRatio(0, 0)).toBe(0);
    expect(computeChargebackRatio(0, 5)).toBe(0);
  });

  it("computes ratio correctly: 6/1000 = 0.006", () => {
    expect(computeChargebackRatio(1000, 6)).toBeCloseTo(0.006, 10);
  });

  it("computes ratio correctly: 8/500 = 0.016", () => {
    expect(computeChargebackRatio(500, 8)).toBeCloseTo(0.016, 10);
  });

  it("computes ratio correctly: 0/1000 = 0", () => {
    expect(computeChargebackRatio(1000, 0)).toBe(0);
  });
});

// ─── getRiskLevel — threshold assignments ───────────────────────────

describe("getRiskLevel — threshold assignments", () => {
  it("returns null for 0% ratio (no transactions)", () => {
    expect(getRiskLevel(0)).toBeNull();
  });

  it("returns null for ratio below WARNING threshold (0.4%)", () => {
    expect(getRiskLevel(computeChargebackRatio(1000, 4))).toBeNull();
  });

  it("returns null for ratio exactly at 0.5% (not strictly greater)", () => {
    const ratio = computeChargebackRatio(1000, 5); // exactly 0.5%
    expect(ratio).toBeCloseTo(CB_WARNING_THRESHOLD, 10);
    expect(getRiskLevel(ratio)).toBeNull();
  });

  it("returns WARNING for 0.6% ratio (6/1000) — Merchant A acceptance case", () => {
    const ratio = computeChargebackRatio(1000, 6);
    expect(ratio).toBeCloseTo(0.006, 5);
    expect(getRiskLevel(ratio)).toBe("WARNING");
  });

  it("returns WARNING for ratio exactly at 0.7% (not strictly greater)", () => {
    const ratio = computeChargebackRatio(1000, 7); // exactly 0.7%
    expect(ratio).toBeCloseTo(CB_CRITICAL_THRESHOLD, 10);
    expect(getRiskLevel(ratio)).toBe("WARNING");
  });

  it("returns CRITICAL for 0.8% ratio (8/1000)", () => {
    const ratio = computeChargebackRatio(1000, 8);
    expect(ratio).toBeCloseTo(0.008, 5);
    expect(getRiskLevel(ratio)).toBe("CRITICAL");
  });

  it("returns CRITICAL for ratio exactly at 1.0% (not strictly greater)", () => {
    const ratio = computeChargebackRatio(1000, 10); // exactly 1.0%
    expect(ratio).toBeCloseTo(CB_HIGH_THRESHOLD, 10);
    expect(getRiskLevel(ratio)).toBe("CRITICAL");
  });

  it("returns HIGH for 1.1% ratio (11/1000)", () => {
    const ratio = computeChargebackRatio(1000, 11);
    expect(getRiskLevel(ratio)).toBe("HIGH");
  });

  it("returns HIGH for 1.6% ratio (8/500) — Merchant B acceptance case", () => {
    const ratio = computeChargebackRatio(500, 8);
    expect(ratio).toBeCloseTo(0.016, 5);
    expect(getRiskLevel(ratio)).toBe("HIGH");
  });
});

// ─── Acceptance test scenario ────────────────────────────────────────

describe("Acceptance test: Merchant A (0.6%) and Merchant B (1.6%)", () => {
  it("Merchant A (1000 tx, 6 cb) gets WARNING", () => {
    const ratio = computeChargebackRatio(1000, 6);
    expect(getRiskLevel(ratio)).toBe("WARNING");
  });

  it("Merchant B (500 tx, 8 cb) gets HIGH", () => {
    const ratio = computeChargebackRatio(500, 8);
    expect(getRiskLevel(ratio)).toBe("HIGH");
  });

  it("Merchant B ratio is greater than Merchant A ratio (B ranks first)", () => {
    const ratioA = computeChargebackRatio(1000, 6); // 0.006
    const ratioB = computeChargebackRatio(500, 8);  // 0.016
    expect(ratioB).toBeGreaterThan(ratioA);
  });
});

// ─── Color coding thresholds ─────────────────────────────────────────

describe("Color coding thresholds", () => {
  /**
   * Maps risk level to display color.
   * Mirrors the logic in ChargebacksPage.tsx and MerchantDetailPage.tsx.
   */
  function colorOf(ratio: number): string {
    const level = getRiskLevel(ratio);
    if (level === "HIGH") return "red";
    if (level === "CRITICAL") return "orange";
    if (level === "WARNING") return "yellow";
    return "green";
  }

  it("< 0.5% → green", () => {
    expect(colorOf(computeChargebackRatio(1000, 4))).toBe("green");
  });

  it("0.5–0.7% → yellow (WARNING)", () => {
    expect(colorOf(computeChargebackRatio(1000, 6))).toBe("yellow");
  });

  it("0.7–1.0% → orange (CRITICAL)", () => {
    expect(colorOf(computeChargebackRatio(1000, 8))).toBe("orange");
  });

  it("> 1.0% → red (HIGH)", () => {
    expect(colorOf(computeChargebackRatio(500, 8))).toBe("red");
  });
});

// ─── Ranking logic ────────────────────────────────────────────────────

describe("Risk endpoint ranking (merchants sorted by ratio descending)", () => {
  it("higher ratio merchant ranks first", () => {
    const merchantA = { ratio: computeChargebackRatio(1000, 6) }; // 0.006
    const merchantB = { ratio: computeChargebackRatio(500, 8) };  // 0.016

    const ranked = [merchantA, merchantB].sort((a, b) => b.ratio - a.ratio);
    expect(ranked[0]).toBe(merchantB);
    expect(ranked[1]).toBe(merchantA);
  });
});
