/**
 * GoHighPayment Backend — public API surface
 */

export {
  createResidualService,
  TIER_BPS,
  TWO_TIER_BPS,
  type ResidualCalculationResult,
  type MerchantVolume,
  type NmiReportingClient,
  type ResidualServiceDeps,
} from "./services/residual.service.js";

export {
  createEmailService,
  type EmailService,
  type EmailServiceConfig,
  type SendEmailParams,
  type SendEmailResult,
} from "./services/email.service.js";

export {
  registerJobHandlers,
  RESIDUAL_CALCULATION_JOB,
  EMAIL_SEND_JOB,
  getPreviousMonthRange,
  type ResidualJobData,
  type EmailSendJobData,
  type JobHandlerDeps,
} from "./jobs/handlers.js";

export {
  scheduleMonthlyResidualJob,
  enqueueResidualCalculation,
  enqueueEmail,
  enqueueEmailSimple,
  type EnqueueEmailParams,
} from "./jobs/queue.js";

export {
  renderTemplate,
  isValidTemplate,
  TEMPLATE_IDS,
} from "./templates/index.js";

export {
  createAdminRouter,
  type AdminRouterDeps,
} from "./routes/admin.js";
