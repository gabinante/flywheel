import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { api } from '../lib/api';

interface Merchant {
  id: string;
  businessName: string;
  contactEmail: string;
  status: string;
  createdAt: string;
  platformFeePercentage: string;
  platformFlatFeeCents: number;
  ghlLocationId: string | null;
}

const statusColors: Record<string, string> = {
  ACTIVE: 'bg-green-100 text-green-700',
  PENDING: 'bg-yellow-100 text-yellow-700',
  SUSPENDED: 'bg-red-100 text-red-700',
  DEACTIVATED: 'bg-gray-100 text-gray-700',
};

export function MerchantsPage() {
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');

  const { data, isLoading } = useQuery({
    queryKey: ['merchants', page, search],
    queryFn: () =>
      api.get<{ merchants: Merchant[]; total: number }>(`/merchants?page=${page}&limit=20&search=${search}`),
  });

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Merchants</h1>
        <input
          type="text"
          placeholder="Search merchants..."
          value={search}
          onChange={e => { setSearch(e.target.value); setPage(1); }}
          className="px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-primary-500 focus:border-primary-500 w-64"
        />
      </div>

      <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="bg-gray-50 border-b border-gray-200">
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Business</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Email</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Status</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Fee</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">GHL</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Created</th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <tr><td colSpan={6} className="text-center py-8 text-gray-500">Loading...</td></tr>
            ) : data?.merchants.length === 0 ? (
              <tr><td colSpan={6} className="text-center py-8 text-gray-500">No merchants found</td></tr>
            ) : (
              data?.merchants.map((m) => (
                <tr key={m.id} className="border-b border-gray-100 hover:bg-gray-50">
                  <td className="px-4 py-3">
                    <Link to={`/merchants/${m.id}`} className="text-primary-600 hover:text-primary-700 font-medium">
                      {m.businessName}
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-600">{m.contactEmail}</td>
                  <td className="px-4 py-3">
                    <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium ${statusColors[m.status] || ''}`}>
                      {m.status}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-600">{m.platformFeePercentage}% + ${(m.platformFlatFeeCents / 100).toFixed(2)}</td>
                  <td className="px-4 py-3 text-sm">{m.ghlLocationId ? '✓' : '-'}</td>
                  <td className="px-4 py-3 text-sm text-gray-500">{new Date(m.createdAt).toLocaleDateString()}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>

        {data && data.total > 20 && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-gray-200">
            <span className="text-sm text-gray-500">{data.total} total</span>
            <div className="flex gap-2">
              <button
                disabled={page <= 1}
                onClick={() => setPage(p => p - 1)}
                className="px-3 py-1 text-sm border rounded disabled:opacity-50"
              >
                Previous
              </button>
              <button
                disabled={page * 20 >= data.total}
                onClick={() => setPage(p => p + 1)}
                className="px-3 py-1 text-sm border rounded disabled:opacity-50"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
