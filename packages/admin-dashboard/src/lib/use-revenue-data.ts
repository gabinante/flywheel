/**
 * React hook for fetching revenue analytics data
 */

import { useState, useEffect, useCallback } from "react";
import type { RevenuePeriod, RevenueAnalyticsData } from "./types.js";
import { fetchRevenueAnalytics } from "./api.js";

export interface UseRevenueDataResult {
  data: RevenueAnalyticsData | null;
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

export function useRevenueData(
  period: RevenuePeriod,
  startDate?: string,
  endDate?: string
): UseRevenueDataResult {
  const [data, setData] = useState<RevenueAnalyticsData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await fetchRevenueAnalytics({
        period,
        startDate,
        endDate,
      });
      setData(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch revenue data");
    } finally {
      setLoading(false);
    }
  }, [period, startDate, endDate]);

  useEffect(() => {
    void fetchData();
  }, [fetchData]);

  return { data, loading, error, refetch: fetchData };
}
