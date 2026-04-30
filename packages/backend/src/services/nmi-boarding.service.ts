/**
 * NMI Partner Boarding API service (production).
 *
 * Handles merchant boarding through NMI's Partner API:
 * - Submit boarding applications with real merchant data
 * - Poll boarding status for PENDING/UNDER_REVIEW applications
 * - Return NMI credentials for encrypted storage on approval
 *
 * Environment variables:
 *   NMI_PARTNER_ID      — NMI Partner ID (required for real boarding)
 *   NMI_PARTNER_KEY     — NMI Partner API key (sensitive, required)
 *   NMI_BOARDING_API_URL — NMI Boarding API base URL (default: https://secure.nmi.com/api/boarding/)
 *
 * Security:
 *   - Partner credentials (NMI_PARTNER_ID, NMI_PARTNER_KEY) are NEVER logged
 *   - Merchant security keys / tokenization keys are NEVER logged
 *   - Application PII (SSN, bank info) decrypted only within the API call scope
 *   - All API calls use HTTPS (enforced)
 */

import { env } from '../utils/env.js';
import { decrypt, isEncrypted } from '../utils/encryption.js';

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

/**
 * Check if NMI partner credentials are configured.
 */
export function isNmiConfigured(): boolean {
  return !!(env.NMI_PARTNER_ID && env.NMI_PARTNER_KEY);
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface BoardingApplication {
  // Business info
  businessName: string;
  businessType: string;
  ein?: string;            // may be encrypted
  website?: string;
  businessPhone?: string;
  businessDescription?: string;
  averageTicketCents?: number;
  monthlyVolumeCents?: number;

  // Business address
  businessAddress: string;
  businessCity: string;
  businessState: string;
  businessZip: string;

  // Owner info
  ownerFirstName: string;
  ownerLastName: string;
  ownerEmail: string;
  ownerPhone?: string;
  ownerDob: string;        // YYYY-MM-DD
  ownerSsnLast4: string;  // may be encrypted
  ownerAddress: string;
  ownerCity: string;
  ownerState: string;
  ownerZip: string;

  // Bank info (for NMI to settle to merchant)
  bankName: string;
  bankRoutingNumber: string;  // may be encrypted
  bankAccountNumber: string;  // may be encrypted
  bankAccountType: 'checking' | 'savings';
}

export interface BoardingResult {
  success: boolean;
  boardingId?: string;
  status?: 'PENDING' | 'APPROVED' | 'DECLINED';
  securityKey?: string;
  tokenizationKey?: string;
  nmiMerchantId?: string;
  error?: string;
}

export interface BoardingStatusResult {
  boardingId: string;
  status: 'PENDING' | 'APPROVED' | 'DECLINED';
  securityKey?: string;
  tokenizationKey?: string;
  nmiMerchantId?: string;
  declineReason?: string;
}

// ---------------------------------------------------------------------------
// PII decryption helper
// ---------------------------------------------------------------------------

/**
 * Decrypt a field value if it's encrypted, otherwise return as-is.
 * Plaintext values only live within the calling function's scope.
 */
function decryptIfNeeded(value: string | undefined | null): string {
  if (!value) return '';
  try {
    if (isEncrypted(value)) {
      return decrypt(value);
    }
  } catch {
    // If decryption fails, return as-is (may be plaintext)
  }
  return value;
}

// ---------------------------------------------------------------------------
// NMI API response parser
// ---------------------------------------------------------------------------

/**
 * Parse NMI's response body.
 * NMI Boarding API may return XML, key=value pairs, or JSON.
 */
export function parseNmiResponse(body: string): Record<string, string> {
  const result: Record<string, string> = {};

  // Try XML first (simple tag extraction)
  const xmlPattern = /<(\w+)>([^<]*)<\/\1>/g;
  let xmlMatch;
  let hasXml = false;
  while ((xmlMatch = xmlPattern.exec(body)) !== null) {
    hasXml = true;
    result[xmlMatch[1]] = xmlMatch[2];
  }
  if (hasXml) return result;

  // Try URL-encoded key=value
  if (body.includes('&') || body.includes('=')) {
    const params = new URLSearchParams(body);
    for (const [key, value] of params.entries()) {
      result[key] = value;
    }
    return result;
  }

  // Try JSON
  try {
    const json = JSON.parse(body) as Record<string, unknown>;
    if (typeof json === 'object' && json !== null) {
      for (const [key, value] of Object.entries(json)) {
        result[key] = String(value);
      }
    }
  } catch {
    result.raw = body;
  }

  return result;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Submit a merchant boarding application to the NMI Partner API.
 *
 * Decrypts PII fields (EIN, SSN, bank info) only for this API call.
 * On instant approval, returns NMI credentials for encryption and storage.
 *
 * Falls back to mock mode when NMI_PARTNER_ID / NMI_PARTNER_KEY are not set.
 */
export async function submitBoardingApplication(
  application: BoardingApplication
): Promise<BoardingResult> {
  if (!isNmiConfigured()) {
    console.warn('[NMI Boarding] Partner credentials not configured — using mock mode');
    return {
      success: true,
      boardingId: `mock_boarding_${Date.now()}`,
      status: 'PENDING',
    };
  }

  // Validate HTTPS
  const apiUrl = env.NMI_BOARDING_API_URL;
  if (!apiUrl.startsWith('https://')) {
    throw new Error(`NMI API calls must use HTTPS. Got: ${apiUrl}`);
  }

  // Decrypt sensitive PII fields — plaintext only within this scope
  const plainEin = decryptIfNeeded(application.ein);
  const plainSsnLast4 = decryptIfNeeded(application.ownerSsnLast4);
  const plainRoutingNumber = decryptIfNeeded(application.bankRoutingNumber);
  const plainAccountNumber = decryptIfNeeded(application.bankAccountNumber);

  // Build form-encoded payload for NMI Partner API
  const formParams = new URLSearchParams();
  formParams.set('partner_id', env.NMI_PARTNER_ID);
  formParams.set('partner_key', env.NMI_PARTNER_KEY);
  formParams.set('type', 'boarding');

  // Business
  formParams.set('legal_name', application.businessName);
  formParams.set('dba_name', application.businessName);
  formParams.set('business_type', application.businessType);
  if (plainEin) formParams.set('federal_tax_id', plainEin);
  if (application.website) formParams.set('website', application.website);
  if (application.businessPhone) formParams.set('business_phone', application.businessPhone);
  if (application.businessDescription) formParams.set('description', application.businessDescription);
  if (application.averageTicketCents) formParams.set('average_ticket', String(application.averageTicketCents / 100));
  if (application.monthlyVolumeCents) formParams.set('annual_volume', String(application.monthlyVolumeCents * 12 / 100));
  formParams.set('business_address1', application.businessAddress);
  formParams.set('business_city', application.businessCity);
  formParams.set('business_state', application.businessState);
  formParams.set('business_zip', application.businessZip);

  // Owner / principal
  formParams.set('owner_first_name', application.ownerFirstName);
  formParams.set('owner_last_name', application.ownerLastName);
  formParams.set('owner_email', application.ownerEmail);
  if (application.ownerPhone) formParams.set('owner_phone', application.ownerPhone);
  formParams.set('owner_dob', application.ownerDob);
  if (plainSsnLast4) formParams.set('owner_ssn', plainSsnLast4);
  formParams.set('owner_address1', application.ownerAddress);
  formParams.set('owner_city', application.ownerCity);
  formParams.set('owner_state', application.ownerState);
  formParams.set('owner_zip', application.ownerZip);

  // Bank
  if (application.bankName) formParams.set('bank_name', application.bankName);
  formParams.set('bank_routing_number', plainRoutingNumber);
  formParams.set('bank_account_number', plainAccountNumber);
  formParams.set('bank_account_type', application.bankAccountType);

  // Log submission (no credentials, no PII)
  console.log(
    `[NMI Boarding] Submitting application for business: ${application.businessName}`
  );

  let responseText: string;
  let httpStatus: number;

  try {
    const response = await fetch(apiUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: formParams.toString(),
      signal: AbortSignal.timeout(30_000),
    });
    httpStatus = response.status;
    responseText = await response.text();
  } catch (error) {
    console.error(
      '[NMI Boarding] Submission request failed:',
      error instanceof Error ? error.message : 'Unknown error'
    );
    return {
      success: false,
      error: error instanceof Error ? error.message : 'Failed to submit boarding application',
    };
  }

  const parsed = parseNmiResponse(responseText);

  // Log status (never log credentials)
  console.log(
    `[NMI Boarding] Response: httpStatus=${httpStatus}, nmiStatus=${parsed.status || parsed.response_code || 'unknown'}`
  );

  if (httpStatus >= 400) {
    return {
      success: false,
      error: `NMI boarding submission failed: HTTP ${httpStatus}`,
    };
  }

  const responseStatus = (parsed.status || parsed.response || '').toUpperCase();

  // Instant approval — credentials returned immediately
  if (
    responseStatus === 'APPROVED' ||
    responseStatus === '1' ||
    parsed.response_code === '100'
  ) {
    return {
      success: true,
      boardingId: parsed.boarding_id || parsed.application_id || parsed.id,
      status: 'APPROVED',
      nmiMerchantId: parsed.merchant_id || parsed.mid,
      securityKey: parsed.security_key || parsed.gateway_key,
      tokenizationKey: parsed.tokenization_key || parsed.collect_key,
    };
  }

  // Pending / under review — poll later
  if (
    responseStatus === 'PENDING' ||
    responseStatus === 'UNDER_REVIEW' ||
    responseStatus === '2' ||
    parsed.response_code === '200'
  ) {
    return {
      success: true,
      boardingId: parsed.boarding_id || parsed.application_id || parsed.id,
      status: 'PENDING',
    };
  }

  // Declined
  if (
    responseStatus === 'DECLINED' ||
    responseStatus === 'REJECTED' ||
    responseStatus === '3' ||
    parsed.response_code === '300'
  ) {
    return {
      success: false,
      boardingId: parsed.boarding_id || parsed.application_id || parsed.id,
      status: 'DECLINED',
      error: parsed.decline_reason || parsed.message || 'Application declined',
    };
  }

  // Unknown status — treat as pending to be safe
  console.warn(
    `[NMI Boarding] Unknown response status: ${responseStatus}. Treating as PENDING.`
  );
  return {
    success: true,
    boardingId: parsed.boarding_id || parsed.application_id || parsed.id,
    status: 'PENDING',
  };
}

/**
 * Check the boarding status of a previously submitted application.
 * Called by the 15-minute polling job for UNDER_REVIEW applications.
 */
export async function checkBoardingStatus(boardingId: string): Promise<BoardingStatusResult | null> {
  if (!isNmiConfigured()) {
    console.warn('[NMI Boarding] Partner credentials not configured — cannot check status');
    return null;
  }

  // Mock boarding IDs have no real status
  if (boardingId.startsWith('mock_boarding_')) {
    return null;
  }

  const apiUrl = env.NMI_BOARDING_API_URL;
  if (!apiUrl.startsWith('https://')) {
    throw new Error(`NMI API calls must use HTTPS. Got: ${apiUrl}`);
  }

  console.log(`[NMI Boarding] Checking status for boarding ID: ${boardingId}`);

  const formParams = new URLSearchParams();
  formParams.set('partner_id', env.NMI_PARTNER_ID);
  formParams.set('partner_key', env.NMI_PARTNER_KEY);
  formParams.set('type', 'boarding_status');
  formParams.set('boarding_id', boardingId);

  let responseText: string;
  let httpStatus: number;

  try {
    const response = await fetch(apiUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: formParams.toString(),
      signal: AbortSignal.timeout(15_000),
    });
    httpStatus = response.status;
    responseText = await response.text();
  } catch (error) {
    console.error(
      `[NMI Boarding] Status check request failed for ${boardingId}:`,
      error instanceof Error ? error.message : 'Unknown error'
    );
    return null;
  }

  if (httpStatus >= 400) {
    console.error(`[NMI Boarding] Status check HTTP error ${httpStatus} for ${boardingId}`);
    return null;
  }

  const parsed = parseNmiResponse(responseText);
  const responseStatus = (parsed.status || parsed.response || '').toUpperCase();

  console.log(
    `[NMI Boarding] Status for ${boardingId}: ${responseStatus}`
  );

  // Approved
  if (
    responseStatus === 'APPROVED' ||
    responseStatus === '1' ||
    parsed.response_code === '100'
  ) {
    return {
      boardingId,
      status: 'APPROVED',
      nmiMerchantId: parsed.merchant_id || parsed.mid,
      securityKey: parsed.security_key || parsed.gateway_key,
      tokenizationKey: parsed.tokenization_key || parsed.collect_key,
    };
  }

  // Declined
  if (
    responseStatus === 'DECLINED' ||
    responseStatus === 'REJECTED' ||
    responseStatus === '3' ||
    parsed.response_code === '300'
  ) {
    return {
      boardingId,
      status: 'DECLINED',
      declineReason: parsed.decline_reason || parsed.message || 'Application declined',
    };
  }

  // Still pending
  return {
    boardingId,
    status: 'PENDING',
  };
}

// ---------------------------------------------------------------------------
// Exports for testing
// ---------------------------------------------------------------------------
export { decryptIfNeeded as _decryptIfNeeded, parseNmiResponse as _parseNmiResponse };
