import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';

interface Chargeback {
  id: string;
  amountCents: number;
  reason: string | null;
  reasonCode: string | null;
  status: string;
  dueDate: string | null;
  createdAt: string;
  merchant: { businessName: string };
  transaction: { amountCents: number; paymentMethod: string };
}

function formatCents(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100);
}

export function ChargebacksPage() {
  const { data, isLoading } = useQuery({
    queryKey: ['admin-chargebacks'],
    queryFn: () => api.get<{ chargebacks: Chargeback[]; total: number }>('/chargebacks?limit=50'),
  });

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Chargebacks</h1>

      <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="bg-gray-50 border-b border-gray-200">
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Date</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Merchant</th>
              <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Amount</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Reason</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Status</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Due</th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <tr><td colSpan={6} className="text-center py-8 text-gray-500">Loading...</td></tr>
            ) : data?.chargebacks.length === 0 ? (
              <tr><td colSpan={6} className="text-center py-8 text-gray-500">No chargebacks</td></tr>
            ) : (
              data?.chargebacks.map((cb) => (
                <tr key={cb.id} className="border-b border-gray-100 hover:bg-gray-50">
                  <td className="px-4 py-3 text-sm text-gray-500">{new Date(cb.createdAt).toLocaleDateString()}</td>
                  <td className="px-4 py-3 text-sm font-medium">{cb.merchant.businessName}</td>
                  <td className="px-4 py-3 text-sm text-right font-medium text-red-600">{formatCents(cb.amountCents)}</td>
                  <td className="px-4 py-3 text-sm text-gray-600">{cb.reason || cb.reasonCode || '-'}</td>
                  <td className="px-4 py-3">
                    <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium ${
                      cb.status === 'WON' ? 'bg-green-100 text-green-700' :
                      cb.status === 'LOST' ? 'bg-red-100 text-red-700' :
                      cb.status === 'OPENED' ? 'bg-yellow-100 text-yellow-700' :
                      'bg-blue-100 text-blue-700'
                    }`}>
                      {cb.status}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-500">{cb.dueDate ? new Date(cb.dueDate).toLocaleDateString() : '-'}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
