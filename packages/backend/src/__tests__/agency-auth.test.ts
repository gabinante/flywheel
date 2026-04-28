import { describe, it, expect, beforeAll, afterAll, beforeEach } from 'vitest';
import { FastifyInstance } from 'fastify';
import { buildApp } from '../index';

const TEST_JWT_SECRET = 'test-secret-key-that-is-long-enough-for-hs256-algorithm';

// ─── Mock Prisma ──────────────────────────────────────────────────────
// We mock Prisma to avoid needing a real database for unit tests.

const mockAgencies = new Map<string, any>();
let idCounter = 0;

function resetMockDb() {
  mockAgencies.clear();
  idCounter = 0;
}

function createMockAgency(data: any) {
  const id = `agency-${++idCounter}`;
  const agency = {
    id,
    name: data.name,
    contactEmail: data.contactEmail,
    passwordHash: data.passwordHash,
    contactPhone: data.contactPhone || null,
    referralCode: data.referralCode,
    referredByAgencyId: data.referredByAgencyId || null,
    tier: 'TIER_1',
    tierOverride: false,
    status: 'ACTIVE',
    payoutEmail: data.payoutEmail || null,
    payoutBankLast4: data.payoutBankLast4 || null,
    taxIdOnFile: false,
    createdAt: new Date(),
    updatedAt: new Date(),
  };
  mockAgencies.set(id, agency);
  return agency;
}

// Mock the prisma module
import { vi } from 'vitest';
vi.mock('../utils/prisma', () => {
  return {
    default: {
      agency: {
        findUnique: vi.fn(async ({ where, include }: any) => {
          let agency: any = null;
          if (where.contactEmail) {
            agency = Array.from(mockAgencies.values()).find(
              (a) => a.contactEmail === where.contactEmail
            );
          } else if (where.referralCode) {
            agency = Array.from(mockAgencies.values()).find(
              (a) => a.referralCode === where.referralCode
            );
          } else if (where.id) {
            agency = mockAgencies.get(where.id);
          }
          if (agency && include?.referredByAgency) {
            const referrer = agency.referredByAgencyId
              ? mockAgencies.get(agency.referredByAgencyId)
              : null;
            return {
              ...agency,
              referredByAgency: referrer ? { name: referrer.name } : null,
            };
          }
          return agency || null;
        }),
        create: vi.fn(async ({ data }: any) => {
          return createMockAgency(data);
        }),
        update: vi.fn(async ({ where, data, include }: any) => {
          const agency = mockAgencies.get(where.id);
          if (!agency) throw new Error('Agency not found');
          Object.assign(agency, data, { updatedAt: new Date() });
          mockAgencies.set(where.id, agency);
          if (include?.referredByAgency) {
            const referrer = agency.referredByAgencyId
              ? mockAgencies.get(agency.referredByAgencyId)
              : null;
            return {
              ...agency,
              referredByAgency: referrer ? { name: referrer.name } : null,
            };
          }
          return agency;
        }),
      },
    },
  };
});

// ─── Tests ────────────────────────────────────────────────────────────

describe('Agency Auth & Profile API', () => {
  let app: FastifyInstance;

  beforeAll(async () => {
    app = await buildApp({ logger: false, jwtSecret: TEST_JWT_SECRET });
    await app.ready();
  });

  afterAll(async () => {
    await app.close();
  });

  beforeEach(() => {
    resetMockDb();
  });

  // ─── Registration Tests ───────────────────────────────────────────

  describe('POST /api/v1/agency/auth/register', () => {
    it('should register a new agency and return JWT', async () => {
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Agency Alpha',
          contactEmail: 'alpha@example.com',
          password: 'securepassword123',
        },
      });

      expect(res.statusCode).toBe(201);
      const body = JSON.parse(res.payload);
      expect(body.agency).toBeDefined();
      expect(body.agency.name).toBe('Agency Alpha');
      expect(body.agency.contactEmail).toBe('alpha@example.com');
      expect(body.agency.referralCode).toBeDefined();
      expect(body.accessToken).toBeDefined();
      expect(body.refreshToken).toBeDefined();
      expect(body.expiresIn).toBe(900);
    });

    it('should register with optional contactPhone', async () => {
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Agency Beta',
          contactEmail: 'beta@example.com',
          password: 'securepassword123',
          contactPhone: '+1234567890',
        },
      });

      expect(res.statusCode).toBe(201);
      const body = JSON.parse(res.payload);
      expect(body.agency).toBeDefined();
    });

    it('should register with valid ref and set referredByAgencyId', async () => {
      // First register agency A
      const resA = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Agency A',
          contactEmail: 'a@example.com',
          password: 'securepassword123',
        },
      });
      expect(resA.statusCode).toBe(201);
      const agencyA = JSON.parse(resA.payload);

      // Register agency B with ref to agency A
      const resB = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Agency B',
          contactEmail: 'b@example.com',
          password: 'securepassword123',
          ref: agencyA.agency.referralCode,
        },
      });
      expect(resB.statusCode).toBe(201);
      const agencyB = JSON.parse(resB.payload);
      expect(agencyB.agency).toBeDefined();

      // Verify the referral by checking the profile
      const profileRes = await app.inject({
        method: 'GET',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${agencyB.accessToken}`,
        },
      });
      expect(profileRes.statusCode).toBe(200);
      const profile = JSON.parse(profileRes.payload);
      expect(profile.referredByAgency).toEqual({ name: 'Agency A' });
    });

    it('should register with invalid ref and set referredByAgencyId to null', async () => {
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Agency C',
          contactEmail: 'c@example.com',
          password: 'securepassword123',
          ref: 'invalid-referral-code',
        },
      });

      expect(res.statusCode).toBe(201);
      const body = JSON.parse(res.payload);
      expect(body.agency).toBeDefined();

      // Verify no referrer
      const profileRes = await app.inject({
        method: 'GET',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${body.accessToken}`,
        },
      });
      expect(profileRes.statusCode).toBe(200);
      const profile = JSON.parse(profileRes.payload);
      expect(profile.referredByAgency).toBeNull();
    });

    it('should reject duplicate email', async () => {
      // Register first
      await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Agency First',
          contactEmail: 'same@example.com',
          password: 'securepassword123',
        },
      });

      // Try to register with same email
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Agency Duplicate',
          contactEmail: 'same@example.com',
          password: 'securepassword123',
        },
      });

      expect(res.statusCode).toBe(409);
      const body = JSON.parse(res.payload);
      expect(body.error).toBe('Conflict');
    });

    it('should reject invalid email format', async () => {
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Agency Bad',
          contactEmail: 'not-an-email',
          password: 'securepassword123',
        },
      });

      expect(res.statusCode).toBe(400);
    });

    it('should reject short password', async () => {
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Agency Bad',
          contactEmail: 'short@example.com',
          password: 'short',
        },
      });

      expect(res.statusCode).toBe(400);
    });

    it('should reject missing name', async () => {
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          contactEmail: 'noname@example.com',
          password: 'securepassword123',
        },
      });

      expect(res.statusCode).toBe(400);
    });
  });

  // ─── Login Tests ──────────────────────────────────────────────────

  describe('POST /api/v1/agency/auth/login', () => {
    it('should login with correct credentials', async () => {
      // Register first
      await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Login Test Agency',
          contactEmail: 'login@example.com',
          password: 'securepassword123',
        },
      });

      // Login
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/login',
        payload: {
          email: 'login@example.com',
          password: 'securepassword123',
        },
      });

      expect(res.statusCode).toBe(200);
      const body = JSON.parse(res.payload);
      expect(body.accessToken).toBeDefined();
      expect(body.refreshToken).toBeDefined();
      expect(body.expiresIn).toBe(900);
      expect(body.agency.name).toBe('Login Test Agency');
    });

    it('should reject incorrect password', async () => {
      await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Wrong Pass Agency',
          contactEmail: 'wrongpass@example.com',
          password: 'correctpassword123',
        },
      });

      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/login',
        payload: {
          email: 'wrongpass@example.com',
          password: 'incorrectpassword123',
        },
      });

      expect(res.statusCode).toBe(401);
    });

    it('should reject non-existent email', async () => {
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/login',
        payload: {
          email: 'nonexistent@example.com',
          password: 'securepassword123',
        },
      });

      expect(res.statusCode).toBe(401);
    });

    it('should return 403 for SUSPENDED agency', async () => {
      // Register
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Suspended Agency',
          contactEmail: 'suspended@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      // Manually suspend the agency in the mock
      const agency = mockAgencies.get(regBody.agency.id);
      if (agency) agency.status = 'SUSPENDED';

      // Try to login
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/login',
        payload: {
          email: 'suspended@example.com',
          password: 'securepassword123',
        },
      });

      expect(res.statusCode).toBe(403);
      const body = JSON.parse(res.payload);
      expect(body.message).toContain('suspended');
    });

    it('should return 403 for CHURNED agency', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Churned Agency',
          contactEmail: 'churned@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const agency = mockAgencies.get(regBody.agency.id);
      if (agency) agency.status = 'CHURNED';

      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/login',
        payload: {
          email: 'churned@example.com',
          password: 'securepassword123',
        },
      });

      expect(res.statusCode).toBe(403);
    });
  });

  // ─── Refresh Token Tests ──────────────────────────────────────────

  describe('POST /api/v1/agency/auth/refresh', () => {
    it('should issue new tokens with valid refresh token', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Refresh Test',
          contactEmail: 'refresh@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/refresh',
        payload: {
          refreshToken: regBody.refreshToken,
        },
      });

      expect(res.statusCode).toBe(200);
      const body = JSON.parse(res.payload);
      expect(body.accessToken).toBeDefined();
      expect(body.refreshToken).toBeDefined();
      expect(body.expiresIn).toBe(900);
    });

    it('should reject invalid refresh token', async () => {
      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/refresh',
        payload: {
          refreshToken: 'invalid-token',
        },
      });

      expect(res.statusCode).toBe(401);
    });

    it('should reject access token used as refresh token', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Access As Refresh',
          contactEmail: 'accessrefresh@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const res = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/refresh',
        payload: {
          refreshToken: regBody.accessToken, // using access token instead of refresh
        },
      });

      expect(res.statusCode).toBe(401);
    });
  });

  // ─── Profile Tests ────────────────────────────────────────────────

  describe('GET /api/v1/agency/profile', () => {
    it('should return agency profile with valid token', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Profile Test Agency',
          contactEmail: 'profile@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const res = await app.inject({
        method: 'GET',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${regBody.accessToken}`,
        },
      });

      expect(res.statusCode).toBe(200);
      const body = JSON.parse(res.payload);
      expect(body.id).toBeDefined();
      expect(body.name).toBe('Profile Test Agency');
      expect(body.contactEmail).toBe('profile@example.com');
      expect(body.referralCode).toBeDefined();
      expect(body.tier).toBe('TIER_1');
      expect(body.status).toBe('ACTIVE');
      expect(body.createdAt).toBeDefined();
    });

    it('should return 401 without token', async () => {
      const res = await app.inject({
        method: 'GET',
        url: '/api/v1/agency/profile',
      });

      expect(res.statusCode).toBe(401);
    });

    it('should reject merchant JWT (wrong type claim)', async () => {
      // Create a token with type: 'merchant' instead of 'agency'
      const merchantToken = app.jwt.sign(
        { sub: 'merchant-123', type: 'merchant' },
        { expiresIn: '15m' }
      );

      const res = await app.inject({
        method: 'GET',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${merchantToken}`,
        },
      });

      expect(res.statusCode).toBe(401);
      const body = JSON.parse(res.payload);
      expect(body.message).toContain('Invalid token type');
    });

    it('should reject admin JWT (wrong type claim)', async () => {
      const adminToken = app.jwt.sign(
        { sub: 'admin-123', type: 'admin' },
        { expiresIn: '15m' }
      );

      const res = await app.inject({
        method: 'GET',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${adminToken}`,
        },
      });

      expect(res.statusCode).toBe(401);
    });

    it('should reject refresh token used as access token', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Refresh As Access',
          contactEmail: 'refreshaccess@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const res = await app.inject({
        method: 'GET',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${regBody.refreshToken}`,
        },
      });

      expect(res.statusCode).toBe(401);
    });
  });

  // ─── Profile Update Tests ─────────────────────────────────────────

  describe('PATCH /api/v1/agency/profile', () => {
    it('should update contactPhone', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Update Phone Agency',
          contactEmail: 'phone@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const res = await app.inject({
        method: 'PATCH',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${regBody.accessToken}`,
        },
        payload: {
          contactPhone: '+1987654321',
        },
      });

      expect(res.statusCode).toBe(200);
      const body = JSON.parse(res.payload);
      expect(body.contactPhone).toBe('+1987654321');
    });

    it('should update payoutEmail', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Update Payout Agency',
          contactEmail: 'payout@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const res = await app.inject({
        method: 'PATCH',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${regBody.accessToken}`,
        },
        payload: {
          payoutEmail: 'payout-bank@example.com',
        },
      });

      expect(res.statusCode).toBe(200);
      const body = JSON.parse(res.payload);
      expect(body.payoutEmail).toBe('payout-bank@example.com');
    });

    it('should update payoutBankLast4', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Update Bank Agency',
          contactEmail: 'bank@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const res = await app.inject({
        method: 'PATCH',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${regBody.accessToken}`,
        },
        payload: {
          payoutBankLast4: '4321',
        },
      });

      expect(res.statusCode).toBe(200);
      const body = JSON.parse(res.payload);
      expect(body.payoutBankLast4).toBe('4321');
    });

    it('should reject changing contactEmail (immutable)', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Immutable Email Agency',
          contactEmail: 'immutable@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const res = await app.inject({
        method: 'PATCH',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${regBody.accessToken}`,
        },
        payload: {
          contactEmail: 'new@example.com',
        },
      });

      expect(res.statusCode).toBe(400);
      const body = JSON.parse(res.payload);
      expect(body.message).toContain('immutable');
      expect(body.message).toContain('contactEmail');
    });

    it('should reject changing referralCode (immutable)', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Immutable Code Agency',
          contactEmail: 'immutablecode@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const res = await app.inject({
        method: 'PATCH',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${regBody.accessToken}`,
        },
        payload: {
          referralCode: 'agency-hacked123',
        },
      });

      expect(res.statusCode).toBe(400);
      const body = JSON.parse(res.payload);
      expect(body.message).toContain('immutable');
      expect(body.message).toContain('referralCode');
    });

    it('should reject changing tier (immutable)', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Immutable Tier Agency',
          contactEmail: 'immutabletier@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const res = await app.inject({
        method: 'PATCH',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${regBody.accessToken}`,
        },
        payload: {
          tier: 'TIER_3',
        },
      });

      expect(res.statusCode).toBe(400);
      const body = JSON.parse(res.payload);
      expect(body.message).toContain('immutable');
      expect(body.message).toContain('tier');
    });

    it('should reject changing referredByAgencyId (immutable)', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Immutable Referrer Agency',
          contactEmail: 'immutableref@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const res = await app.inject({
        method: 'PATCH',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${regBody.accessToken}`,
        },
        payload: {
          referredByAgencyId: 'some-other-agency-id',
        },
      });

      expect(res.statusCode).toBe(400);
      const body = JSON.parse(res.payload);
      expect(body.message).toContain('immutable');
    });

    it('should reject invalid payoutBankLast4 format', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'Bad Bank Agency',
          contactEmail: 'badbank@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const res = await app.inject({
        method: 'PATCH',
        url: '/api/v1/agency/profile',
        headers: {
          authorization: `Bearer ${regBody.accessToken}`,
        },
        payload: {
          payoutBankLast4: '12345', // 5 digits instead of 4
        },
      });

      expect(res.statusCode).toBe(400);
    });

    it('should return 401 without token', async () => {
      const res = await app.inject({
        method: 'PATCH',
        url: '/api/v1/agency/profile',
        payload: {
          contactPhone: '+1111111111',
        },
      });

      expect(res.statusCode).toBe(401);
    });
  });

  // ─── JWT Type Claim Tests ─────────────────────────────────────────

  describe('JWT type claim verification', () => {
    it('access token should contain type: agency', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'JWT Type Agency',
          contactEmail: 'jwttype@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      // Decode the token (without verification) to inspect claims
      const decoded = app.jwt.decode(regBody.accessToken) as any;
      expect(decoded.type).toBe('agency');
      expect(decoded.sub).toBeDefined();
    });

    it('refresh token should contain type: agency and tokenType: refresh', async () => {
      const regRes = await app.inject({
        method: 'POST',
        url: '/api/v1/agency/auth/register',
        payload: {
          name: 'JWT Refresh Type',
          contactEmail: 'jwtrefresh@example.com',
          password: 'securepassword123',
        },
      });
      const regBody = JSON.parse(regRes.payload);

      const decoded = app.jwt.decode(regBody.refreshToken) as any;
      expect(decoded.type).toBe('agency');
      expect(decoded.tokenType).toBe('refresh');
    });
  });

  // ─── Health Check ─────────────────────────────────────────────────

  describe('GET /health', () => {
    it('should return ok status', async () => {
      const res = await app.inject({
        method: 'GET',
        url: '/health',
      });

      expect(res.statusCode).toBe(200);
      const body = JSON.parse(res.payload);
      expect(body.status).toBe('ok');
    });
  });
});
