/**
 * Revenue Analytics API types
 */

export type RevenuePeriod = "trailing_30d" | "trailing_12m" | "custom";

export interface MonthlyBreakdown {
  month: string;
  volume: number;
  nmiResidual: number;
  agencyPayout: number;
  retained: number;
}

export interface RevenueAnalyticsData {
  totalPlatformVolume: number;
  nmiResidualReceived: number;
  totalAgencyPayouts: number;
  netRetainedRevenue: number;
  activeMerchants: number;
  activeAgencies: number;
  newMerchantsThisMonth: number;
  newAgenciesThisMonth: number;
  chargebackRatio: number;
  monthlyBreakdown: MonthlyBreakdown[];
}
