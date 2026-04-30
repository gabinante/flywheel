/**
 * Admin Routes — Unit Tests
 *
 * Tests:
 *  1. POST /residuals/calculate — existing residual calculation
 *  2. GET  /agencies             — list with pagination, filters, sorting
 *  3. GET  /agencies/:id         — full profile including merchants, network, residuals
 *  4. PATCH /agencies/:id        — update status/tier with audit log
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import express from "express";
import { createAdminRouter } from "../routes/admin.js";
import type { PrismaClient } from "@prisma/client";

// ─── Helpers ─────────────────────────────────────────────────────────

/**
 * Lightweight HTTP test helper.
 * Creates a one-shot Express server on a random port, makes the request,
 * then tears the server down.  Retries once on transient socket errors
 * (e.g. "other side closed") which can occur under heavy test parallelism.
 */
async function request(
  app: express.Express,
  method: string,
  path: string,
  body?: unknown
): Promise<{ status: number; body: Record<string, unknown> }> {
  const attempt = (): Promise<{ status: number; body: Record<string, unknown> }> =>
    new Promise((resolve, reject) => {
      const server = app.listen(0, "127.0.0.1", () => {
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

  try {
    return await attempt();
  } catch (err: unknown) {
    // Retry once on transient socket/network errors
    const msg = err instanceof Error ? err.message : String(err);
    if (msg.includes("closed") || msg.includes("ECONNRESET") || msg.includes("socket")) {
      return attempt();
    }
    throw err;
  }
}

// ─── Mock data ────────────────────────────────────────────────────────

const mockAgencyAlpha = {
  id: "agency-alpha-1",
  name: "Alpha Agency",
  contactEmail: "alpha@example.com",
  contactPhone: "+1-555-0001",
  tier: "TIER_1",
  tierOverride: false,
  status: "ACTIVE",
  referralCode: "alpha-ref",
  referredByAgencyId: null,
  payoutEmail: null,
  payoutBankLast4: null,
  taxIdOnFile: false,
  createdAt: new Date("2026-01-01T00:00:00Z"),
  updatedAt: new Date("2026-01-01T00:00:00Z"),
};

const mockAgencyBeta = {
  id: "agency-beta-2",
  name: "Beta Agency",
  contactEmail: "beta@example.com",
  contactPhone: null,
  tier: "TIER_2",
  tierOverride: false,
  status: "SUSPENDED",
  referralCode: "beta-ref",
  referredByAgencyId: "agency-alpha-1",
  payoutEmail: null,
  payoutBankLast4: null,
  taxIdOnFile: false,
  createdAt: new Date("2026-02-01T00:00:00Z"),
  updatedAt: new Date("2026-02-01T00:00:00Z"),
};

// ─── Mock Prisma builder ──────────────────────────────────────────────

type MockPrisma = {
  agency: {
    findMany: ReturnType<typeof vi.fn>;
    findUnique: ReturnType<typeof vi.fn>;
    count: ReturnType<typeof vi.fn>;
    update: ReturnType<typeof vi.fn>;
  };
  residualEntry: {
    groupBy: ReturnType<typeof vi.fn>;
  };
  auditLog: {
    create: ReturnType<typeof vi.fn>;
  };
  merchant: {
    findMany: ReturnType<typeof vi.fn>;
  };
  $transaction: ReturnType<typeof vi.fn>;
  $queryRaw: ReturnType<typeof vi.fn>;
  $executeRaw: ReturnType<typeof vi.fn>;
};

function buildMockPrisma(): MockPrisma {
  const mock: MockPrisma = {
    agency: {
      findMany: vi.fn().mockResolvedValue([]),
      findUnique: vi.fn().mockResolvedValue(null),
      count: vi.fn().mockResolvedValue(0),
      update: vi.fn().mockResolvedValue(null),
    },
    residualEntry: {
      groupBy: vi.fn().mockResolvedValue([]),
    },
    auditLog: {
      create: vi.fn().mockResolvedValue({ id: "audit-1" }),
    },
    merchant: {
      findMany: vi.fn().mockResolvedValue([]),
    },
    $transaction: vi.fn().mockImplementation(async (ops: unknown[]) => {
      return Promise.all(ops);
    }),
    $queryRaw: vi.fn().mockResolvedValue([{ total: BigInt(0) }]),
    $executeRaw: vi.fn().mockResolvedValue(1),
  };
  return mock;
}

// ─── Test suites ──────────────────────────────────────────────────────

describe("Admin Routes — POST /residuals/calculate", () => {
  let app: express.Express;
  let mockPrisma: MockPrisma;

  beforeEach(() => {
    mockPrisma = buildMockPrisma();
    app = express();
    app.use(express.json());
    app.use(
      "/api/v1/admin",
      createAdminRouter({ prisma: mockPrisma as unknown as PrismaClient })
    );
  });

  it("returns 200 with calculation results for valid date range", async () => {
    const res = await request(
      app,
      "POST",
      "/api/v1/admin/residuals/calculate",
      {
        periodStart: "2026-03-01T00:00:00.000Z",
        periodEnd: "2026-04-01T00:00:00.000Z",
      }
    );
    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
  });

  it("returns 400 for missing periodStart", async () => {
    const res = await request(
      app,
      "POST",
      "/api/v1/admin/residuals/calculate",
      { periodEnd: "2026-04-01T00:00:00.000Z" }
    );
    expect(res.status).toBe(400);
    expect(res.body.error).toBe("Invalid request body");
  });

  it("returns 400 when periodEnd <= periodStart", async () => {
    const res = await request(
      app,
      "POST",
      "/api/v1/admin/residuals/calculate",
      {
        periodStart: "2026-04-01T00:00:00.000Z",
        periodEnd: "2026-03-01T00:00:00.000Z",
      }
    );
    expect(res.status).toBe(400);
    expect(res.body.error).toBe("periodEnd must be after periodStart");
  });
});

// ─────────────────────────────────────────────────────────────────────

describe("Admin Routes — GET /agencies", () => {
  let app: express.Express;
  let mockPrisma: MockPrisma;

  beforeEach(() => {
    mockPrisma = buildMockPrisma();
    app = express();
    app.use(express.json());
    app.use(
      "/api/v1/admin",
      createAdminRouter({ prisma: mockPrisma as unknown as PrismaClient })
    );
  });

  it("returns paginated agency list with defaults", async () => {
    mockPrisma.agency.findMany.mockResolvedValue([
      {
        ...mockAgencyAlpha,
        _count: { merchants: 3 },
        referredByAgency: null,
      },
    ]);
    mockPrisma.agency.count.mockResolvedValue(1);
    mockPrisma.residualEntry.groupBy.mockResolvedValue([
      {
        agencyId: "agency-alpha-1",
        _sum: { merchantVolume: 50000, agencyShare: 1500 },
      },
    ]);

    const res = await request(app, "GET", "/api/v1/admin/agencies");
    expect(res.status).toBe(200);
    const body = res.body as {
      data: Array<{
        id: string;
        merchantCount: number;
        portfolioVolume: number;
        monthlyResidual: number;
      }>;
      total: number;
      page: number;
      limit: number;
    };
    expect(body.total).toBe(1);
    expect(body.page).toBe(1);
    expect(body.limit).toBe(20);
    expect(body.data).toHaveLength(1);
    expect(body.data[0].merchantCount).toBe(3);
    expect(body.data[0].portfolioVolume).toBe(50000);
    expect(body.data[0].monthlyResidual).toBe(1500);
  });

  it("returns 200 with empty data when no agencies match", async () => {
    mockPrisma.agency.findMany.mockResolvedValue([]);
    mockPrisma.agency.count.mockResolvedValue(0);

    const res = await request(app, "GET", "/api/v1/admin/agencies");
    expect(res.status).toBe(200);
    const body = res.body as { data: unknown[]; total: number };
    expect(body.data).toHaveLength(0);
    expect(body.total).toBe(0);
  });

  it("passes tier filter to prisma query", async () => {
    mockPrisma.agency.findMany.mockResolvedValue([]);
    mockPrisma.agency.count.mockResolvedValue(0);

    await request(app, "GET", "/api/v1/admin/agencies?tier=TIER_2");

    expect(mockPrisma.agency.findMany).toHaveBeenCalledWith(
      expect.objectContaining({ where: expect.objectContaining({ tier: "TIER_2" }) })
    );
  });

  it("passes status filter to prisma query", async () => {
    mockPrisma.agency.findMany.mockResolvedValue([]);
    mockPrisma.agency.count.mockResolvedValue(0);

    await request(app, "GET", "/api/v1/admin/agencies?status=SUSPENDED");

    expect(mockPrisma.agency.findMany).toHaveBeenCalledWith(
      expect.objectContaining({
        where: expect.objectContaining({ status: "SUSPENDED" }),
      })
    );
  });

  it("passes search filter as OR on name/contactEmail", async () => {
    mockPrisma.agency.findMany.mockResolvedValue([]);
    mockPrisma.agency.count.mockResolvedValue(0);

    await request(app, "GET", "/api/v1/admin/agencies?search=alpha");

    expect(mockPrisma.agency.findMany).toHaveBeenCalledWith(
      expect.objectContaining({
        where: expect.objectContaining({
          OR: [
            { name: { contains: "alpha", mode: "insensitive" } },
            { contactEmail: { contains: "alpha", mode: "insensitive" } },
          ],
        }),
      })
    );
  });

  it("returns 400 for invalid tier filter", async () => {
    const res = await request(
      app,
      "GET",
      "/api/v1/admin/agencies?tier=TIER_99"
    );
    expect(res.status).toBe(400);
    expect(res.body.error).toBe("Invalid query parameters");
  });

  it("sorts by merchantCount in-memory when sort=merchantCount", async () => {
    mockPrisma.agency.findMany.mockResolvedValue([
      {
        ...mockAgencyAlpha,
        id: "agency-low",
        _count: { merchants: 2 },
        referredByAgency: null,
      },
      {
        ...mockAgencyBeta,
        id: "agency-high",
        _count: { merchants: 10 },
        referredByAgency: null,
      },
    ]);
    mockPrisma.agency.count.mockResolvedValue(2);
    mockPrisma.residualEntry.groupBy.mockResolvedValue([]);

    const res = await request(
      app,
      "GET",
      "/api/v1/admin/agencies?sort=merchantCount"
    );
    expect(res.status).toBe(200);
    const data = (res.body as { data: Array<{ id: string; merchantCount: number }> }).data;
    expect(data[0].merchantCount).toBeGreaterThanOrEqual(data[1].merchantCount);
  });

  it("includes referredByAgency name in response", async () => {
    mockPrisma.agency.findMany.mockResolvedValue([
      {
        ...mockAgencyBeta,
        _count: { merchants: 0 },
        referredByAgency: { name: "Alpha Agency" },
      },
    ]);
    mockPrisma.agency.count.mockResolvedValue(1);
    mockPrisma.residualEntry.groupBy.mockResolvedValue([]);

    const res = await request(app, "GET", "/api/v1/admin/agencies");
    expect(res.status).toBe(200);
    const data = (res.body as { data: Array<{ referredByAgency: { name: string } | null }> }).data;
    expect(data[0].referredByAgency).toEqual({ name: "Alpha Agency" });
  });

  it("defaults portfolioVolume to 0 when no residual entries", async () => {
    mockPrisma.agency.findMany.mockResolvedValue([
      {
        ...mockAgencyAlpha,
        _count: { merchants: 1 },
        referredByAgency: null,
      },
    ]);
    mockPrisma.agency.count.mockResolvedValue(1);
    mockPrisma.residualEntry.groupBy.mockResolvedValue([]);

    const res = await request(app, "GET", "/api/v1/admin/agencies");
    expect(res.status).toBe(200);
    const data = (res.body as { data: Array<{ portfolioVolume: number; monthlyResidual: number }> }).data;
    expect(data[0].portfolioVolume).toBe(0);
    expect(data[0].monthlyResidual).toBe(0);
  });
});

// ─────────────────────────────────────────────────────────────────────

describe("Admin Routes — GET /agencies/:id", () => {
  let app: express.Express;
  let mockPrisma: MockPrisma;

  beforeEach(() => {
    mockPrisma = buildMockPrisma();
    app = express();
    app.use(express.json());
    app.use(
      "/api/v1/admin",
      createAdminRouter({ prisma: mockPrisma as unknown as PrismaClient })
    );
  });

  it("returns 404 when agency not found", async () => {
    mockPrisma.agency.findUnique.mockResolvedValue(null);

    const res = await request(app, "GET", "/api/v1/admin/agencies/nonexistent");
    expect(res.status).toBe(404);
    expect(res.body.error).toBe("Agency not found");
  });

  it("returns full agency profile with merchants, network, and history", async () => {
    mockPrisma.agency.findUnique.mockResolvedValue({
      ...mockAgencyAlpha,
      referredByAgency: null,
      referredAgencies: [
        {
          id: "agency-beta-2",
          name: "Beta Agency",
          contactEmail: "beta@example.com",
          tier: "TIER_2",
          status: "SUSPENDED",
          createdAt: new Date("2026-02-01T00:00:00Z"),
          _count: { merchants: 5 },
        },
      ],
      merchants: [
        {
          id: "merch-1",
          name: "Merchant One",
          email: "m1@example.com",
          status: "ACTIVE",
          createdAt: new Date("2026-02-15T00:00:00Z"),
        },
      ],
      residualEntries: [
        {
          id: "res-1",
          periodStart: new Date("2026-03-01T00:00:00Z"),
          periodEnd: new Date("2026-04-01T00:00:00Z"),
          merchantVolume: 100000,
          agencyShare: 3000,
          twoTierShare: 0,
          status: "APPROVED",
          approvedAt: new Date("2026-04-05T00:00:00Z"),
        },
      ],
      residualPayouts: [
        {
          id: "pay-1",
          periodStart: new Date("2026-03-01T00:00:00Z"),
          totalAmount: 3000,
          directShare: 3000,
          twoTierShare: 0,
          method: "ACH",
          status: "PAID",
          paidAt: new Date("2026-04-10T00:00:00Z"),
          createdAt: new Date("2026-04-10T00:00:00Z"),
        },
      ],
    });
    mockPrisma.residualEntry.groupBy.mockResolvedValue([
      {
        agencyId: "agency-beta-2",
        _sum: { merchantVolume: 20000, agencyShare: 600 },
      },
    ]);

    const res = await request(
      app,
      "GET",
      "/api/v1/admin/agencies/agency-alpha-1"
    );
    expect(res.status).toBe(200);

    const body = res.body as {
      id: string;
      merchants: Array<{ id: string }>;
      referredAgencies: Array<{ id: string; merchantCount: number; portfolioVolume: number }>;
      residualHistory: Array<{ id: string }>;
      payoutHistory: Array<{ id: string }>;
    };
    expect(body.id).toBe("agency-alpha-1");
    expect(body.merchants).toHaveLength(1);
    expect(body.referredAgencies).toHaveLength(1);
    expect(body.referredAgencies[0].merchantCount).toBe(5);
    expect(body.referredAgencies[0].portfolioVolume).toBe(20000);
    expect(body.residualHistory).toHaveLength(1);
    expect(body.payoutHistory).toHaveLength(1);
  });

  it("returns referred agencies with zero volume when no residuals", async () => {
    mockPrisma.agency.findUnique.mockResolvedValue({
      ...mockAgencyAlpha,
      referredByAgency: null,
      referredAgencies: [
        {
          id: "referred-agency-1",
          name: "New Agency",
          contactEmail: "new@example.com",
          tier: "TIER_1",
          status: "ACTIVE",
          createdAt: new Date("2026-03-01T00:00:00Z"),
          _count: { merchants: 0 },
        },
      ],
      merchants: [],
      residualEntries: [],
      residualPayouts: [],
    });
    mockPrisma.residualEntry.groupBy.mockResolvedValue([]);

    const res = await request(
      app,
      "GET",
      "/api/v1/admin/agencies/agency-alpha-1"
    );
    expect(res.status).toBe(200);
    const body = res.body as {
      referredAgencies: Array<{ portfolioVolume: number; monthlyResidual: number }>;
    };
    expect(body.referredAgencies[0].portfolioVolume).toBe(0);
    expect(body.referredAgencies[0].monthlyResidual).toBe(0);
  });
});

// ─────────────────────────────────────────────────────────────────────

describe("Admin Routes — PATCH /agencies/:id", () => {
  let app: express.Express;
  let mockPrisma: MockPrisma;

  beforeEach(() => {
    mockPrisma = buildMockPrisma();
    app = express();
    app.use(express.json());
    app.use(
      "/api/v1/admin",
      createAdminRouter({ prisma: mockPrisma as unknown as PrismaClient })
    );
  });

  it("returns 404 when agency does not exist", async () => {
    mockPrisma.agency.findUnique.mockResolvedValue(null);

    const res = await request(
      app,
      "PATCH",
      "/api/v1/admin/agencies/nonexistent",
      { status: "SUSPENDED" }
    );
    expect(res.status).toBe(404);
    expect(res.body.error).toBe("Agency not found");
  });

  it("returns 400 when body is empty", async () => {
    const res = await request(
      app,
      "PATCH",
      "/api/v1/admin/agencies/agency-alpha-1",
      {}
    );
    expect(res.status).toBe(400);
    expect(res.body.error).toBe("No fields to update");
  });

  it("suspends an agency and logs audit entry", async () => {
    mockPrisma.agency.findUnique.mockResolvedValue({
      id: "agency-alpha-1",
      status: "ACTIVE",
      tier: "TIER_1",
      tierOverride: false,
    });

    const updatedAgency = {
      id: "agency-alpha-1",
      name: "Alpha Agency",
      contactEmail: "alpha@example.com",
      tier: "TIER_1",
      tierOverride: false,
      status: "SUSPENDED",
      updatedAt: new Date(),
    };

    // $transaction should execute both operations
    mockPrisma.$transaction.mockImplementation(
      async (ops: Array<Promise<unknown>>) => Promise.all(ops)
    );
    mockPrisma.agency.update.mockResolvedValue(updatedAgency);
    mockPrisma.auditLog.create.mockResolvedValue({ id: "audit-log-1" });

    const res = await request(
      app,
      "PATCH",
      "/api/v1/admin/agencies/agency-alpha-1",
      { status: "SUSPENDED" }
    );
    expect(res.status).toBe(200);
    expect(res.body.status).toBe("SUSPENDED");

    // Verify agency.update was called with SUSPENDED status
    expect(mockPrisma.agency.update).toHaveBeenCalledWith(
      expect.objectContaining({
        where: { id: "agency-alpha-1" },
        data: expect.objectContaining({ status: "SUSPENDED" }),
      })
    );

    // Verify audit log was created
    expect(mockPrisma.auditLog.create).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          action: "agency.patch",
          details: expect.objectContaining({
            agencyId: "agency-alpha-1",
            changes: expect.objectContaining({ status: "SUSPENDED" }),
          }),
        }),
      })
    );
  });

  it("overrides tier and sets tierOverride=true", async () => {
    mockPrisma.agency.findUnique.mockResolvedValue({
      id: "agency-alpha-1",
      status: "ACTIVE",
      tier: "TIER_1",
      tierOverride: false,
    });

    const updatedAgency = {
      id: "agency-alpha-1",
      name: "Alpha Agency",
      contactEmail: "alpha@example.com",
      tier: "TIER_3",
      tierOverride: true,
      status: "ACTIVE",
      updatedAt: new Date(),
    };

    mockPrisma.$transaction.mockImplementation(
      async (ops: Array<Promise<unknown>>) => Promise.all(ops)
    );
    mockPrisma.agency.update.mockResolvedValue(updatedAgency);
    mockPrisma.auditLog.create.mockResolvedValue({ id: "audit-2" });

    const res = await request(
      app,
      "PATCH",
      "/api/v1/admin/agencies/agency-alpha-1",
      { tier: "TIER_3", tierOverride: true }
    );
    expect(res.status).toBe(200);
    expect(res.body.tier).toBe("TIER_3");
    expect(res.body.tierOverride).toBe(true);
  });

  it("reactivates a suspended agency", async () => {
    mockPrisma.agency.findUnique.mockResolvedValue({
      id: "agency-beta-2",
      status: "SUSPENDED",
      tier: "TIER_2",
      tierOverride: false,
    });

    const updatedAgency = {
      id: "agency-beta-2",
      name: "Beta Agency",
      contactEmail: "beta@example.com",
      tier: "TIER_2",
      tierOverride: false,
      status: "ACTIVE",
      updatedAt: new Date(),
    };

    mockPrisma.$transaction.mockImplementation(
      async (ops: Array<Promise<unknown>>) => Promise.all(ops)
    );
    mockPrisma.agency.update.mockResolvedValue(updatedAgency);
    mockPrisma.auditLog.create.mockResolvedValue({ id: "audit-3" });

    const res = await request(
      app,
      "PATCH",
      "/api/v1/admin/agencies/agency-beta-2",
      { status: "ACTIVE" }
    );
    expect(res.status).toBe(200);
    expect(res.body.status).toBe("ACTIVE");
  });

  it("returns 400 for invalid status value", async () => {
    const res = await request(
      app,
      "PATCH",
      "/api/v1/admin/agencies/agency-alpha-1",
      { status: "CHURNED" } // CHURNED is not allowed via PATCH
    );
    expect(res.status).toBe(400);
    expect(res.body.error).toBe("Invalid request body");
  });

  it("includes previous values in audit log details", async () => {
    mockPrisma.agency.findUnique.mockResolvedValue({
      id: "agency-alpha-1",
      status: "ACTIVE",
      tier: "TIER_1",
      tierOverride: false,
    });

    mockPrisma.$transaction.mockImplementation(
      async (ops: Array<Promise<unknown>>) => Promise.all(ops)
    );
    mockPrisma.agency.update.mockResolvedValue({
      id: "agency-alpha-1",
      name: "Alpha Agency",
      contactEmail: "alpha@example.com",
      tier: "TIER_1",
      tierOverride: false,
      status: "SUSPENDED",
      updatedAt: new Date(),
    });
    mockPrisma.auditLog.create.mockResolvedValue({ id: "audit-4" });

    await request(
      app,
      "PATCH",
      "/api/v1/admin/agencies/agency-alpha-1",
      { status: "SUSPENDED" }
    );

    expect(mockPrisma.auditLog.create).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          details: expect.objectContaining({
            previous: expect.objectContaining({
              status: "ACTIVE",
              tier: "TIER_1",
            }),
          }),
        }),
      })
    );
  });
});
