export interface GoHighPaymentConfig {
  merchantKey: string;
  baseUrl?: string;
}

export interface MountOptions {
  merchantKey: string;
  amount: number;
  currency?: string;
  description?: string;
  customerEmail?: string;
  customerName?: string;
  metadata?: Record<string, unknown>;
  onSuccess?: (result: PaymentResult) => void;
  onError?: (error: PaymentError) => void;
  onCancel?: () => void;
}

export interface PaymentResult {
  transactionId: string;
  status: string;
  amountCents: number;
  paymentMethod: string;
}

export interface PaymentError {
  code: string;
  message: string;
}

const DEFAULT_BASE_URL = 'https://api.gohighpayment.com';
const DEFAULT_CHECKOUT_URL = 'https://checkout.gohighpayment.com';

class GoHighPaymentSDK {
  private baseUrl: string;
  private checkoutUrl: string;

  constructor() {
    this.baseUrl = DEFAULT_BASE_URL;
    this.checkoutUrl = DEFAULT_CHECKOUT_URL;
  }

  configure(options: { baseUrl?: string; checkoutUrl?: string }) {
    if (options.baseUrl) this.baseUrl = options.baseUrl;
    if (options.checkoutUrl) this.checkoutUrl = options.checkoutUrl;
  }

  /**
   * Redirect mode - create a session server-side, then redirect the customer
   */
  redirect(options: { sessionId: string }) {
    window.location.href = `${this.checkoutUrl}/${options.sessionId}`;
  }

  /**
   * Embed mode - mount a payment form in an iframe within the current page
   */
  mount(selector: string, options: MountOptions) {
    const container = document.querySelector(selector);
    if (!container) {
      throw new Error(`GoHighPayment: Element not found for selector "${selector}"`);
    }

    const iframe = document.createElement('iframe');
    iframe.style.width = '100%';
    iframe.style.border = 'none';
    iframe.style.minHeight = '500px';

    // Create session first, then load iframe
    this.createSession(options)
      .then((session) => {
        iframe.src = `${this.checkoutUrl}/${session.sessionId}?embed=true`;
        container.appendChild(iframe);

        // Listen for messages from the iframe
        const messageHandler = (event: MessageEvent) => {
          if (event.origin !== new URL(this.checkoutUrl).origin) return;

          const { type, data } = event.data;

          switch (type) {
            case 'ghp:success':
              options.onSuccess?.(data);
              break;
            case 'ghp:error':
              options.onError?.(data);
              break;
            case 'ghp:cancel':
              options.onCancel?.();
              break;
            case 'ghp:resize':
              iframe.style.height = `${data.height}px`;
              break;
          }
        };

        window.addEventListener('message', messageHandler);

        // Return cleanup function
        return () => {
          window.removeEventListener('message', messageHandler);
          container.removeChild(iframe);
        };
      })
      .catch((error) => {
        options.onError?.({
          code: 'SESSION_CREATE_FAILED',
          message: error.message || 'Failed to create checkout session',
        });
      });
  }

  private async createSession(options: MountOptions): Promise<{ sessionId: string; checkoutUrl: string }> {
    const response = await fetch(`${this.baseUrl}/api/v1/checkout/sessions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-API-Key': options.merchantKey,
      },
      body: JSON.stringify({
        amountCents: options.amount,
        currency: options.currency || 'USD',
        description: options.description,
        customerEmail: options.customerEmail,
        customerName: options.customerName,
        metadata: options.metadata,
      }),
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Request failed' }));
      throw new Error(error.error || `HTTP ${response.status}`);
    }

    return response.json();
  }
}

// Export singleton
export const GoHighPayment = new GoHighPaymentSDK();

// Also export as default for UMD usage
export default GoHighPayment;

// Attach to window for script tag usage
if (typeof window !== 'undefined') {
  (window as unknown as Record<string, unknown>).GoHighPayment = GoHighPayment;
}
