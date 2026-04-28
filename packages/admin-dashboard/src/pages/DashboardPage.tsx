import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';

interface SummaryData {
  totalMerchants: number;
  activeMerchants: number;
  todayTransactions: number;
  todayVolumeCents: number;
  openChargebacks: number;
}

function formatCents(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100);
}

export function DashboardPage() {
  const { data, isLoading } = useQuery({
    queryKey: ['admin-summary'],
    queryFn: () => api.get<SummaryData>('/analytics/summary'),
  });

  const cards = [
    { label: 'Total Merchants', value: data?.totalMerchants ?? '-', color: 'bg-primary-50 text-primary-700' },
    { label: 'Active Merchants', value: data?.activeMerchants ?? '-', color: 'bg-green-50 text-green-700' },
    { label: 'Today\'s Transactions', value: data?.todayTransactions ?? '-', color: 'bg-blue-50 text-blue-700' },
    { label: 'Today\'s Volume', value: data ? formatCents(data.todayVolumeCents) : '-', color: 'bg-purple-50 text-purple-700' },
    { label: 'Open Chargebacks', value: data?.openChargebacks ?? '-', color: 'bg-red-50 text-red-700' },
  ];

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Dashboard</h1>

      {isLoading ? (
        <div className="flex items-center justify-center h-40">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4 mb-8">
          {cards.map((card) => (
            <div key={card.label} className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
              <div className="text-sm text-gray-500 mb-1">{card.label}</div>
              <div className={`text-2xl font-bold ${card.color.split(' ')[1]}`}>{card.value}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
