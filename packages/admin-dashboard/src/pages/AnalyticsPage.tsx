import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';

interface DailyMetric {
  date: string;
  totalTransactions: number;
  totalVolumeCents: number;
  totalFeeCents: number;
  cardTransactions: number;
  achTransactions: number;
  declinedCount: number;
  chargebackCount: number;
  refundCount: number;
}

function formatCents(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100);
}

export function AnalyticsPage() {
  const { data, isLoading } = useQuery({
    queryKey: ['admin-analytics'],
    queryFn: () => api.get<{ metrics: DailyMetric[] }>('/analytics/daily'),
  });

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Analytics</h1>

      <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="bg-gray-50 border-b border-gray-200">
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Date</th>
              <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Transactions</th>
              <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Volume</th>
              <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Fees</th>
              <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Card</th>
              <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">ACH</th>
              <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Declined</th>
              <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Chargebacks</th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <tr><td colSpan={8} className="text-center py-8 text-gray-500">Loading...</td></tr>
            ) : !data?.metrics.length ? (
              <tr><td colSpan={8} className="text-center py-8 text-gray-500">No metrics available yet</td></tr>
            ) : (
              data.metrics.map((m) => (
                <tr key={m.date} className="border-b border-gray-100 hover:bg-gray-50">
                  <td className="px-4 py-3 text-sm font-medium">{new Date(m.date).toLocaleDateString()}</td>
                  <td className="px-4 py-3 text-sm text-right">{m.totalTransactions}</td>
                  <td className="px-4 py-3 text-sm text-right font-medium">{formatCents(m.totalVolumeCents)}</td>
                  <td className="px-4 py-3 text-sm text-right text-green-600">{formatCents(m.totalFeeCents)}</td>
                  <td className="px-4 py-3 text-sm text-right">{m.cardTransactions}</td>
                  <td className="px-4 py-3 text-sm text-right">{m.achTransactions}</td>
                  <td className="px-4 py-3 text-sm text-right text-red-600">{m.declinedCount}</td>
                  <td className="px-4 py-3 text-sm text-right text-red-600">{m.chargebackCount}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
