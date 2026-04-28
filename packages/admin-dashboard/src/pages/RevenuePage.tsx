/**
 * RevenuePage — Platform-level revenue dashboard
 *
 * Shows:
 * - Summary cards: platform volume, NMI residual, agency payouts, net retained
 * - Revenue waterfall chart (stacked bar)
 * - Monthly trend line chart
 * - Active merchants/agencies counts
 * - Portfolio chargeback ratio gauge
 */

import React, { useState } from "react";
import type { RevenuePeriod, RevenueAnalyticsData } from "../lib/types.js";
import {
  formatCents,
  formatCentsCompact,
  calcMoMDelta,
} from "../lib/format.js";
import { useRevenueData } from "../lib/use-revenue-data.js";
import { SummaryCard } from "../components/SummaryCard.js";
import { RevenueWaterfallChart } from "../components/RevenueWaterfallChart.js";
import { MonthlyTrendChart } from "../components/MonthlyTrendChart.js";
import { ChargebackGauge } from "../components/ChargebackGauge.js";

export interface RevenuePageProps {
  /** Allow injecting data for testing (bypasses fetch) */
  initialData?: RevenueAnalyticsData;
}

/**
 * Calculate month-over-month deltas from monthly breakdown.
 * Compares the last month to the second-to-last month.
 */
function getMoMDeltas(data: RevenueAnalyticsData) {
  const breakdown = data.monthlyBreakdown;
  if (breakdown.length < 2) {
    return {
      volumeDelta: null,
      residualDelta: null,
      payoutDelta: null,
      retainedDelta: null,
    };
  }
  const current = breakdown[breakdown.length - 1];
  const previous = breakdown[breakdown.length - 2];

  return {
    volumeDelta: calcMoMDelta(current.volume, previous.volume),
    residualDelta: calcMoMDelta(current.nmiResidual, previous.nmiResidual),
    payoutDelta: calcMoMDelta(current.agencyPayout, previous.agencyPayout),
    retainedDelta: calcMoMDelta(current.retained, previous.retained),
  };
}

const PERIOD_OPTIONS: { value: RevenuePeriod; label: string }[] = [
  { value: "trailing_30d", label: "Last 30 Days" },
  { value: "trailing_12m", label: "Last 12 Months" },
];

export function RevenuePage({ initialData }: RevenuePageProps) {
  const [period, setPeriod] = useState<RevenuePeriod>("trailing_12m");

  const { data: fetchedData, loading, error } = useRevenueData(period);
  const data = initialData ?? fetchedData;

  return (
    <div
      style={{
        padding: 24,
        maxWidth: 1200,
        margin: "0 auto",
        color: "#f9fafb",
        fontFamily: "system-ui, -apple-system, sans-serif",
      }}
      data-testid="revenue-page"
    >
      {/* Header */}
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          marginBottom: 24,
        }}
      >
        <h1 style={{ fontSize: 24, fontWeight: 700 }}>Revenue Dashboard</h1>
        <select
          value={period}
          onChange={(e) => setPeriod(e.target.value as RevenuePeriod)}
          style={{
            background: "#1f2937",
            color: "#f9fafb",
            border: "1px solid #374151",
            borderRadius: 8,
            padding: "8px 12px",
            fontSize: 14,
          }}
          data-testid="period-select"
        >
          {PERIOD_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>

      {/* Loading / Error states */}
      {loading && !data && (
        <div style={{ textAlign: "center", padding: 48, color: "#9ca3af" }}>
          Loading revenue data...
        </div>
      )}

      {error && (
        <div
          style={{
            background: "rgba(239,68,68,0.1)",
            border: "1px solid #ef4444",
            borderRadius: 8,
            padding: 16,
            marginBottom: 24,
            color: "#ef4444",
          }}
          data-testid="error-message"
        >
          {error}
        </div>
      )}

      {data && (
        <>
          {/* Summary Cards */}
          {renderSummaryCards(data)}

          {/* Charts Row */}
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "1fr 1fr",
              gap: 16,
              marginBottom: 24,
            }}
          >
            <RevenueWaterfallChart data={data.monthlyBreakdown} />
            <MonthlyTrendChart data={data.monthlyBreakdown} />
          </div>

          {/* Bottom Row: Counts + Chargeback */}
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "1fr 1fr 1fr",
              gap: 16,
            }}
          >
            <SummaryCard
              title="Active Merchants"
              value={data.activeMerchants.toLocaleString()}
              subtitle={`${data.newMerchantsThisMonth} new this month`}
            />
            <SummaryCard
              title="Active Agencies"
              value={data.activeAgencies.toLocaleString()}
              subtitle={`${data.newAgenciesThisMonth} new this month`}
            />
            <ChargebackGauge ratio={data.chargebackRatio} />
          </div>
        </>
      )}
    </div>
  );
}

function renderSummaryCards(data: RevenueAnalyticsData) {
  const deltas = getMoMDeltas(data);

  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(4, 1fr)",
        gap: 16,
        marginBottom: 24,
      }}
      data-testid="summary-cards"
    >
      <SummaryCard
        title="Platform Volume"
        value={formatCentsCompact(data.totalPlatformVolume)}
        delta={deltas.volumeDelta}
      />
      <SummaryCard
        title="NMI Residual"
        value={formatCents(data.nmiResidualReceived)}
        delta={deltas.residualDelta}
      />
      <SummaryCard
        title="Agency Payouts"
        value={formatCents(data.totalAgencyPayouts)}
        delta={deltas.payoutDelta}
      />
      <SummaryCard
        title="Net Retained"
        value={formatCents(data.netRetainedRevenue)}
        delta={deltas.retainedDelta}
      />
    </div>
  );
}
