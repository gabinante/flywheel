/**
 * Revenue Analytics Route — Unit Tests
 *
 * Tests the GET /analytics/revenue endpoint.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import express from "express";
import { createAdminRouter } from "../routes/admin.js";

// Lightweight test helper using fetch
async function request(
  app: express.Express,
  method: string,
  path: string,
  body?: unknown
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

      fetch(url, {
        method,
        headers: { "Content-Type": "application/json" },
        body: body ? JSON.stringify(body) : undefined,
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

describe("Admin Routes — GET /analytics/revenue", () => {
  let app: express.Express;
  let mockPrisma: Record<string, unknown>;

  beforeEach(() => {
    mockPrisma = {
      merchant: {
        findMany: vi.fn().mockResolvedValue([]),
        count: vi.fn().mockResolvedValue(42),
      },
      agency: {
        count: vi.fn().mockResolvedValue(5),
      },
      residualEntry: {
        create: vi.fn(),
        aggregate: vi.fn().mockResolvedValue({
          _sum: { nmiResidualEarned: 45000 },
        }),
      },
      residualPayout: {
        aggregate: vi.fn().mockResolvedValue({
          _sum: { totalAmount: 28000 },
        }),
      },
      dailyMetrics: {
        aggregate: vi.fn().mockResolvedValue({
          _sum: { totalVolume: 15000000, totalTransactions: 10000, totalChargebacks: 40 },
        }),
      },
      $queryRaw: vi.fn().mockResolvedValue([{ total: BigInt(0) }]),
      $executeRaw: vi.fn().mockResolvedValue(1),
    };

    app = express();
    app.use(express.json());

    const router = createAdminRouter({
      prisma: mockPrisma as unknown as import("@prisma/client").PrismaClient,
    });

    app.use("/api/v1/admin", router);
  });

  it("returns 200 with revenue data for trailing_30d", async () => {
    const res = await request(
      app,
      "GET",
      "/api/v1/admin/analytics/revenue?period=trailing_30d"
    );

    expect(res.status).toBe(200);
    expect(res.body).toHaveProperty("totalPlatformVolume");
    expect(res.body).toHaveProperty("nmiResidualReceived");
    expect(res.body).toHaveProperty("totalAgencyPayouts");
    expect(res.body).toHaveProperty("netRetainedRevenue");
    expect(res.body).toHaveProperty("activeMerchants");
    expect(res.body).toHaveProperty("activeAgencies");
    expect(res.body).toHaveProperty("chargebackRatio");
    expect(res.body).toHaveProperty("monthlyBreakdown");
  });

  it("returns 200 with revenue data for trailing_12m", async () => {
    const res = await request(
      app,
      "GET",
      "/api/v1/admin/analytics/revenue?period=trailing_12m"
    );

    expect(res.status).toBe(200);
    expect(res.body.totalPlatformVolume).toBe(15000000);
    expect(res.body.nmiResidualReceived).toBe(45000);
    expect(res.body.totalAgencyPayouts).toBe(28000);
    expect(res.body.netRetainedRevenue).toBe(17000);
  });

  it("returns 200 with custom date range", async () => {
    const res = await request(
      app,
      "GET",
      "/api/v1/admin/analytics/revenue?period=custom&startDate=2026-01-01T00:00:00.000Z&endDate=2026-03-31T23:59:59.999Z"
    );

    expect(res.status).toBe(200);
    expect(res.body).toHaveProperty("monthlyBreakdown");
    expect(Array.isArray(res.body.monthlyBreakdown)).toBe(true);
  });

  it("returns 400 for missing period parameter", async () => {
    const res = await request(
      app,
      "GET",
      "/api/v1/admin/analytics/revenue"
    );

    expect(res.status).toBe(400);
    expect(res.body.error).toBe("Invalid query parameters");
  });

  it("returns 400 for invalid period value", async () => {
    const res = await request(
      app,
      "GET",
      "/api/v1/admin/analytics/revenue?period=invalid"
    );

    expect(res.status).toBe(400);
  });

  it("returns 400 for custom period without dates", async () => {
    const res = await request(
      app,
      "GET",
      "/api/v1/admin/analytics/revenue?period=custom"
    );

    expect(res.status).toBe(400);
  });

  it("returns correct active counts", async () => {
    const res = await request(
      app,
      "GET",
      "/api/v1/admin/analytics/revenue?period=trailing_30d"
    );

    expect(res.status).toBe(200);
    expect(res.body.activeMerchants).toBe(42);
    expect(res.body.activeAgencies).toBe(5);
  });

  it("calculates chargeback ratio correctly", async () => {
    const res = await request(
      app,
      "GET",
      "/api/v1/admin/analytics/revenue?period=trailing_30d"
    );

    expect(res.status).toBe(200);
    // 40 / 10000 = 0.004
    expect(res.body.chargebackRatio).toBe(0.004);
  });

  it("includes monthly breakdown in response", async () => {
    const res = await request(
      app,
      "GET",
      "/api/v1/admin/analytics/revenue?period=trailing_30d"
    );

    expect(res.status).toBe(200);
    const breakdown = res.body.monthlyBreakdown as Array<Record<string, unknown>>;
    expect(Array.isArray(breakdown)).toBe(true);
    // Each entry has the required fields
    if (breakdown.length > 0) {
      expect(breakdown[0]).toHaveProperty("month");
      expect(breakdown[0]).toHaveProperty("volume");
      expect(breakdown[0]).toHaveProperty("nmiResidual");
      expect(breakdown[0]).toHaveProperty("agencyPayout");
      expect(breakdown[0]).toHaveProperty("retained");
    }
  });
});
