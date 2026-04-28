import { env } from '../utils/env.js';

export interface BoardingApplication {
  // Business info
  businessName: string;
  businessType: string;
  ein?: string;
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
  ownerSsnLast4: string;
  ownerAddress: string;
  ownerCity: string;
  ownerState: string;
  ownerZip: string;

  // Bank info (for NMI to settle to merchant)
  bankName: string;
  bankRoutingNumber: string;
  bankAccountNumber: string;
  bankAccountType: 'checking' | 'savings';
}

export interface BoardingResult {
  success: boolean;
  boardingId?: string;
  status?: 'PENDING' | 'APPROVED' | 'DECLINED';
  securityKey?: string;
  tokenizationKey?: string;
  error?: string;
}

export interface BoardingStatusResult {
  boardingId: string;
  status: 'PENDING' | 'APPROVED' | 'DECLINED';
  securityKey?: string;
  tokenizationKey?: string;
  declineReason?: string;
}

/**
 * Submit a merchant application to NMI's Boarding API.
 *
 * NMI's boarding API accepts merchant details and returns a boarding ID.
 * For low-risk merchants, approval can be instant (returning credentials).
 * For others, the application goes to underwriting review.
 *
 * Requires NMI_PARTNER_ID and NMI_PARTNER_KEY env vars.
 */
export async function submitBoardingApplication(
  application: BoardingApplication
): Promise<BoardingResult> {
  // If partner credentials aren't configured, return a mock/placeholder
  if (!env.NMI_PARTNER_ID || !env.NMI_PARTNER_KEY) {
    console.warn('[NMI Boarding] Partner credentials not configured — using mock mode');
    return {
      success: true,
      boardingId: `mock_boarding_${Date.now()}`,
      status: 'PENDING',
    };
  }

  try {
    const response = await fetch(env.NMI_BOARDING_API_URL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Basic ${Buffer.from(`${env.NMI_PARTNER_ID}:${env.NMI_PARTNER_KEY}`).toString('base64')}`,
      },
      body: JSON.stringify({
        // Map our fields to NMI's expected format
        // NOTE: Actual field names depend on NMI's boarding API spec.
        // Update these mappings once you have NMI partner API documentation.
        merchant: {
          dba_name: application.businessName,
          legal_name: application.businessName,
          business_type: application.businessType,
          federal_tax_id: application.ein,
          website: application.website,
          phone: application.businessPhone,
          description: application.businessDescription,
          average_ticket: application.averageTicketCents ? application.averageTicketCents / 100 : undefined,
          monthly_volume: application.monthlyVolumeCents ? application.monthlyVolumeCents / 100 : undefined,
          address: {
            line1: application.businessAddress,
            city: application.businessCity,
            state: application.businessState,
            zip: application.businessZip,
          },
        },
        principal: {
          first_name: application.ownerFirstName,
          last_name: application.ownerLastName,
          email: application.ownerEmail,
          phone: application.ownerPhone,
          date_of_birth: application.ownerDob,
          ssn_last4: application.ownerSsnLast4,
          address: {
            line1: application.ownerAddress,
            city: application.ownerCity,
            state: application.ownerState,
            zip: application.ownerZip,
          },
        },
        bank_account: {
          bank_name: application.bankName,
          routing_number: application.bankRoutingNumber,
          account_number: application.bankAccountNumber,
          account_type: application.bankAccountType,
        },
      }),
      signal: AbortSignal.timeout(30000),
    });

    if (!response.ok) {
      const errorText = await response.text();
      console.error('[NMI Boarding] Submission failed:', errorText);
      return { success: false, error: `NMI boarding submission failed: ${response.status}` };
    }

    const result = await response.json() as {
      boarding_id?: string;
      status?: string;
      security_key?: string;
      tokenization_key?: string;
    };

    return {
      success: true,
      boardingId: result.boarding_id,
      status: (result.status?.toUpperCase() as BoardingResult['status']) || 'PENDING',
      securityKey: result.security_key,
      tokenizationKey: result.tokenization_key,
    };
  } catch (error) {
    console.error('[NMI Boarding] Error:', error);
    return {
      success: false,
      error: error instanceof Error ? error.message : 'Failed to submit boarding application',
    };
  }
}

/**
 * Check the status of a boarding application.
 * Called by the polling job to see if a pending application has been approved.
 */
export async function checkBoardingStatus(boardingId: string): Promise<BoardingStatusResult | null> {
  if (!env.NMI_PARTNER_ID || !env.NMI_PARTNER_KEY) {
    console.warn('[NMI Boarding] Partner credentials not configured — cannot check status');
    return null;
  }

  // Mock boarding IDs won't have real status
  if (boardingId.startsWith('mock_boarding_')) {
    return null;
  }

  try {
    const response = await fetch(`${env.NMI_BOARDING_API_URL}${boardingId}`, {
      method: 'GET',
      headers: {
        'Authorization': `Basic ${Buffer.from(`${env.NMI_PARTNER_ID}:${env.NMI_PARTNER_KEY}`).toString('base64')}`,
      },
      signal: AbortSignal.timeout(15000),
    });

    if (!response.ok) {
      console.error(`[NMI Boarding] Status check failed for ${boardingId}: ${response.status}`);
      return null;
    }

    const result = await response.json() as {
      boarding_id: string;
      status: string;
      security_key?: string;
      tokenization_key?: string;
      decline_reason?: string;
    };

    return {
      boardingId: result.boarding_id,
      status: (result.status?.toUpperCase() as BoardingStatusResult['status']) || 'PENDING',
      securityKey: result.security_key,
      tokenizationKey: result.tokenization_key,
      declineReason: result.decline_reason,
    };
  } catch (error) {
    console.error(`[NMI Boarding] Status check error for ${boardingId}:`, error);
    return null;
  }
}
