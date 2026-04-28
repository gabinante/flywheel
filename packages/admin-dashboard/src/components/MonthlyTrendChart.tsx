/**
 * Monthly Trend Chart — line chart showing volume + revenue
 * over trailing 12 months.
 */

import React from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";
import type { MonthlyBreakdown } from "../lib/types.js";

export interface MonthlyTrendChartProps {
  data: MonthlyBreakdown[];
}

export function MonthlyTrendChart({ data }: MonthlyTrendChartProps) {
  const chartData = data.map((d) => ({
    month: d.month,
    "Volume ($)": d.volume / 100,
    "NMI Residual ($)": d.nmiResidual / 100,
    "Retained ($)": d.retained / 100,
  }));

  return (
    <div
      style={{
        background: "rgba(255,255,255,0.05)",
        borderRadius: 12,
        padding: 24,
      }}
      data-testid="trend-chart"
    >
      <h3 style={{ color: "#f9fafb", fontSize: 16, marginBottom: 16 }}>
        Monthly Trends
      </h3>
      <ResponsiveContainer width="100%" height={300}>
        <LineChart data={chartData}>
          <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
          <XAxis dataKey="month" stroke="#9ca3af" />
          <YAxis
            stroke="#9ca3af"
            tickFormatter={(v: number) => `$${(v / 1000).toFixed(0)}k`}
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
          <Line
            type="monotone"
            dataKey="Volume ($)"
            stroke="#3b82f6"
            strokeWidth={2}
            dot={{ r: 4 }}
          />
          <Line
            type="monotone"
            dataKey="NMI Residual ($)"
            stroke="#a855f7"
            strokeWidth={2}
            dot={{ r: 4 }}
          />
          <Line
            type="monotone"
            dataKey="Retained ($)"
            stroke="#22c55e"
            strokeWidth={2}
            dot={{ r: 4 }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
