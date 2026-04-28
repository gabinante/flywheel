import { useState, useEffect, useCallback, useMemo } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { motion, AnimatePresence } from 'framer-motion';

interface CheckoutSession {
  id: string;
  amountCents: number;
  currency: string;
  description?: string;
  customerEmail?: string;
  customerName?: string;
  status: string;
}

interface CheckoutConfig {
  tokenizationKey: string;
  collectJsUrl: string;
  googlePay: { enabled: boolean; merchantId?: string };
  applePay: { enabled: boolean; merchantId?: string };
}

type PaymentTab = 'card' | 'ach';

declare global {
  interface Window {
    CollectJS?: {
      configure: (config: Record<string, unknown>) => void;
      startPaymentRequest: () => void;
    };
  }
}

const API_BASE = '/api/v1/checkout';

/**
 * Send postMessage to parent when embedded in an iframe (GHL funnel via SDK).
 */
function notifyParent(type: string, data?: unknown) {
  if (window.self !== window.top) {
    try {
      window.parent.postMessage({ type, data }, '*');
    } catch {
      // cross-origin, ignore
    }
  }
}

export function CheckoutPage() {
  const { sessionId } = useParams<{ sessionId: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const isEmbed = useMemo(() => searchParams.get('embed') === 'true' || window.self !== window.top, [searchParams]);
  const [session, setSession] = useState<CheckoutSession | null>(null);
  const [loading, setLoading] = useState(true);
  const [processing, setProcessing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<PaymentTab>('card');
  const [collectReady, setCollectReady] = useState(false);
  const [config, setConfig] = useState<CheckoutConfig | null>(null);
  const [walletProcessing, setWalletProcessing] = useState<string | null>(null);

  // Notify parent of height changes when embedded
  useEffect(() => {
    if (!isEmbed) return;
    const observer = new ResizeObserver(() => {
      notifyParent('ghp:resize', { height: document.documentElement.scrollHeight });
    });
    observer.observe(document.body);
    return () => observer.disconnect();
  }, [isEmbed]);

  // ACH form state
  const [achForm, setAchForm] = useState({
    routingNumber: '',
    accountNumber: '',
    accountType: 'checking' as 'checking' | 'savings',
    nameOnAccount: '',
    email: '',
  });

  // Load session
  useEffect(() => {
    async function fetchSession() {
      try {
        const res = await fetch(`${API_BASE}/session/${sessionId}`);
        if (!res.ok) {
          const err = await res.json();
          setError(err.error || 'Session not found');
          return;
        }
        const data = await res.json();
        setSession(data);
      } catch {
        setError('Failed to load checkout session');
      } finally {
        setLoading(false);
      }
    }
    fetchSession();
  }, [sessionId]);

  // Fetch checkout config (per-session, since each merchant has their own tokenization key)
  useEffect(() => {
    if (!sessionId) return;
    async function fetchConfig() {
      const res = await fetch(`${API_BASE}/config/${sessionId}`);
      if (res.ok) {
        const data = await res.json();
        setConfig(data);
      }
    }
    fetchConfig();
  }, [sessionId]);

  // Load Collect.js for card payments (and wallet buttons)
  useEffect(() => {
    if (tab !== 'card' || !config?.tokenizationKey || !session) return;

    const existing = document.getElementById('collectjs-script');
    if (existing) existing.remove();

    const script = document.createElement('script');
    script.id = 'collectjs-script';
    script.src = config.collectJsUrl;
    script.setAttribute('data-tokenization-key', config.tokenizationKey);
    script.onload = () => {
      const amountDollars = (session.amountCents / 100).toFixed(2);

      const collectConfig: Record<string, unknown> = {
        variant: 'inline',
        fields: {
          ccnumber: { selector: '#cc-number', placeholder: '0000 0000 0000 0000' },
          ccexp: { selector: '#cc-exp', placeholder: 'MM / YY' },
          cvv: { selector: '#cc-cvv', placeholder: 'CVV' },
        },
        callback: handleCollectJsResponse,
      };

      // Enable Google Pay if configured
      if (config.googlePay?.enabled) {
        collectConfig.googleFont = 'Roboto';
        collectConfig.googlepay = {
          selector: '#google-pay-button',
          totalPriceStatus: 'FINAL',
          totalPrice: amountDollars,
          currencyCode: session.currency || 'USD',
          countryCode: 'US',
          merchantName: 'GoHighPayment',
          gatewayMerchantId: config.googlePay.merchantId,
          emailRequired: true,
          buttonType: 'pay',
          buttonColor: 'black',
          buttonSizeMode: 'fill',
        };
      }

      // Enable Apple Pay if configured
      if (config.applePay?.enabled) {
        collectConfig.applepay = {
          selector: '#apple-pay-button',
          totalAmount: amountDollars,
          currencyCode: session.currency || 'USD',
          countryCode: 'US',
          merchantName: 'GoHighPayment',
          type: 'pay',
        };
      }

      window.CollectJS?.configure(collectConfig);
      setCollectReady(true);
    };
    document.head.appendChild(script);
  }, [tab, config, session]);

  const handleCollectJsResponse = useCallback(async (response: { token: string; wallet?: string; card?: { type?: string } }) => {
    setProcessing(true);
    setWalletProcessing(null);
    setError(null);

    // Detect payment method from Collect.js response
    let paymentMethod: 'CARD' | 'GOOGLE_PAY' | 'APPLE_PAY' = 'CARD';
    if (response.wallet === 'google_pay' || response.wallet === 'googlepay') {
      paymentMethod = 'GOOGLE_PAY';
    } else if (response.wallet === 'apple_pay' || response.wallet === 'applepay') {
      paymentMethod = 'APPLE_PAY';
    }

    try {
      const res = await fetch(`${API_BASE}/process`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          sessionId,
          paymentToken: response.token,
          paymentMethod,
          customerEmail: session?.customerEmail,
        }),
      });

      const data = await res.json();

      if (data.success) {
        notifyParent('ghp:success', {
          transactionId: data.transactionId,
          status: data.status,
          amountCents: data.amountCents,
          paymentMethod: data.paymentMethod || paymentMethod,
        });
        if (!isEmbed) {
          navigate(`/confirmation/${sessionId}`);
        }
      } else {
        const msg = data.error || data.responseText || 'Payment failed';
        setError(msg);
        notifyParent('ghp:error', { code: 'PAYMENT_FAILED', message: msg });
      }
    } catch {
      setError('Payment processing error');
      notifyParent('ghp:error', { code: 'NETWORK_ERROR', message: 'Payment processing error' });
    } finally {
      setProcessing(false);
      setWalletProcessing(null);
    }
  }, [sessionId, session, navigate, isEmbed]);

  const handleCardSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!window.CollectJS) {
      setError('Payment form not ready');
      return;
    }
    setProcessing(true);
    window.CollectJS.startPaymentRequest();
  };

  const handleAchSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setProcessing(true);
    setError(null);

    try {
      const res = await fetch(`${API_BASE}/process-ach`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          sessionId,
          ...achForm,
        }),
      });

      const data = await res.json();

      if (data.success) {
        notifyParent('ghp:success', {
          transactionId: data.transactionId,
          status: data.status,
          amountCents: data.amountCents,
          paymentMethod: 'ACH',
        });
        if (!isEmbed) {
          navigate(`/confirmation/${sessionId}`);
        }
      } else {
        const msg = data.error || 'ACH payment failed';
        setError(msg);
        notifyParent('ghp:error', { code: 'PAYMENT_FAILED', message: msg });
      }
    } catch {
      setError('Payment processing error');
      notifyParent('ghp:error', { code: 'NETWORK_ERROR', message: 'Payment processing error' });
    } finally {
      setProcessing(false);
    }
  };

  const formatAmount = (cents: number) => {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
    }).format(cents / 100);
  };

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
      </div>
    );
  }

  if (error && !session) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="bg-white rounded-xl shadow-lg p-8 max-w-md text-center">
          <div className="text-red-500 text-lg font-semibold mb-2">Error</div>
          <p className="text-gray-600">{error}</p>
        </div>
      </div>
    );
  }

  if (!session) return null;

  return (
    <div className={isEmbed ? 'p-0' : 'min-h-screen bg-gradient-to-br from-blue-50 to-slate-100 flex items-center justify-center p-4'}>
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className={`bg-white w-full overflow-hidden ${isEmbed ? 'max-w-full' : 'rounded-2xl shadow-xl max-w-md'}`}
      >
        {/* Header */}
        <div className="bg-primary-600 text-white px-6 py-5">
          <div className="flex items-center gap-2 mb-1">
            <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
            <span className="font-semibold text-lg">GoHighPayment</span>
          </div>
          <div className="text-3xl font-bold">{formatAmount(session.amountCents)}</div>
          {session.description && (
            <div className="text-blue-100 text-sm mt-1">{session.description}</div>
          )}
        </div>

        {/* Digital wallet buttons */}
        {(config?.googlePay?.enabled || config?.applePay?.enabled) && (
          <div className="px-6 pt-5 space-y-3">
            {config?.googlePay?.enabled && (
              <div className="relative">
                <div id="google-pay-button" className="w-full h-10 rounded-lg overflow-hidden" />
                {walletProcessing === 'google' && (
                  <div className="absolute inset-0 flex items-center justify-center bg-white/70 rounded-lg">
                    <span className="animate-spin rounded-full h-5 w-5 border-b-2 border-gray-800" />
                  </div>
                )}
              </div>
            )}
            {config?.applePay?.enabled && (
              <div className="relative">
                <div id="apple-pay-button" className="w-full h-10 rounded-lg overflow-hidden" />
                {walletProcessing === 'apple' && (
                  <div className="absolute inset-0 flex items-center justify-center bg-white/70 rounded-lg">
                    <span className="animate-spin rounded-full h-5 w-5 border-b-2 border-gray-800" />
                  </div>
                )}
              </div>
            )}
            <div className="relative flex items-center">
              <div className="flex-grow border-t border-gray-200" />
              <span className="flex-shrink mx-3 text-xs text-gray-400 uppercase">or pay with</span>
              <div className="flex-grow border-t border-gray-200" />
            </div>
          </div>
        )}

        {/* Payment method tabs */}
        <div className="flex border-b border-gray-200">
          <button
            onClick={() => setTab('card')}
            className={`flex-1 py-3 text-sm font-medium text-center transition-colors ${
              tab === 'card'
                ? 'text-primary-600 border-b-2 border-primary-600'
                : 'text-gray-500 hover:text-gray-700'
            }`}
          >
            Card
          </button>
          <button
            onClick={() => setTab('ach')}
            className={`flex-1 py-3 text-sm font-medium text-center transition-colors ${
              tab === 'ach'
                ? 'text-primary-600 border-b-2 border-primary-600'
                : 'text-gray-500 hover:text-gray-700'
            }`}
          >
            Bank Account
          </button>
        </div>

        <div className="p-6">
          <AnimatePresence mode="wait">
            {error && (
              <motion.div
                initial={{ opacity: 0, height: 0 }}
                animate={{ opacity: 1, height: 'auto' }}
                exit={{ opacity: 0, height: 0 }}
                className="bg-red-50 text-red-700 rounded-lg p-3 mb-4 text-sm"
              >
                {error}
              </motion.div>
            )}
          </AnimatePresence>

          {/* Card form */}
          {tab === 'card' && (
            <motion.form
              key="card"
              initial={{ opacity: 0, x: -10 }}
              animate={{ opacity: 1, x: 0 }}
              onSubmit={handleCardSubmit}
              className="space-y-4"
            >
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Card Number</label>
                <div id="cc-number" className="h-10 bg-gray-50 rounded-lg border border-gray-200" />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Expiry</label>
                  <div id="cc-exp" className="h-10 bg-gray-50 rounded-lg border border-gray-200" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">CVV</label>
                  <div id="cc-cvv" className="h-10 bg-gray-50 rounded-lg border border-gray-200" />
                </div>
              </div>

              <button
                type="submit"
                disabled={processing || !collectReady}
                className="w-full bg-primary-600 hover:bg-primary-700 disabled:bg-gray-400 text-white font-semibold py-3 rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
              >
                {processing ? (
                  <span className="flex items-center justify-center gap-2">
                    <span className="animate-spin rounded-full h-4 w-4 border-b-2 border-white" />
                    Processing...
                  </span>
                ) : (
                  `Pay ${formatAmount(session.amountCents)}`
                )}
              </button>
            </motion.form>
          )}

          {/* ACH form */}
          {tab === 'ach' && (
            <motion.form
              key="ach"
              initial={{ opacity: 0, x: 10 }}
              animate={{ opacity: 1, x: 0 }}
              onSubmit={handleAchSubmit}
              className="space-y-4"
            >
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Name on Account</label>
                <input
                  type="text"
                  required
                  value={achForm.nameOnAccount}
                  onChange={e => setAchForm(f => ({ ...f, nameOnAccount: e.target.value }))}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Routing Number</label>
                <input
                  type="text"
                  required
                  maxLength={9}
                  pattern="[0-9]{9}"
                  value={achForm.routingNumber}
                  onChange={e => setAchForm(f => ({ ...f, routingNumber: e.target.value.replace(/\D/g, '') }))}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 font-mono"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Account Number</label>
                <input
                  type="text"
                  required
                  value={achForm.accountNumber}
                  onChange={e => setAchForm(f => ({ ...f, accountNumber: e.target.value.replace(/\D/g, '') }))}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 font-mono"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Account Type</label>
                <select
                  value={achForm.accountType}
                  onChange={e => setAchForm(f => ({ ...f, accountType: e.target.value as 'checking' | 'savings' }))}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
                >
                  <option value="checking">Checking</option>
                  <option value="savings">Savings</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Email</label>
                <input
                  type="email"
                  value={achForm.email}
                  onChange={e => setAchForm(f => ({ ...f, email: e.target.value }))}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
                  placeholder="Optional"
                />
              </div>

              <button
                type="submit"
                disabled={processing}
                className="w-full bg-primary-600 hover:bg-primary-700 disabled:bg-gray-400 text-white font-semibold py-3 rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
              >
                {processing ? (
                  <span className="flex items-center justify-center gap-2">
                    <span className="animate-spin rounded-full h-4 w-4 border-b-2 border-white" />
                    Processing...
                  </span>
                ) : (
                  `Pay ${formatAmount(session.amountCents)} via ACH`
                )}
              </button>

              <p className="text-xs text-gray-500 text-center">
                ACH payments typically take 2-5 business days to settle.
              </p>
            </motion.form>
          )}
        </div>

        {/* Footer */}
        <div className="px-6 py-3 bg-gray-50 border-t border-gray-100 text-center">
          <p className="text-xs text-gray-400">
            Secured by GoHighPayment
          </p>
        </div>
      </motion.div>
    </div>
  );
}
