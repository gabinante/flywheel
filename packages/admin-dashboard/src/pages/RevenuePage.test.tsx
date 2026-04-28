/**
 * RevenuePage — Unit Tests
 *
 * Tests rendering of revenue dashboard with seeded data.
 */

import { describe, it, expect } from "vitest";
import React from "react";
import { render, screen, within } from "@testing-library/react";
import { RevenuePage } from "./RevenuePage.js";
import type { RevenueAnalyticsData } from "../lib/types.js";

const MOCK_DATA: RevenueAnalyticsData = {
  totalPlatformVolume: 15000000,
  nmiResidualReceived: 45000,
  totalAgencyPayouts: 28000,
  netRetainedRevenue: 17000,
  activeMerchants: 142,
  activeAgencies: 23,
  newMerchantsThisMonth: 12,
  newAgenciesThisMonth: 3,
  chargebackRatio: 0.004,
  monthlyBreakdown: [
    {
      month: "2026-01",
      volume: 4000000,
      nmiResidual: 12000,
      agencyPayout: 8000,
      retained: 4000,
    },
    {
      month: "2026-02",
      volume: 5000000,
      nmiResidual: 15000,
      agencyPayout: 10000,
      retained: 5000,
    },
    {
      month: "2026-03",
      volume: 6000000,
      nmiResidual: 18000,
      agencyPayout: 10000,
      retained: 8000,
    },
  ],
};

describe("RevenuePage", () => {
  it("renders the revenue page with title", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    expect(screen.getByText("Revenue Dashboard")).toBeInTheDocument();
  });

  it("renders 4 summary cards", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    const cards = screen.getAllByTestId("summary-card");
    expect(cards.length).toBeGreaterThanOrEqual(4);
  });

  it("shows platform volume in summary cards", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    const cardsContainer = screen.getByTestId("summary-cards");
    expect(within(cardsContainer).getByText("Platform Volume")).toBeInTheDocument();
  });

  it("shows NMI residual in summary cards", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    const cardsContainer = screen.getByTestId("summary-cards");
    expect(within(cardsContainer).getByText("NMI Residual")).toBeInTheDocument();
  });

  it("shows agency payouts in summary cards", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    const cardsContainer = screen.getByTestId("summary-cards");
    expect(within(cardsContainer).getByText("Agency Payouts")).toBeInTheDocument();
  });

  it("shows net retained in summary cards", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    const cardsContainer = screen.getByTestId("summary-cards");
    expect(within(cardsContainer).getByText("Net Retained")).toBeInTheDocument();
  });

  it("displays revenue waterfall chart", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    expect(screen.getByTestId("waterfall-chart")).toBeInTheDocument();
  });

  it("displays monthly trend chart", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    expect(screen.getByTestId("trend-chart")).toBeInTheDocument();
  });

  it("shows active merchants count", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    // Use getAllByText in case chart legends duplicate text
    expect(screen.getAllByText("Active Merchants").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("142").length).toBeGreaterThanOrEqual(1);
  });

  it("shows active agencies count", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    expect(screen.getAllByText("Active Agencies").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("23").length).toBeGreaterThanOrEqual(1);
  });

  it("displays chargeback gauge", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    expect(screen.getByTestId("chargeback-gauge")).toBeInTheDocument();
    expect(screen.getAllByText("0.40%").length).toBeGreaterThanOrEqual(1);
  });

  it("shows new merchants/agencies this month", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    expect(screen.getAllByText("12 new this month").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("3 new this month").length).toBeGreaterThanOrEqual(1);
  });

  it("renders month-over-month deltas", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    const deltas = screen.getAllByTestId("delta");
    expect(deltas.length).toBeGreaterThanOrEqual(4);
  });

  it("has period selector", () => {
    render(<RevenuePage initialData={MOCK_DATA} />);
    expect(screen.getByTestId("period-select")).toBeInTheDocument();
  });
});
