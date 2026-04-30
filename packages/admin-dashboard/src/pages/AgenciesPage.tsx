import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

// ─── Types ─────────────────────────────────────────────────────────────

type AgencyTier = "TIER_1" | "TIER_2" | "TIER_3";
type AgencyStatus = "ACTIVE" | "SUSPENDED" | "CHURNED";
type SortField = "createdAt" | "merchantCount" | "portfolioVolume";

interface AgencySummary {
  id: string;
  name: string;
  contactEmail: string;
  tier: AgencyTier;
  status: AgencyStatus;
  merchantCount: number;
  portfolioVolume: number;
  monthlyResidual: number;
  referredByAgency: { name: string } | null;
  createdAt: string;
}

interface AgencyListResponse {
  data: AgencySummary[];
  total: number;
  page: number;
  limit: number;
}

// ─── Badge helpers ──────────────────────────────────────────────────────

function TierBadge({ tier }: { tier: AgencyTier }) {
  const styles: Record<AgencyTier, string> = {
    TIER_1: "bg-blue-900/40 text-blue-300 border-blue-700/50",
    TIER_2: "bg-purple-900/40 text-purple-300 border-purple-700/50",
    TIER_3: "bg-amber-900/40 text-amber-300 border-amber-700/50",
  };
  const labels: Record<AgencyTier, string> = {
    TIER_1: "Tier 1",
    TIER_2: "Tier 2",
    TIER_3: "Tier 3",
  };
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium border ${styles[tier]}`}
    >
      {labels[tier]}
    </span>
  );
}

function StatusBadge({ status }: { status: AgencyStatus }) {
  const styles: Record<AgencyStatus, string> = {
    ACTIVE: "bg-emerald-900/40 text-emerald-300 border-emerald-700/50",
    SUSPENDED: "bg-red-900/40 text-red-300 border-red-700/50",
    CHURNED: "bg-gray-900/40 text-gray-400 border-gray-700/50",
  };
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium border ${styles[status]}`}
    >
      {status.charAt(0) + status.slice(1).toLowerCase()}
    </span>
  );
}

// ─── Formatting ─────────────────────────────────────────────────────────

function formatCents(cents: number): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(cents / 100);
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

// ─── API ────────────────────────────────────────────────────────────────

async function fetchAgencies(params: {
  page: number;
  limit: number;
  tier?: AgencyTier;
  status?: AgencyStatus;
  search?: string;
  sort?: SortField;
}): Promise<AgencyListResponse> {
  const q = new URLSearchParams();
  q.set("page", String(params.page));
  q.set("limit", String(params.limit));
  if (params.tier) q.set("tier", params.tier);
  if (params.status) q.set("status", params.status);
  if (params.search) q.set("search", params.search);
  if (params.sort) q.set("sort", params.sort);

  const res = await fetch(`/api/v1/admin/agencies?${q.toString()}`);
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(
      (err as { error?: string }).error ?? `HTTP ${res.status}`
    );
  }
  return res.json() as Promise<AgencyListResponse>;
}

// ─── Component ─────────────────────────────────────────────────────────

export function AgenciesPage() {
  const navigate = useNavigate();

  const [agencies, setAgencies] = useState<AgencySummary[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const limit = 20;

  const [tier, setTier] = useState<AgencyTier | "">("");
  const [status, setStatus] = useState<AgencyStatus | "">("");
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState<SortField>("createdAt");

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    fetchAgencies({
      page,
      limit,
      tier: tier || undefined,
      status: status || undefined,
      search: search || undefined,
      sort,
    })
      .then((data) => {
        if (cancelled) return;
        setAgencies(data.data);
        setTotal(data.total);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "Failed to load agencies");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [page, tier, status, search, sort]);

  const totalPages = Math.ceil(total / limit);

  return (
    <div className="p-6">
      {/* Header */}
      <div className="mb-6">
        <h1 className="text-2xl font-semibold text-white">Agencies</h1>
        <p className="text-sm text-gray-400 mt-1">
          {total} agencies in the affiliate network
        </p>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3 mb-4">
        {/* Search */}
        <input
          type="text"
          placeholder="Search name or email..."
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
            setPage(1);
          }}
          className="bg-[#161b22] border border-white/10 rounded-md px-3 py-2 text-sm text-gray-200 placeholder:text-gray-500 focus:outline-none focus:border-emerald-500/50 w-56"
        />

        {/* Tier filter */}
        <select
          value={tier}
          onChange={(e) => {
            setTier(e.target.value as AgencyTier | "");
            setPage(1);
          }}
          className="bg-[#161b22] border border-white/10 rounded-md px-3 py-2 text-sm text-gray-200 focus:outline-none focus:border-emerald-500/50"
        >
          <option value="">All Tiers</option>
          <option value="TIER_1">Tier 1</option>
          <option value="TIER_2">Tier 2</option>
          <option value="TIER_3">Tier 3</option>
        </select>

        {/* Status filter */}
        <select
          value={status}
          onChange={(e) => {
            setStatus(e.target.value as AgencyStatus | "");
            setPage(1);
          }}
          className="bg-[#161b22] border border-white/10 rounded-md px-3 py-2 text-sm text-gray-200 focus:outline-none focus:border-emerald-500/50"
        >
          <option value="">All Statuses</option>
          <option value="ACTIVE">Active</option>
          <option value="SUSPENDED">Suspended</option>
          <option value="CHURNED">Churned</option>
        </select>

        {/* Sort */}
        <select
          value={sort}
          onChange={(e) => {
            setSort(e.target.value as SortField);
            setPage(1);
          }}
          className="bg-[#161b22] border border-white/10 rounded-md px-3 py-2 text-sm text-gray-200 focus:outline-none focus:border-emerald-500/50"
        >
          <option value="createdAt">Sort: Newest</option>
          <option value="merchantCount">Sort: Merchant Count</option>
          <option value="portfolioVolume">Sort: Portfolio Volume</option>
        </select>
      </div>

      {/* Error */}
      {error && (
        <div className="mb-4 p-3 bg-red-900/30 border border-red-700/50 rounded-md text-sm text-red-300">
          {error}
        </div>
      )}

      {/* Table */}
      <div className="overflow-x-auto rounded-lg border border-white/10">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-white/10 bg-white/[0.02]">
              <th className="text-left px-4 py-3 font-medium text-gray-400">
                Name
              </th>
              <th className="text-left px-4 py-3 font-medium text-gray-400">
                Email
              </th>
              <th className="text-left px-4 py-3 font-medium text-gray-400">
                Tier
              </th>
              <th className="text-left px-4 py-3 font-medium text-gray-400">
                Status
              </th>
              <th className="text-right px-4 py-3 font-medium text-gray-400">
                Merchants
              </th>
              <th className="text-right px-4 py-3 font-medium text-gray-400">
                Portfolio Vol.
              </th>
              <th className="text-right px-4 py-3 font-medium text-gray-400">
                Monthly Residual
              </th>
              <th className="text-left px-4 py-3 font-medium text-gray-400">
                Referred By
              </th>
              <th className="text-left px-4 py-3 font-medium text-gray-400">
                Created
              </th>
            </tr>
          </thead>
          <tbody>
            {loading && (
              <tr>
                <td
                  colSpan={9}
                  className="text-center py-12 text-gray-500"
                >
                  Loading...
                </td>
              </tr>
            )}
            {!loading && agencies.length === 0 && (
              <tr>
                <td
                  colSpan={9}
                  className="text-center py-12 text-gray-500"
                >
                  No agencies found
                </td>
              </tr>
            )}
            {!loading &&
              agencies.map((agency) => (
                <tr
                  key={agency.id}
                  onClick={() => navigate(`/agencies/${agency.id}`)}
                  className="border-b border-white/[0.05] hover:bg-white/[0.03] cursor-pointer transition-colors"
                >
                  <td className="px-4 py-3 font-medium text-white">
                    {agency.name}
                  </td>
                  <td className="px-4 py-3 text-gray-400">
                    {agency.contactEmail}
                  </td>
                  <td className="px-4 py-3">
                    <TierBadge tier={agency.tier} />
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge status={agency.status} />
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums">
                    {agency.merchantCount.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-gray-300">
                    {formatCents(agency.portfolioVolume)}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-emerald-400">
                    {formatCents(agency.monthlyResidual)}
                  </td>
                  <td className="px-4 py-3 text-gray-500 text-xs">
                    {agency.referredByAgency?.name ?? "—"}
                  </td>
                  <td className="px-4 py-3 text-gray-500 text-xs">
                    {formatDate(agency.createdAt)}
                  </td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between mt-4">
          <p className="text-sm text-gray-500">
            Showing {(page - 1) * limit + 1}–
            {Math.min(page * limit, total)} of {total}
          </p>
          <div className="flex gap-2">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page === 1}
              className="px-3 py-1.5 text-sm rounded-md border border-white/10 text-gray-400 hover:text-white hover:border-white/20 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              Previous
            </button>
            <button
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page === totalPages}
              className="px-3 py-1.5 text-sm rounded-md border border-white/10 text-gray-400 hover:text-white hover:border-white/20 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
