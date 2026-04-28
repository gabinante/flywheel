import { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  listAgencies,
  type AgencySummary,
  type AgencyTier,
  type AgencyStatus,
  type PaginationInfo,
  formatCents,
  formatDate,
  TIER_LABELS,
  STATUS_COLORS,
  TIER_COLORS,
} from '../lib/api';

export default function AgenciesPage() {
  const navigate = useNavigate();
  const [agencies, setAgencies] = useState<AgencySummary[]>([]);
  const [pagination, setPagination] = useState<PaginationInfo>({
    page: 1,
    limit: 20,
    total: 0,
    totalPages: 0,
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filters
  const [tierFilter, setTierFilter] = useState<AgencyTier | ''>('');
  const [statusFilter, setStatusFilter] = useState<AgencyStatus | ''>('');
  const [searchQuery, setSearchQuery] = useState('');
  const [sortField, setSortField] = useState('createdAt');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');

  const fetchAgencies = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await listAgencies({
        page: pagination.page,
        limit: pagination.limit,
        tier: tierFilter || undefined,
        status: statusFilter || undefined,
        search: searchQuery || undefined,
        sort: sortField,
        order: sortOrder,
      });
      setAgencies(result.agencies);
      setPagination(result.pagination);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load agencies');
    } finally {
      setLoading(false);
    }
  }, [pagination.page, pagination.limit, tierFilter, statusFilter, searchQuery, sortField, sortOrder]);

  useEffect(() => {
    fetchAgencies();
  }, [fetchAgencies]);

  const handleSort = (field: string) => {
    if (sortField === field) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      setSortField(field);
      setSortOrder('desc');
    }
  };

  const SortIndicator = ({ field }: { field: string }) => {
    if (sortField !== field) return null;
    return <span className="ml-1">{sortOrder === 'asc' ? '\u25B2' : '\u25BC'}</span>;
  };

  return (
    <div className="p-6 max-w-7xl mx-auto">
      {/* Header */}
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-white">Agencies</h1>
        <p className="text-sm text-gray-400 mt-1">
          Manage affiliate agencies and their residual earnings
        </p>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3 mb-6">
        <input
          type="text"
          placeholder="Search by name or email..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="px-3 py-2 bg-gray-800 border border-gray-700 rounded-lg text-sm text-gray-200 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent w-64"
        />
        <select
          value={tierFilter}
          onChange={(e) => setTierFilter(e.target.value as AgencyTier | '')}
          className="px-3 py-2 bg-gray-800 border border-gray-700 rounded-lg text-sm text-gray-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          <option value="">All Tiers</option>
          <option value="TIER_1">Tier 1</option>
          <option value="TIER_2">Tier 2</option>
          <option value="TIER_3">Tier 3</option>
        </select>
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value as AgencyStatus | '')}
          className="px-3 py-2 bg-gray-800 border border-gray-700 rounded-lg text-sm text-gray-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          <option value="">All Statuses</option>
          <option value="ACTIVE">Active</option>
          <option value="SUSPENDED">Suspended</option>
          <option value="CHURNED">Churned</option>
        </select>
      </div>

      {/* Error */}
      {error && (
        <div className="mb-4 p-3 bg-red-900/30 border border-red-800 rounded-lg text-red-400 text-sm">
          {error}
        </div>
      )}

      {/* Table */}
      <div className="bg-gray-900/50 border border-gray-800 rounded-xl overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-800 text-gray-400">
                <th
                  className="text-left px-4 py-3 font-medium cursor-pointer hover:text-gray-200"
                  onClick={() => handleSort('name')}
                >
                  Name <SortIndicator field="name" />
                </th>
                <th className="text-left px-4 py-3 font-medium">Email</th>
                <th className="text-left px-4 py-3 font-medium">Tier</th>
                <th className="text-left px-4 py-3 font-medium">Status</th>
                <th
                  className="text-right px-4 py-3 font-medium cursor-pointer hover:text-gray-200"
                  onClick={() => handleSort('merchantCount')}
                >
                  Merchants <SortIndicator field="merchantCount" />
                </th>
                <th
                  className="text-right px-4 py-3 font-medium cursor-pointer hover:text-gray-200"
                  onClick={() => handleSort('portfolioVolume')}
                >
                  Portfolio Volume <SortIndicator field="portfolioVolume" />
                </th>
                <th className="text-right px-4 py-3 font-medium">Monthly Residual</th>
                <th
                  className="text-left px-4 py-3 font-medium cursor-pointer hover:text-gray-200"
                  onClick={() => handleSort('createdAt')}
                >
                  Created <SortIndicator field="createdAt" />
                </th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr>
                  <td colSpan={8} className="px-4 py-8 text-center text-gray-500">
                    Loading agencies...
                  </td>
                </tr>
              ) : agencies.length === 0 ? (
                <tr>
                  <td colSpan={8} className="px-4 py-8 text-center text-gray-500">
                    No agencies found
                  </td>
                </tr>
              ) : (
                agencies.map((agency) => (
                  <tr
                    key={agency.id}
                    onClick={() => navigate(`/agencies/${agency.id}`)}
                    className="border-b border-gray-800/50 hover:bg-gray-800/30 cursor-pointer transition-colors"
                  >
                    <td className="px-4 py-3 font-medium text-white">
                      {agency.name}
                    </td>
                    <td className="px-4 py-3 text-gray-400">{agency.contactEmail}</td>
                    <td className="px-4 py-3">
                      <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${TIER_COLORS[agency.tier]}`}>
                        {TIER_LABELS[agency.tier]}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${STATUS_COLORS[agency.status]}`}>
                        {agency.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right text-gray-300">
                      {agency.merchantCount}
                    </td>
                    <td className="px-4 py-3 text-right text-gray-300">
                      {formatCents(agency.portfolioVolume)}
                    </td>
                    <td className="px-4 py-3 text-right text-gray-300">
                      {formatCents(agency.monthlyResidual)}
                    </td>
                    <td className="px-4 py-3 text-gray-400">
                      {formatDate(agency.createdAt)}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {pagination.totalPages > 1 && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-gray-800">
            <span className="text-sm text-gray-400">
              Showing {(pagination.page - 1) * pagination.limit + 1} to{' '}
              {Math.min(pagination.page * pagination.limit, pagination.total)} of{' '}
              {pagination.total} agencies
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setPagination((p) => ({ ...p, page: p.page - 1 }))}
                disabled={pagination.page <= 1}
                className="px-3 py-1 text-sm bg-gray-800 border border-gray-700 rounded-lg text-gray-300 hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Previous
              </button>
              <button
                onClick={() => setPagination((p) => ({ ...p, page: p.page + 1 }))}
                disabled={pagination.page >= pagination.totalPages}
                className="px-3 py-1 text-sm bg-gray-800 border border-gray-700 rounded-lg text-gray-300 hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed"
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
