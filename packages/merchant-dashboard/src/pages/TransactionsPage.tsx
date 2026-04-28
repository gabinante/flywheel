import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../lib/api';

interface Transaction {
  id: string;
  amountCents: number;
  status: string;
  paymentMethod: string;
  cardBrand: string | null;
  cardLast4: string | null;
  platformFeeCents: number;
  merchantNetCents: number;
  refundedAmountCents: number;
  createdAt: string;
  customer: { email: string; firstName: string | null; lastName: string | null } | null;
}

function formatCents(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100);
}

export function TransactionsPage() {
  const [page, setPage] = useState(1);
  const [refundingId, setRefundingId] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ['merchant-transactions', page],
    queryFn: () => api.get<{ transactions: Transaction[]; total: number }>(`/transactions?page=${page}&limit=20`),
  });

  const refundMutation = useMutation({
    mutationFn: (txId: string) => api.post(`/transactions/${txId}/refund`, {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['merchant-transactions'] });
      setRefundingId(null);
    },
  });

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Transactions</h1>

      <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="bg-gray-50 border-b border-gray-200">
                <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Date</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Customer</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Method</th>
                <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Amount</th>
                <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Net</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Status</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Actions</th>
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <tr><td colSpan={7} className="text-center py-8 text-gray-500">Loading...</td></tr>
              ) : (
                data?.transactions.map((tx) => (
                  <tr key={tx.id} className="border-b border-gray-100 hover:bg-gray-50">
                    <td className="px-4 py-3 text-sm text-gray-500">{new Date(tx.createdAt).toLocaleString()}</td>
                    <td className="px-4 py-3 text-sm">{tx.customer?.email || '-'}</td>
                    <td className="px-4 py-3 text-sm">
                      {tx.paymentMethod === 'CARD' ? `${tx.cardBrand || ''} ...${tx.cardLast4 || ''}` : 'ACH'}
                    </td>
                    <td className="px-4 py-3 text-sm text-right font-medium">{formatCents(tx.amountCents)}</td>
                    <td className="px-4 py-3 text-sm text-right text-green-600">{formatCents(tx.merchantNetCents)}</td>
                    <td className="px-4 py-3">
                      <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium ${
                        ['CAPTURED', 'SETTLED'].includes(tx.status) ? 'bg-green-100 text-green-700' :
                        tx.status === 'PENDING' ? 'bg-yellow-100 text-yellow-700' :
                        tx.status === 'REFUNDED' ? 'bg-purple-100 text-purple-700' :
                        'bg-red-100 text-red-700'
                      }`}>
                        {tx.status}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      {['CAPTURED', 'SETTLED'].includes(tx.status) && tx.refundedAmountCents < tx.amountCents && (
                        <button
                          onClick={() => {
                            if (refundingId === tx.id) {
                              refundMutation.mutate(tx.id);
                            } else {
                              setRefundingId(tx.id);
                            }
                          }}
                          className={`text-xs font-medium ${refundingId === tx.id ? 'text-red-600' : 'text-primary-600 hover:text-primary-700'}`}
                        >
                          {refundingId === tx.id ? 'Confirm Refund?' : 'Refund'}
                        </button>
                      )}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {data && data.total > 20 && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-gray-200">
            <span className="text-sm text-gray-500">{data.total} total</span>
            <div className="flex gap-2">
              <button disabled={page <= 1} onClick={() => setPage(p => p - 1)} className="px-3 py-1 text-sm border rounded disabled:opacity-50">Previous</button>
              <button disabled={page * 20 >= data.total} onClick={() => setPage(p => p + 1)} className="px-3 py-1 text-sm border rounded disabled:opacity-50">Next</button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
