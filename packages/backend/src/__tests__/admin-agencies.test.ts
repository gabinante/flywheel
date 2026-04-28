/**
 * Admin Agency Management — Unit Tests
 *
 * Tests GET /agencies, GET /agencies/:id, PATCH /agencies/:id endpoints.
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

// ─── Test Data ────────────────────────────────────────────────────────

const mockAgency = {
  id: "agency_1",
  name: "Test Agency",
  contactEmail: "test@agency.com",
  contactPhone: "+1234567890",
  passwordHash: "hashed",
  referralCode: "agency-abc123",
  referredByAgencyId: null,
  referredByAgency: null,
  referredAgencies: [],
  tier: "TIER_1",
  tierOverride: false,
  status: "ACTIVE",
  payoutEmail: "payout@agency.com",
  payoutBankLast4: "1234",
  taxIdOnFile: true,
  merchants: [
    { id: "m1", name: "Merchant 1", status: "ACTIVE", createdAt: new Date() },
    { id: "m2", name: "Merchant 2", status: "ACTIVE", createdAt: new Date() },
  ],
  createdAt: new Date("2026-01-01"),
  updatedAt: new Date("2026-04-01"),
  _count: { merchants: 2 },
};

const mockAgency2 = {
  id: "agency_2",
  name: "Second Agency",
  contactEmail: "second@agency.com",
  tier: "TIER_2",
  status: "ACTIVE",
  referredByAgency: { id: "agency_1", name: "Test Agency" },
  createdAt: new Date("2026-02-01"),
  updatedAt: new Date("2026-04-01"),
  _count: { merchants: 1 },
};

// ─── Tests ────────────────────────────────────────────────────────────

describe("Admin Routes — Agency Management", () => {
  let app: express.Express;
  let mockPrisma: Record<string, unknown>;

  beforeEach(() => {
    mockPrisma = {
      merchant: {
        findMany: vi.fn().mockResolvedValue([]),
      },
      residualEntry: {
        create: vi.fn(),
        aggregate: vi.fn().mockResolvedValue({ _sum: { agencyShare: 5000, merchantVolume: 100000 } }),
        findMany: vi.fn().mockResolvedValue([]),
      },
      residualPayout: {
        findMany: vi.fn().mockResolvedValue([]),
      },
      agency: {
        findMany: vi.fn().mockResolvedValue([mockAgency, mockAgency2]),
        findUnique: vi.fn().mockResolvedValue(mockAgency),
        count: vi.fn().mockResolvedValue(2),
        update: vi.fn().mockImplementation(({ data }) => {
          return Promise.resolve({
            ...mockAgency,
            ...data,
            updatedAt: new Date(),
          });
        }),
      },
      auditLog: {
        create: vi.fn().mockResolvedValue({ id: "audit_1" }),
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

  // ─── GET /agencies ──────────────────────────────────────────────────

  describe("GET /agencies", () => {
    it("returns paginated list of agencies with defaults", async () => {
      const res = await request(app, "GET", "/api/v1/admin/agencies");

      expect(res.status).toBe(200);
      expect(res.body.agencies).toBeDefined();
      expect(Array.isArray(res.body.agencies)).toBe(true);
      expect((res.body.agencies as unknown[]).length).toBe(2);
      expect(res.body.pagination).toBeDefined();
      const pagination = res.body.pagination as Record<string, unknown>;
      expect(pagination.page).toBe(1);
      expect(pagination.limit).toBe(20);
      expect(pagination.total).toBe(2);
    });

    it("filters by tier", async () => {
      const res = await request(app, "GET", "/api/v1/admin/agencies?tier=TIER_2");

      expect(res.status).toBe(200);
      const findManyCall = (mockPrisma.agency as Record<string, unknown>).findMany as ReturnType<typeof vi.fn>;
      expect(findManyCall).toHaveBeenCalledWith(
        expect.objectContaining({
          where: expect.objectContaining({ tier: "TIER_2" }),
        })
      );
    });

    it("filters by status", async () => {
      const res = await request(app, "GET", "/api/v1/admin/agencies?status=SUSPENDED");

      expect(res.status).toBe(200);
      const findManyCall = (mockPrisma.agency as Record<string, unknown>).findMany as ReturnType<typeof vi.fn>;
      expect(findManyCall).toHaveBeenCalledWith(
        expect.objectContaining({
          where: expect.objectContaining({ status: "SUSPENDED" }),
        })
      );
    });

    it("filters by search term", async () => {
      const res = await request(app, "GET", "/api/v1/admin/agencies?search=test");

      expect(res.status).toBe(200);
      const findManyCall = (mockPrisma.agency as Record<string, unknown>).findMany as ReturnType<typeof vi.fn>;
      expect(findManyCall).toHaveBeenCalledWith(
        expect.objectContaining({
          where: expect.objectContaining({
            OR: [
              { name: { contains: "test", mode: "insensitive" } },
              { contactEmail: { contains: "test", mode: "insensitive" } },
            ],
          }),
        })
      );
    });

    it("respects pagination parameters", async () => {
      const res = await request(app, "GET", "/api/v1/admin/agencies?page=2&limit=10");

      expect(res.status).toBe(200);
      const findManyCall = (mockPrisma.agency as Record<string, unknown>).findMany as ReturnType<typeof vi.fn>;
      expect(findManyCall).toHaveBeenCalledWith(
        expect.objectContaining({
          skip: 10,
          take: 10,
        })
      );
    });

    it("enriches agencies with portfolio volume and monthly residual", async () => {
      const res = await request(app, "GET", "/api/v1/admin/agencies");

      expect(res.status).toBe(200);
      const agencies = res.body.agencies as Array<Record<string, unknown>>;
      expect(agencies[0].merchantCount).toBe(2);
      expect(agencies[0].portfolioVolume).toBeDefined();
      expect(agencies[0].monthlyResidual).toBeDefined();
    });
  });

  // ─── GET /agencies/:id ─────────────────────────────────────────────

  describe("GET /agencies/:id", () => {
    it("returns full agency detail with merchants and referrals", async () => {
      const fullAgency = {
        ...mockAgency,
        referredAgencies: [
          {
            id: "ref_1",
            name: "Referred Agency",
            contactEmail: "ref@agency.com",
            tier: "TIER_1",
            status: "ACTIVE",
            createdAt: new Date(),
            _count: { merchants: 3 },
          },
        ],
      };
      (mockPrisma.agency as Record<string, unknown>).findUnique = vi.fn().mockResolvedValue(fullAgency);

      const res = await request(app, "GET", "/api/v1/admin/agencies/agency_1");

      expect(res.status).toBe(200);
      expect(res.body.id).toBe("agency_1");
      expect(res.body.name).toBe("Test Agency");
      expect(res.body.merchants).toBeDefined();
      expect(Array.isArray(res.body.merchants)).toBe(true);
      expect(res.body.referredAgencies).toBeDefined();
      expect(res.body.residualHistory).toBeDefined();
      expect(res.body.payouts).toBeDefined();
    });

    it("returns 404 for non-existent agency", async () => {
      (mockPrisma.agency as Record<string, unknown>).findUnique = vi.fn().mockResolvedValue(null);

      const res = await request(app, "GET", "/api/v1/admin/agencies/nonexistent");

      expect(res.status).toBe(404);
      expect(res.body.error).toBe("Agency not found");
    });

    it("includes residual history aggregated by month", async () => {
      const entries = [
        {
          id: "re_1",
          merchantId: "m1",
          periodStart: new Date("2026-03-01"),
          periodEnd: new Date("2026-04-01"),
          merchantVolume: 50000,
          agencyShare: 500,
          twoTierShare: 0,
          status: "APPROVED",
          createdAt: new Date(),
        },
        {
          id: "re_2",
          merchantId: "m2",
          periodStart: new Date("2026-03-01"),
          periodEnd: new Date("2026-04-01"),
          merchantVolume: 75000,
          agencyShare: 750,
          twoTierShare: 100,
          status: "PENDING",
          createdAt: new Date(),
        },
      ];
      (mockPrisma.residualEntry as Record<string, unknown>).findMany = vi.fn().mockResolvedValue(entries);

      const res = await request(app, "GET", "/api/v1/admin/agencies/agency_1");

      expect(res.status).toBe(200);
      const history = res.body.residualHistory as Array<Record<string, unknown>>;
      expect(history.length).toBeGreaterThan(0);
      // Should aggregate the two entries for March 2026
      const marchEntry = history.find((h) => h.month === "2026-03");
      expect(marchEntry).toBeDefined();
      expect(marchEntry!.agencyShare).toBe(1250); // 500 + 750
      expect(marchEntry!.twoTierShare).toBe(100);
      expect(marchEntry!.volume).toBe(125000); // 50000 + 75000
    });
  });

  // ─── PATCH /agencies/:id ────────────────────────────────────────────

  describe("PATCH /agencies/:id", () => {
    it("suspends an agency and creates audit log", async () => {
      const res = await request(app, "PATCH", "/api/v1/admin/agencies/agency_1", {
        status: "SUSPENDED",
      });

      expect(res.status).toBe(200);
      expect(res.body.status).toBe("SUSPENDED");

      // Verify audit log was created
      const auditCreate = (mockPrisma.auditLog as Record<string, unknown>).create as ReturnType<typeof vi.fn>;
      expect(auditCreate).toHaveBeenCalledWith(
        expect.objectContaining({
          data: expect.objectContaining({
            action: "AGENCY_SUSPENDED",
          }),
        })
      );
    });

    it("reactivates a suspended agency", async () => {
      (mockPrisma.agency as Record<string, unknown>).findUnique = vi.fn().mockResolvedValue({
        ...mockAgency,
        status: "SUSPENDED",
      });

      const res = await request(app, "PATCH", "/api/v1/admin/agencies/agency_1", {
        status: "ACTIVE",
      });

      expect(res.status).toBe(200);
      expect(res.body.status).toBe("ACTIVE");

      const auditCreate = (mockPrisma.auditLog as Record<string, unknown>).create as ReturnType<typeof vi.fn>;
      expect(auditCreate).toHaveBeenCalledWith(
        expect.objectContaining({
          data: expect.objectContaining({
            action: "AGENCY_REACTIVATED",
          }),
        })
      );
    });

    it("overrides agency tier", async () => {
      const res = await request(app, "PATCH", "/api/v1/admin/agencies/agency_1", {
        tier: "TIER_3",
        tierOverride: true,
      });

      expect(res.status).toBe(200);
      expect(res.body.tier).toBe("TIER_3");
      expect(res.body.tierOverride).toBe(true);

      const auditCreate = (mockPrisma.auditLog as Record<string, unknown>).create as ReturnType<typeof vi.fn>;
      expect(auditCreate).toHaveBeenCalled();
    });

    it("returns 404 for non-existent agency", async () => {
      (mockPrisma.agency as Record<string, unknown>).findUnique = vi.fn().mockResolvedValue(null);

      const res = await request(app, "PATCH", "/api/v1/admin/agencies/nonexistent", {
        status: "SUSPENDED",
      });

      expect(res.status).toBe(404);
      expect(res.body.error).toBe("Agency not found");
    });

    it("returns 400 for empty body", async () => {
      const res = await request(app, "PATCH", "/api/v1/admin/agencies/agency_1", {});

      expect(res.status).toBe(400);
    });

    it("returns 400 for invalid status value", async () => {
      const res = await request(app, "PATCH", "/api/v1/admin/agencies/agency_1", {
        status: "INVALID_STATUS",
      });

      expect(res.status).toBe(400);
    });

    it("returns 400 for invalid tier value", async () => {
      const res = await request(app, "PATCH", "/api/v1/admin/agencies/agency_1", {
        tier: "TIER_99",
      });

      expect(res.status).toBe(400);
    });

    it("tracks all field changes in audit log details", async () => {
      const res = await request(app, "PATCH", "/api/v1/admin/agencies/agency_1", {
        tier: "TIER_3",
        tierOverride: true,
      });

      expect(res.status).toBe(200);

      const auditCreate = (mockPrisma.auditLog as Record<string, unknown>).create as ReturnType<typeof vi.fn>;
      const auditCall = auditCreate.mock.calls[0][0];
      const details = auditCall.data.details as Record<string, unknown>;
      const changes = details.changes as Record<string, unknown>;

      expect(changes.tier).toEqual({ from: "TIER_1", to: "TIER_3" });
      expect(changes.tierOverride).toEqual({ from: false, to: true });
    });
  });
});
