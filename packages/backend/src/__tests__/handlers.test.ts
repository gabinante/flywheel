/**
 * Job Handlers — Unit Tests
 */

import { describe, it, expect } from "vitest";
import { getPreviousMonthRange, RESIDUAL_CALCULATION_JOB } from "../jobs/handlers.js";

describe("Job Handlers", () => {
  describe("RESIDUAL_CALCULATION_JOB constant", () => {
    it("has the expected job name", () => {
      expect(RESIDUAL_CALCULATION_JOB).toBe("residual-calculation");
    });
  });

  describe("getPreviousMonthRange", () => {
    it("returns a valid date range", () => {
      const { periodStart, periodEnd } = getPreviousMonthRange();

      expect(periodStart).toBeInstanceOf(Date);
      expect(periodEnd).toBeInstanceOf(Date);
      expect(periodStart.getTime()).toBeLessThan(periodEnd.getTime());
    });

    it("periodStart is the 1st of previous month at 00:00:00 UTC", () => {
      const { periodStart } = getPreviousMonthRange();

      expect(periodStart.getUTCDate()).toBe(1);
      expect(periodStart.getUTCHours()).toBe(0);
      expect(periodStart.getUTCMinutes()).toBe(0);
      expect(periodStart.getUTCSeconds()).toBe(0);
      expect(periodStart.getUTCMilliseconds()).toBe(0);
    });

    it("periodEnd is the 1st of current month at 00:00:00 UTC", () => {
      const { periodEnd } = getPreviousMonthRange();
      const now = new Date();

      expect(periodEnd.getUTCDate()).toBe(1);
      expect(periodEnd.getUTCHours()).toBe(0);
      expect(periodEnd.getUTCMinutes()).toBe(0);
      expect(periodEnd.getUTCMonth()).toBe(now.getUTCMonth());
      expect(periodEnd.getUTCFullYear()).toBe(now.getUTCFullYear());
    });

    it("periodStart month is one month before periodEnd month", () => {
      const { periodStart, periodEnd } = getPreviousMonthRange();

      // Handle year boundary (January → December of previous year)
      const expectedMonth =
        periodEnd.getUTCMonth() === 0 ? 11 : periodEnd.getUTCMonth() - 1;
      const expectedYear =
        periodEnd.getUTCMonth() === 0
          ? periodEnd.getUTCFullYear() - 1
          : periodEnd.getUTCFullYear();

      expect(periodStart.getUTCMonth()).toBe(expectedMonth);
      expect(periodStart.getUTCFullYear()).toBe(expectedYear);
    });
  });
});
