import { env } from '../utils/env.js';

export interface NmiChargeParams {
  paymentToken: string;
  amountCents: number;
  orderId: string;
  customerEmail?: string;
  customerFirstName?: string;
  customerLastName?: string;
  ipAddress?: string;
  /** Merchant's own NMI security key */
  securityKey: string;
}

export interface NmiChargeResult {
  success: boolean;
  transactionId: string;
  responseCode: string;
  responseText: string;
  authCode?: string;
  cardBrand?: string;
  cardLast4?: string;
  cardExpMonth?: string;
  cardExpYear?: string;
}

export interface NmiRefundParams {
  transactionId: string;
  amountCents?: number; // partial refund if provided
  /** Merchant's own NMI security key */
  securityKey: string;
}

export interface NmiVoidParams {
  transactionId: string;
  /** Merchant's own NMI security key */
  securityKey: string;
}

function isMockMode(): boolean {
  return env.NMI_MOCK_MODE === 'true';
}

async function nmiPost(securityKey: string, params: Record<string, string>): Promise<Record<string, string>> {
  const body = new URLSearchParams({
    security_key: securityKey,
    ...params,
  });

  const response = await fetch('https://secure.nmi.com/api/transact.php', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: body.toString(),
  });

  const text = await response.text();
  const result: Record<string, string> = {};
  for (const pair of text.split('&')) {
    const [key, value] = pair.split('=');
    result[decodeURIComponent(key)] = decodeURIComponent(value || '');
  }
  return result;
}

export async function chargeCard(params: NmiChargeParams): Promise<NmiChargeResult> {
  if (isMockMode()) {
    return mockCharge(params);
  }

  const amountDollars = (params.amountCents / 100).toFixed(2);

  const result = await nmiPost(params.securityKey, {
    type: 'sale',
    payment_token: params.paymentToken,
    amount: amountDollars,
    orderid: params.orderId,
    ...(params.customerEmail && { email: params.customerEmail }),
    ...(params.customerFirstName && { first_name: params.customerFirstName }),
    ...(params.customerLastName && { last_name: params.customerLastName }),
    ...(params.ipAddress && { ip_address: params.ipAddress }),
  });

  return {
    success: result.response === '1',
    transactionId: result.transactionid || '',
    responseCode: result.response_code || '',
    responseText: result.responsetext || '',
    authCode: result.authcode,
    cardBrand: result.cc_type,
    cardLast4: result.cc_number?.slice(-4),
  };
}

export async function refundTransaction(params: NmiRefundParams): Promise<NmiChargeResult> {
  if (isMockMode()) {
    return {
      success: true,
      transactionId: params.transactionId,
      responseCode: '100',
      responseText: 'SUCCESS',
    };
  }

  const refundParams: Record<string, string> = {
    type: 'refund',
    transactionid: params.transactionId,
  };

  if (params.amountCents) {
    refundParams.amount = (params.amountCents / 100).toFixed(2);
  }

  const result = await nmiPost(params.securityKey, refundParams);

  return {
    success: result.response === '1',
    transactionId: result.transactionid || params.transactionId,
    responseCode: result.response_code || '',
    responseText: result.responsetext || '',
  };
}

export async function voidTransaction(params: NmiVoidParams): Promise<NmiChargeResult> {
  if (isMockMode()) {
    return {
      success: true,
      transactionId: params.transactionId,
      responseCode: '100',
      responseText: 'Transaction Void Successful',
    };
  }

  const result = await nmiPost(params.securityKey, {
    type: 'void',
    transactionid: params.transactionId,
  });

  return {
    success: result.response === '1',
    transactionId: result.transactionid || params.transactionId,
    responseCode: result.response_code || '',
    responseText: result.responsetext || '',
  };
}

function mockCharge(params: NmiChargeParams): NmiChargeResult {
  const mockTxnId = `mock_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
  return {
    success: true,
    transactionId: mockTxnId,
    responseCode: '100',
    responseText: 'SUCCESS',
    authCode: 'MOCK123',
    cardBrand: 'Visa',
    cardLast4: '4242',
    cardExpMonth: '12',
    cardExpYear: '2028',
  };
}
