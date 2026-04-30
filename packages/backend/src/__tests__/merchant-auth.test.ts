/**
 * Merchant Auth Routes — Unit Tests
 *
 * Verifies the affiliate attribution acceptance criteria from
 * gohighpayment-8:
 *
 * 1. Register merchant with ref=validCode → agencyId set, attributedAt set
 * 2. Register merchant with ref=invalidCode → merchant created, agencyId=null
 * 3. Register merchant with no ref but ghp_ref cookie → attributed from cookie
 * 4. Register merchant with no ref and no cookie → agencyId=null
 * 5. PATCH merchant agencyId when already set → 409 Conflict
 * 6. PATCH merchant agencyId when NOT yet set → allowed
 * 7. PATCH mutable fields (name, phone) → 200 OK
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import express from "express";
import { createMerchantAuthRouter } from "../routes/merchant-auth.js";

// ─── Helpers ─────────────────────────────────────────────────────────

async function request(
  app: express.Express,
  method: string,
  path: string,
  opts: { body?: unknown; headers?: Record<string, string> } = {}
) {
  return new Promise<{ status: number; body: Record<string, unknown>; headers: Record<string, string> }>(
    (resolve, reject) => {
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
            ...opts.headers,
          },
          body: opts.body ? JSON.stringify(opts.body) : undefined,
        })
          .then(async (res) => {
            const json = (await res.json()) as Record<string, unknown>;
            const headers: Record<string, string> = {};
            res.headers.forEach((value, key) => {
              headers[key.toLowerCase()] = value;
            });
            server.close();
            resolve({ status: res.status, body: json, headers });
          })
          .catch((err) => {
            server.close();
            reject(err);
          });
      });
    }
  );
}

// ─── Mock Prisma ─────────────────────────────────────────────────────

interface MockMerchant {
  id: string;
  email: string;
  name: string;
  phone: string | null;
  agencyId: string | null;
  attributedAt: Date | null;
  status: string;
  createdAt: Date;
  updatedAt: Date;
}

interface MockAgency {
  id: string;
  referralCode: string;
}

function createMockPrisma(opts: {
  agencies?: MockAgency[];
  merchants?: MockMerchant[];
}) {
  const merchants: MockMerchant[] = [...(opts.merchants ?? [])];
  const agencies: MockAgency[] = [...(opts.agencies ?? [])];

  return {
    _merchants: merchants,
    _agencies: agencies,

    agency: {
      findUnique: vi.fn().mockImplementation(
        ({ where }: { where: { referralCode?: string; id?: string } }) => {
          if (where.referralCode) {
            const found = agencies.find((a) => a.referralCode === where.referralCode);
            return Promise.resolve(found ? { id: found.id } : null);
          }
          if (where.id) {
            const found = agencies.find((a) => a.id === where.id);
            return Promise.resolve(found ?? null);
          }
          return Promise.resolve(null);
        }
      ),
    },

    merchant: {
      create: vi.fn().mockImplementation(
        ({ data }: { data: Partial<MockMerchant>; select?: unknown }) => {
          // Simulate unique constraint on email
          if (merchants.some((m) => m.email === data.email)) {
            return Promise.reject(
              new Error("Unique constraint failed on the fields: (`email`)")
            );
          }
          const merchant: MockMerchant = {
            id: `m-${Date.now()}`,
            email: data.email!,
            name: data.name!,
            phone: data.phone ?? null,
            agencyId: data.agencyId ?? null,
            attributedAt: data.attributedAt ?? null,
            status: "ACTIVE",
            createdAt: new Date(),
            updatedAt: new Date(),
          };
          merchants.push(merchant);
          return Promise.resolve(merchant);
        }
      ),

      findUnique: vi.fn().mockImplementation(
        ({ where }: { where: { id?: string; email?: string } }) => {
          if (where.id) {
            return Promise.resolve(merchants.find((m) => m.id === where.id) ?? null);
          }
          if (where.email) {
            return Promise.resolve(merchants.find((m) => m.email === where.email) ?? null);
          }
          return Promise.resolve(null);
        }
      ),

      update: vi.fn().mockImplementation(
        ({
          where,
          data,
        }: {
          where: { id: string };
          data: Partial<MockMerchant>;
          select?: unknown;
        }) => {
          const idx = merchants.findIndex((m) => m.id === where.id);
          if (idx === -1) {
            return Promise.reject(new Error("Record to update not found"));
          }
          merchants[idx] = { ...merchants[idx], ...data, updatedAt: new Date() };
          return Promise.resolve(merchants[idx]);
        }
      ),
    },
  };
}

// ─── Test Fixtures ────────────────────────────────────────────────────

const AGENCY_A: MockAgency = { id: "agency-id-a", referralCode: "agency-abc123" };
const AGENCY_B: MockAgency = { id: "agency-id-b", referralCode: "agency-def456" };

const EXISTING_MERCHANT: MockMerchant = {
  id: "merchant-existing",
  email: "existing@example.com",
  name: "Existing Merchant",
  phone: null,
  agencyId: AGENCY_A.id,
  attributedAt: new Date("2026-01-01"),
  status: "ACTIVE",
  createdAt: new Date(),
  updatedAt: new Date(),
};

const UNATTRIBUTED_MERCHANT: MockMerchant = {
  id: "merchant-unattributed",
  email: "unattributed@example.com",
  name: "Unattributed Merchant",
  phone: null,
  agencyId: null,
  attributedAt: null,
  status: "ACTIVE",
  createdAt: new Date(),
  updatedAt: new Date(),
};

// ─── Tests ────────────────────────────────────────────────────────────

describe("POST /register — merchant registration with attribution", () => {
  let app: express.Express;
  let mockPrisma: ReturnType<typeof createMockPrisma>;

  beforeEach(() => {
    mockPrisma = createMockPrisma({ agencies: [AGENCY_A, AGENCY_B] });
    app = express();
    app.use(express.json());
    const router = createMerchantAuthRouter({
      prisma: mockPrisma as unknown as import("@prisma/client").PrismaClient,
    });
    app.use("/api/v1/merchant/auth", router);
  });

  it("attributes merchant to agency when valid ref is in body", async () => {
    const res = await request(app, "POST", "/api/v1/merchant/auth/register", {
      body: {
        name: "Test Merchant",
        email: "test1@example.com",
        ref: AGENCY_A.referralCode,
      },
    });

    expect(res.status).toBe(201);
    expect(res.body.agencyId).toBe(AGENCY_A.id);
    expect(res.body.attributedAt).toBeTruthy();
  });

  it("attributes merchant to agency when valid ref is a query param", async () => {
    const res = await request(
      app,
      "POST",
      `/api/v1/merchant/auth/register?ref=${AGENCY_B.referralCode}`,
      { body: { name: "Test Merchant QP", email: "testqp@example.com" } }
    );

    expect(res.status).toBe(201);
    expect(res.body.agencyId).toBe(AGENCY_B.id);
    expect(res.body.attributedAt).toBeTruthy();
  });

  it("sets ghp_ref cookie when a valid ref is used", async () => {
    const res = await request(app, "POST", "/api/v1/merchant/auth/register", {
      body: {
        name: "Cookie Merchant",
        email: "cookie@example.com",
        ref: AGENCY_A.referralCode,
      },
    });

    expect(res.status).toBe(201);
    const setCookie = res.headers["set-cookie"] ?? "";
    expect(setCookie).toContain("ghp_ref=");
    expect(setCookie).toContain(AGENCY_A.referralCode);
  });

  it("attributes merchant from ghp_ref cookie when no ref param provided", async () => {
    const res = await request(app, "POST", "/api/v1/merchant/auth/register", {
      body: { name: "Cookie Merchant", email: "cookie2@example.com" },
      headers: { Cookie: `ghp_ref=${AGENCY_B.referralCode}` },
    });

    expect(res.status).toBe(201);
    expect(res.body.agencyId).toBe(AGENCY_B.id);
    expect(res.body.attributedAt).toBeTruthy();
  });

  it("creates merchant with agencyId=null for invalid ref (never blocks signup)", async () => {
    const res = await request(app, "POST", "/api/v1/merchant/auth/register", {
      body: {
        name: "Invalid Ref Merchant",
        email: "invalidref@example.com",
        ref: "totally-invalid-code",
      },
    });

    expect(res.status).toBe(201);
    expect(res.body.agencyId).toBeNull();
    expect(res.body.attributedAt).toBeNull();
  });

  it("creates merchant with agencyId=null when no ref at all", async () => {
    const res = await request(app, "POST", "/api/v1/merchant/auth/register", {
      body: { name: "No Ref Merchant", email: "noref@example.com" },
    });

    expect(res.status).toBe(201);
    expect(res.body.agencyId).toBeNull();
    expect(res.body.attributedAt).toBeNull();
  });

  it("returns 400 for missing required fields", async () => {
    const res = await request(app, "POST", "/api/v1/merchant/auth/register", {
      body: { name: "No Email" },
    });

    expect(res.status).toBe(400);
    expect(res.body.error).toBe("Invalid request body");
  });

  it("returns 400 for invalid email format", async () => {
    const res = await request(app, "POST", "/api/v1/merchant/auth/register", {
      body: { name: "Bad Email", email: "not-an-email" },
    });

    expect(res.status).toBe(400);
  });

  it("returns 409 for duplicate email", async () => {
    // First registration succeeds
    await request(app, "POST", "/api/v1/merchant/auth/register", {
      body: { name: "First", email: "dupe@example.com" },
    });

    // Second registration with same email → 409
    const res = await request(app, "POST", "/api/v1/merchant/auth/register", {
      body: { name: "Second", email: "dupe@example.com" },
    });

    expect(res.status).toBe(409);
    expect(res.body.error).toBe("Email already registered");
  });

  it("body ref takes priority over cookie", async () => {
    // Cookie has agency B, body has agency A — body should win
    const res = await request(app, "POST", "/api/v1/merchant/auth/register", {
      body: {
        name: "Priority Merchant",
        email: "priority@example.com",
        ref: AGENCY_A.referralCode, // explicit body ref
      },
      headers: { Cookie: `ghp_ref=${AGENCY_B.referralCode}` }, // cookie for agency B
    });

    expect(res.status).toBe(201);
    expect(res.body.agencyId).toBe(AGENCY_A.id); // body ref wins
  });
});

describe("PATCH /merchant/:id — immutability enforcement", () => {
  let app: express.Express;
  let mockPrisma: ReturnType<typeof createMockPrisma>;

  beforeEach(() => {
    mockPrisma = createMockPrisma({
      agencies: [AGENCY_A, AGENCY_B],
      merchants: [
        { ...EXISTING_MERCHANT },
        { ...UNATTRIBUTED_MERCHANT },
      ],
    });
    app = express();
    app.use(express.json());
    const router = createMerchantAuthRouter({
      prisma: mockPrisma as unknown as import("@prisma/client").PrismaClient,
    });
    app.use("/api/v1/merchant", router);
  });

  it("returns 409 when trying to change agencyId after it has been set", async () => {
    const res = await request(
      app,
      "PATCH",
      `/api/v1/merchant/${EXISTING_MERCHANT.id}`,
      { body: { agencyId: AGENCY_B.id } } // trying to change from A to B
    );

    expect(res.status).toBe(409);
    expect(res.body.error).toContain("IMMUTABLE");
  });

  it("allows updating agencyId when it has NOT been set yet", async () => {
    const res = await request(
      app,
      "PATCH",
      `/api/v1/merchant/${UNATTRIBUTED_MERCHANT.id}`,
      { body: { agencyId: AGENCY_A.id } }
    );

    expect(res.status).toBe(200);
  });

  it("returns 200 for valid mutable field updates (name, phone)", async () => {
    const res = await request(
      app,
      "PATCH",
      `/api/v1/merchant/${EXISTING_MERCHANT.id}`,
      { body: { name: "New Name", phone: "555-0100" } }
    );

    expect(res.status).toBe(200);
    expect(res.body.name).toBe("New Name");
  });

  it("returns 404 for unknown merchant id", async () => {
    const res = await request(app, "PATCH", "/api/v1/merchant/nonexistent-id", {
      body: { name: "Ghost" },
    });

    expect(res.status).toBe(404);
  });

  it("does not change agencyId on existing merchant when no agencyId in body", async () => {
    const res = await request(
      app,
      "PATCH",
      `/api/v1/merchant/${EXISTING_MERCHANT.id}`,
      { body: { phone: "555-9999" } }
    );

    expect(res.status).toBe(200);
    // agencyId should still be the original value
    const updated = mockPrisma._merchants.find((m) => m.id === EXISTING_MERCHANT.id);
    expect(updated?.agencyId).toBe(AGENCY_A.id);
  });
});
