/**
 * Admin Routes — Residual calculation + Agency management
 *
 * POST /api/v1/admin/residuals/calculate
 *   Body: { periodStart: string, periodEnd: string }
 *   Returns: { entriesCreated, entriesSkipped, merchantsProcessed, errors }
 *
 * GET /api/v1/admin/agencies
 *   Query: page, limit, tier, status, search, sort
 *   Returns: { data: Agency[], total, page, limit }
 *
 * GET /api/v1/admin/agencies/:id
 *   Returns full agency profile with merchants, referral tree, residual/payout history
 *
 * PATCH /api/v1/admin/agencies/:id
 *   Body: { status?, tier?, tierOverride? }
 *   All changes logged in AuditLog
 */

import { Router, type Request, type Response } from "express";
import { z } from "zod";
import type { PrismaClient } from "@prisma/client";
import {
  createResidualService,
  type NmiReportingClient,
} from "../services/residual.service.js";

// ─── Local types ───────────────────────────────────────────────────────

interface AgencySummary {
  id: string;
  name: string;
  contactEmail: string;
  tier: string;
  status: string;
  merchantCount: number;
  portfolioVolume: number;
  monthlyResidual: number;
  referredByAgency: { name: string } | null;
  createdAt: Date;
}

interface ResidualAggregate {
  agencyId: string;
  _sum: {
    merchantVolume: number | null;
    agencyShare: number | null;
  };
}

// Prisma findMany returns a complex intersection type. We use this
// minimal interface for the parts we actually access.
interface AgencyRow {
  id: string;
  name: string;
  contactEmail: string;
  tier: string;
  status: string;
  createdAt: Date;
  _count: { merchants: number };
  referredByAgency: { name: string } | null;
}

interface ReferredAgencyRow {
  id: string;
  name: string;
  contactEmail: string;
  tier: string;
  status: string;
  createdAt: Date;
  _count: { merchants: number };
}

// ─── Validation ────────────────────────────────────────────────────────

const calculateResidualsSchema = z.object({
  periodStart: z.string().datetime({ message: "periodStart must be ISO 8601" }),
  periodEnd: z.string().datetime({ message: "periodEnd must be ISO 8601" }),
});

const listAgenciesSchema = z.object({
  page: z.coerce.number().int().min(1).default(1),
  limit: z.coerce.number().int().min(1).max(100).default(20),
  tier: z.enum(["TIER_1", "TIER_2", "TIER_3"]).optional(),
  status: z.enum(["ACTIVE", "SUSPENDED", "CHURNED"]).optional(),
  search: z.string().optional(),
  sort: z.enum(["createdAt", "merchantCount", "portfolioVolume"]).default("createdAt"),
});

const patchAgencySchema = z.object({
  status: z.enum(["ACTIVE", "SUSPENDED"]).optional(),
  tier: z.enum(["TIER_1", "TIER_2", "TIER_3"]).optional(),
  tierOverride: z.boolean().optional(),
});

// ─── Router factory ────────────────────────────────────────────────────

export interface AdminRouterDeps {
  prisma: PrismaClient;
  nmiClient?: NmiReportingClient;
}

export function createAdminRouter(deps: AdminRouterDeps): Router {
  const router = Router();
  const residualService = createResidualService({
    prisma: deps.prisma,
    nmiClient: deps.nmiClient,
  });

  // ── POST /residuals/calculate ──────────────────────────────────────

  /**
   * POST /api/v1/admin/residuals/calculate
   *
   * Trigger a manual residual calculation for a given period.
   * Useful for backfills and recalculations.
   */
  router.post(
    "/residuals/calculate",
    async (req: Request, res: Response): Promise<void> => {
      const parsed = calculateResidualsSchema.safeParse(req.body);
      if (!parsed.success) {
        res.status(400).json({
          error: "Invalid request body",
          details: parsed.error.issues,
        });
        return;
      }

      const periodStart = new Date(parsed.data.periodStart);
      const periodEnd = new Date(parsed.data.periodEnd);

      if (periodEnd <= periodStart) {
        res.status(400).json({
          error: "periodEnd must be after periodStart",
        });
        return;
      }

      try {
        const result = await residualService.calculateResiduals(
          periodStart,
          periodEnd
        );
        res.status(200).json({ success: true, ...result });
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] Residual calculation failed:", message);
        res.status(500).json({
          error: "Residual calculation failed",
          message,
        });
      }
    }
  );

  // ── GET /agencies ──────────────────────────────────────────────────

  /**
   * GET /api/v1/admin/agencies
   *
   * List all agencies with pagination and filtering.
   * Returns computed merchantCount, portfolioVolume (last 30-day period),
   * and monthlyResidual alongside core agency fields.
   */
  router.get(
    "/agencies",
    async (req: Request, res: Response): Promise<void> => {
      const parsed = listAgenciesSchema.safeParse(req.query);
      if (!parsed.success) {
        res.status(400).json({
          error: "Invalid query parameters",
          details: parsed.error.issues,
        });
        return;
      }

      const { page, limit, tier, status, search, sort } = parsed.data;

      // Build where clause
      const where: Record<string, unknown> = {};
      if (tier) where.tier = tier;
      if (status) where.status = status;
      if (search) {
        where.OR = [
          { name: { contains: search, mode: "insensitive" } },
          { contactEmail: { contains: search, mode: "insensitive" } },
        ];
      }

      try {
        const [agencies, total] = await Promise.all([
          deps.prisma.agency.findMany({
            where,
            include: {
              _count: { select: { merchants: true } },
              referredByAgency: { select: { name: true } },
            },
            skip: (page - 1) * limit,
            take: limit,
            // Sort by createdAt here; merchantCount/portfolioVolume sorted in-memory below
            orderBy: sort === "createdAt" ? { createdAt: "desc" } : { createdAt: "desc" },
          }),
          deps.prisma.agency.count({ where }),
        ]);

        // Compute volume / residual from residualEntries for the last 30 days
        const thirtyDaysAgo = new Date(Date.now() - 30 * 24 * 60 * 60 * 1000);
        const agencyRows = agencies as unknown as AgencyRow[];
        const agencyIds = agencyRows.map((a) => a.id);

        const residualAggregates: ResidualAggregate[] = agencyIds.length
          ? (await deps.prisma.residualEntry.groupBy({
              by: ["agencyId"],
              where: {
                agencyId: { in: agencyIds },
                periodStart: { gte: thirtyDaysAgo },
              },
              _sum: { merchantVolume: true, agencyShare: true },
            })) as ResidualAggregate[]
          : [];

        const residualMap = new Map<string, ResidualAggregate>(
          residualAggregates.map((r) => [r.agencyId, r])
        );

        const data: AgencySummary[] = agencyRows.map((a) => ({
          id: a.id,
          name: a.name,
          contactEmail: a.contactEmail,
          tier: a.tier,
          status: a.status,
          merchantCount: a._count.merchants,
          portfolioVolume: residualMap.get(a.id)?._sum?.merchantVolume ?? 0,
          monthlyResidual: residualMap.get(a.id)?._sum?.agencyShare ?? 0,
          referredByAgency: a.referredByAgency
            ? { name: a.referredByAgency.name }
            : null,
          createdAt: a.createdAt,
        }));

        // Sort in-memory if needed
        if (sort === "merchantCount") {
          data.sort((a: AgencySummary, b: AgencySummary) => b.merchantCount - a.merchantCount);
        } else if (sort === "portfolioVolume") {
          data.sort((a: AgencySummary, b: AgencySummary) => b.portfolioVolume - a.portfolioVolume);
        }

        res.status(200).json({ data, total, page, limit });
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] List agencies failed:", message);
        res.status(500).json({ error: "Failed to list agencies", message });
      }
    }
  );

  // ── GET /agencies/:id ──────────────────────────────────────────────

  /**
   * GET /api/v1/admin/agencies/:id
   *
   * Full agency profile including:
   * - All agency fields
   * - merchants list (id, name, status, volume from latest period)
   * - referral tree: agencies this agency referred (depth 1)
   * - residual history (last 12 months)
   * - payout history
   */
  router.get(
    "/agencies/:id",
    async (req: Request, res: Response): Promise<void> => {
      const id = String(req.params.id);

      try {
        const agency = await deps.prisma.agency.findUnique({
          where: { id },
          include: {
            referredByAgency: { select: { id: true, name: true } },
            referredAgencies: {
              select: {
                id: true,
                name: true,
                contactEmail: true,
                tier: true,
                status: true,
                createdAt: true,
                _count: { select: { merchants: true } },
              },
            },
            merchants: {
              select: {
                id: true,
                name: true,
                email: true,
                status: true,
                createdAt: true,
              },
            },
            residualEntries: {
              where: {
                periodStart: {
                  gte: new Date(Date.now() - 365 * 24 * 60 * 60 * 1000),
                },
              },
              orderBy: { periodStart: "desc" },
              select: {
                id: true,
                periodStart: true,
                periodEnd: true,
                merchantVolume: true,
                agencyShare: true,
                twoTierShare: true,
                status: true,
                approvedAt: true,
              },
            },
            residualPayouts: {
              orderBy: { createdAt: "desc" },
              select: {
                id: true,
                periodStart: true,
                totalAmount: true,
                directShare: true,
                twoTierShare: true,
                method: true,
                status: true,
                paidAt: true,
                createdAt: true,
              },
            },
          },
        });

        if (!agency) {
          res.status(404).json({ error: "Agency not found" });
          return;
        }

        // Compute referred agency stats (merchantCount + volume)
        const referredRows = agency.referredAgencies as unknown as ReferredAgencyRow[];
        const referredAgencyIds = referredRows.map((a) => a.id);
        const thirtyDaysAgo = new Date(Date.now() - 30 * 24 * 60 * 60 * 1000);

        const referredAggregates: ResidualAggregate[] = referredAgencyIds.length
          ? (await deps.prisma.residualEntry.groupBy({
              by: ["agencyId"],
              where: {
                agencyId: { in: referredAgencyIds },
                periodStart: { gte: thirtyDaysAgo },
              },
              _sum: { merchantVolume: true, agencyShare: true },
            })) as ResidualAggregate[]
          : [];

        const referredMap = new Map<string, ResidualAggregate>(
          referredAggregates.map((r) => [r.agencyId, r])
        );

        const response = {
          id: agency.id,
          name: agency.name,
          contactEmail: agency.contactEmail,
          contactPhone: agency.contactPhone,
          tier: agency.tier,
          tierOverride: agency.tierOverride,
          status: agency.status,
          payoutEmail: agency.payoutEmail,
          payoutBankLast4: agency.payoutBankLast4,
          taxIdOnFile: agency.taxIdOnFile,
          referralCode: agency.referralCode,
          referredByAgency: agency.referredByAgency,
          createdAt: agency.createdAt,
          updatedAt: agency.updatedAt,
          merchants: agency.merchants,
          referredAgencies: referredRows.map((a) => ({
            id: a.id,
            name: a.name,
            contactEmail: a.contactEmail,
            tier: a.tier,
            status: a.status,
            merchantCount: a._count.merchants,
            portfolioVolume:
              referredMap.get(a.id)?._sum?.merchantVolume ?? 0,
            monthlyResidual: referredMap.get(a.id)?._sum?.agencyShare ?? 0,
            createdAt: a.createdAt,
          })),
          residualHistory: agency.residualEntries,
          payoutHistory: agency.residualPayouts,
        };

        res.status(200).json(response);
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] Get agency failed:", message);
        res.status(500).json({ error: "Failed to get agency", message });
      }
    }
  );

  // ── PATCH /agencies/:id ────────────────────────────────────────────

  /**
   * PATCH /api/v1/admin/agencies/:id
   *
   * Update agency status, tier, or tierOverride.
   * All changes are logged in AuditLog.
   * Suspending an agency: sets status=SUSPENDED (residuals stop accruing).
   * Reactivating: sets status=ACTIVE.
   */
  router.patch(
    "/agencies/:id",
    async (req: Request, res: Response): Promise<void> => {
      const id = String(req.params.id);

      const parsed = patchAgencySchema.safeParse(req.body);
      if (!parsed.success) {
        res.status(400).json({
          error: "Invalid request body",
          details: parsed.error.issues,
        });
        return;
      }

      if (Object.keys(parsed.data).length === 0) {
        res.status(400).json({ error: "No fields to update" });
        return;
      }

      try {
        // Check agency exists
        const existing = await deps.prisma.agency.findUnique({
          where: { id },
          select: { id: true, status: true, tier: true, tierOverride: true },
        });

        if (!existing) {
          res.status(404).json({ error: "Agency not found" });
          return;
        }

        const updateData: Record<string, unknown> = {};
        if (parsed.data.status !== undefined)
          updateData.status = parsed.data.status;
        if (parsed.data.tier !== undefined) updateData.tier = parsed.data.tier;
        if (parsed.data.tierOverride !== undefined)
          updateData.tierOverride = parsed.data.tierOverride;

        // Perform update + create audit log in a transaction
        const [agency] = await deps.prisma.$transaction([
          deps.prisma.agency.update({
            where: { id },
            data: updateData,
            select: {
              id: true,
              name: true,
              contactEmail: true,
              tier: true,
              tierOverride: true,
              status: true,
              updatedAt: true,
            },
          }),
          deps.prisma.auditLog.create({
            data: {
              action: "agency.patch",
              details: {
                agencyId: id,
                changes: updateData,
                previous: {
                  status: existing.status,
                  tier: existing.tier,
                  tierOverride: existing.tierOverride,
                },
              },
            },
          }),
        ]);

        res.status(200).json(agency);
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] Patch agency failed:", message);
        res.status(500).json({ error: "Failed to update agency", message });
      }
    }
  );

  return router;
}
