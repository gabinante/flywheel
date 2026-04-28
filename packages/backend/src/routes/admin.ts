/**
 * Admin Routes — Residual management, approval workflow, and payout creation
 *
 * POST /api/v1/admin/residuals/calculate  — trigger manual calculation
 * GET  /api/v1/admin/residuals            — list entries grouped by agency
 * GET  /api/v1/admin/residuals/summary    — summary totals for a period
 * POST /api/v1/admin/residuals/approve    — batch approve entries
 * POST /api/v1/admin/residuals/hold       — batch hold entries with reason
 * POST /api/v1/admin/residuals/create-payout — create payout for an agency
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

const approveSchema = z.object({
  entryIds: z.array(z.string()).min(1, "entryIds must be a non-empty array"),
});

const holdSchema = z.object({
  entryIds: z.array(z.string()).min(1, "entryIds must be a non-empty array"),
  reason: z.string().min(1, "reason is required"),
});

const createPayoutSchema = z.object({
  agencyId: z.string().min(1, "agencyId is required"),
  periodStart: z.string().min(1, "periodStart is required"),
  method: z.enum(["manual", "ach"]),
  reference: z.string().optional(),
});

// ─── Router factory ──────────────────────────────────────────────────

export interface AdminRouterDeps {
  prisma: PrismaClient;
  nmiClient?: NmiReportingClient;
  /** Admin user ID — in production injected from auth middleware */
  getAdminUserId?: (req: Request) => string;
}

export function createAdminRouter(deps: AdminRouterDeps): Router {
  const router = Router();
  const { prisma } = deps;
  const residualService = createResidualService({
    prisma: deps.prisma,
    nmiClient: deps.nmiClient,
  });

  const getAdminUserId =
    deps.getAdminUserId ??
    ((req: Request) =>
      (req.headers["x-admin-user-id"] as string) ?? "system");

  // ─── POST /residuals/calculate ───────────────────────────────────

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

        res.status(200).json({
          success: true,
          ...result,
        });
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

  // ─── GET /residuals ──────────────────────────────────────────────

  router.get(
    "/residuals",
    async (req: Request, res: Response): Promise<void> => {
      const { periodStart, status, agencyId } = req.query;

      if (!periodStart || typeof periodStart !== "string") {
        res.status(400).json({ error: "periodStart query param is required (YYYY-MM-DD)" });
        return;
      }

      const periodDate = new Date(periodStart as string);
      if (isNaN(periodDate.getTime())) {
        res.status(400).json({ error: "Invalid periodStart date format" });
        return;
      }

      try {
        // Build the where clause
        const where: Record<string, unknown> = {
          periodStart: periodDate,
        };

        if (status && typeof status === "string") {
          const validStatuses = ["PENDING", "APPROVED", "PAID", "HELD"];
          if (!validStatuses.includes(status)) {
            res.status(400).json({ error: `Invalid status. Must be one of: ${validStatuses.join(", ")}` });
            return;
          }
          where.status = status;
        }

        if (agencyId && typeof agencyId === "string") {
          where.agencyId = agencyId;
        }

        const entries = await prisma.residualEntry.findMany({
          where,
          include: {
            agency: { select: { id: true, name: true, tier: true } },
            merchant: { select: { id: true, name: true } },
          },
          orderBy: [{ agencyId: "asc" }, { merchantId: "asc" }],
        });

        // Group entries by agency
        const grouped: Record<
          string,
          {
            agencyId: string;
            agencyName: string;
            agencyTier: string;
            entries: Array<{
              id: string;
              merchantId: string;
              merchantName: string;
              merchantVolume: number;
              nmiResidualEarned: number;
              agencyShare: number;
              twoTierShare: number;
              status: string;
              approvedAt: string | null;
              approvedBy: string | null;
            }>;
          }
        > = {};

        for (const entry of entries) {
          if (!grouped[entry.agencyId]) {
            grouped[entry.agencyId] = {
              agencyId: entry.agencyId,
              agencyName: entry.agency.name,
              agencyTier: entry.agency.tier,
              entries: [],
            };
          }
          grouped[entry.agencyId].entries.push({
            id: entry.id,
            merchantId: entry.merchantId,
            merchantName: entry.merchant.name,
            merchantVolume: entry.merchantVolume,
            nmiResidualEarned: entry.nmiResidualEarned,
            agencyShare: entry.agencyShare,
            twoTierShare: entry.twoTierShare,
            status: entry.status,
            approvedAt: entry.approvedAt?.toISOString() ?? null,
            approvedBy: entry.approvedBy ?? null,
          });
        }

        res.status(200).json({
          periodStart: periodDate.toISOString(),
          agencies: Object.values(grouped),
        });
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] List residuals failed:", message);
        res.status(500).json({ error: "Failed to list residuals", message });
      }
    }
  );

  // ─── GET /residuals/summary ──────────────────────────────────────

  router.get(
    "/residuals/summary",
    async (req: Request, res: Response): Promise<void> => {
      const { periodStart } = req.query;

      if (!periodStart || typeof periodStart !== "string") {
        res.status(400).json({ error: "periodStart query param is required (YYYY-MM-DD)" });
        return;
      }

      const periodDate = new Date(periodStart as string);
      if (isNaN(periodDate.getTime())) {
        res.status(400).json({ error: "Invalid periodStart date format" });
        return;
      }

      try {
        const entries = await prisma.residualEntry.findMany({
          where: { periodStart: periodDate },
          include: {
            agency: { select: { tier: true } },
          },
        });

        let totalAgencyShare = 0;
        let totalTwoTierShare = 0;
        const byStatus: Record<string, number> = {
          PENDING: 0,
          APPROVED: 0,
          PAID: 0,
          HELD: 0,
        };
        const byTier: Record<string, { count: number; agencyShare: number; twoTierShare: number }> = {};

        for (const entry of entries) {
          totalAgencyShare += entry.agencyShare;
          totalTwoTierShare += entry.twoTierShare;
          byStatus[entry.status] = (byStatus[entry.status] ?? 0) + 1;

          const tier = entry.agency.tier;
          if (!byTier[tier]) {
            byTier[tier] = { count: 0, agencyShare: 0, twoTierShare: 0 };
          }
          byTier[tier].count++;
          byTier[tier].agencyShare += entry.agencyShare;
          byTier[tier].twoTierShare += entry.twoTierShare;
        }

        res.status(200).json({
          periodStart: periodDate.toISOString(),
          totalEntries: entries.length,
          totalAgencyShare,
          totalTwoTierShare,
          totalOwed: totalAgencyShare + totalTwoTierShare,
          byStatus,
          byTier,
        });
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] Residuals summary failed:", message);
        res.status(500).json({ error: "Failed to get residuals summary", message });
      }
    }
  );

  // ─── POST /residuals/approve ─────────────────────────────────────

  router.post(
    "/residuals/approve",
    async (req: Request, res: Response): Promise<void> => {
      const parsed = approveSchema.safeParse(req.body);
      if (!parsed.success) {
        res.status(400).json({
          error: "Invalid request body",
          details: parsed.error.issues,
        });
        return;
      }

      const adminUserId = getAdminUserId(req);
      const { entryIds } = parsed.data;
      const now = new Date();

      try {
        // Only approve entries that are currently PENDING
        const result = await prisma.residualEntry.updateMany({
          where: {
            id: { in: entryIds },
            status: "PENDING",
          },
          data: {
            status: "APPROVED",
            approvedAt: now,
            approvedBy: adminUserId,
          },
        });

        // Create audit log entries for each approved entry
        for (const entryId of entryIds) {
          await prisma.auditLog.create({
            data: {
              action: "RESIDUAL_APPROVED",
              performedBy: adminUserId,
              details: {
                entryId,
                approvedAt: now.toISOString(),
              },
            },
          });
        }

        res.status(200).json({
          success: true,
          countApproved: result.count,
        });
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] Residual approval failed:", message);
        res.status(500).json({ error: "Failed to approve residuals", message });
      }
    }
  );

  // ─── POST /residuals/hold ────────────────────────────────────────

  router.post(
    "/residuals/hold",
    async (req: Request, res: Response): Promise<void> => {
      const parsed = holdSchema.safeParse(req.body);
      if (!parsed.success) {
        res.status(400).json({
          error: "Invalid request body",
          details: parsed.error.issues,
        });
        return;
      }

      const adminUserId = getAdminUserId(req);
      const { entryIds, reason } = parsed.data;

      try {
        // Only hold entries that are currently PENDING
        const result = await prisma.residualEntry.updateMany({
          where: {
            id: { in: entryIds },
            status: "PENDING",
          },
          data: {
            status: "HELD",
          },
        });

        // Create audit log entries with hold reason
        for (const entryId of entryIds) {
          await prisma.auditLog.create({
            data: {
              action: "RESIDUAL_HELD",
              performedBy: adminUserId,
              details: {
                entryId,
                reason,
                heldAt: new Date().toISOString(),
              },
            },
          });
        }

        res.status(200).json({
          success: true,
          countHeld: result.count,
        });
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] Residual hold failed:", message);
        res.status(500).json({ error: "Failed to hold residuals", message });
      }
    }
  );

  // ─── POST /residuals/create-payout ───────────────────────────────

  router.post(
    "/residuals/create-payout",
    async (req: Request, res: Response): Promise<void> => {
      const parsed = createPayoutSchema.safeParse(req.body);
      if (!parsed.success) {
        res.status(400).json({
          error: "Invalid request body",
          details: parsed.error.issues,
        });
        return;
      }

      const adminUserId = getAdminUserId(req);
      const { agencyId, periodStart, method, reference } = parsed.data;
      const periodDate = new Date(periodStart);

      if (isNaN(periodDate.getTime())) {
        res.status(400).json({ error: "Invalid periodStart date format" });
        return;
      }

      try {
        // Get all entries for this agency+period
        const allEntries = await prisma.residualEntry.findMany({
          where: {
            agencyId,
            periodStart: periodDate,
          },
        });

        if (allEntries.length === 0) {
          res.status(404).json({ error: "No residual entries found for this agency and period" });
          return;
        }

        // Check that ALL entries are APPROVED — cannot create payout otherwise
        const nonApproved = allEntries.filter((e: { status: string }) => e.status !== "APPROVED");
        if (nonApproved.length > 0) {
          const statusCounts: Record<string, number> = {};
          for (const e of nonApproved) {
            statusCounts[e.status] = (statusCounts[e.status] ?? 0) + 1;
          }
          res.status(400).json({
            error: "All entries must be APPROVED before creating a payout",
            nonApprovedCounts: statusCounts,
          });
          return;
        }

        // Aggregate amounts
        let totalDirectShare = 0;
        let totalTwoTierShare = 0;
        for (const entry of allEntries) {
          totalDirectShare += entry.agencyShare;
          totalTwoTierShare += entry.twoTierShare;
        }
        const totalAmount = totalDirectShare + totalTwoTierShare;

        // Create payout record and mark entries as PAID in a transaction
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const payout = await prisma.$transaction(async (tx: any) => {
          const payoutRecord = await tx.residualPayout.create({
            data: {
              agencyId,
              periodStart: periodDate,
              totalAmount,
              directShare: totalDirectShare,
              twoTierShare: totalTwoTierShare,
              method,
              reference: reference ?? null,
              status: "PAID",
              paidAt: new Date(),
            },
          });

          // Mark all entries as PAID
          await tx.residualEntry.updateMany({
            where: {
              agencyId,
              periodStart: periodDate,
              status: "APPROVED",
            },
            data: {
              status: "PAID",
            },
          });

          // Audit log
          await tx.auditLog.create({
            data: {
              action: "RESIDUAL_PAYOUT_CREATED",
              performedBy: adminUserId,
              details: {
                payoutId: payoutRecord.id,
                agencyId,
                periodStart: periodDate.toISOString(),
                totalAmount,
                directShare: totalDirectShare,
                twoTierShare: totalTwoTierShare,
                method,
                reference: reference ?? null,
                entryCount: allEntries.length,
              },
            },
          });

          return payoutRecord;
        });

        res.status(201).json({
          success: true,
          payout: {
            id: payout.id,
            agencyId: payout.agencyId,
            periodStart: payout.periodStart.toISOString(),
            totalAmount: payout.totalAmount,
            directShare: payout.directShare,
            twoTierShare: payout.twoTierShare,
            method: payout.method,
            reference: payout.reference,
            status: payout.status,
            paidAt: payout.paidAt?.toISOString() ?? null,
          },
        });
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        console.error("[Admin] Payout creation failed:", message);
        res.status(500).json({ error: "Failed to create payout", message });
      }
    }
  );

  return router;
}
