/**
 * NMI Payment Gateway Service
 *
 * Handles interactions with the NMI (Network Merchants Inc) payment gateway
 * including transactions, Customer Vault, and recurring billing.
 *
 * All API calls are logged with structured JSON including:
 * - correlationId for end-to-end tracing
 * - duration_ms for performance monitoring
 * - Sensitive fields (security_key) are never logged
 *
 * NMI API docs: https://secure.nmi.com/merchants/resources/integration/integration_portal.php
 */

import { createTimer } from '../utils/logger.js';

export interface NmiConfig {
  securityKey: string;
  apiUrl?: string;
}

export interface NmiTransactionParams {
  amount: number; // in cents
  paymentToken: string;
  currency?: string;
  orderId?: string;
  description?: string;
  correlationId?: string;
  merchantId?: string;
}

export interface NmiAuthParams {
  amount: number; // 0 for $0 auth
  paymentToken: string;
  currency?: string;
  orderId?: string;
  correlationId?: string;
  merchantId?: string;
}

export interface NmiVaultParams {
  paymentToken: string;
  customerId: string;
  firstName?: string;
  lastName?: string;
  email?: string;
  correlationId?: string;
  merchantId?: string;
}

export interface NmiRecurringPlanParams {
  planId: string;
  planName: string;
  amount: number; // in cents
  dayFrequency?: number;
  monthFrequency?: number;
  dayOfMonth?: number;
  correlationId?: string;
  merchantId?: string;
}

export interface NmiSubscriptionParams {
  planId: string;
  customerVaultId: string;
  startDate?: string; // YYYYMMDD
  correlationId?: string;
  merchantId?: string;
}

export interface NmiResponse {
  response: string; // '1' = approved, '2' = declined, '3' = error
  responsetext: string;
  transactionid?: string;
  customer_vault_id?: string;
  subscription_id?: string;
  [key: string]: string | undefined;
}

/**
 * Logger interface — accepts any pino-compatible logger.
 */
interface NmiLogger {
  info(obj: Record<string, unknown>, msg: string): void;
  warn(obj: Record<string, unknown>, msg: string): void;
  error(obj: Record<string, unknown>, msg: string): void;
}

/**
 * Convert billing interval to NMI frequency parameters.
 */
export function intervalToNmiFrequency(interval: string): {
  dayFrequency?: number;
  monthFrequency?: number;
} {
  switch (interval) {
    case 'WEEKLY':
      return { dayFrequency: 7 };
    case 'MONTHLY':
      return { monthFrequency: 1 };
    case 'QUARTERLY':
      return { monthFrequency: 3 };
    case 'ANNUAL':
      return { monthFrequency: 12 };
    default:
      return { monthFrequency: 1 };
  }
}

/**
 * Format date as YYYYMMDD for NMI API.
 */
function formatNmiDate(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  return `${y}${m}${d}`;
}

/**
 * Parse NMI URL-encoded response into key-value pairs.
 */
function parseNmiResponse(responseText: string): NmiResponse {
  const params = new URLSearchParams(responseText);
  const result: Record<string, string> = {};
  for (const [key, value] of params.entries()) {
    result[key] = value;
  }
  return result as unknown as NmiResponse;
}

export class NmiService {
  private securityKey: string;
  private apiUrl: string;
  private logger: NmiLogger | null;

  constructor(config: NmiConfig & { logger?: NmiLogger }) {
    this.securityKey = config.securityKey;
    this.apiUrl = config.apiUrl ?? 'https://secure.nmi.com/api/transact.php';
    this.logger = config.logger ?? null;
  }

  /**
   * Make an API call to NMI with structured logging and duration tracking.
   */
  private async apiCall(
    params: Record<string, string | number>,
    meta?: { action?: string; correlationId?: string; merchantId?: string }
  ): Promise<NmiResponse> {
    const timer = createTimer();
    const action = meta?.action ?? params.type?.toString() ?? 'nmi_api_call';

    this.logger?.info(
      {
        action: `nmi_${action}_initiated`,
        correlationId: meta?.correlationId,
        merchantId: meta?.merchantId,
        nmiAction: params.type ?? params.recurring ?? params.customer_vault,
      },
      `NMI ${action} initiated`
    );

    const body = new URLSearchParams();
    body.set('security_key', this.securityKey);

    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== null) {
        body.set(key, String(value));
      }
    }

    try {
      const response = await fetch(this.apiUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: body.toString(),
      });

      const text = await response.text();
      const parsed = parseNmiResponse(text);
      const duration_ms = timer.elapsed();

      const approved = parsed.response === '1';
      const logMeta = {
        action: `nmi_${action}_completed`,
        correlationId: meta?.correlationId,
        merchantId: meta?.merchantId,
        duration_ms,
        nmiResponse: parsed.response,
        nmiResponseText: parsed.responsetext,
        transactionId: parsed.transactionid,
        approved,
      };

      if (approved) {
        this.logger?.info(logMeta, `NMI ${action} completed successfully`);
      } else {
        this.logger?.warn(logMeta, `NMI ${action} returned non-approved response`);
      }

      return parsed;
    } catch (err) {
      const duration_ms = timer.elapsed();
      this.logger?.error(
        {
          action: `nmi_${action}_failed`,
          correlationId: meta?.correlationId,
          merchantId: meta?.merchantId,
          duration_ms,
          err,
        },
        `NMI ${action} failed`
      );
      throw err;
    }
  }

  /**
   * Process a sale transaction.
   */
  async sale(params: NmiTransactionParams): Promise<NmiResponse> {
    const amountDollars = (params.amount / 100).toFixed(2);

    return this.apiCall(
      {
        type: 'sale',
        payment_token: params.paymentToken,
        amount: amountDollars,
        currency: params.currency ?? 'USD',
        ...(params.orderId && { orderid: params.orderId }),
        ...(params.description && { order_description: params.description }),
      },
      {
        action: 'sale',
        correlationId: params.correlationId,
        merchantId: params.merchantId,
      }
    );
  }

  /**
   * Process an authorization (no capture).
   * Use for $0 auth on trial subscriptions.
   */
  async authorize(params: NmiAuthParams): Promise<NmiResponse> {
    const amountDollars = (params.amount / 100).toFixed(2);

    return this.apiCall(
      {
        type: 'auth',
        payment_token: params.paymentToken,
        amount: amountDollars,
        currency: params.currency ?? 'USD',
        ...(params.orderId && { orderid: params.orderId }),
      },
      {
        action: 'authorize',
        correlationId: params.correlationId,
        merchantId: params.merchantId,
      }
    );
  }

  /**
   * Add a customer to the NMI Customer Vault.
   * Required for recurring billing — stores payment info securely.
   */
  async addToCustomerVault(params: NmiVaultParams): Promise<NmiResponse> {
    return this.apiCall(
      {
        customer_vault: 'add_customer',
        payment_token: params.paymentToken,
        customer_id: params.customerId,
        ...(params.firstName && { first_name: params.firstName }),
        ...(params.lastName && { last_name: params.lastName }),
        ...(params.email && { email: params.email }),
      },
      {
        action: 'add_to_vault',
        correlationId: params.correlationId,
        merchantId: params.merchantId,
      }
    );
  }

  /**
   * Create a recurring plan in NMI.
   * Plans define billing frequency and amount.
   */
  async createRecurringPlan(
    params: NmiRecurringPlanParams
  ): Promise<NmiResponse> {
    const amountDollars = (params.amount / 100).toFixed(2);

    const apiParams: Record<string, string | number> = {
      recurring: 'add_plan',
      plan_id: params.planId,
      plan_name: params.planName,
      plan_amount: amountDollars,
      plan_payments: '0', // 0 = unlimited recurring payments
    };

    if (params.dayFrequency) {
      apiParams.day_frequency = params.dayFrequency;
    } else if (params.monthFrequency) {
      apiParams.month_frequency = params.monthFrequency;
    }

    if (params.dayOfMonth) {
      apiParams.day_of_month = params.dayOfMonth;
    }

    return this.apiCall(apiParams, {
      action: 'create_plan',
      correlationId: params.correlationId,
      merchantId: params.merchantId,
    });
  }

  /**
   * Add a subscription for a customer to a recurring plan.
   * The customer must already be in the Customer Vault.
   */
  async addSubscription(params: NmiSubscriptionParams): Promise<NmiResponse> {
    const startDate =
      params.startDate ?? formatNmiDate(new Date());

    return this.apiCall(
      {
        recurring: 'add_subscription',
        plan_id: params.planId,
        customer_vault_id: params.customerVaultId,
        start_date: startDate,
      },
      {
        action: 'add_subscription',
        correlationId: params.correlationId,
        merchantId: params.merchantId,
      }
    );
  }

  /**
   * Cancel (delete) a recurring subscription in NMI.
   */
  async cancelSubscription(
    nmiSubscriptionId: string,
    meta?: { correlationId?: string; merchantId?: string }
  ): Promise<NmiResponse> {
    return this.apiCall(
      {
        recurring: 'delete_subscription',
        subscription_id: nmiSubscriptionId,
      },
      {
        action: 'cancel_subscription',
        correlationId: meta?.correlationId,
        merchantId: meta?.merchantId,
      }
    );
  }

  /**
   * Process a sale using a Customer Vault ID (for manual charges).
   */
  async saleWithVault(
    customerVaultId: string,
    amount: number,
    currency: string = 'USD',
    meta?: { correlationId?: string; merchantId?: string }
  ): Promise<NmiResponse> {
    const amountDollars = (amount / 100).toFixed(2);

    return this.apiCall(
      {
        type: 'sale',
        customer_vault_id: customerVaultId,
        amount: amountDollars,
        currency,
      },
      {
        action: 'sale_with_vault',
        correlationId: meta?.correlationId,
        merchantId: meta?.merchantId,
      }
    );
  }
}

export default NmiService;
