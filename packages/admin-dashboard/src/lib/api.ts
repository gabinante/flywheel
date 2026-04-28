/**
 * Revenue Analytics API client
 */

import type { RevenuePeriod, RevenueAnalyticsData } from "./types.js";

const API_BASE = "/api/v1/admin";

export interface FetchRevenueParams {
  period: RevenuePeriod;
  startDate?: string;
  endDate?: string;
}

export async function fetchRevenueAnalytics(
  params: FetchRevenueParams
): Promise<RevenueAnalyticsData> {
  const searchParams = new URLSearchParams();
  searchParams.set("period", params.period);
  if (params.startDate) searchParams.set("startDate", params.startDate);
  if (params.endDate) searchParams.set("endDate", params.endDate);

  const response = await fetch(
    `${API_BASE}/analytics/revenue?${searchParams.toString()}`
  );

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: "Unknown error" }));
    throw new Error(
      (error as { error?: string }).error || `HTTP ${response.status}`
    );
  }

  return response.json() as Promise<RevenueAnalyticsData>;
}
