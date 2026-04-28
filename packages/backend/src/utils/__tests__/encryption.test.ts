import crypto from "crypto";
import {
  encrypt,
  decrypt,
  isEncrypted,
  validateEncryptionKey,
  reEncrypt,
  _resetKeys,
} from "../encryption";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Generate a random 32-byte hex key */
function randomHexKey(): string {
  return crypto.randomBytes(32).toString("hex");
}

/** Generate a random 32-byte base64 key */
function randomBase64Key(): string {
  return crypto.randomBytes(32).toString("base64");
}

// ---------------------------------------------------------------------------
// Setup / Teardown
// ---------------------------------------------------------------------------

const ORIGINAL_ENV = { ...process.env };

beforeEach(() => {
  _resetKeys();
  // Set a valid default key for most tests
  process.env.ENCRYPTION_KEY = randomHexKey();
  delete process.env.ENCRYPTION_KEY_PREV;
});

afterAll(() => {
  process.env = ORIGINAL_ENV;
});

// ---------------------------------------------------------------------------
// encrypt / decrypt round-trip
// ---------------------------------------------------------------------------

describe("encrypt / decrypt round-trip", () => {
  it("round-trips a normal string", () => {
    const original = "sk_live_abc123XYZ";
    const encrypted = encrypt(original);
    expect(encrypted).not.toEqual(original);
    expect(decrypt(encrypted)).toEqual(original);
  });

  it("round-trips a short string", () => {
    const original = "a";
    expect(decrypt(encrypt(original))).toEqual(original);
  });

  it("round-trips a number-as-string", () => {
    const original = "123456789";
    expect(decrypt(encrypt(original))).toEqual(original);
  });

  it("round-trips an empty string", () => {
    const original = "";
    expect(decrypt(encrypt(original))).toEqual(original);
  });

  it("round-trips a long string", () => {
    const original = "A".repeat(10_000);
    expect(decrypt(encrypt(original))).toEqual(original);
  });

  it("round-trips unicode content", () => {
    const original = "Hechos y datos: 42 \u2014 \u00e9\u00e8\u00ea\u00f1";
    expect(decrypt(encrypt(original))).toEqual(original);
  });

  it("round-trips SSN-like values", () => {
    const original = "1234";
    expect(decrypt(encrypt(original))).toEqual(original);
  });

  it("round-trips bank routing number", () => {
    const original = "021000021";
    expect(decrypt(encrypt(original))).toEqual(original);
  });

  it("round-trips bank account number", () => {
    const original = "9876543210";
    expect(decrypt(encrypt(original))).toEqual(original);
  });

  it("produces different ciphertexts for the same plaintext (unique IV)", () => {
    const original = "same-plaintext";
    const a = encrypt(original);
    const b = encrypt(original);
    expect(a).not.toEqual(b); // different IVs
    expect(decrypt(a)).toEqual(original);
    expect(decrypt(b)).toEqual(original);
  });
});

// ---------------------------------------------------------------------------
// Encrypted envelope structure
// ---------------------------------------------------------------------------

describe("encrypted envelope structure", () => {
  it("produces a base64-encoded JSON with required fields", () => {
    const encrypted = encrypt("test");
    const json = Buffer.from(encrypted, "base64").toString("utf8");
    const envelope = JSON.parse(json);

    expect(envelope).toHaveProperty("v");
    expect(envelope).toHaveProperty("iv");
    expect(envelope).toHaveProperty("ciphertext");
    expect(envelope).toHaveProperty("tag");
    expect(typeof envelope.v).toBe("number");
    expect(typeof envelope.iv).toBe("string");
    expect(typeof envelope.ciphertext).toBe("string");
    expect(typeof envelope.tag).toBe("string");
  });

  it("uses version 2 for current key", () => {
    const encrypted = encrypt("test");
    const json = Buffer.from(encrypted, "base64").toString("utf8");
    const envelope = JSON.parse(json);
    expect(envelope.v).toBe(2);
  });

  it("IV is 16 bytes (32 hex chars)", () => {
    const encrypted = encrypt("test");
    const json = Buffer.from(encrypted, "base64").toString("utf8");
    const envelope = JSON.parse(json);
    expect(envelope.iv).toHaveLength(32);
  });
});

// ---------------------------------------------------------------------------
// isEncrypted
// ---------------------------------------------------------------------------

describe("isEncrypted", () => {
  it("returns true for an encrypted value", () => {
    const encrypted = encrypt("secret");
    expect(isEncrypted(encrypted)).toBe(true);
  });

  it("returns false for plaintext", () => {
    expect(isEncrypted("sk_live_abc123")).toBe(false);
  });

  it("returns false for empty string", () => {
    expect(isEncrypted("")).toBe(false);
  });

  it("returns false for short strings", () => {
    expect(isEncrypted("abc")).toBe(false);
  });

  it("returns false for random base64 that is not a valid envelope", () => {
    expect(isEncrypted(Buffer.from("just some text").toString("base64"))).toBe(
      false
    );
  });

  it("returns false for null/undefined-ish values", () => {
    // @ts-expect-error testing runtime behavior
    expect(isEncrypted(null)).toBe(false);
    // @ts-expect-error testing runtime behavior
    expect(isEncrypted(undefined)).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Key validation
// ---------------------------------------------------------------------------

describe("validateEncryptionKey", () => {
  it("succeeds with a valid hex key", () => {
    process.env.ENCRYPTION_KEY = randomHexKey();
    _resetKeys();
    expect(() => validateEncryptionKey()).not.toThrow();
  });

  it("succeeds with a valid base64 key", () => {
    process.env.ENCRYPTION_KEY = randomBase64Key();
    _resetKeys();
    expect(() => validateEncryptionKey()).not.toThrow();
  });

  it("throws when ENCRYPTION_KEY is missing", () => {
    delete process.env.ENCRYPTION_KEY;
    _resetKeys();
    expect(() => validateEncryptionKey()).toThrow(
      /ENCRYPTION_KEY environment variable is required/
    );
  });

  it("throws when ENCRYPTION_KEY is wrong length (too short hex)", () => {
    process.env.ENCRYPTION_KEY = "abcd1234"; // only 4 bytes
    _resetKeys();
    expect(() => validateEncryptionKey()).toThrow(/must be exactly 32 bytes/);
  });

  it("throws when ENCRYPTION_KEY is wrong length (too long hex)", () => {
    process.env.ENCRYPTION_KEY = "a".repeat(128); // 64 bytes
    _resetKeys();
    expect(() => validateEncryptionKey()).toThrow(/must be exactly 32 bytes/);
  });
});

// ---------------------------------------------------------------------------
// Key rotation
// ---------------------------------------------------------------------------

describe("key rotation", () => {
  it("decrypts v1 values with ENCRYPTION_KEY_PREV", () => {
    // Encrypt with key A (as current)
    const keyA = randomHexKey();
    const keyB = randomHexKey();

    process.env.ENCRYPTION_KEY = keyA;
    _resetKeys();
    const encrypted = encrypt("rotated-secret");

    // Now rotate: keyB is current, keyA is previous
    // But the encrypted value has v:2 from keyA.
    // We need to simulate a v1 value by manually creating one.
    const iv = crypto.randomBytes(16);
    const cipher = crypto.createCipheriv(
      "aes-256-gcm",
      Buffer.from(keyA, "hex"),
      iv
    );
    const enc = Buffer.concat([
      cipher.update("old-secret", "utf8"),
      cipher.final(),
    ]);
    const tag = cipher.getAuthTag();

    const v1Envelope = Buffer.from(
      JSON.stringify({
        v: 1,
        iv: iv.toString("hex"),
        ciphertext: enc.toString("hex"),
        tag: tag.toString("hex"),
      })
    ).toString("base64");

    // Set keyB as current, keyA as previous
    process.env.ENCRYPTION_KEY = keyB;
    process.env.ENCRYPTION_KEY_PREV = keyA;
    _resetKeys();

    expect(decrypt(v1Envelope)).toEqual("old-secret");
  });

  it("decrypts v2 values with current ENCRYPTION_KEY during rotation", () => {
    const keyA = randomHexKey();
    const keyB = randomHexKey();

    // Encrypt with keyB (current)
    process.env.ENCRYPTION_KEY = keyB;
    _resetKeys();
    const encrypted = encrypt("new-secret");

    // Simulate rotation: keyB is still current, keyA is previous
    process.env.ENCRYPTION_KEY = keyB;
    process.env.ENCRYPTION_KEY_PREV = keyA;
    _resetKeys();

    expect(decrypt(encrypted)).toEqual("new-secret");
  });

  it("reEncrypt converts v1 to v2", () => {
    const keyA = randomHexKey();
    const keyB = randomHexKey();

    // Create a v1 envelope with keyA
    const iv = crypto.randomBytes(16);
    const cipher = crypto.createCipheriv(
      "aes-256-gcm",
      Buffer.from(keyA, "hex"),
      iv
    );
    const enc = Buffer.concat([
      cipher.update("rotate-me", "utf8"),
      cipher.final(),
    ]);
    const tag = cipher.getAuthTag();

    const v1Envelope = Buffer.from(
      JSON.stringify({
        v: 1,
        iv: iv.toString("hex"),
        ciphertext: enc.toString("hex"),
        tag: tag.toString("hex"),
      })
    ).toString("base64");

    // Set keyB as current, keyA as previous
    process.env.ENCRYPTION_KEY = keyB;
    process.env.ENCRYPTION_KEY_PREV = keyA;
    _resetKeys();

    const reEncrypted = reEncrypt(v1Envelope);

    // Should now be v2
    const json = Buffer.from(reEncrypted, "base64").toString("utf8");
    const envelope = JSON.parse(json);
    expect(envelope.v).toBe(2);

    // Should decrypt correctly with just keyB
    delete process.env.ENCRYPTION_KEY_PREV;
    _resetKeys();
    expect(decrypt(reEncrypted)).toEqual("rotate-me");
  });

  it("falls back to current key for v1 when no previous key", () => {
    const key = randomHexKey();
    process.env.ENCRYPTION_KEY = key;
    _resetKeys();

    // Create v1 envelope with same key
    const iv = crypto.randomBytes(16);
    const cipher = crypto.createCipheriv(
      "aes-256-gcm",
      Buffer.from(key, "hex"),
      iv
    );
    const enc = Buffer.concat([
      cipher.update("fallback-test", "utf8"),
      cipher.final(),
    ]);
    const tag = cipher.getAuthTag();

    const v1Envelope = Buffer.from(
      JSON.stringify({
        v: 1,
        iv: iv.toString("hex"),
        ciphertext: enc.toString("hex"),
        tag: tag.toString("hex"),
      })
    ).toString("base64");

    expect(decrypt(v1Envelope)).toEqual("fallback-test");
  });
});

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

describe("error handling", () => {
  it("throws on tampered ciphertext", () => {
    const encrypted = encrypt("tamper-test");
    const json = Buffer.from(encrypted, "base64").toString("utf8");
    const envelope = JSON.parse(json);

    // Flip a byte in the ciphertext
    const bytes = Buffer.from(envelope.ciphertext, "hex");
    bytes[0] = bytes[0] ^ 0xff;
    envelope.ciphertext = bytes.toString("hex");

    const tampered = Buffer.from(JSON.stringify(envelope)).toString("base64");
    expect(() => decrypt(tampered)).toThrow();
  });

  it("throws on tampered tag", () => {
    const encrypted = encrypt("tag-test");
    const json = Buffer.from(encrypted, "base64").toString("utf8");
    const envelope = JSON.parse(json);

    envelope.tag = "00".repeat(16);
    const tampered = Buffer.from(JSON.stringify(envelope)).toString("base64");
    expect(() => decrypt(tampered)).toThrow();
  });

  it("throws on invalid base64 input to decrypt", () => {
    expect(() => decrypt("not-valid-base64!!!")).toThrow(
      /Failed to parse encrypted value/
    );
  });

  it("throws on malformed envelope (missing fields)", () => {
    const bad = Buffer.from(JSON.stringify({ v: 2 })).toString("base64");
    expect(() => decrypt(bad)).toThrow(/missing required fields/);
  });

  it("throws on unknown key version", () => {
    const encrypted = encrypt("version-test");
    const json = Buffer.from(encrypted, "base64").toString("utf8");
    const envelope = JSON.parse(json);
    envelope.v = 99;
    const bad = Buffer.from(JSON.stringify(envelope)).toString("base64");
    expect(() => decrypt(bad)).toThrow(/Unknown encryption key version/);
  });

  it("throws when decrypting with wrong key", () => {
    const encrypted = encrypt("wrong-key-test");

    // Switch to a different key
    process.env.ENCRYPTION_KEY = randomHexKey();
    _resetKeys();

    expect(() => decrypt(encrypted)).toThrow();
  });
});
