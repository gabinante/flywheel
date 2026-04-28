/**
 * GoHighPayment Backend — public API surface
 */

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
  registerJobHandlers,
  RESIDUAL_CALCULATION_JOB,
  getPreviousMonthRange,
  type ResidualJobData,
  type JobHandlerDeps,
} from "./jobs/handlers.js";

export {
  scheduleMonthlyResidualJob,
  enqueueResidualCalculation,
} from "./jobs/queue.js";

export {
  createAdminRouter,
  type AdminRouterDeps,
} from "./routes/admin.js";

export {
  createRevenueAnalyticsService,
  type RevenueAnalyticsParams,
  type RevenueAnalyticsResult,
  type MonthlyBreakdown,
  type RevenuePeriod,
  type RevenueAnalyticsServiceDeps,
} from "./services/revenue-analytics.service.js";
