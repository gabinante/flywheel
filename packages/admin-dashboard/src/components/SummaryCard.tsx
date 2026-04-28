/**
 * SummaryCard — displays a metric with optional month-over-month delta.
 */

import React from "react";
import { formatDelta } from "../lib/format.js";

export interface SummaryCardProps {
  title: string;
  value: string;
  delta?: number | null;
  subtitle?: string;
}

export function SummaryCard({ title, value, delta, subtitle }: SummaryCardProps) {
  const deltaColor =
    delta === null || delta === undefined
      ? "inherit"
      : delta >= 0
        ? "#22c55e"
        : "#ef4444";

  return (
    <div
      style={{
        background: "rgba(255,255,255,0.05)",
        borderRadius: 12,
        padding: "20px 24px",
        minWidth: 200,
        flex: 1,
      }}
      data-testid="summary-card"
    >
      <div style={{ fontSize: 13, color: "#9ca3af", marginBottom: 4 }}>
        {title}
      </div>
      <div style={{ fontSize: 28, fontWeight: 700, color: "#f9fafb" }}>
        {value}
      </div>
      {delta !== undefined && (
        <div
          style={{ fontSize: 13, color: deltaColor, marginTop: 4 }}
          data-testid="delta"
        >
          {formatDelta(delta ?? null)} MoM
        </div>
      )}
      {subtitle && (
        <div style={{ fontSize: 12, color: "#6b7280", marginTop: 2 }}>
          {subtitle}
        </div>
      )}
    </div>
  );
}
