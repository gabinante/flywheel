/**
 * Attribution utilities — immutability guard for Merchant.agencyId.
 *
 * Uses an injected PrismaClient so no Prisma instance is created at
 * module load time.  Safe to import in tests.
 */

import type { PrismaClient } from "@prisma/client";

/**
 * Thrown when an attempt is made to change an already-set agencyId.
 */
export class AttributionImmutableError extends Error {
  constructor() {
    super("IMMUTABLE: agencyId cannot be changed after initial attribution");
    this.name = "AttributionImmutableError";
  }
}

/**
 * Guard that throws AttributionImmutableError if the given merchantId
 * already has an agencyId set AND the caller is attempting to provide a
 * new agencyId value.
 *
 * Passing `incomingAgencyId = undefined` is treated as "no change
 * requested" and always passes without touching the database.
 *
 * @param merchantId - The Merchant record to check
 * @param incomingAgencyId - The new value the caller wants to set
 * @param prismaClient - Prisma client (always injected; never imported as singleton here)
 */
export async function enforceAttributionImmutability(
  merchantId: string,
  incomingAgencyId: string | null | undefined,
  prismaClient: PrismaClient
): Promise<void> {
  if (incomingAgencyId === undefined) return; // caller is not changing agencyId

  const merchant = await prismaClient.merchant.findUnique({
    where: { id: merchantId },
    select: { agencyId: true },
  });

  if (!merchant) return; // merchant not found — let downstream handle

  if (merchant.agencyId !== null && merchant.agencyId !== undefined) {
    throw new AttributionImmutableError();
  }
}
