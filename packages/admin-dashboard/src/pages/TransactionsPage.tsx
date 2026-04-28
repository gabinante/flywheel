import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
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
  createdAt: string;
  merchant: { businessName: string };
  customer: { email: string; firstName: string | null; lastName: string | null } | null;
}

const statusColors: Record<string, string> = {
  CAPTURED: 'bg-green-100 text-green-700',
  SETTLED: 'bg-green-100 text-green-700',
  PENDING: 'bg-yellow-100 text-yellow-700',
  DECLINED: 'bg-red-100 text-red-700',
  REFUNDED: 'bg-purple-100 text-purple-700',
  VOIDED: 'bg-gray-100 text-gray-700',
  ERROR: 'bg-red-100 text-red-700',
  RETURNED: 'bg-orange-100 text-orange-700',
};

function formatCents(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100);
}

export function TransactionsPage() {
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState('');
  const [paymentMethod, setPaymentMethod] = useState('');

  const { data, isLoading } = useQuery({
    queryKey: ['admin-transactions', page, status, paymentMethod],
    queryFn: () =>
      api.get<{ transactions: Transaction[]; total: number }>(
        `/transactions?page=${page}&limit=20${status ? `&status=${status}` : ''}${paymentMethod ? `&paymentMethod=${paymentMethod}` : ''}`
      ),
  });

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Transactions</h1>
        <div className="flex gap-2">
          <select value={status} onChange={e => { setStatus(e.target.value); setPage(1); }} className="px-3 py-2 border border-gray-300 rounded-lg text-sm">
            <option value="">All statuses</option>
            {['PENDING', 'CAPTURED', 'SETTLED', 'DECLINED', 'REFUNDED', 'VOIDED', 'ERROR', 'RETURNED'].map(s => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
          <select value={paymentMethod} onChange={e => { setPaymentMethod(e.target.value); setPage(1); }} className="px-3 py-2 border border-gray-300 rounded-lg text-sm">
            <option value="">All methods</option>
            <option value="CARD">Card</option>
            <option value="ACH">ACH</option>
          </select>
        </div>
      </div>

      <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="bg-gray-50 border-b border-gray-200">
                <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Date</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Merchant</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Customer</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Method</th>
                <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Amount</th>
                <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Fee</th>
                <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">Net</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Status</th>
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <tr><td colSpan={8} className="text-center py-8 text-gray-500">Loading...</td></tr>
              ) : data?.transactions.length === 0 ? (
                <tr><td colSpan={8} className="text-center py-8 text-gray-500">No transactions found</td></tr>
              ) : (
                data?.transactions.map((tx) => (
                  <tr key={tx.id} className="border-b border-gray-100 hover:bg-gray-50">
                    <td className="px-4 py-3 text-sm text-gray-500">{new Date(tx.createdAt).toLocaleString()}</td>
                    <td className="px-4 py-3 text-sm font-medium">{tx.merchant.businessName}</td>
                    <td className="px-4 py-3 text-sm text-gray-600">{tx.customer?.email || '-'}</td>
                    <td className="px-4 py-3 text-sm">
                      {tx.paymentMethod === 'CARD' ? (
                        <span>{tx.cardBrand} ...{tx.cardLast4}</span>
                      ) : (
                        <span className="text-blue-600">ACH</span>
                      )}
                    </td>
                    <td className="px-4 py-3 text-sm text-right font-medium">{formatCents(tx.amountCents)}</td>
                    <td className="px-4 py-3 text-sm text-right text-gray-500">{formatCents(tx.platformFeeCents)}</td>
                    <td className="px-4 py-3 text-sm text-right">{formatCents(tx.merchantNetCents)}</td>
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
