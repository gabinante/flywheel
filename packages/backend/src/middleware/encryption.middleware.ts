/**
 * Prisma middleware for transparent field-level encryption.
 *
 * Intercepts create/update operations to encrypt sensitive fields before
 * persistence, and decrypts them after read operations.
 *
 * Encrypted models and fields:
 *   Merchant:    nmiSecurityKey, nmiTokenizationKey, seamlesschexApiKey
 *   Application: ssnLast4, bankRoutingNumber, bankAccountNumber
 */

import { encrypt, decrypt, isEncrypted } from "../utils/encryption";

// ---------------------------------------------------------------------------
// Types matching Prisma's middleware interface
// ---------------------------------------------------------------------------

/**
 * Matches Prisma.MiddlewareParams — defined locally to avoid requiring
 * a generated Prisma client at type-check time.
 */
export interface MiddlewareParams {
  model?: string;
  action: string;
  args: any;
  dataPath: string[];
  runInTransaction: boolean;
}

export type Middleware = (
  params: MiddlewareParams,
  next: (params: MiddlewareParams) => Promise<any>
) => Promise<any>;

// ---------------------------------------------------------------------------
// Configuration: models and their encrypted fields
// ---------------------------------------------------------------------------

const ENCRYPTED_FIELDS: Record<string, string[]> = {
  Merchant: ["nmiSecurityKey", "nmiTokenizationKey", "seamlesschexApiKey"],
  Application: ["ssnLast4", "bankRoutingNumber", "bankAccountNumber"],
};

// Operations that write data (need encryption before persistence)
const WRITE_OPERATIONS = ["create", "update", "upsert", "createMany", "updateMany"];

// Operations that read data (need decryption after retrieval)
const READ_OPERATIONS = ["findUnique", "findFirst", "findMany"];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Encrypt sensitive fields in a data object.
 */
function encryptFields(data: Record<string, any>, fields: string[]): void {
  if (!data || typeof data !== "object") return;

  for (const field of fields) {
    if (field in data && typeof data[field] === "string" && data[field] !== "") {
      // Skip if already encrypted (idempotency safety)
      if (!isEncrypted(data[field])) {
        data[field] = encrypt(data[field]);
      }
    }
  }
}

/**
 * Decrypt sensitive fields in a result object or array.
 */
function decryptFields(
  result: Record<string, any> | Record<string, any>[] | null,
  fields: string[]
): void {
  if (!result) return;

  if (Array.isArray(result)) {
    for (const item of result) {
      decryptFields(item, fields);
    }
    return;
  }

  for (const field of fields) {
    if (field in result && typeof result[field] === "string" && result[field] !== "") {
      if (isEncrypted(result[field])) {
        result[field] = decrypt(result[field]);
      }
    }
  }
}

/**
 * Encrypt fields in nested data structures used by Prisma
 * (handles data, create, update, where clauses with data).
 */
function encryptNestedData(
  params: MiddlewareParams,
  fields: string[]
): void {
  if (!params?.args) return;

  // Direct data (create, update)
  if (params.args.data) {
    encryptFields(params.args.data, fields);
  }

  // Upsert: both create and update blocks
  if (params.args.create) {
    encryptFields(params.args.create, fields);
  }
  if (params.args.update) {
    encryptFields(params.args.update, fields);
  }

  // createMany: array of data
  if (params.args.data && Array.isArray(params.args.data)) {
    for (const item of params.args.data) {
      encryptFields(item, fields);
    }
  }
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

/**
 * Prisma middleware that transparently encrypts/decrypts sensitive fields.
 *
 * Usage:
 *   prisma.$use(encryptionMiddleware);
 */
export const encryptionMiddleware: Middleware = async (
  params: MiddlewareParams,
  next: (params: MiddlewareParams) => Promise<any>
) => {
  const model = params.model as string | undefined;
  if (!model || !(model in ENCRYPTED_FIELDS)) {
    return next(params);
  }

  const fields = ENCRYPTED_FIELDS[model];

  // Encrypt on write operations
  if (WRITE_OPERATIONS.includes(params.action)) {
    encryptNestedData(params, fields);
  }

  // Execute the query
  const result = await next(params);

  // Decrypt on read operations
  if (READ_OPERATIONS.includes(params.action) && result) {
    decryptFields(result, fields);
  }

  return result;
};

export default encryptionMiddleware;
