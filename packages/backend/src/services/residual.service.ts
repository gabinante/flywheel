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
 * When the NMI Reporting API is available, inject an implementation.
 * Otherwise the service falls back to our DB.
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

        // Skip suspended agencies — residuals stop accruing
        if (agency.status === "SUSPENDED") continue;

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
              nmiResidualEarned: 0,
              agencyBps,
              agencyShare,
              twoTierAgencyId: null,
              twoTierShare: 0,
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

        // 3b. Two-tier referral
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
    console.log("[ResidualService] Audit log:", JSON.stringify(data));
  }
}
