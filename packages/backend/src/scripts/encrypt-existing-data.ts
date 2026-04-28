/**
 * Data migration script: Encrypt existing plaintext sensitive data.
 *
 * This script iterates all Merchant and Application records, encrypting
 * plaintext credential and PII fields in place. It is idempotent — already
 * encrypted values are skipped via isEncrypted() checks.
 *
 * Usage:
 *   ENCRYPTION_KEY=<hex-key> npx ts-node packages/backend/src/scripts/encrypt-existing-data.ts
 *
 * The script processes records in batches of 100, each in a transaction,
 * to balance performance and safety.
 */

import { PrismaClient } from "@prisma/client";
import { encrypt, isEncrypted, validateEncryptionKey } from "../utils/encryption";

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const BATCH_SIZE = 100;

interface FieldConfig {
  model: string;
  fields: string[];
}

const MODELS_TO_ENCRYPT: FieldConfig[] = [
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
// Migration logic
// ---------------------------------------------------------------------------

async function encryptModel(
  prisma: PrismaClient,
  config: FieldConfig
): Promise<{ total: number; encrypted: number; skipped: number }> {
  const { model, fields } = config;
  const modelDelegate = (prisma as any)[model.charAt(0).toLowerCase() + model.slice(1)];

  if (!modelDelegate) {
    console.error(`  Model "${model}" not found on Prisma client. Skipping.`);
    return { total: 0, encrypted: 0, skipped: 0 };
  }

  const totalCount = await modelDelegate.count();
  console.log(`\n[${model}] Found ${totalCount} records`);

  let encrypted = 0;
  let skipped = 0;
  let offset = 0;

  while (offset < totalCount) {
    const batch = await modelDelegate.findMany({
      skip: offset,
      take: BATCH_SIZE,
    });

    // Process each record in a transaction
    const updates: Array<{ id: string; data: Record<string, string> }> = [];

    for (const record of batch) {
      const data: Record<string, string> = {};
      let needsUpdate = false;

      for (const field of fields) {
        const value = record[field];
        if (typeof value === "string" && value !== "" && !isEncrypted(value)) {
          data[field] = encrypt(value);
          needsUpdate = true;
        }
      }

      if (needsUpdate) {
        updates.push({ id: record.id, data });
        encrypted++;
      } else {
        skipped++;
      }
    }

    // Execute batch updates in a transaction
    if (updates.length > 0) {
      await prisma.$transaction(
        updates.map(({ id, data }) =>
          modelDelegate.update({ where: { id }, data })
        )
      );
    }

    console.log(
      `  Processed ${Math.min(offset + BATCH_SIZE, totalCount)}/${totalCount} ` +
        `(${updates.length} encrypted, ${batch.length - updates.length} skipped)`
    );

    offset += BATCH_SIZE;
  }

  return { total: totalCount, encrypted, skipped };
}

async function main(): Promise<void> {
  console.log("=== Field-Level Encryption Migration ===\n");

  // Validate encryption key before proceeding
  try {
    validateEncryptionKey();
    console.log("ENCRYPTION_KEY validated successfully.\n");
  } catch (err: any) {
    console.error(`FATAL: ${err.message}`);
    process.exit(1);
  }

  // Create a raw Prisma client (no encryption middleware — we handle it manually)
  const prisma = new PrismaClient();

  try {
    await prisma.$connect();
    console.log("Database connected.\n");

    const results: Array<{
      model: string;
      total: number;
      encrypted: number;
      skipped: number;
    }> = [];

    for (const config of MODELS_TO_ENCRYPT) {
      const result = await encryptModel(prisma, config);
      results.push({ model: config.model, ...result });
    }

    // Summary
    console.log("\n=== Migration Summary ===");
    for (const r of results) {
      console.log(
        `  ${r.model}: ${r.total} total, ${r.encrypted} encrypted, ${r.skipped} skipped`
      );
    }

    const totalEncrypted = results.reduce((sum, r) => sum + r.encrypted, 0);
    const totalSkipped = results.reduce((sum, r) => sum + r.skipped, 0);
    console.log(
      `\n  Total: ${totalEncrypted} fields encrypted, ${totalSkipped} already encrypted/empty`
    );
    console.log("\nMigration complete.");
  } catch (err: any) {
    console.error(`\nMigration failed: ${err.message}`);
    console.error(err.stack);
    process.exit(1);
  } finally {
    await prisma.$disconnect();
  }
}

main();
