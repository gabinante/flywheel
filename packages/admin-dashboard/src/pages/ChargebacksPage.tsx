import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";

interface ChargebackRisk {
  merchantId: string;
  merchantName: string;
  transactionCount: number;
  chargebackCount: number;
  ratio: number;
  riskLevel: string | null;
  riskUpdatedAt: string | null;
}

async function fetchChargebackRisk(riskLevel?: string): Promise<ChargebackRisk[]> {
  const params = riskLevel ? `?riskLevel=${riskLevel}` : "";
  const res = await fetch(`/api/v1/admin/chargebacks/risk${params}`, {
    headers: { Authorization: `Bearer ${localStorage.getItem("admin_token") ?? ""}` },
  });
  if (!res.ok) throw new Error("Failed to fetch chargeback risk data");
  return res.json();
}

function RiskBadge({ riskLevel, ratio }: { riskLevel: string | null; ratio: number }) {
  const pct = (ratio * 100).toFixed(2) + "%";
  if (riskLevel === "HIGH" || ratio > 0.01) {
    return (
      <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-red-100 text-red-700">
        {pct} HIGH
      </span>
    );
  }
  if (riskLevel === "CRITICAL" || ratio > 0.007) {
    return (
      <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-orange-100 text-orange-700">
        {pct} CRITICAL
      </span>
    );
  }
  if (riskLevel === "WARNING" || ratio > 0.005) {
    return (
      <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-yellow-100 text-yellow-700">
        {pct} WARNING
      </span>
    );
  }
  return (
    <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-green-100 text-green-700">
      {pct} OK
    </span>
  );
}

export function ChargebacksPage() {
  const [riskFilter, setRiskFilter] = useState<string>("");

  const { data, isLoading, error } = useQuery<ChargebackRisk[]>({
    queryKey: ["chargeback-risk", riskFilter],
    queryFn: () => fetchChargebackRisk(riskFilter || undefined),
  });

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Chargeback Risk Monitor</h1>
        <p className="text-sm text-gray-500">30-day rolling window · Updated daily 06:00 UTC</p>
      </div>

      {/* Filter bar */}
      <div className="flex items-center gap-3 mb-4">
        <label className="text-sm font-medium text-gray-700">Filter by risk level:</label>
        <select
          value={riskFilter}
          onChange={(e) => setRiskFilter(e.target.value)}
          className="px-3 py-1.5 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          <option value="">All merchants</option>
          <option value="WARNING">WARNING (&gt;0.5%)</option>
          <option value="CRITICAL">CRITICAL (&gt;0.7%)</option>
          <option value="HIGH">HIGH (&gt;1.0%)</option>
        </select>
      </div>

      {/* Summary cards */}
      {data && (
        <div className="grid grid-cols-3 gap-4 mb-6">
          {(["WARNING", "CRITICAL", "HIGH"] as const).map((level) => {
            const count = data.filter((r) => r.riskLevel === level).length;
            const colors = {
              WARNING: "bg-yellow-50 border-yellow-200 text-yellow-800",
              CRITICAL: "bg-orange-50 border-orange-200 text-orange-800",
              HIGH: "bg-red-50 border-red-200 text-red-800",
            };
            return (
              <div
                key={level}
                className={`rounded-xl border p-4 cursor-pointer transition-colors ${colors[level]}`}
                onClick={() => setRiskFilter(riskFilter === level ? "" : level)}
              >
                <div className="text-2xl font-bold">{count}</div>
                <div className="text-sm font-medium mt-1">{level}</div>
              </div>
            );
          })}
        </div>
      )}

      <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="bg-gray-50 border-b border-gray-200">
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">
                Merchant
              </th>
              <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">
                Transactions (30d)
              </th>
              <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase">
                Chargebacks (30d)
              </th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">
                CB Ratio
              </th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">
                Action
              </th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <tr>
                <td colSpan={5} className="text-center py-8 text-gray-500">
                  Loading…
                </td>
              </tr>
            ) : error ? (
              <tr>
                <td colSpan={5} className="text-center py-8 text-red-500">
                  Failed to load chargeback risk data
                </td>
              </tr>
            ) : !data || data.length === 0 ? (
              <tr>
                <td colSpan={5} className="text-center py-8 text-gray-500">
                  No merchants match the selected filter
                </td>
              </tr>
            ) : (
              data.map((row) => (
                <tr
                  key={row.merchantId}
                  className="border-b border-gray-100 hover:bg-gray-50"
                >
                  <td className="px-4 py-3 text-sm font-medium text-gray-900">
                    {row.merchantName}
                  </td>
                  <td className="px-4 py-3 text-sm text-right text-gray-600">
                    {row.transactionCount.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-sm text-right text-gray-600">
                    {row.chargebackCount}
                  </td>
                  <td className="px-4 py-3">
                    <RiskBadge riskLevel={row.riskLevel} ratio={row.ratio} />
                  </td>
                  <td className="px-4 py-3 text-sm">
                    <Link
                      to={`/merchants/${row.merchantId}`}
                      className="text-blue-600 hover:text-blue-700 font-medium"
                    >
                      View
                    </Link>
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
