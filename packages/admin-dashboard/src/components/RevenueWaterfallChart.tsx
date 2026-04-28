/**
 * Revenue Waterfall Chart — stacked bar chart showing
 * NMI residual → agency share → retained per month.
 */

import React from "react";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";
import type { MonthlyBreakdown } from "../lib/types.js";

export interface RevenueWaterfallChartProps {
  data: MonthlyBreakdown[];
}

export function RevenueWaterfallChart({ data }: RevenueWaterfallChartProps) {
  const chartData = data.map((d) => ({
    month: d.month,
    "Agency Payout": d.agencyPayout / 100,
    Retained: d.retained / 100,
  }));

  return (
    <div
      style={{
        background: "rgba(255,255,255,0.05)",
        borderRadius: 12,
        padding: 24,
      }}
      data-testid="waterfall-chart"
    >
      <h3 style={{ color: "#f9fafb", fontSize: 16, marginBottom: 16 }}>
        Revenue Waterfall
      </h3>
      <ResponsiveContainer width="100%" height={300}>
        <BarChart data={chartData}>
          <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
          <XAxis dataKey="month" stroke="#9ca3af" />
          <YAxis
            stroke="#9ca3af"
            tickFormatter={(v: number) => `$${v.toLocaleString()}`}
          />
          <Tooltip
            contentStyle={{
              background: "#1f2937",
              border: "1px solid #374151",
              borderRadius: 8,
            }}
            formatter={(value: number) => `$${value.toLocaleString()}`}
          />
          <Legend />
          <Bar
            dataKey="Agency Payout"
            stackId="revenue"
            fill="#f59e0b"
            radius={[0, 0, 0, 0]}
          />
          <Bar
            dataKey="Retained"
            stackId="revenue"
            fill="#22c55e"
            radius={[4, 4, 0, 0]}
          />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}
