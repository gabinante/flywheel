/**
 * Format utilities — Unit Tests
 */

import { describe, it, expect } from "vitest";
import {
  formatCents,
  formatCentsCompact,
  formatPercent,
  calcMoMDelta,
  formatDelta,
} from "./format.js";

describe("formatCents", () => {
  it("formats 0 cents", () => {
    expect(formatCents(0)).toBe("$0");
  });

  it("formats 15000000 cents as $150,000", () => {
    expect(formatCents(15000000)).toBe("$150,000");
  });

  it("formats 45000 cents as $450", () => {
    expect(formatCents(45000)).toBe("$450");
  });
});

describe("formatCentsCompact", () => {
  it("formats large values compactly", () => {
    const result = formatCentsCompact(1500000000);
    // Should be something like "$15M"
    expect(result).toContain("$");
    expect(result).toContain("M");
  });
});

describe("formatPercent", () => {
  it("formats 0.004 as 0.40%", () => {
    expect(formatPercent(0.004)).toBe("0.40%");
  });

  it("formats 0 as 0.00%", () => {
    expect(formatPercent(0)).toBe("0.00%");
  });
});

describe("calcMoMDelta", () => {
  it("returns positive delta for growth", () => {
    expect(calcMoMDelta(120, 100)).toBe(20);
  });

  it("returns negative delta for decline", () => {
    expect(calcMoMDelta(80, 100)).toBe(-20);
  });

  it("returns null when previous is 0", () => {
    expect(calcMoMDelta(100, 0)).toBeNull();
  });

  it("returns null when previous is undefined", () => {
    expect(calcMoMDelta(100, undefined)).toBeNull();
  });
});

describe("formatDelta", () => {
  it("formats positive delta", () => {
    expect(formatDelta(12.5)).toBe("+12.5%");
  });

  it("formats negative delta", () => {
    expect(formatDelta(-3.2)).toBe("-3.2%");
  });

  it("formats null as N/A", () => {
    expect(formatDelta(null)).toBe("N/A");
  });
});
