/**
 * GoHighPayment Backend — public API surface
 *
 * Exports service/job/route factories for the Express admin API.
 */

// ─── Residual service + admin routes (Express-based admin API) ──────
export {
  createResidualService,
  TIER_BPS,
  TWO_TIER_BPS,
  type AgencyTier,
  type ResidualCalculationResult,
  type MerchantVolume,
  type NmiReportingClient,
  type ResidualServiceDeps,
} from "./services/residual.service.js";

export {
  createAdminRouter,
  type AdminRouterDeps,
} from "./routes/admin.js";
