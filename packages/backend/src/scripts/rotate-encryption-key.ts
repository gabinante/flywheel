/**
 * Key rotation script: Re-encrypt all sensitive fields with the new key.
 *
 * Prerequisites:
 *   1. Set ENCRYPTION_KEY to the NEW key
 *   2. Set ENCRYPTION_KEY_PREV to the OLD key
 *   3. Run this script
 *   4. After completion, remove ENCRYPTION_KEY_PREV from environment
 *
 * Usage:
 *   ENCRYPTION_KEY=<new-key> ENCRYPTION_KEY_PREV=<old-key> \
 *     npx ts-node packages/backend/src/scripts/rotate-encryption-key.ts
 *
 * The script:
 *   - Reads each encrypted value
 *   - Decrypts with the appropriate key version (v1 uses PREV, v2 uses current)
 *   - Re-encrypts with the current key (ENCRYPTION_KEY), producing v2
 *   - Updates the record
 *
 * Idempotent: values already at v2 with the current key are skipped.
 */

import { PrismaClient } from "@prisma/client";
import {
  decrypt,
  encrypt,
  isEncrypted,
  validateEncryptionKey,
} from "../utils/encryption";

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const BATCH_SIZE = 100;

interface FieldConfig {
  model: string;
  fields: string[];
}

const MODELS_TO_ROTATE: FieldConfig[] = [
  {
    model: "Merchant",
    fields: ["nmiSecurityKey", "nmiTokenizationKey", "seamlesschexApiKey"],
  },
  {
    model: "Application",
    fields: ["ssnLast4", "bankRoutingNumber", "bankAccountNumber"],
  },
];

// ---------------------------------------------------------------------------
// Rotation logic
// ---------------------------------------------------------------------------

function needsRotation(value: string): boolean {
  if (!isEncrypted(value)) return false;

  try {
    const json = Buffer.from(value, "base64").toString("utf8");
    const envelope = JSON.parse(json);
    // Only rotate if version is not the current (v2) or if we want to force rotation
    return envelope.v !== 2;
  } catch {
    return false;
  }
}

async function rotateModel(
  prisma: PrismaClient,
  config: FieldConfig
): Promise<{ total: number; rotated: number; skipped: number }> {
  const { model, fields } = config;
  const modelDelegate = (prisma as any)[
    model.charAt(0).toLowerCase() + model.slice(1)
  ];

  if (!modelDelegate) {
    console.error(`  Model "${model}" not found on Prisma client. Skipping.`);
    return { total: 0, rotated: 0, skipped: 0 };
  }

  const totalCount = await modelDelegate.count();
  console.log(`\n[${model}] Found ${totalCount} records`);

  let rotated = 0;
  let skipped = 0;
  let offset = 0;

  while (offset < totalCount) {
    const batch = await modelDelegate.findMany({
      skip: offset,
      take: BATCH_SIZE,
    });

    const updates: Array<{ id: string; data: Record<string, string> }> = [];

    for (const record of batch) {
      const data: Record<string, string> = {};
      let needsUpdate = false;

      for (const field of fields) {
        const value = record[field];
        if (typeof value === "string" && value !== "" && isEncrypted(value)) {
          if (needsRotation(value)) {
            // Decrypt with old key, re-encrypt with new key
            const plaintext = decrypt(value);
            data[field] = encrypt(plaintext);
            needsUpdate = true;
          }
        }
      }

      if (needsUpdate) {
        updates.push({ id: record.id, data });
        rotated++;
      } else {
        skipped++;
      }
    }

    if (updates.length > 0) {
      await prisma.$transaction(
        updates.map(({ id, data }) =>
          modelDelegate.update({ where: { id }, data })
        )
      );
    }

    console.log(
      `  Processed ${Math.min(offset + BATCH_SIZE, totalCount)}/${totalCount} ` +
        `(${updates.length} rotated, ${batch.length - updates.length} skipped)`
    );

    offset += BATCH_SIZE;
  }

  return { total: totalCount, rotated, skipped };
}

async function main(): Promise<void> {
  console.log("=== Encryption Key Rotation ===\n");

  // Validate keys
  try {
    validateEncryptionKey();
  } catch (err: any) {
    console.error(`FATAL: ${err.message}`);
    process.exit(1);
  }

  if (!process.env.ENCRYPTION_KEY_PREV) {
    console.warn(
      "WARNING: ENCRYPTION_KEY_PREV is not set. " +
        "This is fine if all values are already on v2, " +
        "but v1 values will fail to decrypt.\n"
    );
  }

  const prisma = new PrismaClient();

  try {
    await prisma.$connect();
    console.log("Database connected.\n");

    const results: Array<{
      model: string;
      total: number;
      rotated: number;
      skipped: number;
    }> = [];

    for (const config of MODELS_TO_ROTATE) {
      const result = await rotateModel(prisma, config);
      results.push({ model: config.model, ...result });
    }

    // Summary
    console.log("\n=== Rotation Summary ===");
    for (const r of results) {
      console.log(
        `  ${r.model}: ${r.total} total, ${r.rotated} rotated, ${r.skipped} already current`
      );
    }

    const totalRotated = results.reduce((sum, r) => sum + r.rotated, 0);
    console.log(`\n  Total: ${totalRotated} records re-encrypted`);
    console.log(
      "\nRotation complete. You can now remove ENCRYPTION_KEY_PREV from your environment."
    );
  } catch (err: any) {
    console.error(`\nRotation failed: ${err.message}`);
    console.error(err.stack);
    process.exit(1);
  } finally {
    await prisma.$disconnect();
  }
}

main();
