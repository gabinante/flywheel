/**
 * Field-level AES-256-GCM encryption utility.
 *
 * Environment variables:
 *   ENCRYPTION_KEY      — current key, hex-encoded (64 chars = 32 bytes)
 *   ENCRYPTION_KEY_PREV — previous key for rotation (optional)
 *
 * Encrypted values are stored as base64-encoded JSON:
 *   { v: <key_version>, iv: <hex>, ciphertext: <hex>, tag: <hex> }
 *
 * Key version 1 = ENCRYPTION_KEY_PREV, version 2 = ENCRYPTION_KEY (current).
 * When ENCRYPTION_KEY_PREV is not set, version 1 also uses ENCRYPTION_KEY.
 */

import crypto from 'node:crypto';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const ALGORITHM = 'aes-256-gcm';
const IV_LENGTH = 16; // 128-bit IV
const KEY_LENGTH = 32; // 256-bit key
const CURRENT_VERSION = 2;

// ---------------------------------------------------------------------------
// Key management
// ---------------------------------------------------------------------------

/**
 * Parse a hex-encoded or base64-encoded key and validate its length.
 */
function parseKey(raw: string, label: string): Buffer {
  // Try hex first (64 hex chars = 32 bytes)
  if (/^[0-9a-fA-F]{64}$/.test(raw)) {
    return Buffer.from(raw, 'hex');
  }

  // Try base64
  const buf = Buffer.from(raw, 'base64');
  if (buf.length === KEY_LENGTH) {
    return buf;
  }

  throw new Error(
    `${label} must be exactly ${KEY_LENGTH} bytes (${KEY_LENGTH * 2} hex chars or ${Math.ceil((KEY_LENGTH * 4) / 3)} base64 chars). Got ${buf.length} bytes.`,
  );
}

let _currentKey: Buffer | null = null;
let _previousKey: Buffer | null = null;
let _validated = false;

/**
 * Resolve and cache encryption keys from environment variables.
 * Throws if ENCRYPTION_KEY is missing or invalid.
 */
function getKeys(): { current: Buffer; previous: Buffer | null } {
  if (_validated && _currentKey) {
    return { current: _currentKey, previous: _previousKey };
  }

  const rawCurrent = process.env.ENCRYPTION_KEY;
  if (!rawCurrent) {
    throw new Error(
      'ENCRYPTION_KEY environment variable is required but not set. ' +
        'It must be a 32-byte key, hex-encoded (64 chars) or base64-encoded.',
    );
  }

  _currentKey = parseKey(rawCurrent, 'ENCRYPTION_KEY');

  const rawPrev = process.env.ENCRYPTION_KEY_PREV;
  if (rawPrev) {
    _previousKey = parseKey(rawPrev, 'ENCRYPTION_KEY_PREV');
  }

  _validated = true;
  return { current: _currentKey, previous: _previousKey };
}

/**
 * Select the correct decryption key for a given version.
 *
 * Version 2 (current) -> ENCRYPTION_KEY
 * Version 1 (previous) -> ENCRYPTION_KEY_PREV, falling back to ENCRYPTION_KEY
 */
function keyForVersion(version: number): Buffer {
  const { current, previous } = getKeys();

  if (version === CURRENT_VERSION) {
    return current;
  }

  if (version === 1) {
    return previous ?? current;
  }

  throw new Error(`Unknown encryption key version: ${version}`);
}

// ---------------------------------------------------------------------------
// Envelope type
// ---------------------------------------------------------------------------

interface EncryptedEnvelope {
  /** Key version */
  v: number;
  /** Initialization vector (hex) */
  iv: string;
  /** Ciphertext (hex) */
  ciphertext: string;
  /** GCM auth tag (hex) */
  tag: string;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Encrypt a plaintext string using AES-256-GCM.
 *
 * @returns base64-encoded JSON envelope
 */
export function encrypt(plaintext: string): string {
  const { current } = getKeys();
  const iv = crypto.randomBytes(IV_LENGTH);

  const cipher = crypto.createCipheriv(ALGORITHM, current, iv);
  const encrypted = Buffer.concat([
    cipher.update(plaintext, 'utf8'),
    cipher.final(),
  ]);
  const tag = cipher.getAuthTag();

  const envelope: EncryptedEnvelope = {
    v: CURRENT_VERSION,
    iv: iv.toString('hex'),
    ciphertext: encrypted.toString('hex'),
    tag: tag.toString('hex'),
  };

  return Buffer.from(JSON.stringify(envelope)).toString('base64');
}

/**
 * Decrypt a base64-encoded encrypted envelope back to plaintext.
 */
export function decrypt(encoded: string): string {
  let envelope: EncryptedEnvelope;
  try {
    const json = Buffer.from(encoded, 'base64').toString('utf8');
    envelope = JSON.parse(json) as EncryptedEnvelope;
  } catch {
    throw new Error('Failed to parse encrypted value — not a valid envelope');
  }

  if (
    typeof envelope.v !== 'number' ||
    typeof envelope.iv !== 'string' ||
    typeof envelope.ciphertext !== 'string' ||
    typeof envelope.tag !== 'string'
  ) {
    throw new Error('Malformed encrypted envelope — missing required fields');
  }

  const key = keyForVersion(envelope.v);
  const iv = Buffer.from(envelope.iv, 'hex');
  const ciphertext = Buffer.from(envelope.ciphertext, 'hex');
  const tag = Buffer.from(envelope.tag, 'hex');

  const decipher = crypto.createDecipheriv(ALGORITHM, key, iv);
  decipher.setAuthTag(tag);

  const decrypted = Buffer.concat([
    decipher.update(ciphertext),
    decipher.final(),
  ]);

  return decrypted.toString('utf8');
}

/**
 * Check whether a value looks like it's already encrypted (base64-encoded envelope).
 * Used by the migration script to skip already-encrypted values (idempotency).
 */
export function isEncrypted(value: string): boolean {
  if (!value || value.length < 10) {
    return false;
  }

  try {
    const json = Buffer.from(value, 'base64').toString('utf8');
    const parsed = JSON.parse(json);
    return (
      typeof parsed === 'object' &&
      parsed !== null &&
      typeof parsed.v === 'number' &&
      typeof parsed.iv === 'string' &&
      typeof parsed.ciphertext === 'string' &&
      typeof parsed.tag === 'string'
    );
  } catch {
    return false;
  }
}

/**
 * Validate ENCRYPTION_KEY on startup. Call this early in the application
 * boot sequence to fail fast if the key is missing or malformed.
 */
export function validateEncryptionKey(): void {
  getKeys(); // Throws if invalid
}

/**
 * Re-encrypt a value: decrypt with the version it was encrypted with,
 * then re-encrypt with the current key/version.
 * Used for key rotation.
 */
export function reEncrypt(encoded: string): string {
  const plaintext = decrypt(encoded);
  return encrypt(plaintext);
}

/**
 * Reset cached keys — used for testing only.
 * @internal
 */
export function _resetKeys(): void {
  _currentKey = null;
  _previousKey = null;
  _validated = false;
}
