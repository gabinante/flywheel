import { useState, useEffect, useCallback } from "react";

// ─── Types ────────────────────────────────────────���──────────────────

interface ResidualEntry {
  id: string;
  merchantId: string;
  merchantName: string;
  merchantVolume: number;
  nmiResidualEarned: number;
  agencyShare: number;
  twoTierShare: number;
  status: string;
  approvedAt: string | null;
  approvedBy: string | null;
}

interface AgencyGroup {
  agencyId: string;
  agencyName: string;
  agencyTier: string;
  entries: ResidualEntry[];
}

interface ResidualSummary {
  periodStart: string;
  totalEntries: number;
  totalAgencyShare: number;
  totalTwoTierShare: number;
  totalOwed: number;
  byStatus: Record<string, number>;
  byTier: Record<string, { count: number; agencyShare: number; twoTierShare: number }>;
}

interface PayoutRecord {
  id: string;
  agencyId: string;
  periodStart: string;
  totalAmount: number;
  directShare: number;
  twoTierShare: number;
  method: string;
  reference: string | null;
  status: string;
  paidAt: string | null;
}

// ─── API helpers ─────────────────────────────────────────────────────

const API_BASE = "/api/v1/admin";

async function fetchResiduals(periodStart: string, status?: string, agencyId?: string) {
  const params = new URLSearchParams({ periodStart });
  if (status) params.set("status", status);
  if (agencyId) params.set("agencyId", agencyId);
  const res = await fetch(`${API_BASE}/residuals?${params}`);
  if (!res.ok) throw new Error(`Failed to fetch residuals: ${res.statusText}`);
  return res.json() as Promise<{ periodStart: string; agencies: AgencyGroup[] }>;
}

async function fetchSummary(periodStart: string) {
  const params = new URLSearchParams({ periodStart });
  const res = await fetch(`${API_BASE}/residuals/summary?${params}`);
  if (!res.ok) throw new Error(`Failed to fetch summary: ${res.statusText}`);
  return res.json() as Promise<ResidualSummary>;
}

async function approveEntries(entryIds: string[]) {
  const res = await fetch(`${API_BASE}/residuals/approve`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ entryIds }),
  });
  if (!res.ok) throw new Error(`Failed to approve: ${res.statusText}`);
  return res.json() as Promise<{ success: boolean; countApproved: number }>;
}

async function holdEntries(entryIds: string[], reason: string) {
  const res = await fetch(`${API_BASE}/residuals/hold`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ entryIds, reason }),
  });
  if (!res.ok) throw new Error(`Failed to hold: ${res.statusText}`);
  return res.json() as Promise<{ success: boolean; countHeld: number }>;
}

async function createPayout(agencyId: string, periodStart: string, method: "manual" | "ach", reference?: string) {
  const res = await fetch(`${API_BASE}/residuals/create-payout`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ agencyId, periodStart, method, reference: reference || undefined }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error((err as { error?: string }).error ?? `Failed to create payout: ${res.statusText}`);
  }
  return res.json() as Promise<{ success: boolean; payout: PayoutRecord }>;
}

// ─── Formatting helpers ──────────────────────────────────────────────

function formatCents(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

function getStatusColor(status: string): string {
  switch (status) {
    case "PENDING": return "#f59e0b";
    case "APPROVED": return "#22c55e";
    case "PAID": return "#6366f1";
    case "HELD": return "#ef4444";
    default: return "#94a3b8";
  }
}

function getMonthOptions(): { value: string; label: string }[] {
  const options: { value: string; label: string }[] = [];
  const now = new Date();
  for (let i = 0; i < 12; i++) {
    const d = new Date(now.getFullYear(), now.getMonth() - i, 1);
    const value = d.toISOString().split("T")[0];
    const label = d.toLocaleDateString("en-US", { year: "numeric", month: "long" });
    options.push({ value, label });
  }
  return options;
}

// ─── Styles (inline, dark-mode-first) ────────────────────────────────

const cardStyle: React.CSSProperties = {
  background: "rgba(255,255,255,0.03)",
  border: "1px solid rgba(255,255,255,0.06)",
  borderRadius: 12,
  padding: "16px 20px",
  minWidth: 160,
};

const tableStyle: React.CSSProperties = {
  width: "100%",
  borderCollapse: "collapse",
  fontSize: 13,
};

const thStyle: React.CSSProperties = {
  textAlign: "left",
  padding: "10px 12px",
  borderBottom: "1px solid rgba(255,255,255,0.08)",
  color: "#94a3b8",
  fontSize: 11,
  fontWeight: 600,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
};

const tdStyle: React.CSSProperties = {
  padding: "10px 12px",
  borderBottom: "1px solid rgba(255,255,255,0.04)",
};

const btnBase: React.CSSProperties = {
  border: "none",
  borderRadius: 8,
  padding: "8px 16px",
  fontSize: 13,
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.15s",
};

const btnPrimary: React.CSSProperties = {
  ...btnBase,
  background: "#6366f1",
  color: "#fff",
};

const btnSuccess: React.CSSProperties = {
  ...btnBase,
  background: "#22c55e",
  color: "#fff",
};

const btnDanger: React.CSSProperties = {
  ...btnBase,
  background: "#ef4444",
  color: "#fff",
};

const btnOutline: React.CSSProperties = {
  ...btnBase,
  background: "transparent",
  color: "#94a3b8",
  border: "1px solid rgba(255,255,255,0.1)",
};

// ─── Modal component ─────────────────────────────────────────────────

function Modal({
  open,
  title,
  onClose,
  children,
}: {
  open: boolean;
  title: string;
  onClose: () => void;
  children: React.ReactNode;
}) {
  if (!open) return null;
  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.6)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        zIndex: 1000,
      }}
      onClick={onClose}
    >
      <div
        style={{
          background: "#1a1a24",
          border: "1px solid rgba(255,255,255,0.08)",
          borderRadius: 16,
          padding: 24,
          minWidth: 400,
          maxWidth: 500,
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <h3 style={{ margin: "0 0 16px", fontSize: 16, fontWeight: 700 }}>{title}</h3>
        {children}
      </div>
    </div>
  );
}

// ─── StatusBadge ─────────────────────────────────────────────────────

function StatusBadge({ status }: { status: string }) {
  return (
    <span
      style={{
        display: "inline-block",
        padding: "3px 10px",
        borderRadius: 20,
        fontSize: 11,
        fontWeight: 600,
        color: getStatusColor(status),
        background: `${getStatusColor(status)}15`,
        border: `1px solid ${getStatusColor(status)}30`,
      }}
    >
      {status}
    </span>
  );
}

// ─── TierBadge ───────────────────────────────────────────────────────

function TierBadge({ tier }: { tier: string }) {
  const colors: Record<string, string> = {
    TIER_1: "#60a5fa",
    TIER_2: "#a78bfa",
    TIER_3: "#f472b6",
  };
  const color = colors[tier] ?? "#94a3b8";
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        borderRadius: 6,
        fontSize: 10,
        fontWeight: 700,
        color,
        background: `${color}15`,
        marginLeft: 8,
      }}
    >
      {tier.replace("_", " ")}
    </span>
  );
}

// ─── Main ResidualsPage ──────────────────────────────────────────────

export default function ResidualsPage() {
  const monthOptions = getMonthOptions();
  const [periodStart, setPeriodStart] = useState(monthOptions[0].value);
  const [statusFilter, setStatusFilter] = useState<string>("");
  const [agencies, setAgencies] = useState<AgencyGroup[]>([]);
  const [summary, setSummary] = useState<ResidualSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Selection
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

  // Expanded agencies
  const [expandedAgencies, setExpandedAgencies] = useState<Set<string>>(new Set());

  // Modals
  const [showApproveAllModal, setShowApproveAllModal] = useState(false);
  const [showHoldModal, setShowHoldModal] = useState(false);
  const [holdReason, setHoldReason] = useState("");
  const [showPayoutModal, setShowPayoutModal] = useState(false);
  const [payoutAgencyId, setPayoutAgencyId] = useState<string | null>(null);
  const [payoutMethod, setPayoutMethod] = useState<"manual" | "ach">("ach");
  const [payoutReference, setPayoutReference] = useState("");

  const [actionLoading, setActionLoading] = useState(false);
  const [actionMessage, setActionMessage] = useState<string | null>(null);

  // ─── Data loading ────────────────────────────────────────────────

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    setSelectedIds(new Set());
    try {
      const [residualData, summaryData] = await Promise.all([
        fetchResiduals(periodStart, statusFilter || undefined),
        fetchSummary(periodStart),
      ]);
      setAgencies(residualData.agencies);
      setSummary(summaryData);
      // Auto-expand all agencies
      setExpandedAgencies(new Set(residualData.agencies.map((a) => a.agencyId)));
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to load data");
    } finally {
      setLoading(false);
    }
  }, [periodStart, statusFilter]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // ─── Selection helpers ───────────────────────────────────────────

  function toggleSelect(id: string) {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleAgencyExpand(agencyId: string) {
    setExpandedAgencies((prev) => {
      const next = new Set(prev);
      if (next.has(agencyId)) next.delete(agencyId);
      else next.add(agencyId);
      return next;
    });
  }

  function selectAllPending() {
    const pendingIds = new Set<string>();
    for (const agency of agencies) {
      for (const entry of agency.entries) {
        if (entry.status === "PENDING") pendingIds.add(entry.id);
      }
    }
    setSelectedIds(pendingIds);
  }

  // ─── Actions ─────────────────────────────────────────────────────

  async function handleApproveSelected() {
    if (selectedIds.size === 0) return;
    setActionLoading(true);
    setActionMessage(null);
    try {
      const result = await approveEntries(Array.from(selectedIds));
      setActionMessage(`Approved ${result.countApproved} entries`);
      setSelectedIds(new Set());
      await loadData();
    } catch (err: unknown) {
      setActionMessage(`Error: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setActionLoading(false);
    }
  }

  async function handleApproveAllPending() {
    const pendingIds: string[] = [];
    for (const agency of agencies) {
      for (const entry of agency.entries) {
        if (entry.status === "PENDING") pendingIds.push(entry.id);
      }
    }
    if (pendingIds.length === 0) return;
    setActionLoading(true);
    setActionMessage(null);
    try {
      const result = await approveEntries(pendingIds);
      setActionMessage(`Approved ${result.countApproved} entries`);
      setShowApproveAllModal(false);
      await loadData();
    } catch (err: unknown) {
      setActionMessage(`Error: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setActionLoading(false);
    }
  }

  async function handleHoldSelected() {
    if (selectedIds.size === 0 || !holdReason.trim()) return;
    setActionLoading(true);
    setActionMessage(null);
    try {
      const result = await holdEntries(Array.from(selectedIds), holdReason.trim());
      setActionMessage(`Held ${result.countHeld} entries`);
      setSelectedIds(new Set());
      setShowHoldModal(false);
      setHoldReason("");
      await loadData();
    } catch (err: unknown) {
      setActionMessage(`Error: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setActionLoading(false);
    }
  }

  async function handleCreatePayout() {
    if (!payoutAgencyId) return;
    setActionLoading(true);
    setActionMessage(null);
    try {
      const result = await createPayout(
        payoutAgencyId,
        periodStart,
        payoutMethod,
        payoutReference.trim() || undefined
      );
      setActionMessage(
        `Payout created: ${formatCents(result.payout.totalAmount)} via ${result.payout.method}`
      );
      setShowPayoutModal(false);
      setPayoutAgencyId(null);
      setPayoutReference("");
      await loadData();
    } catch (err: unknown) {
      setActionMessage(`Error: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setActionLoading(false);
    }
  }

  function canCreatePayout(agency: AgencyGroup): boolean {
    return (
      agency.entries.length > 0 &&
      agency.entries.every((e) => e.status === "APPROVED")
    );
  }

  // ─── Render ──────────────────────────────────────────────────────

  const pendingCount = summary?.byStatus.PENDING ?? 0;

  return (
    <div>
      {/* Header */}
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 24 }}>
        <div>
          <h2 style={{ margin: 0, fontSize: 22, fontWeight: 700 }}>Residual Approval</h2>
          <p style={{ margin: "4px 0 0", color: "#64748b", fontSize: 13 }}>
            Review and approve monthly residual entries before payout
          </p>
        </div>
        <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
          <select
            value={periodStart}
            onChange={(e) => setPeriodStart(e.target.value)}
            style={{
              background: "rgba(255,255,255,0.05)",
              border: "1px solid rgba(255,255,255,0.1)",
              borderRadius: 8,
              padding: "8px 12px",
              color: "#e2e8f0",
              fontSize: 13,
            }}
          >
            {monthOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>

          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            style={{
              background: "rgba(255,255,255,0.05)",
              border: "1px solid rgba(255,255,255,0.1)",
              borderRadius: 8,
              padding: "8px 12px",
              color: "#e2e8f0",
              fontSize: 13,
            }}
          >
            <option value="">All Statuses</option>
            <option value="PENDING">Pending</option>
            <option value="APPROVED">Approved</option>
            <option value="HELD">Held</option>
            <option value="PAID">Paid</option>
          </select>
        </div>
      </div>

      {/* Summary cards */}
      {summary && (
        <div style={{ display: "flex", gap: 16, marginBottom: 24, flexWrap: "wrap" }}>
          <div style={cardStyle}>
            <div style={{ fontSize: 11, color: "#64748b", marginBottom: 4, textTransform: "uppercase", letterSpacing: "0.05em" }}>
              Total Owed
            </div>
            <div style={{ fontSize: 22, fontWeight: 700 }}>{formatCents(summary.totalOwed)}</div>
            <div style={{ fontSize: 11, color: "#64748b", marginTop: 2 }}>
              {summary.totalEntries} entries
            </div>
          </div>
          <div style={cardStyle}>
            <div style={{ fontSize: 11, color: "#f59e0b", marginBottom: 4, textTransform: "uppercase", letterSpacing: "0.05em" }}>
              Pending
            </div>
            <div style={{ fontSize: 22, fontWeight: 700, color: "#f59e0b" }}>
              {summary.byStatus.PENDING ?? 0}
            </div>
          </div>
          <div style={cardStyle}>
            <div style={{ fontSize: 11, color: "#22c55e", marginBottom: 4, textTransform: "uppercase", letterSpacing: "0.05em" }}>
              Approved
            </div>
            <div style={{ fontSize: 22, fontWeight: 700, color: "#22c55e" }}>
              {summary.byStatus.APPROVED ?? 0}
            </div>
          </div>
          <div style={cardStyle}>
            <div style={{ fontSize: 11, color: "#ef4444", marginBottom: 4, textTransform: "uppercase", letterSpacing: "0.05em" }}>
              Held
            </div>
            <div style={{ fontSize: 22, fontWeight: 700, color: "#ef4444" }}>
              {summary.byStatus.HELD ?? 0}
            </div>
          </div>
          <div style={cardStyle}>
            <div style={{ fontSize: 11, color: "#6366f1", marginBottom: 4, textTransform: "uppercase", letterSpacing: "0.05em" }}>
              Paid
            </div>
            <div style={{ fontSize: 22, fontWeight: 700, color: "#6366f1" }}>
              {summary.byStatus.PAID ?? 0}
            </div>
          </div>
        </div>
      )}

      {/* Action bar */}
      <div
        style={{
          display: "flex",
          gap: 12,
          marginBottom: 16,
          alignItems: "center",
          flexWrap: "wrap",
        }}
      >
        <button
          style={{ ...btnSuccess, opacity: selectedIds.size === 0 ? 0.5 : 1 }}
          disabled={selectedIds.size === 0 || actionLoading}
          onClick={handleApproveSelected}
        >
          Approve Selected ({selectedIds.size})
        </button>
        <button
          style={{ ...btnDanger, opacity: selectedIds.size === 0 ? 0.5 : 1 }}
          disabled={selectedIds.size === 0 || actionLoading}
          onClick={() => setShowHoldModal(true)}
        >
          Hold Selected ({selectedIds.size})
        </button>
        <button
          style={{ ...btnPrimary, opacity: pendingCount === 0 ? 0.5 : 1 }}
          disabled={pendingCount === 0 || actionLoading}
          onClick={() => setShowApproveAllModal(true)}
        >
          Approve All Pending ({pendingCount})
        </button>
        <button style={btnOutline} onClick={selectAllPending} disabled={actionLoading}>
          Select All Pending
        </button>

        {actionMessage && (
          <span
            style={{
              fontSize: 13,
              color: actionMessage.startsWith("Error") ? "#ef4444" : "#22c55e",
              marginLeft: "auto",
            }}
          >
            {actionMessage}
          </span>
        )}
      </div>

      {/* Loading / Error */}
      {loading && <div style={{ padding: 40, textAlign: "center", color: "#64748b" }}>Loading...</div>}
      {error && <div style={{ padding: 20, color: "#ef4444" }}>{error}</div>}

      {/* Agency tables */}
      {!loading &&
        agencies.map((agency) => {
          const isExpanded = expandedAgencies.has(agency.agencyId);
          const allApproved = canCreatePayout(agency);

          return (
            <div
              key={agency.agencyId}
              style={{
                background: "rgba(255,255,255,0.02)",
                border: "1px solid rgba(255,255,255,0.06)",
                borderRadius: 12,
                marginBottom: 16,
                overflow: "hidden",
              }}
            >
              {/* Agency header row */}
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  padding: "12px 16px",
                  cursor: "pointer",
                  borderBottom: isExpanded ? "1px solid rgba(255,255,255,0.06)" : "none",
                }}
                onClick={() => toggleAgencyExpand(agency.agencyId)}
              >
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <span style={{ color: "#64748b", fontSize: 12, width: 16, textAlign: "center" }}>
                    {isExpanded ? "\u25BC" : "\u25B6"}
                  </span>
                  <span style={{ fontWeight: 600, fontSize: 14 }}>{agency.agencyName}</span>
                  <TierBadge tier={agency.agencyTier} />
                  <span style={{ color: "#64748b", fontSize: 12, marginLeft: 8 }}>
                    {agency.entries.length} entries
                  </span>
                </div>
                <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
                  <span style={{ fontSize: 13, color: "#94a3b8" }}>
                    {formatCents(
                      agency.entries.reduce((sum, e) => sum + e.agencyShare + e.twoTierShare, 0)
                    )}
                  </span>
                  {allApproved && (
                    <button
                      style={{ ...btnPrimary, fontSize: 12, padding: "6px 12px" }}
                      onClick={(e) => {
                        e.stopPropagation();
                        setPayoutAgencyId(agency.agencyId);
                        setShowPayoutModal(true);
                      }}
                      disabled={actionLoading}
                    >
                      Create Payout
                    </button>
                  )}
                </div>
              </div>

              {/* Entries table */}
              {isExpanded && (
                <table style={tableStyle}>
                  <thead>
                    <tr>
                      <th style={{ ...thStyle, width: 40 }}></th>
                      <th style={thStyle}>Merchant</th>
                      <th style={{ ...thStyle, textAlign: "right" }}>Volume</th>
                      <th style={{ ...thStyle, textAlign: "right" }}>NMI Residual</th>
                      <th style={{ ...thStyle, textAlign: "right" }}>Agency Share</th>
                      <th style={{ ...thStyle, textAlign: "right" }}>2-Tier Share</th>
                      <th style={thStyle}>Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {agency.entries.map((entry) => (
                      <tr
                        key={entry.id}
                        style={{
                          background: selectedIds.has(entry.id)
                            ? "rgba(99,102,241,0.08)"
                            : "transparent",
                        }}
                      >
                        <td style={tdStyle}>
                          <input
                            type="checkbox"
                            checked={selectedIds.has(entry.id)}
                            onChange={() => toggleSelect(entry.id)}
                            disabled={entry.status !== "PENDING"}
                            style={{ cursor: entry.status === "PENDING" ? "pointer" : "not-allowed" }}
                          />
                        </td>
                        <td style={tdStyle}>{entry.merchantName}</td>
                        <td style={{ ...tdStyle, textAlign: "right", fontFamily: "monospace" }}>
                          {formatCents(entry.merchantVolume)}
                        </td>
                        <td style={{ ...tdStyle, textAlign: "right", fontFamily: "monospace" }}>
                          {formatCents(entry.nmiResidualEarned)}
                        </td>
                        <td style={{ ...tdStyle, textAlign: "right", fontFamily: "monospace", color: "#22c55e" }}>
                          {formatCents(entry.agencyShare)}
                        </td>
                        <td style={{ ...tdStyle, textAlign: "right", fontFamily: "monospace", color: "#a78bfa" }}>
                          {formatCents(entry.twoTierShare)}
                        </td>
                        <td style={tdStyle}>
                          <StatusBadge status={entry.status} />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          );
        })}

      {!loading && agencies.length === 0 && !error && (
        <div style={{ padding: 40, textAlign: "center", color: "#64748b" }}>
          No residual entries found for this period.
        </div>
      )}

      {/* ─── Approve All Modal ──────────────────────────────────────── */}
      <Modal
        open={showApproveAllModal}
        title="Approve All Pending Entries"
        onClose={() => setShowApproveAllModal(false)}
      >
        <p style={{ color: "#94a3b8", fontSize: 14, margin: "0 0 20px" }}>
          Are you sure you want to approve all <strong>{pendingCount}</strong> pending
          residual entries for this period? This action cannot be undone.
        </p>
        <div style={{ display: "flex", gap: 12, justifyContent: "flex-end" }}>
          <button
            style={btnOutline}
            onClick={() => setShowApproveAllModal(false)}
            disabled={actionLoading}
          >
            Cancel
          </button>
          <button style={btnSuccess} onClick={handleApproveAllPending} disabled={actionLoading}>
            {actionLoading ? "Approving..." : `Approve All (${pendingCount})`}
          </button>
        </div>
      </Modal>

      {/* ─── Hold Modal ─────────────────────────────────────────────── */}
      <Modal
        open={showHoldModal}
        title="Hold Selected Entries"
        onClose={() => {
          setShowHoldModal(false);
          setHoldReason("");
        }}
      >
        <p style={{ color: "#94a3b8", fontSize: 14, margin: "0 0 12px" }}>
          Holding <strong>{selectedIds.size}</strong> entries. Provide a reason:
        </p>
        <textarea
          value={holdReason}
          onChange={(e) => setHoldReason(e.target.value)}
          placeholder="e.g., chargeback review"
          style={{
            width: "100%",
            minHeight: 80,
            background: "rgba(255,255,255,0.05)",
            border: "1px solid rgba(255,255,255,0.1)",
            borderRadius: 8,
            padding: 12,
            color: "#e2e8f0",
            fontSize: 13,
            resize: "vertical",
            boxSizing: "border-box",
          }}
        />
        <div style={{ display: "flex", gap: 12, justifyContent: "flex-end", marginTop: 16 }}>
          <button
            style={btnOutline}
            onClick={() => {
              setShowHoldModal(false);
              setHoldReason("");
            }}
            disabled={actionLoading}
          >
            Cancel
          </button>
          <button
            style={{ ...btnDanger, opacity: !holdReason.trim() ? 0.5 : 1 }}
            onClick={handleHoldSelected}
            disabled={actionLoading || !holdReason.trim()}
          >
            {actionLoading ? "Holding..." : "Hold Entries"}
          </button>
        </div>
      </Modal>

      {/* ─── Payout Modal ───────────────────────────────────────────── */}
      <Modal
        open={showPayoutModal}
        title="Create Payout"
        onClose={() => {
          setShowPayoutModal(false);
          setPayoutAgencyId(null);
          setPayoutReference("");
        }}
      >
        {payoutAgencyId && (() => {
          const agency = agencies.find((a) => a.agencyId === payoutAgencyId);
          if (!agency) return null;
          const total = agency.entries.reduce((s, e) => s + e.agencyShare + e.twoTierShare, 0);
          return (
            <>
              <p style={{ color: "#94a3b8", fontSize: 14, margin: "0 0 16px" }}>
                Creating payout for <strong>{agency.agencyName}</strong>
                <br />
                Total: <strong style={{ color: "#22c55e" }}>{formatCents(total)}</strong>
                {" "}({agency.entries.length} entries)
              </p>
              <div style={{ marginBottom: 12 }}>
                <label style={{ display: "block", fontSize: 12, color: "#64748b", marginBottom: 4 }}>
                  Payment Method
                </label>
                <select
                  value={payoutMethod}
                  onChange={(e) => setPayoutMethod(e.target.value as "manual" | "ach")}
                  style={{
                    width: "100%",
                    background: "rgba(255,255,255,0.05)",
                    border: "1px solid rgba(255,255,255,0.1)",
                    borderRadius: 8,
                    padding: "8px 12px",
                    color: "#e2e8f0",
                    fontSize: 13,
                  }}
                >
                  <option value="ach">ACH Transfer</option>
                  <option value="manual">Manual / Check</option>
                </select>
              </div>
              <div style={{ marginBottom: 16 }}>
                <label style={{ display: "block", fontSize: 12, color: "#64748b", marginBottom: 4 }}>
                  Reference (optional)
                </label>
                <input
                  type="text"
                  value={payoutReference}
                  onChange={(e) => setPayoutReference(e.target.value)}
                  placeholder="e.g., ACH-2026-03-001"
                  style={{
                    width: "100%",
                    background: "rgba(255,255,255,0.05)",
                    border: "1px solid rgba(255,255,255,0.1)",
                    borderRadius: 8,
                    padding: "8px 12px",
                    color: "#e2e8f0",
                    fontSize: 13,
                    boxSizing: "border-box",
                  }}
                />
              </div>
              <div style={{ display: "flex", gap: 12, justifyContent: "flex-end" }}>
                <button
                  style={btnOutline}
                  onClick={() => {
                    setShowPayoutModal(false);
                    setPayoutAgencyId(null);
                    setPayoutReference("");
                  }}
                  disabled={actionLoading}
                >
                  Cancel
                </button>
                <button
                  style={btnPrimary}
                  onClick={handleCreatePayout}
                  disabled={actionLoading}
                >
                  {actionLoading ? "Creating..." : `Create Payout (${formatCents(total)})`}
                </button>
              </div>
            </>
          );
        })()}
      </Modal>
    </div>
  );
}
