/**
 * GHL OAuth Routes — Unit Tests
 *
 * Verifies the affiliate attribution acceptance criteria for the
 * GHL OAuth flow from gohighpayment-8:
 *
 * 1. GET /install with ?ref stores referralCode in GhlOAuthState
 * 2. GET /install with ?ref sets ghp_ref cookie
 * 3. GET /install without ?ref but with cookie preserves cookie ref in state
 * 4. GET /callback with valid state + referralCode attributes new merchant
 * 5. GET /callback with valid state + no referralCode: merchant created without attribution
 * 6. GET /callback: immutability — existing merchant agencyId NOT overridden
 * 7. GET /callback with invalid state → 400
 * 8. GET /callback with expired state → 400
 * 9. GET /callback: cookie fallback attribution when state has no referralCode
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import express from "express";
import {
  createGhlRouter,
  type GhlTokenResponse,
  type GhlLocationInfo,
} from "../routes/ghl.js";

// ─── Helpers ─────────────────────────────────────────────────────────

async function request(
  app: express.Express,
  method: string,
  path: string,
  opts: {
    body?: unknown;
    headers?: Record<string, string>;
    followRedirects?: boolean;
  } = {}
) {
  return new Promise<{
    status: number;
    body: Record<string, unknown>;
    headers: Record<string, string>;
    location?: string;
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
        redirect: "manual", // don't follow redirects automatically
        headers: {
          "Content-Type": "application/json",
          ...opts.headers,
        },
        body: opts.body ? JSON.stringify(opts.body) : undefined,
      })
        .then(async (res) => {
          let body: Record<string, unknown> = {};
          const contentType = res.headers.get("content-type") ?? "";
          if (contentType.includes("application/json")) {
            body = (await res.json()) as Record<string, unknown>;
          } else {
            await res.text(); // drain
          }
          const headers: Record<string, string> = {};
          res.headers.forEach((value, key) => {
            headers[key.toLowerCase()] = value;
          });
          server.close();
          resolve({
            status: res.status,
            body,
            headers,
            location: res.headers.get("location") ?? undefined,
          });
        })
        .catch((err) => {
          server.close();
          reject(err);
        });
    });
  });
}

// ─── Mock Prisma ─────────────────────────────────────────────────────

interface MockOAuthState {
  id: string;
  state: string;
  referralCode: string | null;
  createdAt: Date;
  expiresAt: Date;
}

interface MockMerchant {
  id: string;
  email: string;
  name: string;
  phone: string | null;
  ghlLocationId: string | null;
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
  oauthStates?: MockOAuthState[];
}) {
  const agencies: MockAgency[] = [...(opts.agencies ?? [])];
  const merchants: MockMerchant[] = [...(opts.merchants ?? [])];
  const oauthStates: MockOAuthState[] = [...(opts.oauthStates ?? [])];
  let idCounter = 0;

  return {
    _agencies: agencies,
    _merchants: merchants,
    _oauthStates: oauthStates,

    agency: {
      findUnique: vi.fn().mockImplementation(
        ({ where }: { where: { referralCode?: string } }) => {
          if (where.referralCode) {
            const found = agencies.find((a) => a.referralCode === where.referralCode);
            return Promise.resolve(found ? { id: found.id } : null);
          }
          return Promise.resolve(null);
        }
      ),
    },

    merchant: {
      findUnique: vi.fn().mockImplementation(
        ({
          where,
        }: {
          where: { ghlLocationId?: string; id?: string };
        }) => {
          let found: MockMerchant | undefined;
          if (where.ghlLocationId) {
            found = merchants.find((m) => m.ghlLocationId === where.ghlLocationId);
          } else if (where.id) {
            found = merchants.find((m) => m.id === where.id);
          }
          return Promise.resolve(
            found ? { id: found.id, agencyId: found.agencyId } : null
          );
        }
      ),

      create: vi.fn().mockImplementation(
        ({ data }: { data: Partial<MockMerchant>; select?: unknown }) => {
          const merchant: MockMerchant = {
            id: `m-${++idCounter}`,
            email: data.email!,
            name: data.name!,
            phone: data.phone ?? null,
            ghlLocationId: data.ghlLocationId ?? null,
            agencyId: data.agencyId ?? null,
            attributedAt: data.attributedAt ?? null,
            status: "ACTIVE",
            createdAt: new Date(),
            updatedAt: new Date(),
          };
          merchants.push(merchant);
          return Promise.resolve({ id: merchant.id });
        }
      ),

      update: vi.fn().mockImplementation(
        ({
          where,
          data,
        }: {
          where: { id: string };
          data: Partial<MockMerchant>;
        }) => {
          const idx = merchants.findIndex((m) => m.id === where.id);
          if (idx === -1) throw new Error("Record to update not found");
          merchants[idx] = { ...merchants[idx], ...data };
          return Promise.resolve({ id: merchants[idx].id });
        }
      ),
    },

    ghlOAuthState: {
      create: vi.fn().mockImplementation(
        ({ data }: { data: Partial<MockOAuthState> }) => {
          const record: MockOAuthState = {
            id: `state-${++idCounter}`,
            state: data.state!,
            referralCode: data.referralCode ?? null,
            createdAt: new Date(),
            expiresAt: data.expiresAt!,
          };
          oauthStates.push(record);
          return Promise.resolve(record);
        }
      ),

      findUnique: vi.fn().mockImplementation(
        ({ where }: { where: { state: string } }) => {
          const found = oauthStates.find((s) => s.state === where.state);
          return Promise.resolve(found ?? null);
        }
      ),

      delete: vi.fn().mockImplementation(({ where }: { where: { state: string } }) => {
        const idx = oauthStates.findIndex((s) => s.state === where.state);
        if (idx !== -1) oauthStates.splice(idx, 1);
        return Promise.resolve({});
      }),
    },
  };
}

// ─── Test Fixtures ────────────────────────────────────────────────────

const AGENCY_A: MockAgency = { id: "agency-id-a", referralCode: "agency-abc123" };
const AGENCY_B: MockAgency = { id: "agency-id-b", referralCode: "agency-def456" };

const MOCK_TOKEN_RESPONSE: GhlTokenResponse = {
  access_token: "ghl-access-token-xxx",
  refresh_token: "ghl-refresh-token-yyy",
  locationId: "loc-12345",
};

const MOCK_LOCATION: GhlLocationInfo = {
  id: "loc-12345",
  name: "Test Business",
  email: "business@example.com",
  phone: "555-1234",
};

const MOCK_EXCHANGE_GHL_CODE = vi
  .fn()
  .mockResolvedValue(MOCK_TOKEN_RESPONSE);

const MOCK_FETCH_GHL_LOCATION = vi
  .fn()
  .mockResolvedValue(MOCK_LOCATION);

// ─── Tests ────────────────────────────────────────────────────────────

describe("GET /install — initiates GHL OAuth with optional ref", () => {
  let app: express.Express;
  let mockPrisma: ReturnType<typeof createMockPrisma>;

  beforeEach(() => {
    mockPrisma = createMockPrisma({ agencies: [AGENCY_A] });
    app = express();
    const router = createGhlRouter({
      prisma: mockPrisma as unknown as import("@prisma/client").PrismaClient,
      ghlClientId: "test-client-id",
      appBaseUrl: "http://localhost:3000",
      exchangeGhlCode: MOCK_EXCHANGE_GHL_CODE,
      fetchGhlLocation: MOCK_FETCH_GHL_LOCATION,
    });
    app.use("/api/v1/ghl", router);
  });

  it("redirects to GHL OAuth URL", async () => {
    const res = await request(app, "GET", "/api/v1/ghl/install");
    expect(res.status).toBe(302);
    expect(res.location).toContain("marketplace.gohighlevel.com");
  });

  it("stores referralCode in GhlOAuthState when ?ref provided", async () => {
    await request(app, "GET", `/api/v1/ghl/install?ref=${AGENCY_A.referralCode}`);

    expect(mockPrisma.ghlOAuthState.create).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ referralCode: AGENCY_A.referralCode }),
      })
    );
  });

  it("stores null referralCode when no ?ref provided", async () => {
    await request(app, "GET", "/api/v1/ghl/install");

    expect(mockPrisma.ghlOAuthState.create).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ referralCode: null }),
      })
    );
  });

  it("sets ghp_ref cookie when ?ref is provided", async () => {
    const res = await request(
      app,
      "GET",
      `/api/v1/ghl/install?ref=${AGENCY_A.referralCode}`
    );

    const setCookie = res.headers["set-cookie"] ?? "";
    expect(setCookie).toContain("ghp_ref=");
    expect(setCookie).toContain(AGENCY_A.referralCode);
  });

  it("does NOT set ghp_ref cookie when no ?ref provided", async () => {
    const res = await request(app, "GET", "/api/v1/ghl/install");

    const setCookie = res.headers["set-cookie"] ?? "";
    expect(setCookie).not.toContain("ghp_ref=");
  });

  it("uses cookie ref when no ?ref query param but ghp_ref cookie present", async () => {
    await request(app, "GET", "/api/v1/ghl/install", {
      headers: { Cookie: `ghp_ref=${AGENCY_B.referralCode}` },
    });

    // Should store the cookie's referral code in the state
    expect(mockPrisma.ghlOAuthState.create).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({ referralCode: AGENCY_B.referralCode }),
      })
    );
  });

  it("state is included in the GHL redirect URL", async () => {
    const res = await request(app, "GET", "/api/v1/ghl/install");
    expect(res.location).toContain("state=");
  });
});

describe("GET /callback — GHL OAuth callback with attribution", () => {
  let app: express.Express;
  let mockPrisma: ReturnType<typeof createMockPrisma>;

  const VALID_STATE = "valid-state-token-abc";
  const FUTURE_EXPIRY = new Date(Date.now() + 10 * 60 * 1000); // 10 min from now

  function buildApp(overrides?: {
    exchangeGhlCode?: typeof MOCK_EXCHANGE_GHL_CODE;
    fetchGhlLocation?: typeof MOCK_FETCH_GHL_LOCATION;
  }) {
    return createGhlRouter({
      prisma: mockPrisma as unknown as import("@prisma/client").PrismaClient,
      ghlClientId: "test-client-id",
      ghlClientSecret: "test-client-secret",
      appBaseUrl: "http://localhost:3000",
      exchangeGhlCode: overrides?.exchangeGhlCode ?? MOCK_EXCHANGE_GHL_CODE,
      fetchGhlLocation: overrides?.fetchGhlLocation ?? MOCK_FETCH_GHL_LOCATION,
    });
  }

  beforeEach(() => {
    vi.clearAllMocks();
    MOCK_EXCHANGE_GHL_CODE.mockResolvedValue(MOCK_TOKEN_RESPONSE);
    MOCK_FETCH_GHL_LOCATION.mockResolvedValue(MOCK_LOCATION);
  });

  it("attributes new merchant to agency when state has referralCode", async () => {
    mockPrisma = createMockPrisma({
      agencies: [AGENCY_A],
      oauthStates: [
        {
          id: "state-1",
          state: VALID_STATE,
          referralCode: AGENCY_A.referralCode,
          createdAt: new Date(),
          expiresAt: FUTURE_EXPIRY,
        },
      ],
    });
    app = express();
    app.use("/api/v1/ghl", buildApp());

    const res = await request(
      app,
      "GET",
      `/api/v1/ghl/callback?code=auth-code&state=${VALID_STATE}`
    );

    expect(res.status).toBe(200);
    expect(res.body.success).toBe(true);
    expect(res.body.merchantId).toBeTruthy();

    // Verify merchant was created with agencyId
    const created = mockPrisma._merchants[0];
    expect(created?.agencyId).toBe(AGENCY_A.id);
    expect(created?.attributedAt).toBeTruthy();
  });

  it("creates merchant without attribution when no referralCode in state", async () => {
    mockPrisma = createMockPrisma({
      agencies: [AGENCY_A],
      oauthStates: [
        {
          id: "state-2",
          state: VALID_STATE,
          referralCode: null,
          createdAt: new Date(),
          expiresAt: FUTURE_EXPIRY,
        },
      ],
    });
    app = express();
    app.use("/api/v1/ghl", buildApp());

    const res = await request(
      app,
      "GET",
      `/api/v1/ghl/callback?code=auth-code&state=${VALID_STATE}`
    );

    expect(res.status).toBe(200);
    const created = mockPrisma._merchants[0];
    expect(created?.agencyId).toBeNull();
  });

  it("does NOT override existing merchant agencyId (immutability)", async () => {
    const existingMerchant: MockMerchant = {
      id: "existing-m",
      email: "existing@example.com",
      name: "Existing",
      phone: null,
      ghlLocationId: MOCK_TOKEN_RESPONSE.locationId,
      agencyId: AGENCY_A.id, // already attributed to A
      attributedAt: new Date("2026-01-01"),
      status: "ACTIVE",
      createdAt: new Date(),
      updatedAt: new Date(),
    };
    mockPrisma = createMockPrisma({
      agencies: [AGENCY_A, AGENCY_B],
      merchants: [existingMerchant],
      oauthStates: [
        {
          id: "state-3",
          state: VALID_STATE,
          referralCode: AGENCY_B.referralCode, // trying to attribute to B
          createdAt: new Date(),
          expiresAt: FUTURE_EXPIRY,
        },
      ],
    });
    app = express();
    app.use("/api/v1/ghl", buildApp());

    const res = await request(
      app,
      "GET",
      `/api/v1/ghl/callback?code=auth-code&state=${VALID_STATE}`
    );

    expect(res.status).toBe(200);
    // The existing merchant's agencyId should NOT have been changed
    expect(mockPrisma.merchant.update).not.toHaveBeenCalled();
    const merchant = mockPrisma._merchants[0];
    expect(merchant?.agencyId).toBe(AGENCY_A.id); // still A, not B
  });

  it("returns 400 for unknown state token", async () => {
    mockPrisma = createMockPrisma({ agencies: [AGENCY_A] });
    app = express();
    app.use("/api/v1/ghl", buildApp());

    const res = await request(
      app,
      "GET",
      "/api/v1/ghl/callback?code=auth-code&state=unknown-state"
    );

    expect(res.status).toBe(400);
    expect(res.body.error).toContain("Invalid or expired");
  });

  it("returns 400 for expired state token", async () => {
    const pastExpiry = new Date(Date.now() - 1000); // already expired
    mockPrisma = createMockPrisma({
      agencies: [AGENCY_A],
      oauthStates: [
        {
          id: "state-4",
          state: VALID_STATE,
          referralCode: null,
          createdAt: new Date(),
          expiresAt: pastExpiry,
        },
      ],
    });
    app = express();
    app.use("/api/v1/ghl", buildApp());

    const res = await request(
      app,
      "GET",
      `/api/v1/ghl/callback?code=auth-code&state=${VALID_STATE}`
    );

    expect(res.status).toBe(400);
    expect(res.body.error).toContain("expired");
  });

  it("returns 400 when required query params are missing", async () => {
    mockPrisma = createMockPrisma({ agencies: [AGENCY_A] });
    app = express();
    app.use("/api/v1/ghl", buildApp());

    // Missing state
    const res = await request(app, "GET", "/api/v1/ghl/callback?code=auth-code");
    expect(res.status).toBe(400);
  });

  it("returns 502 when GHL token exchange fails", async () => {
    const failingExchange = vi
      .fn()
      .mockRejectedValue(new Error("GHL token exchange failed"));

    mockPrisma = createMockPrisma({
      agencies: [AGENCY_A],
      oauthStates: [
        {
          id: "state-5",
          state: VALID_STATE,
          referralCode: null,
          createdAt: new Date(),
          expiresAt: FUTURE_EXPIRY,
        },
      ],
    });
    app = express();
    app.use(
      "/api/v1/ghl",
      buildApp({ exchangeGhlCode: failingExchange as typeof MOCK_EXCHANGE_GHL_CODE })
    );

    const res = await request(
      app,
      "GET",
      `/api/v1/ghl/callback?code=bad-code&state=${VALID_STATE}`
    );

    expect(res.status).toBe(502);
    expect(res.body.error).toContain("Failed to exchange OAuth code");
  });

  it("cleans up used state record after successful callback", async () => {
    mockPrisma = createMockPrisma({
      agencies: [AGENCY_A],
      oauthStates: [
        {
          id: "state-6",
          state: VALID_STATE,
          referralCode: AGENCY_A.referralCode,
          createdAt: new Date(),
          expiresAt: FUTURE_EXPIRY,
        },
      ],
    });
    app = express();
    app.use("/api/v1/ghl", buildApp());

    await request(
      app,
      "GET",
      `/api/v1/ghl/callback?code=auth-code&state=${VALID_STATE}`
    );

    // State record should be deleted after use
    expect(mockPrisma.ghlOAuthState.delete).toHaveBeenCalledWith(
      expect.objectContaining({ where: { state: VALID_STATE } })
    );
    expect(mockPrisma._oauthStates).toHaveLength(0);
  });

  it("attributes merchant via cookie fallback when state has no referralCode", async () => {
    mockPrisma = createMockPrisma({
      agencies: [AGENCY_B],
      oauthStates: [
        {
          id: "state-7",
          state: VALID_STATE,
          referralCode: null, // no code in state
          createdAt: new Date(),
          expiresAt: FUTURE_EXPIRY,
        },
      ],
    });
    app = express();
    app.use("/api/v1/ghl", buildApp());

    // Pass the ghp_ref cookie in the callback request
    const res = await request(
      app,
      "GET",
      `/api/v1/ghl/callback?code=auth-code&state=${VALID_STATE}`,
      { headers: { Cookie: `ghp_ref=${AGENCY_B.referralCode}` } }
    );

    expect(res.status).toBe(200);
    const created = mockPrisma._merchants[0];
    expect(created?.agencyId).toBe(AGENCY_B.id);
  });

  it("sets ghp_ref cookie in response when referral code is resolved", async () => {
    mockPrisma = createMockPrisma({
      agencies: [AGENCY_A],
      oauthStates: [
        {
          id: "state-8",
          state: VALID_STATE,
          referralCode: AGENCY_A.referralCode,
          createdAt: new Date(),
          expiresAt: FUTURE_EXPIRY,
        },
      ],
    });
    app = express();
    app.use("/api/v1/ghl", buildApp());

    const res = await request(
      app,
      "GET",
      `/api/v1/ghl/callback?code=auth-code&state=${VALID_STATE}`
    );

    const setCookie = res.headers["set-cookie"] ?? "";
    expect(setCookie).toContain("ghp_ref=");
    expect(setCookie).toContain(AGENCY_A.referralCode);
  });
});
