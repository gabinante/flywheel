/**
 * Checkout Page - Subscription Flow
 *
 * Extends the checkout page to support subscription enrollment.
 * Uses Collect.js for secure card tokenization and calls the
 * /api/v1/checkout/subscribe endpoint.
 */

import React, { useState, useEffect, useCallback } from 'react';

// ---- Types ----

interface SubscriptionPlan {
  id: string;
  name: string;
  amount: number;
  currency: string;
  interval: string;
  trialDays: number;
}

interface SubscriptionResult {
  subscription: {
    id: string;
    status: string;
    planName: string;
    amount: number;
    currency: string;
    interval: string;
    currentPeriodStart: string;
    currentPeriodEnd: string;
    trialEnd: string | null;
  };
  transaction: {
    id: string;
    amount: number;
    status: string;
  };
}

// ---- Helpers ----

function formatCurrency(cents: number, currency: string = 'USD'): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency,
  }).format(cents / 100);
}

function intervalLabel(interval: string): string {
  switch (interval) {
    case 'WEEKLY': return '/week';
    case 'MONTHLY': return '/month';
    case 'QUARTERLY': return '/quarter';
    case 'ANNUAL': return '/year';
    default: return '';
  }
}

// ---- Component ----

interface CheckoutSubscriptionProps {
  sessionId: string;
  plan: SubscriptionPlan;
  apiBase?: string;
}

export function CheckoutSubscription({
  sessionId,
  plan,
  apiBase = '/api/v1',
}: CheckoutSubscriptionProps): React.ReactElement {
  const [step, setStep] = useState<'form' | 'processing' | 'success' | 'error'>('form');
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<SubscriptionResult | null>(null);
  const [paymentToken, setPaymentToken] = useState<string | null>(null);

  // Initialize Collect.js when component mounts
  useEffect(() => {
    // Collect.js is loaded via script tag in the HTML
    const collectJs = (window as any).CollectJS;
    if (collectJs) {
      collectJs.configure({
        variant: 'inline',
        callback: (response: { token: string }) => {
          setPaymentToken(response.token);
        },
        fields: {
          ccnumber: { selector: '#cc-number', placeholder: 'Card Number' },
          ccexp: { selector: '#cc-exp', placeholder: 'MM/YY' },
          cvv: { selector: '#cc-cvv', placeholder: 'CVV' },
        },
      });
    }
  }, []);

  const handleSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();

    if (!paymentToken) {
      // Trigger Collect.js tokenization
      const collectJs = (window as any).CollectJS;
      if (collectJs) {
        collectJs.startPaymentRequest();
      }
      return;
    }

    setStep('processing');
    setError(null);

    try {
      const response = await fetch(`${apiBase}/checkout/subscribe`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          planId: plan.id,
          sessionId,
          paymentToken,
        }),
      });

      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.error ?? data.detail ?? 'Subscription failed');
      }

      setResult(data);
      setStep('success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'An error occurred');
      setStep('error');
    }
  }, [paymentToken, plan.id, sessionId, apiBase]);

  // When paymentToken is set (Collect.js callback), auto-submit
  useEffect(() => {
    if (paymentToken && step === 'form') {
      handleSubmit(new Event('submit') as any);
    }
  }, [paymentToken, step, handleSubmit]);

  // ---- Render States ----

  if (step === 'success' && result) {
    return (
      <div style={{ maxWidth: '480px', margin: '0 auto', padding: '2rem' }}>
        <div style={{
          background: '#052e16',
          border: '1px solid #166534',
          borderRadius: '0.75rem',
          padding: '2rem',
          textAlign: 'center',
        }}>
          <div style={{ fontSize: '2rem', marginBottom: '0.5rem' }}>&#10003;</div>
          <h2 style={{ color: '#22c55e', marginBottom: '0.5rem' }}>Subscription Active!</h2>
          <p style={{ color: '#86efac', fontSize: '0.875rem' }}>
            {result.subscription.trialEnd
              ? `Your trial starts now. You'll be charged ${formatCurrency(result.subscription.amount, result.subscription.currency)} after the trial ends.`
              : `You've been charged ${formatCurrency(result.transaction.amount, result.subscription.currency)}.`
            }
          </p>
          <div style={{ marginTop: '1rem', color: '#94a3b8', fontSize: '0.75rem' }}>
            Plan: {result.subscription.planName} ({formatCurrency(result.subscription.amount, result.subscription.currency)}{intervalLabel(result.subscription.interval)})
          </div>
        </div>
      </div>
    );
  }

  if (step === 'processing') {
    return (
      <div style={{ maxWidth: '480px', margin: '0 auto', padding: '2rem', textAlign: 'center' }}>
        <div style={{ color: '#94a3b8', fontSize: '1rem' }}>
          Processing your subscription...
        </div>
      </div>
    );
  }

  return (
    <div style={{ maxWidth: '480px', margin: '0 auto', padding: '2rem' }}>
      {/* Plan Summary */}
      <div style={{
        background: '#1e293b',
        borderRadius: '0.75rem',
        padding: '1.5rem',
        marginBottom: '1.5rem',
      }}>
        <h2 style={{ color: '#f1f5f9', fontSize: '1.25rem', marginBottom: '0.5rem' }}>
          {plan.name}
        </h2>
        <div style={{ color: '#3b82f6', fontSize: '2rem', fontWeight: 'bold' }}>
          {formatCurrency(plan.amount, plan.currency)}
          <span style={{ fontSize: '0.875rem', color: '#94a3b8', fontWeight: 'normal' }}>
            {intervalLabel(plan.interval)}
          </span>
        </div>
        {plan.trialDays > 0 && (
          <div style={{
            marginTop: '0.5rem',
            padding: '0.375rem 0.75rem',
            background: '#172554',
            color: '#60a5fa',
            borderRadius: '0.25rem',
            fontSize: '0.75rem',
            display: 'inline-block',
          }}>
            {plan.trialDays}-day free trial
          </div>
        )}
      </div>

      {/* Error */}
      {(step === 'error' || error) && (
        <div style={{
          background: '#7f1d1d',
          color: '#fca5a5',
          padding: '0.75rem',
          borderRadius: '0.5rem',
          marginBottom: '1rem',
          fontSize: '0.875rem',
        }}>
          {error}
        </div>
      )}

      {/* Payment Form */}
      <form onSubmit={handleSubmit}>
        <div style={{ marginBottom: '1rem' }}>
          <label style={{ display: 'block', color: '#94a3b8', marginBottom: '0.25rem', fontSize: '0.75rem' }}>
            Card Number
          </label>
          <div
            id="cc-number"
            style={{
              background: '#0f172a',
              border: '1px solid #334155',
              borderRadius: '0.25rem',
              padding: '0.75rem',
              minHeight: '40px',
            }}
          />
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem', marginBottom: '1.5rem' }}>
          <div>
            <label style={{ display: 'block', color: '#94a3b8', marginBottom: '0.25rem', fontSize: '0.75rem' }}>
              Expiration
            </label>
            <div
              id="cc-exp"
              style={{
                background: '#0f172a',
                border: '1px solid #334155',
                borderRadius: '0.25rem',
                padding: '0.75rem',
                minHeight: '40px',
              }}
            />
          </div>
          <div>
            <label style={{ display: 'block', color: '#94a3b8', marginBottom: '0.25rem', fontSize: '0.75rem' }}>
              CVV
            </label>
            <div
              id="cc-cvv"
              style={{
                background: '#0f172a',
                border: '1px solid #334155',
                borderRadius: '0.25rem',
                padding: '0.75rem',
                minHeight: '40px',
              }}
            />
          </div>
        </div>

        <button
          type="submit"
          style={{
            width: '100%',
            padding: '0.875rem',
            background: '#3b82f6',
            color: 'white',
            border: 'none',
            borderRadius: '0.5rem',
            cursor: 'pointer',
            fontSize: '1rem',
            fontWeight: 600,
          }}
        >
          {plan.trialDays > 0
            ? `Start ${plan.trialDays}-Day Free Trial`
            : `Subscribe - ${formatCurrency(plan.amount, plan.currency)}${intervalLabel(plan.interval)}`
          }
        </button>

        <p style={{ color: '#64748b', fontSize: '0.7rem', textAlign: 'center', marginTop: '0.75rem' }}>
          {plan.trialDays > 0
            ? `After your trial, you'll be charged ${formatCurrency(plan.amount, plan.currency)}${intervalLabel(plan.interval)}. Cancel anytime.`
            : 'You can cancel your subscription at any time.'
          }
        </p>
      </form>
    </div>
  );
}

export default CheckoutSubscription;
