import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

// ─── Types ─────────────────────────────────────────────────────────────

type AgencyTier = "TIER_1" | "TIER_2" | "TIER_3";
type AgencyStatus = "ACTIVE" | "SUSPENDED" | "CHURNED";
type ResidualStatus = "PENDING" | "APPROVED" | "PAID" | "HELD";
type PayoutStatus = "PENDING" | "APPROVED" | "PAID" | "FAILED";
type Tab = "overview" | "merchants" | "network" | "residuals" | "payouts";

interface Merchant {
  id: string;
  name: string;
  email: string;
  status: string;
  createdAt: string;
}

interface ReferredAgency {
  id: string;
  name: string;
  contactEmail: string;
  tier: AgencyTier;
  status: AgencyStatus;
  merchantCount: number;
  portfolioVolume: number;
  monthlyResidual: number;
  createdAt: string;
}

interface ResidualEntry {
  id: string;
  periodStart: string;
  periodEnd: string;
  merchantVolume: number;
  agencyShare: number;
  twoTierShare: number;
  status: ResidualStatus;
  approvedAt: string | null;
}

interface ResidualPayout {
  id: string;
  periodStart: string;
  totalAmount: number;
  directShare: number;
  twoTierShare: number;
  method: string;
  status: PayoutStatus;
  paidAt: string | null;
  createdAt: string;
}

interface AgencyDetail {
  id: string;
  name: string;
  contactEmail: string;
  contactPhone: string | null;
  tier: AgencyTier;
  tierOverride: boolean;
  status: AgencyStatus;
  referralCode: string;
  payoutEmail: string | null;
  payoutBankLast4: string | null;
  taxIdOnFile: boolean;
  referredByAgency: { id: string; name: string } | null;
  createdAt: string;
  updatedAt: string;
  merchants: Merchant[];
  referredAgencies: ReferredAgency[];
  residualHistory: ResidualEntry[];
  payoutHistory: ResidualPayout[];
}

// ─── API ────────────────────────────────────────────────────────────────

async function fetchAgency(id: string): Promise<AgencyDetail> {
  const res = await fetch(`/api/v1/admin/agencies/${id}`);
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(
      (err as { error?: string }).error ?? `HTTP ${res.status}`
    );
  }
  return res.json() as Promise<AgencyDetail>;
}

async function patchAgency(
  id: string,
  body: { status?: AgencyStatus; tier?: AgencyTier; tierOverride?: boolean }
): Promise<AgencyDetail> {
  const res = await fetch(`/api/v1/admin/agencies/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(
      (err as { error?: string }).error ?? `HTTP ${res.status}`
    );
  }
  return res.json() as Promise<AgencyDetail>;
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
      className={`inline-flex items-center px-2.5 py-1 rounded text-sm font-medium border ${styles[tier]}`}
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
      className={`inline-flex items-center px-2.5 py-1 rounded text-sm font-medium border ${styles[status]}`}
    >
      {status.charAt(0) + status.slice(1).toLowerCase()}
    </span>
  );
}

function ResidualStatusBadge({ status }: { status: ResidualStatus }) {
  const styles: Record<ResidualStatus, string> = {
    PENDING: "text-yellow-400",
    APPROVED: "text-blue-400",
    PAID: "text-emerald-400",
    HELD: "text-red-400",
  };
  return (
    <span className={`text-xs font-medium ${styles[status]}`}>
      {status.charAt(0) + status.slice(1).toLowerCase()}
    </span>
  );
}

function PayoutStatusBadge({ status }: { status: PayoutStatus }) {
  const styles: Record<PayoutStatus, string> = {
    PENDING: "text-yellow-400",
    APPROVED: "text-blue-400",
    PAID: "text-emerald-400",
    FAILED: "text-red-400",
  };
  return (
    <span className={`text-xs font-medium ${styles[status]}`}>
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

function formatDate(iso: string | null): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function formatMonth(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
  });
}

// ─── Tab button ──────────────────────────────────────────────────────────

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className={[
        "px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors",
        active
          ? "border-emerald-500 text-emerald-400"
          : "border-transparent text-gray-500 hover:text-gray-300",
      ].join(" ")}
    >
      {children}
    </button>
  );
}

// ─── Overview metrics ────────────────────────────────────────────────────

function MetricCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-white/[0.03] border border-white/10 rounded-lg p-4">
      <p className="text-xs text-gray-500 mb-1">{label}</p>
      <p className="text-xl font-semibold text-white">{value}</p>
    </div>
  );
}

// ─── Main component ──────────────────────────────────────────────────────

export function AgencyDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const [agency, setAgency] = useState<AgencyDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>("overview");
  const [actionLoading, setActionLoading] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    setError(null);
    fetchAgency(id)
      .then(setAgency)
      .catch((err: unknown) => {
        setError(
          err instanceof Error ? err.message : "Failed to load agency"
        );
      })
      .finally(() => setLoading(false));
  }, [id]);

  async function handleSuspend() {
    if (!id || !agency) return;
    setActionLoading(true);
    setActionError(null);
    try {
      const updated = await patchAgency(id, { status: "SUSPENDED" });
      setAgency((prev) => (prev ? { ...prev, status: updated.status } : prev));
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : "Action failed");
    } finally {
      setActionLoading(false);
    }
  }

  async function handleReactivate() {
    if (!id || !agency) return;
    setActionLoading(true);
    setActionError(null);
    try {
      const updated = await patchAgency(id, { status: "ACTIVE" });
      setAgency((prev) => (prev ? { ...prev, status: updated.status } : prev));
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : "Action failed");
    } finally {
      setActionLoading(false);
    }
  }

  async function handleForceTier(tier: AgencyTier) {
    if (!id || !agency) return;
    setActionLoading(true);
    setActionError(null);
    try {
      const updated = await patchAgency(id, { tier, tierOverride: true });
      setAgency((prev) =>
        prev
          ? { ...prev, tier: updated.tier, tierOverride: updated.tierOverride }
          : prev
      );
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : "Action failed");
    } finally {
      setActionLoading(false);
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="h-8 w-8 rounded-full border-2 border-white/10 border-t-emerald-500 animate-spin" />
      </div>
    );
  }

  if (error || !agency) {
    return (
      <div className="p-6">
        <div className="p-4 bg-red-900/30 border border-red-700/50 rounded-lg text-red-300">
          {error ?? "Agency not found"}
        </div>
        <button
          onClick={() => navigate("/agencies")}
          className="mt-4 text-sm text-gray-400 hover:text-white"
        >
          ← Back to Agencies
        </button>
      </div>
    );
  }

  // Compute overview metrics
  const totalVolume = agency.residualHistory.reduce(
    (s, e) => s + e.merchantVolume,
    0
  );
  const totalResidual = agency.residualHistory.reduce(
    (s, e) => s + e.agencyShare,
    0
  );

  return (
    <div className="p-6 max-w-6xl">
      {/* Back link */}
      <button
        onClick={() => navigate("/agencies")}
        className="text-sm text-gray-500 hover:text-gray-300 mb-4 inline-flex items-center gap-1"
      >
        ← Agencies
      </button>

      {/* Header */}
      <div className="flex items-start justify-between mb-6">
        <div>
          <div className="flex items-center gap-3 mb-2">
            <h1 className="text-2xl font-semibold text-white">{agency.name}</h1>
            <StatusBadge status={agency.status} />
            <TierBadge tier={agency.tier} />
            {agency.tierOverride && (
              <span className="text-xs text-amber-400 border border-amber-700/50 rounded px-1.5 py-0.5">
                Override
              </span>
            )}
          </div>
          <p className="text-sm text-gray-400">
            {agency.contactEmail}
            {agency.contactPhone && ` · ${agency.contactPhone}`}
          </p>
          <p className="text-xs text-gray-600 mt-1">
            Referral code: <span className="font-mono text-gray-500">{agency.referralCode}</span>
            {agency.referredByAgency && (
              <> · Referred by: {agency.referredByAgency.name}</>
            )}
          </p>
        </div>

        {/* Action buttons */}
        <div className="flex items-center gap-2">
          {agency.status === "ACTIVE" ? (
            <button
              onClick={handleSuspend}
              disabled={actionLoading}
              className="px-3 py-1.5 text-sm rounded-md bg-red-900/40 border border-red-700/50 text-red-300 hover:bg-red-900/60 disabled:opacity-50 transition-colors"
            >
              Suspend
            </button>
          ) : agency.status === "SUSPENDED" ? (
            <button
              onClick={handleReactivate}
              disabled={actionLoading}
              className="px-3 py-1.5 text-sm rounded-md bg-emerald-900/40 border border-emerald-700/50 text-emerald-300 hover:bg-emerald-900/60 disabled:opacity-50 transition-colors"
            >
              Reactivate
            </button>
          ) : null}

          {/* Force tier dropdown */}
          <div className="relative">
            <select
              onChange={(e) => {
                if (e.target.value)
                  handleForceTier(e.target.value as AgencyTier);
                e.target.value = "";
              }}
              disabled={actionLoading}
              className="appearance-none bg-[#161b22] border border-white/10 rounded-md px-3 py-1.5 text-sm text-gray-400 hover:border-white/20 focus:outline-none disabled:opacity-50 cursor-pointer"
              defaultValue=""
            >
              <option value="" disabled>
                Force Tier...
              </option>
              <option value="TIER_1">Force Tier 1</option>
              <option value="TIER_2">Force Tier 2</option>
              <option value="TIER_3">Force Tier 3</option>
            </select>
          </div>
        </div>
      </div>

      {/* Action error */}
      {actionError && (
        <div className="mb-4 p-3 bg-red-900/30 border border-red-700/50 rounded-md text-sm text-red-300">
          {actionError}
        </div>
      )}

      {/* Tabs */}
      <div className="flex border-b border-white/10 mb-6 gap-1">
        <TabButton active={tab === "overview"} onClick={() => setTab("overview")}>
          Overview
        </TabButton>
        <TabButton active={tab === "merchants"} onClick={() => setTab("merchants")}>
          Merchants ({agency.merchants.length})
        </TabButton>
        <TabButton active={tab === "network"} onClick={() => setTab("network")}>
          Network ({agency.referredAgencies.length})
        </TabButton>
        <TabButton active={tab === "residuals"} onClick={() => setTab("residuals")}>
          Residuals ({agency.residualHistory.length})
        </TabButton>
        <TabButton active={tab === "payouts"} onClick={() => setTab("payouts")}>
          Payouts ({agency.payoutHistory.length})
        </TabButton>
      </div>

      {/* Tab content */}

      {tab === "overview" && (
        <div>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
            <MetricCard
              label="Merchants"
              value={agency.merchants.length.toLocaleString()}
            />
            <MetricCard
              label="Referred Agencies"
              value={agency.referredAgencies.length.toLocaleString()}
            />
            <MetricCard
              label="Portfolio Volume (12mo)"
              value={formatCents(totalVolume)}
            />
            <MetricCard
              label="Total Residual (12mo)"
              value={formatCents(totalResidual)}
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="bg-white/[0.02] border border-white/10 rounded-lg p-4">
              <h3 className="text-sm font-medium text-gray-300 mb-3">
                Account Details
              </h3>
              <dl className="space-y-2 text-sm">
                <div className="flex justify-between">
                  <dt className="text-gray-500">Tax ID on file</dt>
                  <dd className="text-gray-200">
                    {agency.taxIdOnFile ? "Yes" : "No"}
                  </dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-gray-500">Payout email</dt>
                  <dd className="text-gray-200">{agency.payoutEmail ?? "—"}</dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-gray-500">Bank last 4</dt>
                  <dd className="text-gray-200">
                    {agency.payoutBankLast4
                      ? `····${agency.payoutBankLast4}`
                      : "—"}
                  </dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-gray-500">Member since</dt>
                  <dd className="text-gray-200">
                    {formatDate(agency.createdAt)}
                  </dd>
                </div>
              </dl>
            </div>
          </div>
        </div>
      )}

      {tab === "merchants" && (
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
                  Status
                </th>
                <th className="text-left px-4 py-3 font-medium text-gray-400">
                  Joined
                </th>
              </tr>
            </thead>
            <tbody>
              {agency.merchants.length === 0 && (
                <tr>
                  <td
                    colSpan={4}
                    className="text-center py-10 text-gray-500"
                  >
                    No merchants attributed to this agency
                  </td>
                </tr>
              )}
              {agency.merchants.map((m) => (
                <tr
                  key={m.id}
                  className="border-b border-white/[0.05] hover:bg-white/[0.02]"
                >
                  <td className="px-4 py-3 text-white">{m.name}</td>
                  <td className="px-4 py-3 text-gray-400">{m.email}</td>
                  <td className="px-4 py-3">
                    <span
                      className={`text-xs font-medium ${
                        m.status === "ACTIVE"
                          ? "text-emerald-400"
                          : "text-gray-500"
                      }`}
                    >
                      {m.status.charAt(0) + m.status.slice(1).toLowerCase()}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-gray-500 text-xs">
                    {formatDate(m.createdAt)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tab === "network" && (
        <div className="overflow-x-auto rounded-lg border border-white/10">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/10 bg-white/[0.02]">
                <th className="text-left px-4 py-3 font-medium text-gray-400">
                  Agency
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
                  Volume (30d)
                </th>
                <th className="text-right px-4 py-3 font-medium text-gray-400">
                  Residual (30d)
                </th>
              </tr>
            </thead>
            <tbody>
              {agency.referredAgencies.length === 0 && (
                <tr>
                  <td
                    colSpan={6}
                    className="text-center py-10 text-gray-500"
                  >
                    No agencies referred by this agency
                  </td>
                </tr>
              )}
              {agency.referredAgencies.map((a) => (
                <tr
                  key={a.id}
                  onClick={() => navigate(`/agencies/${a.id}`)}
                  className="border-b border-white/[0.05] hover:bg-white/[0.03] cursor-pointer transition-colors"
                >
                  <td className="px-4 py-3">
                    <p className="font-medium text-white">{a.name}</p>
                    <p className="text-xs text-gray-500">{a.contactEmail}</p>
                  </td>
                  <td className="px-4 py-3">
                    <TierBadge tier={a.tier} />
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge status={a.status} />
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums">
                    {a.merchantCount.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-gray-300">
                    {formatCents(a.portfolioVolume)}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-emerald-400">
                    {formatCents(a.monthlyResidual)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tab === "residuals" && (
        <div className="overflow-x-auto rounded-lg border border-white/10">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/10 bg-white/[0.02]">
                <th className="text-left px-4 py-3 font-medium text-gray-400">
                  Period
                </th>
                <th className="text-right px-4 py-3 font-medium text-gray-400">
                  Merchant Vol.
                </th>
                <th className="text-right px-4 py-3 font-medium text-gray-400">
                  Agency Share
                </th>
                <th className="text-right px-4 py-3 font-medium text-gray-400">
                  2-Tier Share
                </th>
                <th className="text-left px-4 py-3 font-medium text-gray-400">
                  Status
                </th>
                <th className="text-left px-4 py-3 font-medium text-gray-400">
                  Approved
                </th>
              </tr>
            </thead>
            <tbody>
              {agency.residualHistory.length === 0 && (
                <tr>
                  <td
                    colSpan={6}
                    className="text-center py-10 text-gray-500"
                  >
                    No residual history in the last 12 months
                  </td>
                </tr>
              )}
              {agency.residualHistory.map((entry) => (
                <tr
                  key={entry.id}
                  className="border-b border-white/[0.05] hover:bg-white/[0.02]"
                >
                  <td className="px-4 py-3 text-gray-300">
                    {formatMonth(entry.periodStart)}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-gray-400">
                    {formatCents(entry.merchantVolume)}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-emerald-400">
                    {formatCents(entry.agencyShare)}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-gray-400">
                    {entry.twoTierShare > 0
                      ? formatCents(entry.twoTierShare)
                      : "—"}
                  </td>
                  <td className="px-4 py-3">
                    <ResidualStatusBadge status={entry.status} />
                  </td>
                  <td className="px-4 py-3 text-gray-500 text-xs">
                    {formatDate(entry.approvedAt)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tab === "payouts" && (
        <div className="overflow-x-auto rounded-lg border border-white/10">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/10 bg-white/[0.02]">
                <th className="text-left px-4 py-3 font-medium text-gray-400">
                  Period
                </th>
                <th className="text-right px-4 py-3 font-medium text-gray-400">
                  Total
                </th>
                <th className="text-right px-4 py-3 font-medium text-gray-400">
                  Direct
                </th>
                <th className="text-right px-4 py-3 font-medium text-gray-400">
                  2-Tier
                </th>
                <th className="text-left px-4 py-3 font-medium text-gray-400">
                  Method
                </th>
                <th className="text-left px-4 py-3 font-medium text-gray-400">
                  Status
                </th>
                <th className="text-left px-4 py-3 font-medium text-gray-400">
                  Paid
                </th>
              </tr>
            </thead>
            <tbody>
              {agency.payoutHistory.length === 0 && (
                <tr>
                  <td
                    colSpan={7}
                    className="text-center py-10 text-gray-500"
                  >
                    No payouts yet
                  </td>
                </tr>
              )}
              {agency.payoutHistory.map((payout) => (
                <tr
                  key={payout.id}
                  className="border-b border-white/[0.05] hover:bg-white/[0.02]"
                >
                  <td className="px-4 py-3 text-gray-300">
                    {formatMonth(payout.periodStart)}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-white font-medium">
                    {formatCents(payout.totalAmount)}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-gray-400">
                    {formatCents(payout.directShare)}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-gray-400">
                    {payout.twoTierShare > 0
                      ? formatCents(payout.twoTierShare)
                      : "—"}
                  </td>
                  <td className="px-4 py-3 text-gray-400">
                    {payout.method}
                  </td>
                  <td className="px-4 py-3">
                    <PayoutStatusBadge status={payout.status} />
                  </td>
                  <td className="px-4 py-3 text-gray-500 text-xs">
                    {formatDate(payout.paidAt)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
