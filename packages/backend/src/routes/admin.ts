/**
 * Admin Routes — agency management endpoints
 *
 * GET   /api/v1/admin/agencies          — list agencies (paginated, filterable)
 * GET   /api/v1/admin/agencies/:id      — full agency detail
 * PATCH /api/v1/admin/agencies/:id      — update agency status/tier
 * POST  /api/v1/admin/residuals/calculate — trigger residual calculation
 */

import { Router, type Request, type Response } from "express";
import { z } from "zod";
import type { PrismaClient } from "@prisma/client";
import {
  createResidualService,
  type NmiReportingClient,
} from "../services/residual.service.js";

// ─── Validation Schemas ──────────────────────────────────────────────

const calculateResidualsSchema = z.object({
  periodStart: z.string().datetime({ message: "periodStart must be ISO 8601" }),
  periodEnd: z.string().datetime({ message: "periodEnd must be ISO 8601" }),
});

const listAgenciesQuerySchema = z.object({
  page: z.coerce.number().int().min(1).default(1),
  limit: z.coerce.number().int().min(1).max(100).default(20),
  tier: z.enum(["TIER_1", "TIER_2", "TIER_3"]).optional(),
  status: z.enum(["ACTIVE", "SUSPENDED", "CHURNED"]).optional(),
  search: z.string().optional(),
  sort: z.enum(["createdAt", "merchantCount", "portfolioVolume", "name"]).default("createdAt"),
  order: z.enum(["asc", "desc"]).default("desc"),
});

const updateAgencySchema = z.object({
  status: z.enum(["ACTIVE", "SUSPENDED"]).optional(),
  tier: z.enum(["TIER_1", "TIER_2", "TIER_3"]).optional(),
  tierOverride: z.boolean().optional(),
}).refine((data) => Object.keys(data).length > 0, {
  message: "At least one field must be provided",
});

// ─── Router factory ───────────────────────────────────────────────────

export interface AdminRouterDeps {
  prisma: PrismaClient;
  nmiClient?: NmiReportingClient;
}

export function createAdminRouter(deps: AdminRouterDeps): Router {
  const router = Router();
  const { prisma } = deps;
  const residualService = createResidualService({
    prisma: deps.prisma,
    nmiClient: deps.nmiClient,
  });

  // ─── POST /residuals/calculate ────────────────────────────────────
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
        res.status(500).json({ error: "Residual calculation failed", message });
      }
    }
  );

  // ─── GET /agencies ────────────────────────────────────────────────
  router.get(
    "/agencies",
    async (req: Request, res: Response): Promise<void> => {
      const parsed = listAgenciesQuerySchema.safeParse(req.query);
      if (!parsed.success) {
        res.status(400).json({
          error: "Invalid query parameters",
          details: parsed.error.issues,
        });
        return;
      }

      const { page, limit, tier, status, search, sort, order } = parsed.data;
      const skip = (page - 1) * limit;

      try {
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

        // Get agencies with merchant count
        const [agencies, total] = await Promise.all([
          prisma.agency.findMany({
            where,
            skip,
            take: limit,
            orderBy: sort === "createdAt" || sort === "name"
              ? { [sort]: order }
              : { createdAt: order },
            include: {
              referredByAgency: { select: { id: true, name: true } },
              _count: { select: { merchants: true } },
            },
          }),
          prisma.agency.count({ where }),
        ]);

        // Compute portfolio volume and monthly residual for each agency
        const now = new Date();
        const currentPeriodStart = new Date(now.getFullYear(), now.getMonth(), 1);
        const currentPeriodEnd = new Date(now.getFullYear(), now.getMonth() + 1, 1);

        const enrichedAgencies = await Promise.all(
          agencies.map(async (agency) => {
            // Sum residual entries for current period
            const residualAgg = await prisma.residualEntry.aggregate({
              where: {
                agencyId: agency.id,
                periodStart: { gte: currentPeriodStart },
                periodEnd: { lte: currentPeriodEnd },
              },
              _sum: {
                agencyShare: true,
                merchantVolume: true,
              },
            });

            return {
              id: agency.id,
              name: agency.name,
              contactEmail: agency.contactEmail,
              tier: agency.tier,
              status: agency.status,
              merchantCount: agency._count.merchants,
              portfolioVolume: residualAgg._sum.merchantVolume ?? 0,
              monthlyResidual: residualAgg._sum.agencyShare ?? 0,
              referredByAgency: agency.referredByAgency,
              createdAt: agency.createdAt,
            };
          })
        );

        // Post-sort by computed fields if needed
        if (sort === "merchantCount") {
          enrichedAgencies.sort((a, b) =>
            order === "desc"
              ? b.merchantCount - a.merchantCount
              : a.merchantCount - b.merchantCount
          );
        } else if (sort === "portfolioVolume") {
          enrichedAgencies.sort((a, b) =>
            order === "desc"
              ? b.portfolioVolume - a.portfolioVolume
              : a.portfolioVolume - b.portfolioVolume
          );
        }

        res.status(200).json({
          agencies: enrichedAgencies,
          pagination: {
            page,
            limit,
            total,
            totalPages: Math.ceil(total / limit),
          },
        });
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] List agencies failed:", message);
        res.status(500).json({ error: "Failed to list agencies", message });
      }
    }
  );

  // ─── GET /agencies/:id ────────────────────────────────────────────
  router.get(
    "/agencies/:id",
    async (req: Request, res: Response): Promise<void> => {
      const { id } = req.params;

      try {
        const agency = await prisma.agency.findUnique({
          where: { id },
          include: {
            referredByAgency: { select: { id: true, name: true } },
            merchants: {
              select: {
                id: true,
                name: true,
                status: true,
                createdAt: true,
              },
            },
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
          },
        });

        if (!agency) {
          res.status(404).json({ error: "Agency not found" });
          return;
        }

        // Get residual history (last 12 months)
        const twelveMonthsAgo = new Date();
        twelveMonthsAgo.setMonth(twelveMonthsAgo.getMonth() - 12);

        const residualEntries = await prisma.residualEntry.findMany({
          where: {
            agencyId: id,
            periodStart: { gte: twelveMonthsAgo },
          },
          orderBy: { periodStart: "desc" },
          select: {
            id: true,
            merchantId: true,
            periodStart: true,
            periodEnd: true,
            merchantVolume: true,
            agencyShare: true,
            twoTierShare: true,
            status: true,
            createdAt: true,
          },
        });

        // Aggregate residuals by month for chart
        const monthlyResiduals: Record<string, { agencyShare: number; twoTierShare: number; volume: number }> = {};
        for (const entry of residualEntries) {
          const key = `${entry.periodStart.getUTCFullYear()}-${String(entry.periodStart.getUTCMonth() + 1).padStart(2, "0")}`;
          if (!monthlyResiduals[key]) {
            monthlyResiduals[key] = { agencyShare: 0, twoTierShare: 0, volume: 0 };
          }
          monthlyResiduals[key].agencyShare += entry.agencyShare;
          monthlyResiduals[key].twoTierShare += entry.twoTierShare;
          monthlyResiduals[key].volume += entry.merchantVolume;
        }

        // Get payout history
        const payouts = await prisma.residualPayout.findMany({
          where: { agencyId: id },
          orderBy: { periodStart: "desc" },
          select: {
            id: true,
            periodStart: true,
            totalAmount: true,
            directShare: true,
            twoTierShare: true,
            method: true,
            status: true,
            paidAt: true,
          },
        });

        // Get merchant volumes for current period
        const now = new Date();
        const currentPeriodStart = new Date(now.getFullYear(), now.getMonth(), 1);

        const merchantsWithVolume = await Promise.all(
          agency.merchants.map(async (merchant) => {
            const volumeAgg = await prisma.residualEntry.aggregate({
              where: {
                merchantId: merchant.id,
                agencyId: id,
                periodStart: { gte: currentPeriodStart },
              },
              _sum: {
                merchantVolume: true,
              },
            });
            return {
              ...merchant,
              volumeThisMonth: volumeAgg._sum.merchantVolume ?? 0,
            };
          })
        );

        // Enrich referred agencies with volume stats
        const referredAgenciesWithStats = await Promise.all(
          agency.referredAgencies.map(async (ref) => {
            const volAgg = await prisma.residualEntry.aggregate({
              where: {
                agencyId: ref.id,
                periodStart: { gte: currentPeriodStart },
              },
              _sum: { merchantVolume: true },
            });
            return {
              id: ref.id,
              name: ref.name,
              contactEmail: ref.contactEmail,
              tier: ref.tier,
              status: ref.status,
              merchantCount: ref._count.merchants,
              portfolioVolume: volAgg._sum.merchantVolume ?? 0,
              createdAt: ref.createdAt,
            };
          })
        );

        res.status(200).json({
          id: agency.id,
          name: agency.name,
          contactEmail: agency.contactEmail,
          contactPhone: agency.contactPhone,
          referralCode: agency.referralCode,
          tier: agency.tier,
          tierOverride: agency.tierOverride,
          status: agency.status,
          payoutEmail: agency.payoutEmail,
          payoutBankLast4: agency.payoutBankLast4,
          taxIdOnFile: agency.taxIdOnFile,
          referredByAgency: agency.referredByAgency,
          createdAt: agency.createdAt,
          updatedAt: agency.updatedAt,
          merchants: merchantsWithVolume,
          referredAgencies: referredAgenciesWithStats,
          residualHistory: Object.entries(monthlyResiduals)
            .map(([month, data]) => ({ month, ...data }))
            .sort((a, b) => a.month.localeCompare(b.month)),
          payouts,
        });
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] Get agency detail failed:", message);
        res.status(500).json({ error: "Failed to get agency details", message });
      }
    }
  );

  // ─── PATCH /agencies/:id ──────────────────────────────────────────
  router.patch(
    "/agencies/:id",
    async (req: Request, res: Response): Promise<void> => {
      const { id } = req.params;

      const parsed = updateAgencySchema.safeParse(req.body);
      if (!parsed.success) {
        res.status(400).json({
          error: "Invalid request body",
          details: parsed.error.issues,
        });
        return;
      }

      try {
        // Verify agency exists
        const existing = await prisma.agency.findUnique({ where: { id } });
        if (!existing) {
          res.status(404).json({ error: "Agency not found" });
          return;
        }

        const updates = parsed.data;
        const auditDetails: Record<string, unknown> = {
          agencyId: id,
          changes: {},
        };

        // Track changes for audit
        if (updates.status !== undefined && updates.status !== existing.status) {
          (auditDetails.changes as Record<string, unknown>).status = {
            from: existing.status,
            to: updates.status,
          };
        }
        if (updates.tier !== undefined && updates.tier !== existing.tier) {
          (auditDetails.changes as Record<string, unknown>).tier = {
            from: existing.tier,
            to: updates.tier,
          };
        }
        if (updates.tierOverride !== undefined && updates.tierOverride !== existing.tierOverride) {
          (auditDetails.changes as Record<string, unknown>).tierOverride = {
            from: existing.tierOverride,
            to: updates.tierOverride,
          };
        }

        // Perform update
        const updated = await prisma.agency.update({
          where: { id },
          data: {
            ...(updates.status !== undefined && { status: updates.status }),
            ...(updates.tier !== undefined && { tier: updates.tier }),
            ...(updates.tierOverride !== undefined && { tierOverride: updates.tierOverride }),
          },
        });

        // Write audit log
        const adminId = (req as Request & { adminId?: string }).adminId ?? "system";
        await prisma.auditLog.create({
          data: {
            action: updates.status === "SUSPENDED"
              ? "AGENCY_SUSPENDED"
              : updates.status === "ACTIVE"
              ? "AGENCY_REACTIVATED"
              : updates.tier
              ? "AGENCY_TIER_CHANGED"
              : "AGENCY_UPDATED",
            details: auditDetails,
            performedBy: adminId,
          },
        });

        res.status(200).json({
          id: updated.id,
          name: updated.name,
          contactEmail: updated.contactEmail,
          tier: updated.tier,
          tierOverride: updated.tierOverride,
          status: updated.status,
          updatedAt: updated.updatedAt,
        });
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] Update agency failed:", message);
        res.status(500).json({ error: "Failed to update agency", message });
      }
    }
  );

  return router;
}
