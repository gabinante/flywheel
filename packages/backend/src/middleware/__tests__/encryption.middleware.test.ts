/**
 * Tests for the Prisma encryption middleware.
 *
 * These tests use a mock Prisma `next()` function to verify that:
 *   - Sensitive fields are encrypted before write operations
 *   - Sensitive fields are decrypted after read operations
 *   - Non-sensitive fields are left untouched
 *   - Non-target models are passed through unchanged
 */

import crypto from "crypto";
import { encryptionMiddleware } from "../encryption.middleware";
import { encrypt, decrypt, isEncrypted, _resetKeys } from "../../utils/encryption";

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

const ORIGINAL_ENV = { ...process.env };

beforeEach(() => {
  _resetKeys();
  process.env.ENCRYPTION_KEY = crypto.randomBytes(32).toString("hex");
  delete process.env.ENCRYPTION_KEY_PREV;
});

afterAll(() => {
  process.env = ORIGINAL_ENV;
});

// Helper: create a mock next function that captures params and returns result
function mockNext(returnValue: any = null) {
  const fn = jest.fn().mockResolvedValue(returnValue);
  return fn;
}

// ---------------------------------------------------------------------------
// Write operations — encrypt before persistence
// ---------------------------------------------------------------------------

describe("write operations (encrypt before persistence)", () => {
  it("encrypts Merchant fields on create", async () => {
    const next = mockNext({ id: "1" });

    await encryptionMiddleware(
      {
        model: "Merchant",
        action: "create",
        args: {
          data: {
            name: "Test Merchant",
            nmiSecurityKey: "sk_live_abc123",
            nmiTokenizationKey: "tok_xyz789",
            seamlesschexApiKey: "sch_key_456",
          },
        },
        dataPath: [],
        runInTransaction: false,
      },
      next
    );

    const passedParams = next.mock.calls[0][0];
    const data = passedParams.args.data;

    // Sensitive fields should be encrypted
    expect(data.nmiSecurityKey).not.toEqual("sk_live_abc123");
    expect(isEncrypted(data.nmiSecurityKey)).toBe(true);
    expect(decrypt(data.nmiSecurityKey)).toEqual("sk_live_abc123");

    expect(isEncrypted(data.nmiTokenizationKey)).toBe(true);
    expect(decrypt(data.nmiTokenizationKey)).toEqual("tok_xyz789");

    expect(isEncrypted(data.seamlesschexApiKey)).toBe(true);
    expect(decrypt(data.seamlesschexApiKey)).toEqual("sch_key_456");

    // Non-sensitive fields should be untouched
    expect(data.name).toEqual("Test Merchant");
  });

  it("encrypts Application fields on create", async () => {
    const next = mockNext({ id: "1" });

    await encryptionMiddleware(
      {
        model: "Application",
        action: "create",
        args: {
          data: {
            applicantName: "John Doe",
            ssnLast4: "1234",
            bankRoutingNumber: "021000021",
            bankAccountNumber: "9876543210",
          },
        },
        dataPath: [],
        runInTransaction: false,
      },
      next
    );

    const data = next.mock.calls[0][0].args.data;

    expect(isEncrypted(data.ssnLast4)).toBe(true);
    expect(decrypt(data.ssnLast4)).toEqual("1234");

    expect(isEncrypted(data.bankRoutingNumber)).toBe(true);
    expect(decrypt(data.bankRoutingNumber)).toEqual("021000021");

    expect(isEncrypted(data.bankAccountNumber)).toBe(true);
    expect(decrypt(data.bankAccountNumber)).toEqual("9876543210");

    // Non-sensitive
    expect(data.applicantName).toEqual("John Doe");
  });

  it("encrypts fields on update", async () => {
    const next = mockNext({ id: "1" });

    await encryptionMiddleware(
      {
        model: "Merchant",
        action: "update",
        args: {
          where: { id: "1" },
          data: {
            nmiSecurityKey: "new_key_value",
          },
        },
        dataPath: [],
        runInTransaction: false,
      },
      next
    );

    const data = next.mock.calls[0][0].args.data;
    expect(isEncrypted(data.nmiSecurityKey)).toBe(true);
    expect(decrypt(data.nmiSecurityKey)).toEqual("new_key_value");
  });

  it("encrypts fields on upsert (create and update)", async () => {
    const next = mockNext({ id: "1" });

    await encryptionMiddleware(
      {
        model: "Merchant",
        action: "upsert",
        args: {
          where: { id: "1" },
          create: {
            nmiSecurityKey: "create_key",
          },
          update: {
            nmiSecurityKey: "update_key",
          },
        },
        dataPath: [],
        runInTransaction: false,
      },
      next
    );

    const params = next.mock.calls[0][0];
    expect(isEncrypted(params.args.create.nmiSecurityKey)).toBe(true);
    expect(decrypt(params.args.create.nmiSecurityKey)).toEqual("create_key");
    expect(isEncrypted(params.args.update.nmiSecurityKey)).toBe(true);
    expect(decrypt(params.args.update.nmiSecurityKey)).toEqual("update_key");
  });

  it("skips empty string fields", async () => {
    const next = mockNext({ id: "1" });

    await encryptionMiddleware(
      {
        model: "Merchant",
        action: "create",
        args: {
          data: {
            nmiSecurityKey: "",
            nmiTokenizationKey: "valid_key",
          },
        },
        dataPath: [],
        runInTransaction: false,
      },
      next
    );

    const data = next.mock.calls[0][0].args.data;
    expect(data.nmiSecurityKey).toEqual("");
    expect(isEncrypted(data.nmiTokenizationKey)).toBe(true);
  });

  it("skips already-encrypted values (idempotency)", async () => {
    const encrypted = encrypt("already_encrypted");
    const next = mockNext({ id: "1" });

    await encryptionMiddleware(
      {
        model: "Merchant",
        action: "update",
        args: {
          data: {
            nmiSecurityKey: encrypted,
          },
        },
        dataPath: [],
        runInTransaction: false,
      },
      next
    );

    const data = next.mock.calls[0][0].args.data;
    // Should remain the same (not double-encrypted)
    expect(data.nmiSecurityKey).toEqual(encrypted);
    expect(decrypt(data.nmiSecurityKey)).toEqual("already_encrypted");
  });
});

// ---------------------------------------------------------------------------
// Read operations — decrypt after retrieval
// ---------------------------------------------------------------------------

describe("read operations (decrypt after retrieval)", () => {
  it("decrypts Merchant fields on findUnique", async () => {
    const encKey = encrypt("my_secret_key");
    const encTok = encrypt("my_token_key");
    const encSch = encrypt("my_sch_key");

    const next = mockNext({
      id: "1",
      name: "Test Merchant",
      nmiSecurityKey: encKey,
      nmiTokenizationKey: encTok,
      seamlesschexApiKey: encSch,
    });

    const result = await encryptionMiddleware(
      {
        model: "Merchant",
        action: "findUnique",
        args: { where: { id: "1" } },
        dataPath: [],
        runInTransaction: false,
      },
      next
    );

    expect(result.nmiSecurityKey).toEqual("my_secret_key");
    expect(result.nmiTokenizationKey).toEqual("my_token_key");
    expect(result.seamlesschexApiKey).toEqual("my_sch_key");
    expect(result.name).toEqual("Test Merchant"); // untouched
  });

  it("decrypts Application fields on findFirst", async () => {
    const result = await encryptionMiddleware(
      {
        model: "Application",
        action: "findFirst",
        args: {},
        dataPath: [],
        runInTransaction: false,
      },
      mockNext({
        id: "1",
        ssnLast4: encrypt("1234"),
        bankRoutingNumber: encrypt("021000021"),
        bankAccountNumber: encrypt("9876543210"),
        applicantName: "John",
      })
    );

    expect(result.ssnLast4).toEqual("1234");
    expect(result.bankRoutingNumber).toEqual("021000021");
    expect(result.bankAccountNumber).toEqual("9876543210");
    expect(result.applicantName).toEqual("John");
  });

  it("decrypts arrays on findMany", async () => {
    const result = await encryptionMiddleware(
      {
        model: "Merchant",
        action: "findMany",
        args: {},
        dataPath: [],
        runInTransaction: false,
      },
      mockNext([
        { id: "1", nmiSecurityKey: encrypt("key1"), nmiTokenizationKey: null, seamlesschexApiKey: "" },
        { id: "2", nmiSecurityKey: encrypt("key2"), nmiTokenizationKey: encrypt("tok2"), seamlesschexApiKey: encrypt("sch2") },
      ])
    );

    expect(result[0].nmiSecurityKey).toEqual("key1");
    expect(result[1].nmiSecurityKey).toEqual("key2");
    expect(result[1].nmiTokenizationKey).toEqual("tok2");
    expect(result[1].seamlesschexApiKey).toEqual("sch2");
  });

  it("handles null result gracefully", async () => {
    const result = await encryptionMiddleware(
      {
        model: "Merchant",
        action: "findUnique",
        args: { where: { id: "999" } },
        dataPath: [],
        runInTransaction: false,
      },
      mockNext(null)
    );

    expect(result).toBeNull();
  });

  it("leaves plaintext values alone on read (for migration compatibility)", async () => {
    const result = await encryptionMiddleware(
      {
        model: "Merchant",
        action: "findUnique",
        args: { where: { id: "1" } },
        dataPath: [],
        runInTransaction: false,
      },
      mockNext({
        id: "1",
        nmiSecurityKey: "plaintext_key", // not encrypted
        nmiTokenizationKey: null,
      })
    );

    // Should return plaintext as-is (isEncrypted returns false)
    expect(result.nmiSecurityKey).toEqual("plaintext_key");
  });
});

// ---------------------------------------------------------------------------
// Non-target models — pass through unchanged
// ---------------------------------------------------------------------------

describe("non-target models", () => {
  it("passes through non-encrypted models unchanged", async () => {
    const data = { id: "1", email: "test@example.com" };
    const next = mockNext(data);

    const result = await encryptionMiddleware(
      {
        model: "User",
        action: "create",
        args: { data },
        dataPath: [],
        runInTransaction: false,
      },
      next
    );

    // Data should not be modified
    expect(next.mock.calls[0][0].args.data.email).toEqual("test@example.com");
    expect(result).toEqual(data);
  });

  it("passes through when model is undefined", async () => {
    const next = mockNext({ id: "1" });

    const result = await encryptionMiddleware(
      {
        model: undefined,
        action: "findMany",
        args: {},
        dataPath: [],
        runInTransaction: false,
      } as any,
      next
    );

    expect(result).toEqual({ id: "1" });
  });
});
