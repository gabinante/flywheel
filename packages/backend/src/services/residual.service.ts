/**
 * Residual Calculation Service
 *
 * Calculates agency residual shares from merchant processing volume.
 * Ledger is append-only — never UPDATE a ResidualEntry. To correct an
 * error, create an adjustment entry with negative amounts.
 *
 * NMI is source of truth when available; our transaction table is fallback.
 */

import type { PrismaClient } from "@prisma/client";

// ─── Types ───────────────────────────────────────────────────────────

/** Agency tier enum — mirrors the Prisma AgencyTier enum */
export type AgencyTier = "TIER_1" | "TIER_2" | "TIER_3";

// ─── Constants ────────────────────────────────────────────────────────

/** Basis-point rates per agency tier */
export const TIER_BPS: Record<AgencyTier, number> = {
  TIER_1: 10,
  TIER_2: 12,
  TIER_3: 15,
};

/** Two-tier referral bonus in basis points */
export const TWO_TIER_BPS = 3;

// ─── Interfaces ──────────────────────────────────────────────────────

export interface ResidualCalculationResult {
  entriesCreated: number;
  entriesSkipped: number;
  merchantsProcessed: number;
  errors: Array<{ merchantId: string; error: string }>;
}

export interface MerchantVolume {
  merchantId: string;
  volumeCents: number;
}

/**
 * Optional NMI reporting client interface.
 * When the NMI Reporting API is available (WS7 7.2), inject an
 * implementation. Otherwise the service falls back to our DB.
 */
export interface NmiReportingClient {
  getMerchantVolume(
    nmiMerchantId: string,
    periodStart: Date,
    periodEnd: Date
  ): Promise<number>; // returns volume in cents
}

// ─── Service ──────────────────────────────────────────────────────────

export interface ResidualServiceDeps {
  prisma: PrismaClient;
  nmiClient?: NmiReportingClient;
}

export function createResidualService(deps: ResidualServiceDeps) {
  const { prisma, nmiClient } = deps;

  /**
   * Get merchant processing volume for a period.
   *
   * Primary: NMI Reporting API (when available).
   * Fallback: sum of our CAPTURED/SETTLED transactions, excluding chargebacks.
   */
  async function getMerchantVolume(
    merchantId: string,
    nmiMerchantId: string | null,
    periodStart: Date,
    periodEnd: Date
  ): Promise<number> {
    // Try NMI Reporting API first
    if (nmiClient && nmiMerchantId) {
      try {
        const volume = await nmiClient.getMerchantVolume(
          nmiMerchantId,
          periodStart,
          periodEnd
        );
        return volume;
      } catch {
        // Fall through to DB fallback
      }
    }

    // Fallback: sum transactions from our DB
    // Exclude transactions that have matching chargebacks
    const result = await prisma.$queryRaw<[{ total: bigint | null }]>`
      SELECT COALESCE(SUM(t.amount), 0) AS total
      FROM "Transaction" t
      WHERE t."merchantId" = ${merchantId}
        AND t.status IN ('CAPTURED', 'SETTLED')
        AND t."createdAt" >= ${periodStart}
        AND t."createdAt" < ${periodEnd}
        AND NOT EXISTS (
          SELECT 1 FROM "Chargeback" cb
          WHERE cb."transactionId" = t.id
        )
    `;

    return Number(result[0]?.total ?? 0);
  }

  /**
   * Calculate residuals for a billing period.
   *
   * For every active merchant with an agency attribution:
   * 1. Pull volume (NMI first, then DB fallback)
   * 2. Compute agency share = floor(volume * bps / 100000)
   * 3. If the agency was referred, compute two-tier share for referrer
   * 4. Insert ResidualEntry rows (unique constraint prevents duplicates)
   */
  async function calculateResiduals(
    periodStart: Date,
    periodEnd: Date
  ): Promise<ResidualCalculationResult> {
    const result: ResidualCalculationResult = {
      entriesCreated: 0,
      entriesSkipped: 0,
      merchantsProcessed: 0,
      errors: [],
    };

    // 1. Query all active merchants with an agency
    const merchants = await prisma.merchant.findMany({
      where: {
        agencyId: { not: null },
        status: "ACTIVE",
      },
      include: {
        agency: {
          include: {
            referredByAgency: true,
          },
        },
      },
    });

    // 2. Process each merchant
    for (const merchant of merchants) {
      try {
        const agency = merchant.agency;
        if (!agency) continue;

        const volumeCents = await getMerchantVolume(
          merchant.id,
          merchant.nmiMerchantId,
          periodStart,
          periodEnd
        );

        result.merchantsProcessed++;

        // Skip merchants with zero volume
        if (volumeCents <= 0) continue;

        const agencyBps = TIER_BPS[agency.tier as AgencyTier];

        // Agency share calculation:
        // agencyShare = floor(volumeCents * rateBps / 100_000)
        //
        // The rate values (10, 12, 15) represent the agency's share per
        // $1,000 of merchant processing volume, expressed in cents.
        // Example: TIER_1 rate 10 on $100,000 volume (10,000,000 cents)
        //   → floor(10,000,000 * 10 / 100,000) = 1,000 cents = $10
        //
        // All amounts are integer cents — no floating point.
        const agencyShare = Math.floor(volumeCents * agencyBps / 100_000);

        // 3a. Create ResidualEntry for the agency
        try {
          await prisma.residualEntry.create({
            data: {
              agencyId: agency.id,
              merchantId: merchant.id,
              periodStart,
              periodEnd,
              merchantVolume: volumeCents,
              nmiResidualEarned: 0, // Populated when NMI reporting available
              agencyBps,
              agencyShare,
              twoTierAgencyId: null,
              twoTierShare: 0,
              status: "PENDING",
            },
          });
          result.entriesCreated++;
        } catch (err: unknown) {
          // Unique constraint violation → duplicate for this period
          if (isPrismaUniqueConstraintError(err)) {
            result.entriesSkipped++;
          } else {
            throw err;
          }
        }

        // 3b. Two-tier referral: if agency was referred, create entry for referrer
        if (agency.referredByAgencyId && agency.referredByAgency) {
          const twoTierShare = Math.floor(volumeCents * TWO_TIER_BPS / 100_000);

          try {
            await prisma.residualEntry.create({
              data: {
                agencyId: agency.referredByAgencyId,
                merchantId: merchant.id,
                periodStart,
                periodEnd,
                merchantVolume: volumeCents,
                nmiResidualEarned: 0,
                agencyBps: TWO_TIER_BPS,
                agencyShare: 0,
                twoTierAgencyId: agency.id,
                twoTierShare,
                status: "PENDING",
              },
            });
            result.entriesCreated++;
          } catch (err: unknown) {
            if (isPrismaUniqueConstraintError(err)) {
              result.entriesSkipped++;
            } else {
              throw err;
            }
          }
        }
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        result.errors.push({ merchantId: merchant.id, error: message });
      }
    }

    // 4. Log in AuditLog
    await logAuditEntry(prisma, {
      action: "RESIDUAL_CALCULATION",
      periodStart,
      periodEnd,
      entriesCreated: result.entriesCreated,
      entriesSkipped: result.entriesSkipped,
      merchantsProcessed: result.merchantsProcessed,
      errorCount: result.errors.length,
    });

    return result;
  }

  return {
    calculateResiduals,
    getMerchantVolume,
  };
}

// ─── Helpers ──────────────────────────────────────────────────────────

function isPrismaUniqueConstraintError(err: unknown): boolean {
  if (err && typeof err === "object" && "code" in err) {
    return (err as { code: string }).code === "P2002";
  }
  return false;
}

/**
 * Create an audit log entry for the residual calculation run.
 *
 * If the AuditLog table doesn't exist yet, we store the entry in a
 * generic JSON format. This gracefully handles the case where the
 * AuditLog migration hasn't been applied yet.
 */
async function logAuditEntry(
  prisma: PrismaClient,
  data: {
    action: string;
    periodStart: Date;
    periodEnd: Date;
    entriesCreated: number;
    entriesSkipped: number;
    merchantsProcessed: number;
    errorCount: number;
  }
): Promise<void> {
  try {
    // Attempt to insert into AuditLog table via raw SQL
    // (model may not exist in Prisma schema yet)
    await prisma.$executeRaw`
      INSERT INTO "AuditLog" ("id", "action", "details", "createdAt")
      VALUES (
        gen_random_uuid()::text,
        ${data.action},
        ${JSON.stringify({
          periodStart: data.periodStart.toISOString(),
          periodEnd: data.periodEnd.toISOString(),
          entriesCreated: data.entriesCreated,
          entriesSkipped: data.entriesSkipped,
          merchantsProcessed: data.merchantsProcessed,
          errorCount: data.errorCount,
        })}::jsonb,
        NOW()
      )
    `;
  } catch {
    // AuditLog table may not exist yet — log to console as fallback
    console.log("[ResidualService] Audit log:", JSON.stringify(data));
  }
}
