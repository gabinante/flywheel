import { env } from '../utils/env.js';

const BASE_URL = 'https://api.seamlesschex.com/v1';
const SANDBOX_URL = 'https://sandbox.seamlesschex.com/v1';

function getBaseUrl(): string {
  return env.SEAMLESSCHEX_SANDBOX === 'true' ? SANDBOX_URL : BASE_URL;
}

export interface CreateCheckParams {
  amountCents: number;
  routingNumber: string;
  accountNumber: string;
  accountType: 'checking' | 'savings';
  nameOnAccount: string;
  memo?: string;
  merchantReference?: string;
  email?: string;
  phone?: string;
  address?: string;
  city?: string;
  state?: string;
  zip?: string;
  /** Merchant's own Seamlesschex API key */
  apiKey: string;
}

export interface CreateCheckResult {
  success: boolean;
  checkId: string;
  status: string;
  error?: string;
}

export interface CheckStatusResult {
  checkId: string;
  status: string;
  returnCode?: string;
  returnReason?: string;
  settledAt?: string;
}

async function seamlesschexRequest(
  apiKey: string,
  method: string,
  path: string,
  body?: Record<string, unknown>
): Promise<Record<string, unknown>> {
  const url = `${getBaseUrl()}${path}`;

  const response = await fetch(url, {
    method,
    headers: {
      'Authorization': `Bearer ${apiKey}`,
      'Content-Type': 'application/json',
    },
    ...(body && { body: JSON.stringify(body) }),
  });

  return response.json() as Promise<Record<string, unknown>>;
}

export async function createCheck(params: CreateCheckParams): Promise<CreateCheckResult> {
  if (env.SEAMLESSCHEX_SANDBOX === 'true' && !params.apiKey) {
    return mockCreateCheck();
  }

  const amountDollars = (params.amountCents / 100).toFixed(2);

  const result = await seamlesschexRequest(params.apiKey, 'POST', '/check/create', {
    amount: amountDollars,
    routing_number: params.routingNumber,
    account_number: params.accountNumber,
    account_type: params.accountType,
    name: params.nameOnAccount,
    memo: params.memo || '',
    number: params.merchantReference || '',
    email: params.email || '',
    phone: params.phone || '',
    address: params.address || '',
    city: params.city || '',
    state: params.state || '',
    zip: params.zip || '',
  });

  if (result.success) {
    const data = result.data as Record<string, string>;
    return {
      success: true,
      checkId: data.check_id,
      status: data.status || 'created',
    };
  }

  return {
    success: false,
    checkId: '',
    status: 'error',
    error: (result.message as string) || 'Unknown error',
  };
}

export async function getCheckStatus(checkId: string, apiKey: string): Promise<CheckStatusResult> {
  if (env.SEAMLESSCHEX_SANDBOX === 'true' && !apiKey) {
    return { checkId, status: 'cleared' };
  }

  const result = await seamlesschexRequest(apiKey, 'GET', `/check/${checkId}`);
  const data = result.data as Record<string, string>;

  return {
    checkId,
    status: data?.status || 'unknown',
    returnCode: data?.return_code,
    returnReason: data?.return_reason,
    settledAt: data?.settled_at,
  };
}

export async function voidCheck(checkId: string, apiKey: string): Promise<{ success: boolean; error?: string }> {
  if (env.SEAMLESSCHEX_SANDBOX === 'true' && !apiKey) {
    return { success: true };
  }

  const result = await seamlesschexRequest(apiKey, 'PUT', `/check/${checkId}/void`);
  return {
    success: !!result.success,
    error: result.message as string | undefined,
  };
}

function mockCreateCheck(): CreateCheckResult {
  return {
    success: true,
    checkId: `mock_chk_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`,
    status: 'created',
  };
}
