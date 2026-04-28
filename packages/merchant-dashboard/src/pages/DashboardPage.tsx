import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { useEffect } from 'react';
import { api } from '../lib/api';

interface DashboardData {
  today: { transactions: number; volumeCents: number };
  month: { transactions: number; volumeCents: number };
  recentTransactions: Array<{
    id: string;
    amountCents: number;
    status: string;
    paymentMethod: string;
    cardBrand: string | null;
    cardLast4: string | null;
    createdAt: string;
    customer: { email: string } | null;
  }>;
}

function formatCents(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100);
}

const statusColors: Record<string, string> = {
  CAPTURED: 'bg-green-100 text-green-700',
  SETTLED: 'bg-green-100 text-green-700',
  PENDING: 'bg-yellow-100 text-yellow-700',
  DECLINED: 'bg-red-100 text-red-700',
  REFUNDED: 'bg-purple-100 text-purple-700',
};

export function DashboardPage() {
  const navigate = useNavigate();

  const { data: settings } = useQuery({
    queryKey: ['merchant-settings'],
    queryFn: () => api.get<{ status: string; processingConfigured: boolean }>('/settings'),
  });

  // Redirect PENDING merchants to onboarding
  useEffect(() => {
    if (settings && settings.status === 'PENDING') {
      navigate('/onboarding', { replace: true });
    }
  }, [settings, navigate]);

  const { data, isLoading } = useQuery({
    queryKey: ['merchant-dashboard'],
    queryFn: () => api.get<DashboardData>('/dashboard'),
    enabled: settings?.status === 'ACTIVE',
  });

  if (isLoading || !settings || settings.status !== 'ACTIVE') {
    return <div className="flex items-center justify-center h-40"><div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" /></div>;
  }

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Dashboard</h1>

      {/* Stats */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8">
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <div className="text-sm text-gray-500">Today's Volume</div>
          <div className="text-2xl font-bold text-primary-600">{formatCents(data?.today.volumeCents ?? 0)}</div>
          <div className="text-xs text-gray-400">{data?.today.transactions ?? 0} transactions</div>
        </div>
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <div className="text-sm text-gray-500">Today's Transactions</div>
          <div className="text-2xl font-bold text-gray-900">{data?.today.transactions ?? 0}</div>
        </div>
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <div className="text-sm text-gray-500">30-Day Volume</div>
          <div className="text-2xl font-bold">{formatCents(data?.month.volumeCents ?? 0)}</div>
          <div className="text-xs text-gray-400">{data?.month.transactions ?? 0} transactions</div>
        </div>
      </div>

      {/* Recent transactions */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-200">
          <h2 className="font-semibold text-gray-900">Recent Transactions</h2>
        </div>
        <table className="w-full">
          <thead>
            <tr className="bg-gray-50 border-b border-gray-200">
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Date</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Customer</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Method</th>
              <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Amount</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Status</th>
            </tr>
          </thead>
          <tbody>
            {!data?.recentTransactions.length ? (
              <tr><td colSpan={5} className="text-center py-8 text-gray-500">No transactions yet</td></tr>
            ) : (
              data.recentTransactions.map((tx) => (
                <tr key={tx.id} className="border-b border-gray-100 hover:bg-gray-50">
                  <td className="px-4 py-3 text-sm text-gray-500">{new Date(tx.createdAt).toLocaleString()}</td>
                  <td className="px-4 py-3 text-sm">{tx.customer?.email || '-'}</td>
                  <td className="px-4 py-3 text-sm">
                    {tx.paymentMethod === 'CARD' ? `${tx.cardBrand || ''} ...${tx.cardLast4 || ''}` :
                     tx.paymentMethod === 'GOOGLE_PAY' ? 'Google Pay' :
                     tx.paymentMethod === 'APPLE_PAY' ? 'Apple Pay' : 'ACH'}
                  </td>
                  <td className="px-4 py-3 text-sm text-right font-medium">{formatCents(tx.amountCents)}</td>
                  <td className="px-4 py-3">
                    <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium ${statusColors[tx.status] || ''}`}>
                      {tx.status}
                    </span>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
