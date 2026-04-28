/**
 * Subscriptions Page
 *
 * Merchant dashboard page for managing subscription plans and viewing subscriptions.
 * Provides CRUD for plans and a filterable list of subscriptions.
 */

import React, { useEffect, useState, useCallback } from 'react';

// ---- Types ----

interface SubscriptionPlan {
  id: string;
  name: string;
  amount: number;
  currency: string;
  interval: string;
  trialDays: number;
  active: boolean;
  createdAt: string;
}

interface Subscription {
  id: string;
  customerId: string;
  status: string;
  currentPeriodStart: string | null;
  currentPeriodEnd: string | null;
  trialEnd: string | null;
  failedAttempts: number;
  canceledAt: string | null;
  cancelReason: string | null;
  createdAt: string;
  plan: SubscriptionPlan;
}

interface PaginatedResponse<T> {
  data: T[];
  pagination: {
    page: number;
    limit: number;
    total: number;
    totalPages: number;
  };
}

type StatusFilter = '' | 'TRIALING' | 'ACTIVE' | 'PAST_DUE' | 'PAUSED' | 'CANCELED';

// ---- API Helpers ----

const API_BASE = '/api/v1/merchant';

async function apiRequest<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Request failed' }));
    throw new Error(error.error ?? `HTTP ${response.status}`);
  }

  return response.json();
}

// ---- Formatting Helpers ----

function formatCurrency(cents: number, currency: string = 'USD'): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency,
  }).format(cents / 100);
}

function formatDate(dateStr: string | null): string {
  if (!dateStr) return '-';
  return new Date(dateStr).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

function intervalLabel(interval: string): string {
  switch (interval) {
    case 'WEEKLY': return 'Weekly';
    case 'MONTHLY': return 'Monthly';
    case 'QUARTERLY': return 'Quarterly';
    case 'ANNUAL': return 'Annual';
    default: return interval;
  }
}

function statusBadge(status: string): { label: string; color: string } {
  switch (status) {
    case 'TRIALING': return { label: 'Trialing', color: '#3b82f6' };
    case 'ACTIVE': return { label: 'Active', color: '#22c55e' };
    case 'PAST_DUE': return { label: 'Past Due', color: '#f59e0b' };
    case 'PAUSED': return { label: 'Paused', color: '#6b7280' };
    case 'CANCELED': return { label: 'Canceled', color: '#ef4444' };
    default: return { label: status, color: '#6b7280' };
  }
}

// ---- Component ----

export function SubscriptionsPage(): React.ReactElement {
  // Plans state
  const [plans, setPlans] = useState<SubscriptionPlan[]>([]);
  const [showCreatePlan, setShowCreatePlan] = useState(false);
  const [newPlan, setNewPlan] = useState({
    name: '',
    amount: '',
    interval: 'MONTHLY',
    trialDays: '0',
  });

  // Subscriptions state
  const [subscriptions, setSubscriptions] = useState<Subscription[]>([]);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('');
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);

  // UI state
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'plans' | 'subscriptions'>('plans');

  // ---- Data Fetching ----

  const fetchPlans = useCallback(async () => {
    try {
      const data = await apiRequest<SubscriptionPlan[]>('/subscriptions/plans');
      setPlans(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load plans');
    }
  }, []);

  const fetchSubscriptions = useCallback(async () => {
    try {
      const params = new URLSearchParams({ page: String(page), limit: '20' });
      if (statusFilter) params.set('status', statusFilter);

      const data = await apiRequest<PaginatedResponse<Subscription>>(
        `/subscriptions?${params.toString()}`
      );
      setSubscriptions(data.data);
      setTotalPages(data.pagination.totalPages);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load subscriptions');
    }
  }, [page, statusFilter]);

  useEffect(() => {
    setLoading(true);
    Promise.all([fetchPlans(), fetchSubscriptions()])
      .finally(() => setLoading(false));
  }, [fetchPlans, fetchSubscriptions]);

  // ---- Actions ----

  const handleCreatePlan = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    try {
      await apiRequest('/subscriptions/plans', {
        method: 'POST',
        body: JSON.stringify({
          name: newPlan.name,
          amount: parseInt(newPlan.amount, 10),
          interval: newPlan.interval,
          trialDays: parseInt(newPlan.trialDays, 10),
        }),
      });
      setNewPlan({ name: '', amount: '', interval: 'MONTHLY', trialDays: '0' });
      setShowCreatePlan(false);
      await fetchPlans();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create plan');
    }
  };

  const handleTogglePlan = async (planId: string, currentActive: boolean) => {
    try {
      await apiRequest(`/subscriptions/plans/${planId}`, {
        method: 'PATCH',
        body: JSON.stringify({ active: !currentActive }),
      });
      await fetchPlans();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update plan');
    }
  };

  const handleCancelSubscription = async (subscriptionId: string) => {
    if (!window.confirm('Are you sure you want to cancel this subscription?')) {
      return;
    }

    try {
      await apiRequest(`/subscriptions/${subscriptionId}/cancel`, {
        method: 'POST',
        body: JSON.stringify({ reason: 'Canceled by merchant' }),
      });
      await fetchSubscriptions();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to cancel subscription');
    }
  };

  // ---- Render ----

  if (loading) {
    return (
      <div style={{ padding: '2rem', color: '#94a3b8' }}>
        Loading subscriptions...
      </div>
    );
  }

  return (
    <div style={{ padding: '2rem', maxWidth: '1200px', margin: '0 auto' }}>
      <h1 style={{ color: '#f1f5f9', marginBottom: '1.5rem', fontSize: '1.5rem' }}>
        Subscriptions & Recurring Billing
      </h1>

      {error && (
        <div style={{
          background: '#7f1d1d',
          color: '#fca5a5',
          padding: '0.75rem 1rem',
          borderRadius: '0.5rem',
          marginBottom: '1rem',
        }}>
          {error}
          <button
            onClick={() => setError(null)}
            style={{ float: 'right', background: 'none', border: 'none', color: '#fca5a5', cursor: 'pointer' }}
          >
            Dismiss
          </button>
        </div>
      )}

      {/* Tab Navigation */}
      <div style={{ display: 'flex', gap: '1rem', marginBottom: '1.5rem', borderBottom: '1px solid #334155' }}>
        {(['plans', 'subscriptions'] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            style={{
              padding: '0.75rem 1rem',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === tab ? '2px solid #3b82f6' : '2px solid transparent',
              color: activeTab === tab ? '#f1f5f9' : '#94a3b8',
              cursor: 'pointer',
              fontSize: '0.875rem',
              fontWeight: 500,
              textTransform: 'capitalize',
            }}
          >
            {tab}
          </button>
        ))}
      </div>

      {/* Plans Tab */}
      {activeTab === 'plans' && (
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
            <h2 style={{ color: '#e2e8f0', fontSize: '1.1rem' }}>Subscription Plans</h2>
            <button
              onClick={() => setShowCreatePlan(!showCreatePlan)}
              style={{
                padding: '0.5rem 1rem',
                background: '#3b82f6',
                color: 'white',
                border: 'none',
                borderRadius: '0.375rem',
                cursor: 'pointer',
                fontSize: '0.875rem',
              }}
            >
              {showCreatePlan ? 'Cancel' : '+ New Plan'}
            </button>
          </div>

          {showCreatePlan && (
            <form onSubmit={handleCreatePlan} style={{
              background: '#1e293b',
              padding: '1.5rem',
              borderRadius: '0.5rem',
              marginBottom: '1rem',
              display: 'grid',
              gridTemplateColumns: '1fr 1fr',
              gap: '1rem',
            }}>
              <div>
                <label style={{ display: 'block', color: '#94a3b8', marginBottom: '0.25rem', fontSize: '0.75rem' }}>
                  Plan Name
                </label>
                <input
                  type="text"
                  value={newPlan.name}
                  onChange={(e) => setNewPlan({ ...newPlan, name: e.target.value })}
                  required
                  style={{ width: '100%', padding: '0.5rem', background: '#0f172a', color: '#f1f5f9', border: '1px solid #334155', borderRadius: '0.25rem' }}
                />
              </div>
              <div>
                <label style={{ display: 'block', color: '#94a3b8', marginBottom: '0.25rem', fontSize: '0.75rem' }}>
                  Amount (cents)
                </label>
                <input
                  type="number"
                  min="1"
                  value={newPlan.amount}
                  onChange={(e) => setNewPlan({ ...newPlan, amount: e.target.value })}
                  required
                  style={{ width: '100%', padding: '0.5rem', background: '#0f172a', color: '#f1f5f9', border: '1px solid #334155', borderRadius: '0.25rem' }}
                />
              </div>
              <div>
                <label style={{ display: 'block', color: '#94a3b8', marginBottom: '0.25rem', fontSize: '0.75rem' }}>
                  Interval
                </label>
                <select
                  value={newPlan.interval}
                  onChange={(e) => setNewPlan({ ...newPlan, interval: e.target.value })}
                  style={{ width: '100%', padding: '0.5rem', background: '#0f172a', color: '#f1f5f9', border: '1px solid #334155', borderRadius: '0.25rem' }}
                >
                  <option value="WEEKLY">Weekly</option>
                  <option value="MONTHLY">Monthly</option>
                  <option value="QUARTERLY">Quarterly</option>
                  <option value="ANNUAL">Annual</option>
                </select>
              </div>
              <div>
                <label style={{ display: 'block', color: '#94a3b8', marginBottom: '0.25rem', fontSize: '0.75rem' }}>
                  Trial Days
                </label>
                <input
                  type="number"
                  min="0"
                  value={newPlan.trialDays}
                  onChange={(e) => setNewPlan({ ...newPlan, trialDays: e.target.value })}
                  style={{ width: '100%', padding: '0.5rem', background: '#0f172a', color: '#f1f5f9', border: '1px solid #334155', borderRadius: '0.25rem' }}
                />
              </div>
              <div style={{ gridColumn: '1 / -1' }}>
                <button
                  type="submit"
                  style={{
                    padding: '0.5rem 1.5rem',
                    background: '#22c55e',
                    color: 'white',
                    border: 'none',
                    borderRadius: '0.375rem',
                    cursor: 'pointer',
                    fontSize: '0.875rem',
                  }}
                >
                  Create Plan
                </button>
              </div>
            </form>
          )}

          {/* Plans Table */}
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.875rem' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #334155' }}>
                  {['Name', 'Amount', 'Interval', 'Trial', 'Status', 'Created', 'Actions'].map((h) => (
                    <th key={h} style={{ textAlign: 'left', padding: '0.75rem', color: '#94a3b8', fontWeight: 500 }}>
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {plans.map((plan) => (
                  <tr key={plan.id} style={{ borderBottom: '1px solid #1e293b' }}>
                    <td style={{ padding: '0.75rem', color: '#f1f5f9' }}>{plan.name}</td>
                    <td style={{ padding: '0.75rem', color: '#f1f5f9' }}>
                      {formatCurrency(plan.amount, plan.currency)}
                    </td>
                    <td style={{ padding: '0.75rem', color: '#94a3b8' }}>{intervalLabel(plan.interval)}</td>
                    <td style={{ padding: '0.75rem', color: '#94a3b8' }}>
                      {plan.trialDays > 0 ? `${plan.trialDays} days` : '-'}
                    </td>
                    <td style={{ padding: '0.75rem' }}>
                      <span style={{
                        padding: '0.125rem 0.5rem',
                        borderRadius: '9999px',
                        fontSize: '0.75rem',
                        background: plan.active ? '#052e16' : '#1c1917',
                        color: plan.active ? '#22c55e' : '#6b7280',
                      }}>
                        {plan.active ? 'Active' : 'Inactive'}
                      </span>
                    </td>
                    <td style={{ padding: '0.75rem', color: '#94a3b8' }}>{formatDate(plan.createdAt)}</td>
                    <td style={{ padding: '0.75rem' }}>
                      <button
                        onClick={() => handleTogglePlan(plan.id, plan.active)}
                        style={{
                          padding: '0.25rem 0.5rem',
                          background: 'none',
                          border: '1px solid #334155',
                          color: '#94a3b8',
                          borderRadius: '0.25rem',
                          cursor: 'pointer',
                          fontSize: '0.75rem',
                        }}
                      >
                        {plan.active ? 'Deactivate' : 'Activate'}
                      </button>
                    </td>
                  </tr>
                ))}
                {plans.length === 0 && (
                  <tr>
                    <td colSpan={7} style={{ padding: '2rem', textAlign: 'center', color: '#64748b' }}>
                      No plans created yet. Click "+ New Plan" to get started.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Subscriptions Tab */}
      {activeTab === 'subscriptions' && (
        <div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
            <h2 style={{ color: '#e2e8f0', fontSize: '1.1rem' }}>Subscriptions</h2>
            <select
              value={statusFilter}
              onChange={(e) => { setStatusFilter(e.target.value as StatusFilter); setPage(1); }}
              style={{ padding: '0.5rem', background: '#1e293b', color: '#f1f5f9', border: '1px solid #334155', borderRadius: '0.25rem', fontSize: '0.875rem' }}
            >
              <option value="">All Statuses</option>
              <option value="TRIALING">Trialing</option>
              <option value="ACTIVE">Active</option>
              <option value="PAST_DUE">Past Due</option>
              <option value="PAUSED">Paused</option>
              <option value="CANCELED">Canceled</option>
            </select>
          </div>

          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.875rem' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #334155' }}>
                  {['Customer', 'Plan', 'Status', 'Period', 'Failures', 'Created', 'Actions'].map((h) => (
                    <th key={h} style={{ textAlign: 'left', padding: '0.75rem', color: '#94a3b8', fontWeight: 500 }}>
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {subscriptions.map((sub) => {
                  const badge = statusBadge(sub.status);
                  return (
                    <tr key={sub.id} style={{ borderBottom: '1px solid #1e293b' }}>
                      <td style={{ padding: '0.75rem', color: '#f1f5f9', fontFamily: 'monospace', fontSize: '0.75rem' }}>
                        {sub.customerId.slice(0, 16)}...
                      </td>
                      <td style={{ padding: '0.75rem', color: '#f1f5f9' }}>{sub.plan.name}</td>
                      <td style={{ padding: '0.75rem' }}>
                        <span style={{
                          padding: '0.125rem 0.5rem',
                          borderRadius: '9999px',
                          fontSize: '0.75rem',
                          background: `${badge.color}20`,
                          color: badge.color,
                        }}>
                          {badge.label}
                        </span>
                      </td>
                      <td style={{ padding: '0.75rem', color: '#94a3b8' }}>
                        {formatDate(sub.currentPeriodStart)} - {formatDate(sub.currentPeriodEnd)}
                      </td>
                      <td style={{ padding: '0.75rem', color: sub.failedAttempts > 0 ? '#f59e0b' : '#94a3b8' }}>
                        {sub.failedAttempts}
                      </td>
                      <td style={{ padding: '0.75rem', color: '#94a3b8' }}>{formatDate(sub.createdAt)}</td>
                      <td style={{ padding: '0.75rem' }}>
                        {sub.status !== 'CANCELED' && (
                          <button
                            onClick={() => handleCancelSubscription(sub.id)}
                            style={{
                              padding: '0.25rem 0.5rem',
                              background: 'none',
                              border: '1px solid #7f1d1d',
                              color: '#ef4444',
                              borderRadius: '0.25rem',
                              cursor: 'pointer',
                              fontSize: '0.75rem',
                            }}
                          >
                            Cancel
                          </button>
                        )}
                      </td>
                    </tr>
                  );
                })}
                {subscriptions.length === 0 && (
                  <tr>
                    <td colSpan={7} style={{ padding: '2rem', textAlign: 'center', color: '#64748b' }}>
                      No subscriptions found.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div style={{ display: 'flex', justifyContent: 'center', gap: '0.5rem', marginTop: '1rem' }}>
              <button
                disabled={page <= 1}
                onClick={() => setPage(page - 1)}
                style={{
                  padding: '0.5rem 0.75rem',
                  background: '#1e293b',
                  color: page <= 1 ? '#475569' : '#f1f5f9',
                  border: '1px solid #334155',
                  borderRadius: '0.25rem',
                  cursor: page <= 1 ? 'not-allowed' : 'pointer',
                  fontSize: '0.875rem',
                }}
              >
                Previous
              </button>
              <span style={{ padding: '0.5rem', color: '#94a3b8', fontSize: '0.875rem' }}>
                Page {page} of {totalPages}
              </span>
              <button
                disabled={page >= totalPages}
                onClick={() => setPage(page + 1)}
                style={{
                  padding: '0.5rem 0.75rem',
                  background: '#1e293b',
                  color: page >= totalPages ? '#475569' : '#f1f5f9',
                  border: '1px solid #334155',
                  borderRadius: '0.25rem',
                  cursor: page >= totalPages ? 'not-allowed' : 'pointer',
                  fontSize: '0.875rem',
                }}
              >
                Next
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default SubscriptionsPage;
