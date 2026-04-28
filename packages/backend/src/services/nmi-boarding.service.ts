/**
 * NMI Partner Boarding API service.
 *
 * Handles merchant boarding through NMI's Partner API:
 * - Submit boarding applications with real merchant data
 * - Poll boarding status for UNDER_REVIEW applications
 * - Store encrypted NMI credentials on approval
 *
 * Environment variables:
 *   NMI_PARTNER_ID  — NMI Partner ID for boarding API
 *   NMI_PARTNER_KEY — NMI Partner API key (sensitive)
 *   NMI_API_URL     — NMI API base URL (defaults to production)
 *
 * Security:
 *   - Partner credentials are never logged
 *   - Merchant PII is decrypted only for the API call, then discarded
 *   - All API calls use HTTPS
 *   - Returned security keys are encrypted before storage
 */

import https from "https";
import { URL } from "url";
import { encrypt, decrypt, isEncrypted } from "../utils/encryption";
import { createTimer } from "../utils/logger.js";

/** Logger interface — accepts any pino-compatible logger */
interface BoardingLogger {
  info(obj: Record<string, unknown>, msg: string): void;
  warn(obj: Record<string, unknown>, msg: string): void;
  error(obj: Record<string, unknown>, msg: string): void;
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const NMI_API_URL_DEFAULT = "https://secure.nmi.com/api/v2/partner";
const NMI_SANDBOX_URL = "https://secure.nmi.com/api/v2/partner";

/**
 * NMI Partner API configuration — loaded lazily from environment.
 */
interface NmiConfig {
  partnerId: string;
  partnerKey: string;
  apiUrl: string;
}

let _config: NmiConfig | null = null;

/**
 * Load and validate NMI partner configuration.
 * Throws if required credentials are not set.
 */
function getConfig(): NmiConfig {
  if (_config) return _config;

  const partnerId = process.env.NMI_PARTNER_ID;
  const partnerKey = process.env.NMI_PARTNER_KEY;

  if (!partnerId || !partnerKey) {
    throw new Error(
      "NMI_PARTNER_ID and NMI_PARTNER_KEY are required for NMI boarding. " +
        "Set these environment variables to enable real NMI boarding."
    );
  }

  _config = {
    partnerId,
    partnerKey,
    apiUrl: process.env.NMI_API_URL || NMI_API_URL_DEFAULT,
  };

  return _config;
}

/**
 * Check if NMI partner credentials are configured.
 */
export function isNmiConfigured(): boolean {
  return !!(process.env.NMI_PARTNER_ID && process.env.NMI_PARTNER_KEY);
}

/**
 * Reset cached config — for testing only.
 * @internal
 */
export function _resetConfig(): void {
  _config = null;
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Input application data for boarding submission. */
export interface BoardingApplicationInput {
  /** Application ID in our system */
  applicationId: string;
  /** Merchant ID in our system */
  merchantId: string;

  // Business info
  businessLegalName: string;
  businessDba: string;
  businessEin: string; // may be encrypted
  businessType: string; // LLC, Corp, Sole Prop, etc.
  businessAddress: string;
  businessCity: string;
  businessState: string;
  businessZip: string;
  businessPhone: string;
  businessEmail: string;
  businessWebsite?: string;
  businessStartDate?: string;
  annualVolume?: string;
  averageTicket?: string;

  // Owner info
  ownerFirstName: string;
  ownerLastName: string;
  ownerEmail: string;
  ownerPhone: string;
  ownerDob: string;
  ownerSsn: string; // may be encrypted (ssnLast4 or full SSN)
  ownerAddress: string;
  ownerCity: string;
  ownerState: string;
  ownerZip: string;
  ownerOwnershipPct?: string;

  // Bank info
  bankRoutingNumber: string; // may be encrypted
  bankAccountNumber: string; // may be encrypted
  bankAccountType: string; // checking / savings
  bankName?: string;
}

/** NMI boarding submission response. */
export interface BoardingSubmissionResult {
  /** Whether the submission was accepted by NMI */
  success: boolean;
  /** NMI's application/boarding ID */
  nmiApplicationId?: string;
  /** Boarding status: APPROVED, PENDING, UNDER_REVIEW, DECLINED */
  status: "APPROVED" | "PENDING" | "UNDER_REVIEW" | "DECLINED";
  /** Reason for decline, if status is DECLINED */
  declineReason?: string;
  /** NMI merchant ID (set on instant approval) */
  nmiMerchantId?: string;
  /** NMI gateway security key (set on instant approval) */
  securityKey?: string;
  /** NMI tokenization key (set on instant approval) */
  tokenizationKey?: string;
  /** Raw status message from NMI */
  message?: string;
}

/** NMI boarding status check response. */
export interface BoardingStatusResult {
  /** NMI's application/boarding ID */
  nmiApplicationId: string;
  /** Current status */
  status: "APPROVED" | "PENDING" | "UNDER_REVIEW" | "DECLINED";
  /** Reason for decline, if any */
  declineReason?: string;
  /** NMI merchant ID (set on approval) */
  nmiMerchantId?: string;
  /** NMI gateway security key (set on approval) */
  securityKey?: string;
  /** NMI tokenization key (set on approval) */
  tokenizationKey?: string;
  /** Status message from NMI */
  message?: string;
}

// ---------------------------------------------------------------------------
// HTTP client for NMI Partner API
// ---------------------------------------------------------------------------

interface NmiApiResponse {
  statusCode: number;
  body: string;
}

/**
 * Make an HTTPS POST request to NMI Partner API.
 * Uses Node's built-in https module for minimal dependencies.
 */
function nmiPost(
  endpoint: string,
  payload: Record<string, string>
): Promise<NmiApiResponse> {
  const config = getConfig();
  const url = new URL(endpoint, config.apiUrl);

  // Ensure HTTPS
  if (url.protocol !== "https:") {
    throw new Error(
      `NMI API calls must use HTTPS. Got: ${url.protocol}`
    );
  }

  // Build URL-encoded form body (NMI uses form POST)
  const formParams = new URLSearchParams();
  formParams.set("partner_id", config.partnerId);
  formParams.set("partner_key", config.partnerKey);
  for (const [key, value] of Object.entries(payload)) {
    if (value !== undefined && value !== null && value !== "") {
      formParams.set(key, value);
    }
  }
  const body = formParams.toString();

  return new Promise((resolve, reject) => {
    const req = https.request(
      {
        hostname: url.hostname,
        port: url.port || 443,
        path: url.pathname + url.search,
        method: "POST",
        headers: {
          "Content-Type": "application/x-www-form-urlencoded",
          "Content-Length": Buffer.byteLength(body),
        },
      },
      (res) => {
        let data = "";
        res.on("data", (chunk: Buffer) => {
          data += chunk.toString();
        });
        res.on("end", () => {
          resolve({
            statusCode: res.statusCode || 0,
            body: data,
          });
        });
      }
    );

    req.on("error", (err) => {
      reject(new Error(`NMI API request failed: ${err.message}`));
    });

    req.setTimeout(30_000, () => {
      req.destroy();
      reject(new Error("NMI API request timed out (30s)"));
    });

    req.write(body);
    req.end();
  });
}

/**
 * Parse NMI's XML or key=value response body.
 * NMI Partner API typically returns key=value pairs or XML.
 */
function parseNmiResponse(body: string): Record<string, string> {
  const result: Record<string, string> = {};

  // Try XML parsing first (simple tag extraction)
  const xmlPattern = /<(\w+)>([^<]*)<\/\1>/g;
  let xmlMatch;
  let hasXml = false;
  while ((xmlMatch = xmlPattern.exec(body)) !== null) {
    hasXml = true;
    result[xmlMatch[1]] = xmlMatch[2];
  }
  if (hasXml) return result;

  // Fall back to key=value parsing (URL-encoded or newline-separated)
  if (body.includes("&") || body.includes("=")) {
    const params = new URLSearchParams(body);
    for (const [key, value] of params.entries()) {
      result[key] = value;
    }
    return result;
  }

  // Try JSON
  try {
    const json = JSON.parse(body);
    if (typeof json === "object" && json !== null) {
      for (const [key, value] of Object.entries(json)) {
        result[key] = String(value);
      }
    }
  } catch {
    // Not JSON, return raw body under a 'raw' key
    result.raw = body;
  }

  return result;
}

// ---------------------------------------------------------------------------
// Helper: decrypt field if encrypted
// ---------------------------------------------------------------------------

/**
 * Decrypt a field value if it's encrypted, otherwise return as-is.
 * This ensures PII is only in plaintext for the API call.
 */
function decryptIfNeeded(value: string): string {
  if (!value) return value;
  try {
    if (isEncrypted(value)) {
      return decrypt(value);
    }
  } catch {
    // If decryption fails, return as-is (might be plaintext already)
  }
  return value;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Submit a merchant boarding application to NMI Partner API.
 *
 * Decrypts PII fields (SSN, bank info) only for the API call.
 * On instant approval, returns NMI credentials for encryption and storage.
 *
 * @throws Error if NMI partner credentials are not configured
 */
export async function submitBoardingApplication(
  input: BoardingApplicationInput,
  logger?: BoardingLogger
): Promise<BoardingSubmissionResult> {
  if (!isNmiConfigured()) {
    throw new Error(
      "NMI partner credentials not configured. " +
        "Set NMI_PARTNER_ID and NMI_PARTNER_KEY environment variables."
    );
  }

  // Decrypt sensitive fields just before the API call.
  // These plaintext values only live in this function scope.
  const plainEin = decryptIfNeeded(input.businessEin);
  const plainSsn = decryptIfNeeded(input.ownerSsn);
  const plainRoutingNumber = decryptIfNeeded(input.bankRoutingNumber);
  const plainAccountNumber = decryptIfNeeded(input.bankAccountNumber);

  // Map our application fields to NMI Partner API fields
  const payload: Record<string, string> = {
    // Action
    type: "boarding",

    // Business information
    legal_name: input.businessLegalName,
    dba_name: input.businessDba,
    federal_tax_id: plainEin,
    business_type: input.businessType,
    business_address1: input.businessAddress,
    business_city: input.businessCity,
    business_state: input.businessState,
    business_zip: input.businessZip,
    business_phone: input.businessPhone,
    business_email: input.businessEmail,
    ...(input.businessWebsite && { website: input.businessWebsite }),
    ...(input.businessStartDate && { business_start_date: input.businessStartDate }),
    ...(input.annualVolume && { annual_volume: input.annualVolume }),
    ...(input.averageTicket && { average_ticket: input.averageTicket }),

    // Owner / principal information
    owner_first_name: input.ownerFirstName,
    owner_last_name: input.ownerLastName,
    owner_email: input.ownerEmail,
    owner_phone: input.ownerPhone,
    owner_dob: input.ownerDob,
    owner_ssn: plainSsn,
    owner_address1: input.ownerAddress,
    owner_city: input.ownerCity,
    owner_state: input.ownerState,
    owner_zip: input.ownerZip,
    ...(input.ownerOwnershipPct && { owner_ownership_pct: input.ownerOwnershipPct }),

    // Bank information
    bank_routing_number: plainRoutingNumber,
    bank_account_number: plainAccountNumber,
    bank_account_type: input.bankAccountType,
    ...(input.bankName && { bank_name: input.bankName }),

    // Reference IDs
    partner_reference_id: input.applicationId,
  };

  // Log submission (without sensitive data)
  const timer = createTimer();
  logger?.info(
    {
      action: 'nmi_boarding_submit_initiated',
      merchantId: input.merchantId,
      applicationId: input.applicationId,
      businessName: input.businessLegalName,
    },
    `Submitting boarding application for merchant ${input.merchantId}`
  );

  let response: NmiApiResponse;
  try {
    response = await nmiPost("/boarding", payload);
  } catch (err) {
    const duration_ms = timer.elapsed();
    logger?.error(
      {
        action: 'nmi_boarding_submit_failed',
        applicationId: input.applicationId,
        merchantId: input.merchantId,
        duration_ms,
        err,
      },
      `Boarding API request failed for application ${input.applicationId}`
    );
    throw err;
  }

  const parsed = parseNmiResponse(response.body);
  const duration_ms = timer.elapsed();

  // Log response status (never log credentials)
  logger?.info(
    {
      action: 'nmi_boarding_submit_response',
      applicationId: input.applicationId,
      merchantId: input.merchantId,
      status: parsed.status || parsed.response_code || "unknown",
      httpStatus: response.statusCode,
      duration_ms,
    },
    `Boarding response for application ${input.applicationId}: status=${parsed.status || parsed.response_code || "unknown"}`
  );

  // Check for API-level errors
  if (response.statusCode >= 400) {
    return {
      success: false,
      status: "DECLINED",
      message: parsed.message || parsed.error || `HTTP ${response.statusCode}`,
      declineReason: parsed.message || parsed.error || "API error",
    };
  }

  // Parse NMI response into our result format
  const responseStatus = (
    parsed.status || parsed.response || ""
  ).toUpperCase();

  // Handle instant approval
  if (
    responseStatus === "APPROVED" ||
    responseStatus === "1" ||
    parsed.response_code === "100"
  ) {
    return {
      success: true,
      nmiApplicationId: parsed.boarding_id || parsed.application_id || parsed.id,
      status: "APPROVED",
      nmiMerchantId: parsed.merchant_id || parsed.mid,
      securityKey: parsed.security_key || parsed.gateway_key,
      tokenizationKey: parsed.tokenization_key || parsed.collect_key,
      message: parsed.message || "Approved",
    };
  }

  // Handle pending/under review
  if (
    responseStatus === "PENDING" ||
    responseStatus === "UNDER_REVIEW" ||
    responseStatus === "2" ||
    parsed.response_code === "200"
  ) {
    return {
      success: true,
      nmiApplicationId: parsed.boarding_id || parsed.application_id || parsed.id,
      status: "UNDER_REVIEW",
      message: parsed.message || "Under review",
    };
  }

  // Handle decline
  if (
    responseStatus === "DECLINED" ||
    responseStatus === "REJECTED" ||
    responseStatus === "3" ||
    parsed.response_code === "300"
  ) {
    return {
      success: false,
      nmiApplicationId: parsed.boarding_id || parsed.application_id || parsed.id,
      status: "DECLINED",
      declineReason:
        parsed.decline_reason || parsed.message || "Application declined",
      message: parsed.message || "Declined",
    };
  }

  // Unknown status — treat as pending to be safe
  logger?.warn(
    {
      action: 'nmi_boarding_unknown_status',
      applicationId: input.applicationId,
      merchantId: input.merchantId,
      responseStatus,
    },
    `Unknown boarding response status: ${responseStatus}. Treating as UNDER_REVIEW.`
  );
  return {
    success: true,
    nmiApplicationId: parsed.boarding_id || parsed.application_id || parsed.id,
    status: "UNDER_REVIEW",
    message: parsed.message || `Unknown status: ${responseStatus}`,
  };
}

/**
 * Check the boarding status of a previously submitted application.
 *
 * @param nmiApplicationId — The NMI boarding/application ID from the initial submission
 * @returns Current boarding status with credentials if approved
 *
 * @throws Error if NMI partner credentials are not configured
 */
export async function checkBoardingStatus(
  nmiApplicationId: string,
  logger?: BoardingLogger
): Promise<BoardingStatusResult> {
  if (!isNmiConfigured()) {
    throw new Error(
      "NMI partner credentials not configured. " +
        "Set NMI_PARTNER_ID and NMI_PARTNER_KEY environment variables."
    );
  }

  const statusTimer = createTimer();
  logger?.info(
    {
      action: 'nmi_boarding_status_check_initiated',
      nmiApplicationId,
    },
    `Checking boarding status for NMI application ${nmiApplicationId}`
  );

  const payload: Record<string, string> = {
    type: "boarding_status",
    boarding_id: nmiApplicationId,
  };

  let response: NmiApiResponse;
  try {
    response = await nmiPost("/boarding/status", payload);
  } catch (err) {
    const duration_ms = statusTimer.elapsed();
    logger?.error(
      {
        action: 'nmi_boarding_status_check_failed',
        nmiApplicationId,
        duration_ms,
        err,
      },
      `Boarding status check failed for ${nmiApplicationId}`
    );
    throw err;
  }

  const parsed = parseNmiResponse(response.body);
  const statusDuration = statusTimer.elapsed();

  logger?.info(
    {
      action: 'nmi_boarding_status_check_response',
      nmiApplicationId,
      status: parsed.status || parsed.response_code || "unknown",
      duration_ms: statusDuration,
    },
    `Boarding status check result for ${nmiApplicationId}: status=${parsed.status || parsed.response_code || "unknown"}`
  );

  const responseStatus = (
    parsed.status || parsed.response || ""
  ).toUpperCase();

  // Approved — credentials should be in the response
  if (
    responseStatus === "APPROVED" ||
    responseStatus === "1" ||
    parsed.response_code === "100"
  ) {
    return {
      nmiApplicationId,
      status: "APPROVED",
      nmiMerchantId: parsed.merchant_id || parsed.mid,
      securityKey: parsed.security_key || parsed.gateway_key,
      tokenizationKey: parsed.tokenization_key || parsed.collect_key,
      message: parsed.message || "Approved",
    };
  }

  // Still pending / under review
  if (
    responseStatus === "PENDING" ||
    responseStatus === "UNDER_REVIEW" ||
    responseStatus === "2" ||
    parsed.response_code === "200"
  ) {
    return {
      nmiApplicationId,
      status: "UNDER_REVIEW",
      message: parsed.message || "Still under review",
    };
  }

  // Declined
  if (
    responseStatus === "DECLINED" ||
    responseStatus === "REJECTED" ||
    responseStatus === "3" ||
    parsed.response_code === "300"
  ) {
    return {
      nmiApplicationId,
      status: "DECLINED",
      declineReason:
        parsed.decline_reason || parsed.message || "Application declined",
      message: parsed.message || "Declined",
    };
  }

  // Unknown — treat as still under review
  return {
    nmiApplicationId,
    status: "UNDER_REVIEW",
    message: parsed.message || `Unknown status: ${responseStatus}`,
  };
}

/**
 * Process an approval result: encrypt and return credentials for storage.
 *
 * This helper takes raw NMI credentials from an approval response and
 * encrypts them for safe storage on the Merchant record.
 *
 * @returns Object with encrypted credential values ready for Merchant.update()
 */
export function encryptNmiCredentials(result: {
  nmiMerchantId?: string;
  securityKey?: string;
  tokenizationKey?: string;
}): {
  nmiMerchantId: string | null;
  nmiSecurityKey: string | null;
  nmiTokenizationKey: string | null;
} {
  return {
    nmiMerchantId: result.nmiMerchantId || null,
    nmiSecurityKey: result.securityKey ? encrypt(result.securityKey) : null,
    nmiTokenizationKey: result.tokenizationKey
      ? encrypt(result.tokenizationKey)
      : null,
  };
}

// ---------------------------------------------------------------------------
// Exports for testing
// ---------------------------------------------------------------------------

export {
  parseNmiResponse as _parseNmiResponse,
  decryptIfNeeded as _decryptIfNeeded,
  nmiPost as _nmiPost,
  getConfig as _getConfig,
};
