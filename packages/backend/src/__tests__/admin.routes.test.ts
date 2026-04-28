/**
 * Admin Routes — Unit Tests
 *
 * Tests the residual approval workflow endpoints:
 *   POST /residuals/calculate
 *   GET  /residuals
 *   GET  /residuals/summary
 *   POST /residuals/approve
 *   POST /residuals/hold
 *   POST /residuals/create-payout
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import express from "express";
import { createAdminRouter } from "../routes/admin.js";

// ─── Test helpers ────────────────────────────────────────────────────

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
        headers: {
          "Content-Type": "application/json",
          "x-admin-user-id": "admin-test-user",
        },
        body: body ? JSON.stringify(body) : undefined,
      })
        .then(async (res) => {
          const text = await res.text();
          let json: Record<string, unknown> = {};
          try {
            json = JSON.parse(text) as Record<string, unknown>;
          } catch {
            // non-JSON response — return empty object with status
          }
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

// ─── Mock data ───────────────────────────────────────────────────────

function makeMockEntry(overrides: Record<string, unknown> = {}) {
  return {
    id: "entry-1",
    agencyId: "agency-1",
    merchantId: "merchant-1",
    periodStart: new Date("2026-03-01"),
    periodEnd: new Date("2026-04-01"),
    merchantVolume: 10000000,
    nmiResidualEarned: 30000,
    agencyBps: 10,
    agencyShare: 1000,
    twoTierAgencyId: null,
    twoTierShare: 0,
    status: "PENDING",
    approvedAt: null,
    approvedBy: null,
    createdAt: new Date(),
    updatedAt: new Date(),
    agency: { id: "agency-1", name: "Agency Alpha", tier: "TIER_1" },
    merchant: { id: "merchant-1", name: "Merchant One" },
    ...overrides,
  };
}

// ─── Tests ───────────────────────────────────────────────────────────

describe("Admin Routes — /residuals/calculate", () => {
  let app: express.Express;
  let mockPrisma: Record<string, unknown>;

  beforeEach(() => {
    mockPrisma = {
      merchant: { findMany: vi.fn().mockResolvedValue([]) },
      residualEntry: { create: vi.fn() },
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
    expect(res.body.entriesCreated).toBe(0);
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

describe("Admin Routes — GET /residuals", () => {
  let app: express.Express;
  let mockPrisma: Record<string, unknown>;

  beforeEach(() => {
    mockPrisma = {
      merchant: { findMany: vi.fn().mockResolvedValue([]) },
      residualEntry: {
        create: vi.fn(),
        findMany: vi.fn().mockResolvedValue([]),
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

  it("returns 400 when periodStart is missing", async () => {
    const res = await request(app, "GET", "/api/v1/admin/residuals");
    expect(res.status).toBe(400);
    expect(res.body.error).toContain("periodStart");
  });

  it("returns entries grouped by agency", async () => {
    const entries = [
      makeMockEntry({ id: "e1", agencyId: "a1", merchantId: "m1" }),
      makeMockEntry({
        id: "e2",
        agencyId: "a1",
        merchantId: "m2",
        merchant: { id: "m2", name: "Merchant Two" },
      }),
      makeMockEntry({
        id: "e3",
        agencyId: "a2",
        merchantId: "m3",
        agency: { id: "a2", name: "Agency Beta", tier: "TIER_2" },
        merchant: { id: "m3", name: "Merchant Three" },
      }),
    ];

    (mockPrisma.residualEntry as Record<string, unknown>).findMany = vi
      .fn()
      .mockResolvedValue(entries);

    const res = await request(
      app,
      "GET",
      "/api/v1/admin/residuals?periodStart=2026-03-01"
    );

    expect(res.status).toBe(200);
    const agencies = res.body.agencies as Array<Record<string, unknown>>;
    expect(agencies).toHaveLength(2);

    const a1 = agencies.find((a) => a.agencyId === "a1");
    expect(a1).toBeDefined();
    expect((a1!.entries as Array<unknown>).length).toBe(2);

    const a2 = agencies.find((a) => a.agencyId === "a2");
    expect(a2).toBeDefined();
    expect((a2!.entries as Array<unknown>).length).toBe(1);
  });

  it("passes status filter to query", async () => {
    (mockPrisma.residualEntry as Record<string, unknown>).findMany = vi
      .fn()
      .mockResolvedValue([]);

    const res = await request(
      app,
      "GET",
      "/api/v1/admin/residuals?periodStart=2026-03-01&status=APPROVED"
    );

    expect(res.status).toBe(200);
    const findManyMock = (mockPrisma.residualEntry as Record<string, ReturnType<typeof vi.fn>>).findMany;
    expect(findManyMock).toHaveBeenCalledWith(
      expect.objectContaining({
        where: expect.objectContaining({ status: "APPROVED" }),
      })
    );
  });

  it("rejects invalid status", async () => {
    const res = await request(
      app,
      "GET",
      "/api/v1/admin/residuals?periodStart=2026-03-01&status=INVALID"
    );
    expect(res.status).toBe(400);
    expect(res.body.error).toContain("Invalid status");
  });

  it("passes agencyId filter to query", async () => {
    (mockPrisma.residualEntry as Record<string, unknown>).findMany = vi
      .fn()
      .mockResolvedValue([]);

    const res = await request(
      app,
      "GET",
      "/api/v1/admin/residuals?periodStart=2026-03-01&agencyId=agency-xyz"
    );

    expect(res.status).toBe(200);
    const findManyMock = (mockPrisma.residualEntry as Record<string, ReturnType<typeof vi.fn>>).findMany;
    expect(findManyMock).toHaveBeenCalledWith(
      expect.objectContaining({
        where: expect.objectContaining({ agencyId: "agency-xyz" }),
      })
    );
  });
});

describe("Admin Routes — GET /residuals/summary", () => {
  let app: express.Express;
  let mockPrisma: Record<string, unknown>;

  beforeEach(() => {
    mockPrisma = {
      merchant: { findMany: vi.fn().mockResolvedValue([]) },
      residualEntry: {
        create: vi.fn(),
        findMany: vi.fn().mockResolvedValue([]),
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

  it("returns 400 when periodStart is missing", async () => {
    const res = await request(app, "GET", "/api/v1/admin/residuals/summary");
    expect(res.status).toBe(400);
  });

  it("returns correct summary totals", async () => {
    const entries = [
      {
        id: "e1",
        agencyShare: 1000,
        twoTierShare: 0,
        status: "PENDING",
        agency: { tier: "TIER_1" },
      },
      {
        id: "e2",
        agencyShare: 1200,
        twoTierShare: 150,
        status: "APPROVED",
        agency: { tier: "TIER_2" },
      },
      {
        id: "e3",
        agencyShare: 500,
        twoTierShare: 0,
        status: "HELD",
        agency: { tier: "TIER_1" },
      },
      {
        id: "e4",
        agencyShare: 800,
        twoTierShare: 100,
        status: "PAID",
        agency: { tier: "TIER_3" },
      },
    ];

    (mockPrisma.residualEntry as Record<string, unknown>).findMany = vi
      .fn()
      .mockResolvedValue(entries);

    const res = await request(
      app,
      "GET",
      "/api/v1/admin/residuals/summary?periodStart=2026-03-01"
    );

    expect(res.status).toBe(200);
    expect(res.body.totalEntries).toBe(4);
    expect(res.body.totalAgencyShare).toBe(3500); // 1000 + 1200 + 500 + 800
    expect(res.body.totalTwoTierShare).toBe(250); // 0 + 150 + 0 + 100
    expect(res.body.totalOwed).toBe(3750); // 3500 + 250

    const byStatus = res.body.byStatus as Record<string, number>;
    expect(byStatus.PENDING).toBe(1);
    expect(byStatus.APPROVED).toBe(1);
    expect(byStatus.HELD).toBe(1);
    expect(byStatus.PAID).toBe(1);

    const byTier = res.body.byTier as Record<string, Record<string, number>>;
    expect(byTier.TIER_1.count).toBe(2);
    expect(byTier.TIER_1.agencyShare).toBe(1500); // 1000 + 500
    expect(byTier.TIER_2.count).toBe(1);
    expect(byTier.TIER_3.count).toBe(1);
  });
});

describe("Admin Routes — POST /residuals/approve", () => {
  let app: express.Express;
  let mockPrisma: Record<string, unknown>;

  beforeEach(() => {
    mockPrisma = {
      merchant: { findMany: vi.fn().mockResolvedValue([]) },
      residualEntry: {
        create: vi.fn(),
        findMany: vi.fn().mockResolvedValue([]),
        updateMany: vi.fn().mockResolvedValue({ count: 3 }),
      },
      auditLog: {
        create: vi.fn().mockResolvedValue({}),
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

  it("approves entries and returns count", async () => {
    const res = await request(app, "POST", "/api/v1/admin/residuals/approve", {
      entryIds: ["e1", "e2", "e3"],
    });

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
    expect(res.body.countApproved).toBe(3);

    // Verify updateMany was called with correct args
    const updateManyMock = (mockPrisma.residualEntry as Record<string, ReturnType<typeof vi.fn>>).updateMany;
    expect(updateManyMock).toHaveBeenCalledWith(
      expect.objectContaining({
        where: {
          id: { in: ["e1", "e2", "e3"] },
          status: "PENDING",
        },
        data: expect.objectContaining({
          status: "APPROVED",
          approvedBy: "admin-test-user",
        }),
      })
    );
  });

  it("creates audit log entries for each approved entry", async () => {
    await request(app, "POST", "/api/v1/admin/residuals/approve", {
      entryIds: ["e1", "e2"],
    });

    const createMock = (mockPrisma.auditLog as Record<string, ReturnType<typeof vi.fn>>).create;
    expect(createMock).toHaveBeenCalledTimes(2);
    expect(createMock).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          action: "RESIDUAL_APPROVED",
          performedBy: "admin-test-user",
        }),
      })
    );
  });

  it("returns 400 for empty entryIds", async () => {
    const res = await request(app, "POST", "/api/v1/admin/residuals/approve", {
      entryIds: [],
    });
    expect(res.status).toBe(400);
  });

  it("returns 400 for missing entryIds", async () => {
    const res = await request(app, "POST", "/api/v1/admin/residuals/approve", {});
    expect(res.status).toBe(400);
  });
});

describe("Admin Routes — POST /residuals/hold", () => {
  let app: express.Express;
  let mockPrisma: Record<string, unknown>;

  beforeEach(() => {
    mockPrisma = {
      merchant: { findMany: vi.fn().mockResolvedValue([]) },
      residualEntry: {
        create: vi.fn(),
        findMany: vi.fn().mockResolvedValue([]),
        updateMany: vi.fn().mockResolvedValue({ count: 1 }),
      },
      auditLog: {
        create: vi.fn().mockResolvedValue({}),
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

  it("holds entries with reason and returns count", async () => {
    const res = await request(app, "POST", "/api/v1/admin/residuals/hold", {
      entryIds: ["e1"],
      reason: "chargeback review",
    });

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
    expect(res.body.countHeld).toBe(1);
  });

  it("records hold reason in audit log", async () => {
    await request(app, "POST", "/api/v1/admin/residuals/hold", {
      entryIds: ["e1"],
      reason: "chargeback review",
    });

    const createMock = (mockPrisma.auditLog as Record<string, ReturnType<typeof vi.fn>>).create;
    expect(createMock).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          action: "RESIDUAL_HELD",
          performedBy: "admin-test-user",
          details: expect.objectContaining({
            entryId: "e1",
            reason: "chargeback review",
          }),
        }),
      })
    );
  });

  it("returns 400 when reason is missing", async () => {
    const res = await request(app, "POST", "/api/v1/admin/residuals/hold", {
      entryIds: ["e1"],
    });
    expect(res.status).toBe(400);
  });

  it("returns 400 when reason is empty", async () => {
    const res = await request(app, "POST", "/api/v1/admin/residuals/hold", {
      entryIds: ["e1"],
      reason: "",
    });
    expect(res.status).toBe(400);
  });
});

describe("Admin Routes — POST /residuals/create-payout", () => {
  let app: express.Express;
  let mockPrisma: Record<string, unknown>;
  let mockTx: Record<string, unknown>;

  beforeEach(() => {
    mockTx = {
      residualPayout: {
        create: vi.fn().mockResolvedValue({
          id: "payout-1",
          agencyId: "agency-1",
          periodStart: new Date("2026-03-01"),
          totalAmount: 2350,
          directShare: 2200,
          twoTierShare: 150,
          method: "ach",
          reference: "REF-001",
          status: "PAID",
          paidAt: new Date(),
        }),
      },
      residualEntry: {
        updateMany: vi.fn().mockResolvedValue({ count: 2 }),
      },
      auditLog: {
        create: vi.fn().mockResolvedValue({}),
      },
    };

    mockPrisma = {
      merchant: { findMany: vi.fn().mockResolvedValue([]) },
      residualEntry: {
        create: vi.fn(),
        findMany: vi.fn().mockResolvedValue([
          {
            id: "e1",
            agencyId: "agency-1",
            agencyShare: 1000,
            twoTierShare: 150,
            status: "APPROVED",
          },
          {
            id: "e2",
            agencyId: "agency-1",
            agencyShare: 1200,
            twoTierShare: 0,
            status: "APPROVED",
          },
        ]),
        updateMany: vi.fn(),
      },
      residualPayout: { create: vi.fn() },
      auditLog: { create: vi.fn() },
      $queryRaw: vi.fn().mockResolvedValue([{ total: BigInt(0) }]),
      $executeRaw: vi.fn().mockResolvedValue(1),
      $transaction: vi.fn().mockImplementation(async (fn: (tx: unknown) => Promise<unknown>) => {
        return fn(mockTx);
      }),
    };

    app = express();
    app.use(express.json());
    const router = createAdminRouter({
      prisma: mockPrisma as unknown as import("@prisma/client").PrismaClient,
    });
    app.use("/api/v1/admin", router);
  });

  it("creates payout for agency with all approved entries", async () => {
    const res = await request(
      app,
      "POST",
      "/api/v1/admin/residuals/create-payout",
      {
        agencyId: "agency-1",
        periodStart: "2026-03-01",
        method: "ach",
        reference: "REF-001",
      }
    );

    expect(res.status).toBe(201);
    expect(res.body.success).toBe(true);
    const payout = res.body.payout as Record<string, unknown>;
    expect(payout.id).toBe("payout-1");
    expect(payout.status).toBe("PAID");
  });

  it("aggregates directShare and twoTierShare correctly", async () => {
    await request(app, "POST", "/api/v1/admin/residuals/create-payout", {
      agencyId: "agency-1",
      periodStart: "2026-03-01",
      method: "manual",
    });

    const createMock = (mockTx.residualPayout as Record<string, ReturnType<typeof vi.fn>>).create;
    expect(createMock).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          totalAmount: 2350, // 1000 + 1200 + 150 + 0
          directShare: 2200, // 1000 + 1200
          twoTierShare: 150, // 150 + 0
        }),
      })
    );
  });

  it("marks entries as PAID in transaction", async () => {
    await request(app, "POST", "/api/v1/admin/residuals/create-payout", {
      agencyId: "agency-1",
      periodStart: "2026-03-01",
      method: "ach",
    });

    const updateManyMock = (mockTx.residualEntry as Record<string, ReturnType<typeof vi.fn>>).updateMany;
    expect(updateManyMock).toHaveBeenCalledWith(
      expect.objectContaining({
        data: { status: "PAID" },
      })
    );
  });

  it("rejects payout when not all entries are APPROVED", async () => {
    (mockPrisma.residualEntry as Record<string, unknown>).findMany = vi
      .fn()
      .mockResolvedValue([
        { id: "e1", agencyShare: 1000, twoTierShare: 0, status: "APPROVED" },
        { id: "e2", agencyShare: 500, twoTierShare: 0, status: "PENDING" },
      ]);

    const res = await request(
      app,
      "POST",
      "/api/v1/admin/residuals/create-payout",
      {
        agencyId: "agency-1",
        periodStart: "2026-03-01",
        method: "ach",
      }
    );

    expect(res.status).toBe(400);
    expect(res.body.error).toContain("All entries must be APPROVED");
  });

  it("returns 404 when no entries exist for agency+period", async () => {
    (mockPrisma.residualEntry as Record<string, unknown>).findMany = vi
      .fn()
      .mockResolvedValue([]);

    const res = await request(
      app,
      "POST",
      "/api/v1/admin/residuals/create-payout",
      {
        agencyId: "nonexistent",
        periodStart: "2026-03-01",
        method: "manual",
      }
    );

    expect(res.status).toBe(404);
  });

  it("returns 400 for invalid method", async () => {
    const res = await request(
      app,
      "POST",
      "/api/v1/admin/residuals/create-payout",
      {
        agencyId: "agency-1",
        periodStart: "2026-03-01",
        method: "paypal",
      }
    );
    expect(res.status).toBe(400);
  });

  it("creates audit log with payout details", async () => {
    await request(app, "POST", "/api/v1/admin/residuals/create-payout", {
      agencyId: "agency-1",
      periodStart: "2026-03-01",
      method: "ach",
      reference: "REF-001",
    });

    const auditCreateMock = (mockTx.auditLog as Record<string, ReturnType<typeof vi.fn>>).create;
    expect(auditCreateMock).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          action: "RESIDUAL_PAYOUT_CREATED",
          performedBy: "admin-test-user",
          details: expect.objectContaining({
            agencyId: "agency-1",
            totalAmount: 2350,
            method: "ach",
            entryCount: 2,
          }),
        }),
      })
    );
  });
});
