import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";

interface MerchantDetail {
  id: string;
  name: string;
  email: string;
  phone: string | null;
  nmiMerchantId: string | null;
  ghlLocationId: string | null;
  status: string;
  chargebackRiskLevel: string | null;
  chargebackRiskUpdatedAt: string | null;
  agencyId: string | null;
  createdAt: string;
}

interface ChargebackRiskRow {
  merchantId: string;
  merchantName: string;
  transactionCount: number;
  chargebackCount: number;
  ratio: number;
  riskLevel: string | null;
}

async function fetchMerchant(id: string): Promise<MerchantDetail> {
  const res = await fetch(`/api/v1/admin/agencies`, {
    headers: { Authorization: `Bearer ${localStorage.getItem("admin_token") ?? ""}` },
  });
  // Merchants endpoint may vary — try the risk endpoint for now
  if (!res.ok) throw new Error("Failed to fetch merchant");
  const data = await res.json();
  // Find merchant in agency list or return placeholder
  return data as MerchantDetail;
}

async function fetchChargebackRisk(merchantId: string): Promise<ChargebackRiskRow | null> {
  const res = await fetch(`/api/v1/admin/chargebacks/risk`, {
    headers: { Authorization: `Bearer ${localStorage.getItem("admin_token") ?? ""}` },
  });
  if (!res.ok) return null;
  const list: ChargebackRiskRow[] = await res.json();
  return list.find((r) => r.merchantId === merchantId) ?? null;
}

/**
 * Color-coded chargeback ratio badge.
 * green < 0.5%, yellow 0.5–0.7%, orange 0.7–1.0%, red > 1.0%
 */
function ChargebackRatioBadge({
  riskLevel,
  ratio,
}: {
  riskLevel: string | null;
  ratio: number;
}) {
  const pct = (ratio * 100).toFixed(2) + "%";

  if (riskLevel === "HIGH" || ratio > 0.01) {
    return (
      <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-sm font-semibold bg-red-100 text-red-800">
        🔴 {pct} — HIGH
      </span>
    );
  }
  if (riskLevel === "CRITICAL" || ratio > 0.007) {
    return (
      <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-sm font-semibold bg-orange-100 text-orange-800">
        🟠 {pct} — CRITICAL
      </span>
    );
  }
  if (riskLevel === "WARNING" || ratio > 0.005) {
    return (
      <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-sm font-semibold bg-yellow-100 text-yellow-800">
        🟡 {pct} — WARNING
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-sm font-semibold bg-green-100 text-green-800">
      🟢 {pct} — OK
    </span>
  );
}

export function MerchantDetailPage() {
  const { id } = useParams<{ id: string }>();

  const { data: cbRisk, isLoading: cbLoading } = useQuery<ChargebackRiskRow | null>({
    queryKey: ["chargeback-risk", id],
    queryFn: () => fetchChargebackRisk(id!),
    enabled: !!id,
  });

  if (cbLoading) {
    return (
      <div className="flex items-center justify-center h-40">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600" />
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-gray-900">
          {cbRisk?.merchantName ?? `Merchant ${id}`}
        </h1>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Chargeback Risk Card */}
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <h2 className="font-semibold text-gray-900 mb-4">Chargeback Risk (30d)</h2>
          {cbRisk ? (
            <dl className="space-y-3 text-sm">
              <div className="flex justify-between items-center">
                <dt className="text-gray-500">CB Ratio</dt>
                <dd>
                  <ChargebackRatioBadge
                    riskLevel={cbRisk.riskLevel}
                    ratio={cbRisk.ratio}
                  />
                </dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-gray-500">Transactions (30d)</dt>
                <dd className="font-medium">{cbRisk.transactionCount.toLocaleString()}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-gray-500">Chargebacks (30d)</dt>
                <dd className="font-medium">{cbRisk.chargebackCount}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-gray-500">Raw Ratio</dt>
                <dd className="font-mono text-xs">
                  {(cbRisk.ratio * 100).toFixed(4)}%
                </dd>
              </div>
            </dl>
          ) : (
            <p className="text-sm text-gray-400">
              No chargeback data available for this merchant.
            </p>
          )}
        </div>

        {/* Threshold Guide */}
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <h2 className="font-semibold text-gray-900 mb-4">Risk Thresholds</h2>
          <dl className="space-y-3 text-sm">
            <div className="flex justify-between items-center">
              <dt className="text-gray-500">OK</dt>
              <dd>
                <span className="inline-block px-2 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-700">
                  &lt; 0.5%
                </span>
              </dd>
            </div>
            <div className="flex justify-between items-center">
              <dt className="text-gray-500">WARNING</dt>
              <dd>
                <span className="inline-block px-2 py-0.5 rounded-full text-xs font-medium bg-yellow-100 text-yellow-700">
                  0.5% – 0.7%
                </span>
              </dd>
            </div>
            <div className="flex justify-between items-center">
              <dt className="text-gray-500">CRITICAL</dt>
              <dd>
                <span className="inline-block px-2 py-0.5 rounded-full text-xs font-medium bg-orange-100 text-orange-700">
                  0.7% – 1.0%
                </span>
              </dd>
            </div>
            <div className="flex justify-between items-center">
              <dt className="text-gray-500">HIGH (auto-flag)</dt>
              <dd>
                <span className="inline-block px-2 py-0.5 rounded-full text-xs font-medium bg-red-100 text-red-700">
                  &gt; 1.0%
                </span>
              </dd>
            </div>
          </dl>
        </div>
      </div>
    </div>
  );
}
