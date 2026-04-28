/**
 * ChargebackGauge — visual indicator of portfolio chargeback ratio.
 *
 * Industry standard thresholds:
 *   < 0.65% green (healthy)
 *   0.65% - 0.9% amber (warning)
 *   >= 0.9% red (critical — Visa/MC threshold breach)
 */

import React from "react";
import { formatPercent } from "../lib/format.js";

export interface ChargebackGaugeProps {
  ratio: number; // decimal, e.g. 0.004
}

function getGaugeColor(ratio: number): string {
  if (ratio < 0.0065) return "#22c55e";   // green — healthy
  if (ratio < 0.009) return "#f59e0b";    // amber — warning
  return "#ef4444";                         // red — critical
}

function getGaugeLabel(ratio: number): string {
  if (ratio < 0.0065) return "Healthy";
  if (ratio < 0.009) return "Warning";
  return "Critical";
}

export function ChargebackGauge({ ratio }: ChargebackGaugeProps) {
  const color = getGaugeColor(ratio);
  const label = getGaugeLabel(ratio);
  // Gauge fill: 0 - 1.5% range mapped to 0-100%
  const fillPct = Math.min((ratio / 0.015) * 100, 100);

  return (
    <div
      style={{
        background: "rgba(255,255,255,0.05)",
        borderRadius: 12,
        padding: "20px 24px",
      }}
      data-testid="chargeback-gauge"
    >
      <div style={{ fontSize: 13, color: "#9ca3af", marginBottom: 8 }}>
        Chargeback Ratio (30d)
      </div>
      <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
        <span style={{ fontSize: 28, fontWeight: 700, color }}>
          {formatPercent(ratio)}
        </span>
        <span style={{ fontSize: 13, color }}>{label}</span>
      </div>
      <div
        style={{
          marginTop: 12,
          background: "#374151",
          borderRadius: 4,
          height: 8,
          overflow: "hidden",
        }}
      >
        <div
          style={{
            width: `${fillPct}%`,
            height: "100%",
            background: color,
            borderRadius: 4,
            transition: "width 0.5s ease",
          }}
          data-testid="gauge-fill"
        />
      </div>
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          fontSize: 10,
          color: "#6b7280",
          marginTop: 4,
        }}
      >
        <span>0%</span>
        <span>0.65%</span>
        <span>0.9%</span>
        <span>1.5%</span>
      </div>
    </div>
  );
}
