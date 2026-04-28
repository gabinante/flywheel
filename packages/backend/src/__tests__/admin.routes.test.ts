/**
 * Admin Routes — Unit Tests
 *
 * Tests the POST /residuals/calculate endpoint.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import express from "express";
import { createAdminRouter } from "../routes/admin.js";

// Lightweight supertest-like helper using fetch
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

describe("Admin Routes — /residuals/calculate", () => {
  let app: express.Express;
  let mockPrisma: Record<string, unknown>;

  beforeEach(() => {
    mockPrisma = {
      merchant: {
        findMany: vi.fn().mockResolvedValue([]),
      },
      residualEntry: {
        create: vi.fn(),
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

  it("returns 200 with calculation results for valid date range", async () => {
    const res = await request(app, "POST", "/api/v1/admin/residuals/calculate", {
      periodStart: "2026-03-01T00:00:00.000Z",
      periodEnd: "2026-04-01T00:00:00.000Z",
    });

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
    expect(res.body.entriesCreated).toBe(0); // no merchants seeded
    expect(res.body.merchantsProcessed).toBe(0);
  });

  it("returns 400 for missing periodStart", async () => {
    const res = await request(app, "POST", "/api/v1/admin/residuals/calculate", {
      periodEnd: "2026-04-01T00:00:00.000Z",
    });

    expect(res.status).toBe(400);
    expect(res.body.error).toBe("Invalid request body");
  });

  it("returns 400 for non-ISO date strings", async () => {
    const res = await request(app, "POST", "/api/v1/admin/residuals/calculate", {
      periodStart: "March 2026",
      periodEnd: "April 2026",
    });

    expect(res.status).toBe(400);
  });

  it("returns 400 when periodEnd <= periodStart", async () => {
    const res = await request(app, "POST", "/api/v1/admin/residuals/calculate", {
      periodStart: "2026-04-01T00:00:00.000Z",
      periodEnd: "2026-03-01T00:00:00.000Z",
    });

    expect(res.status).toBe(400);
    expect(res.body.error).toBe("periodEnd must be after periodStart");
  });
});
