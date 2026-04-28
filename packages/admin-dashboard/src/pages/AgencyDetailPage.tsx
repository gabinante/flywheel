import { useState, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts';
import {
  getAgency,
  updateAgency,
  type AgencyDetail,
  type AgencyTier,
  formatCents,
  formatDate,
  TIER_LABELS,
  STATUS_COLORS,
  TIER_COLORS,
} from '../lib/api';

type TabName = 'overview' | 'merchants' | 'network' | 'residuals' | 'payouts';

export default function AgencyDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [agency, setAgency] = useState<AgencyDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<TabName>('overview');
  const [actionLoading, setActionLoading] = useState(false);

  const fetchAgency = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    setError(null);
    try {
      const data = await getAgency(id);
      setAgency(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load agency');
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchAgency();
  }, [fetchAgency]);

  const handleStatusToggle = async () => {
    if (!agency || !id) return;
    setActionLoading(true);
    try {
      const newStatus = agency.status === 'ACTIVE' ? 'SUSPENDED' : 'ACTIVE';
      await updateAgency(id, { status: newStatus });
      await fetchAgency();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update status');
    } finally {
      setActionLoading(false);
    }
  };

  const handleTierOverride = async (tier: AgencyTier) => {
    if (!id) return;
    setActionLoading(true);
    try {
      await updateAgency(id, { tier, tierOverride: true });
      await fetchAgency();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update tier');
    } finally {
      setActionLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="p-6 flex items-center justify-center min-h-[400px]">
        <span className="text-gray-400">Loading agency details...</span>
      </div>
    );
  }

  if (error || !agency) {
    return (
      <div className="p-6">
        <div className="p-4 bg-red-900/30 border border-red-800 rounded-lg text-red-400">
          {error ?? 'Agency not found'}
        </div>
        <button
          onClick={() => navigate('/agencies')}
          className="mt-4 text-sm text-blue-400 hover:text-blue-300"
        >
          Back to Agencies
        </button>
      </div>
    );
  }

  const totalMerchants = agency.merchants.length;
  const activeMerchants = agency.merchants.filter((m) => m.status === 'ACTIVE').length;
  const totalVolume = agency.merchants.reduce((sum, m) => sum + m.volumeThisMonth, 0);
  const totalResidual = agency.residualHistory.reduce(
    (sum, m) => sum + m.agencyShare + m.twoTierShare,
    0
  );

  const tabs: { name: TabName; label: string }[] = [
    { name: 'overview', label: 'Overview' },
    { name: 'merchants', label: `Merchants (${totalMerchants})` },
    { name: 'network', label: `Network (${agency.referredAgencies.length})` },
    { name: 'residuals', label: 'Residuals' },
    { name: 'payouts', label: `Payouts (${agency.payouts.length})` },
  ];

  return (
    <div className="p-6 max-w-7xl mx-auto">
      {/* Back button */}
      <button
        onClick={() => navigate('/agencies')}
        className="mb-4 text-sm text-gray-400 hover:text-gray-200 flex items-center gap-1"
      >
        <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M15 19l-7-7 7-7" />
        </svg>
        Back to Agencies
      </button>

      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold text-white">{agency.name}</h1>
            <span className={`inline-flex items-center px-2.5 py-0.5 rounded text-xs font-medium ${STATUS_COLORS[agency.status]}`}>
              {agency.status}
            </span>
            <span className={`inline-flex items-center px-2.5 py-0.5 rounded text-xs font-medium ${TIER_COLORS[agency.tier]}`}>
              {TIER_LABELS[agency.tier]}
              {agency.tierOverride && ' (override)'}
            </span>
          </div>
          <p className="text-sm text-gray-400 mt-1">{agency.contactEmail}</p>
        </div>

        {/* Action buttons */}
        <div className="flex gap-2">
          <button
            onClick={handleStatusToggle}
            disabled={actionLoading}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition-colors disabled:opacity-50 ${
              agency.status === 'ACTIVE'
                ? 'bg-red-600/20 text-red-400 hover:bg-red-600/30 border border-red-800'
                : 'bg-green-600/20 text-green-400 hover:bg-green-600/30 border border-green-800'
            }`}
          >
            {agency.status === 'ACTIVE' ? 'Suspend' : 'Reactivate'}
          </button>

          <div className="relative group">
            <button
              disabled={actionLoading}
              className="px-4 py-2 bg-gray-800 border border-gray-700 rounded-lg text-sm font-medium text-gray-300 hover:bg-gray-700 disabled:opacity-50"
            >
              Override Tier
            </button>
            <div className="absolute right-0 mt-1 w-32 bg-gray-800 border border-gray-700 rounded-lg shadow-xl opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-all z-10">
              {(['TIER_1', 'TIER_2', 'TIER_3'] as AgencyTier[]).map((tier) => (
                <button
                  key={tier}
                  onClick={() => handleTierOverride(tier)}
                  className="w-full text-left px-3 py-2 text-sm text-gray-300 hover:bg-gray-700 first:rounded-t-lg last:rounded-b-lg"
                >
                  {TIER_LABELS[tier]}
                </button>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* Tabs */}
      <div className="border-b border-gray-800 mb-6">
        <div className="flex gap-1 -mb-px overflow-x-auto">
          {tabs.map((tab) => (
            <button
              key={tab.name}
              onClick={() => setActiveTab(tab.name)}
              className={`px-4 py-2.5 text-sm font-medium border-b-2 transition-colors whitespace-nowrap ${
                activeTab === tab.name
                  ? 'border-blue-500 text-blue-400'
                  : 'border-transparent text-gray-400 hover:text-gray-200 hover:border-gray-600'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {/* Tab Content */}
      {activeTab === 'overview' && <OverviewTab agency={agency} totalVolume={totalVolume} activeMerchants={activeMerchants} totalResidual={totalResidual} />}
      {activeTab === 'merchants' && <MerchantsTab agency={agency} />}
      {activeTab === 'network' && <NetworkTab agency={agency} />}
      {activeTab === 'residuals' && <ResidualsTab agency={agency} />}
      {activeTab === 'payouts' && <PayoutsTab agency={agency} />}
    </div>
  );
}

// ─── Tab Components ────────────────────────────────────────────────────

function OverviewTab({
  agency,
  totalVolume,
  activeMerchants,
  totalResidual,
}: {
  agency: AgencyDetail;
  totalVolume: number;
  activeMerchants: number;
  totalResidual: number;
}) {
  const statCards = [
    { label: 'Active Merchants', value: String(activeMerchants) },
    { label: 'Portfolio Volume (this month)', value: formatCents(totalVolume) },
    { label: 'Total Residuals (trailing 12mo)', value: formatCents(totalResidual) },
    { label: 'Referred Agencies', value: String(agency.referredAgencies.length) },
  ];

  return (
    <div className="space-y-6">
      {/* Stat cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {statCards.map((stat) => (
          <div
            key={stat.label}
            className="bg-gray-900/50 border border-gray-800 rounded-xl p-4"
          >
            <p className="text-xs text-gray-500 mb-1">{stat.label}</p>
            <p className="text-xl font-semibold text-white">{stat.value}</p>
          </div>
        ))}
      </div>

      {/* Details */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="bg-gray-900/50 border border-gray-800 rounded-xl p-5">
          <h3 className="text-sm font-medium text-gray-300 mb-3">Contact Info</h3>
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between">
              <dt className="text-gray-500">Email</dt>
              <dd className="text-gray-300">{agency.contactEmail}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500">Phone</dt>
              <dd className="text-gray-300">{agency.contactPhone ?? 'Not set'}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500">Payout Email</dt>
              <dd className="text-gray-300">{agency.payoutEmail ?? 'Not set'}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500">Bank (last 4)</dt>
              <dd className="text-gray-300">{agency.payoutBankLast4 ?? 'Not set'}</dd>
            </div>
          </dl>
        </div>

        <div className="bg-gray-900/50 border border-gray-800 rounded-xl p-5">
          <h3 className="text-sm font-medium text-gray-300 mb-3">Account Details</h3>
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between">
              <dt className="text-gray-500">Referral Code</dt>
              <dd className="text-gray-300 font-mono">{agency.referralCode}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500">Tier Override</dt>
              <dd className="text-gray-300">{agency.tierOverride ? 'Yes' : 'No'}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500">W-9 on File</dt>
              <dd className="text-gray-300">{agency.taxIdOnFile ? 'Yes' : 'No'}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500">Referred By</dt>
              <dd className="text-gray-300">{agency.referredByAgency?.name ?? 'Direct'}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500">Created</dt>
              <dd className="text-gray-300">{formatDate(agency.createdAt)}</dd>
            </div>
          </dl>
        </div>
      </div>

      {/* Residual chart */}
      {agency.residualHistory.length > 0 && (
        <div className="bg-gray-900/50 border border-gray-800 rounded-xl p-5">
          <h3 className="text-sm font-medium text-gray-300 mb-4">
            Monthly Residual (trailing 12 months)
          </h3>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={agency.residualHistory}>
                <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                <XAxis dataKey="month" stroke="#6B7280" tick={{ fontSize: 12 }} />
                <YAxis
                  stroke="#6B7280"
                  tick={{ fontSize: 12 }}
                  tickFormatter={(v: number) => `$${(v / 100).toFixed(0)}`}
                />
                <Tooltip
                  contentStyle={{
                    backgroundColor: '#1F2937',
                    border: '1px solid #374151',
                    borderRadius: '8px',
                  }}
                  formatter={(value: number, name: string) => [
                    formatCents(value),
                    name === 'agencyShare' ? 'Direct Share' : 'Two-Tier Share',
                  ]}
                />
                <Line
                  type="monotone"
                  dataKey="agencyShare"
                  stroke="#3B82F6"
                  strokeWidth={2}
                  dot={false}
                />
                <Line
                  type="monotone"
                  dataKey="twoTierShare"
                  stroke="#8B5CF6"
                  strokeWidth={2}
                  dot={false}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}
    </div>
  );
}

function MerchantsTab({ agency }: { agency: AgencyDetail }) {
  return (
    <div className="bg-gray-900/50 border border-gray-800 rounded-xl overflow-hidden">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-gray-800 text-gray-400">
            <th className="text-left px-4 py-3 font-medium">Name</th>
            <th className="text-left px-4 py-3 font-medium">Status</th>
            <th className="text-right px-4 py-3 font-medium">Volume (this month)</th>
            <th className="text-left px-4 py-3 font-medium">Created</th>
          </tr>
        </thead>
        <tbody>
          {agency.merchants.length === 0 ? (
            <tr>
              <td colSpan={4} className="px-4 py-8 text-center text-gray-500">
                No merchants attributed to this agency
              </td>
            </tr>
          ) : (
            agency.merchants.map((m) => (
              <tr key={m.id} className="border-b border-gray-800/50">
                <td className="px-4 py-3 font-medium text-white">{m.name}</td>
                <td className="px-4 py-3">
                  <span
                    className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
                      m.status === 'ACTIVE'
                        ? 'bg-green-500/20 text-green-400'
                        : 'bg-gray-500/20 text-gray-400'
                    }`}
                  >
                    {m.status}
                  </span>
                </td>
                <td className="px-4 py-3 text-right text-gray-300">
                  {formatCents(m.volumeThisMonth)}
                </td>
                <td className="px-4 py-3 text-gray-400">
                  {formatDate(m.createdAt)}
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

function NetworkTab({ agency }: { agency: AgencyDetail }) {
  return (
    <div className="space-y-4">
      {agency.referredByAgency && (
        <div className="bg-gray-900/50 border border-gray-800 rounded-xl p-4">
          <p className="text-sm text-gray-400">
            Referred by:{' '}
            <span className="text-white font-medium">{agency.referredByAgency.name}</span>
          </p>
        </div>
      )}

      <div className="bg-gray-900/50 border border-gray-800 rounded-xl overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-gray-800 text-gray-400">
              <th className="text-left px-4 py-3 font-medium">Agency Name</th>
              <th className="text-left px-4 py-3 font-medium">Email</th>
              <th className="text-left px-4 py-3 font-medium">Tier</th>
              <th className="text-left px-4 py-3 font-medium">Status</th>
              <th className="text-right px-4 py-3 font-medium">Merchants</th>
              <th className="text-right px-4 py-3 font-medium">Portfolio Volume</th>
              <th className="text-left px-4 py-3 font-medium">Created</th>
            </tr>
          </thead>
          <tbody>
            {agency.referredAgencies.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-4 py-8 text-center text-gray-500">
                  No referred agencies
                </td>
              </tr>
            ) : (
              agency.referredAgencies.map((ref) => (
                <tr key={ref.id} className="border-b border-gray-800/50">
                  <td className="px-4 py-3 font-medium text-white">{ref.name}</td>
                  <td className="px-4 py-3 text-gray-400">{ref.contactEmail}</td>
                  <td className="px-4 py-3">
                    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${TIER_COLORS[ref.tier]}`}>
                      {TIER_LABELS[ref.tier]}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${STATUS_COLORS[ref.status]}`}>
                      {ref.status}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right text-gray-300">{ref.merchantCount}</td>
                  <td className="px-4 py-3 text-right text-gray-300">
                    {formatCents(ref.portfolioVolume)}
                  </td>
                  <td className="px-4 py-3 text-gray-400">{formatDate(ref.createdAt)}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ResidualsTab({ agency }: { agency: AgencyDetail }) {
  return (
    <div className="space-y-6">
      {/* Chart */}
      {agency.residualHistory.length > 0 && (
        <div className="bg-gray-900/50 border border-gray-800 rounded-xl p-5">
          <h3 className="text-sm font-medium text-gray-300 mb-4">
            Monthly Residual Trend
          </h3>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={agency.residualHistory}>
                <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                <XAxis dataKey="month" stroke="#6B7280" tick={{ fontSize: 12 }} />
                <YAxis
                  stroke="#6B7280"
                  tick={{ fontSize: 12 }}
                  tickFormatter={(v: number) => `$${(v / 100).toFixed(0)}`}
                />
                <Tooltip
                  contentStyle={{
                    backgroundColor: '#1F2937',
                    border: '1px solid #374151',
                    borderRadius: '8px',
                  }}
                  formatter={(value: number, name: string) => [
                    formatCents(value),
                    name === 'agencyShare'
                      ? 'Direct Share'
                      : name === 'twoTierShare'
                      ? 'Two-Tier Share'
                      : 'Volume',
                  ]}
                />
                <Line type="monotone" dataKey="agencyShare" stroke="#3B82F6" strokeWidth={2} dot={false} name="agencyShare" />
                <Line type="monotone" dataKey="twoTierShare" stroke="#8B5CF6" strokeWidth={2} dot={false} name="twoTierShare" />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}

      {/* Table */}
      <div className="bg-gray-900/50 border border-gray-800 rounded-xl overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-gray-800 text-gray-400">
              <th className="text-left px-4 py-3 font-medium">Month</th>
              <th className="text-right px-4 py-3 font-medium">Volume</th>
              <th className="text-right px-4 py-3 font-medium">Direct Share</th>
              <th className="text-right px-4 py-3 font-medium">Two-Tier Share</th>
              <th className="text-right px-4 py-3 font-medium">Total</th>
            </tr>
          </thead>
          <tbody>
            {agency.residualHistory.length === 0 ? (
              <tr>
                <td colSpan={5} className="px-4 py-8 text-center text-gray-500">
                  No residual history
                </td>
              </tr>
            ) : (
              [...agency.residualHistory].reverse().map((entry) => (
                <tr key={entry.month} className="border-b border-gray-800/50">
                  <td className="px-4 py-3 text-white font-medium">{entry.month}</td>
                  <td className="px-4 py-3 text-right text-gray-300">
                    {formatCents(entry.volume)}
                  </td>
                  <td className="px-4 py-3 text-right text-blue-400">
                    {formatCents(entry.agencyShare)}
                  </td>
                  <td className="px-4 py-3 text-right text-purple-400">
                    {formatCents(entry.twoTierShare)}
                  </td>
                  <td className="px-4 py-3 text-right text-white font-medium">
                    {formatCents(entry.agencyShare + entry.twoTierShare)}
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

function PayoutsTab({ agency }: { agency: AgencyDetail }) {
  return (
    <div className="bg-gray-900/50 border border-gray-800 rounded-xl overflow-hidden">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-gray-800 text-gray-400">
            <th className="text-left px-4 py-3 font-medium">Period</th>
            <th className="text-right px-4 py-3 font-medium">Total</th>
            <th className="text-right px-4 py-3 font-medium">Direct Share</th>
            <th className="text-right px-4 py-3 font-medium">Two-Tier</th>
            <th className="text-left px-4 py-3 font-medium">Method</th>
            <th className="text-left px-4 py-3 font-medium">Status</th>
            <th className="text-left px-4 py-3 font-medium">Paid</th>
          </tr>
        </thead>
        <tbody>
          {agency.payouts.length === 0 ? (
            <tr>
              <td colSpan={7} className="px-4 py-8 text-center text-gray-500">
                No payouts yet
              </td>
            </tr>
          ) : (
            agency.payouts.map((p) => (
              <tr key={p.id} className="border-b border-gray-800/50">
                <td className="px-4 py-3 text-white">{formatDate(p.periodStart)}</td>
                <td className="px-4 py-3 text-right text-white font-medium">
                  {formatCents(p.totalAmount)}
                </td>
                <td className="px-4 py-3 text-right text-blue-400">
                  {formatCents(p.directShare)}
                </td>
                <td className="px-4 py-3 text-right text-purple-400">
                  {formatCents(p.twoTierShare)}
                </td>
                <td className="px-4 py-3 text-gray-300">{p.method}</td>
                <td className="px-4 py-3">
                  <span
                    className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
                      p.status === 'PAID'
                        ? 'bg-green-500/20 text-green-400'
                        : p.status === 'PENDING'
                        ? 'bg-yellow-500/20 text-yellow-400'
                        : p.status === 'APPROVED'
                        ? 'bg-blue-500/20 text-blue-400'
                        : 'bg-red-500/20 text-red-400'
                    }`}
                  >
                    {p.status}
                  </span>
                </td>
                <td className="px-4 py-3 text-gray-400">
                  {p.paidAt ? formatDate(p.paidAt) : '-'}
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}
