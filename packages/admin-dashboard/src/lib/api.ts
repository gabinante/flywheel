/**
 * API client for the admin dashboard.
 * All agency management endpoints go through /api/v1/admin/.
 */

const API_BASE = '/api/v1/admin';

// ─── Types ─────────────────────────────────────────────────────────────

export type AgencyTier = 'TIER_1' | 'TIER_2' | 'TIER_3';
export type AgencyStatus = 'ACTIVE' | 'SUSPENDED' | 'CHURNED';

export interface AgencySummary {
  id: string;
  name: string;
  contactEmail: string;
  tier: AgencyTier;
  status: AgencyStatus;
  merchantCount: number;
  portfolioVolume: number;
  monthlyResidual: number;
  referredByAgency: { id: string; name: string } | null;
  createdAt: string;
}

export interface MerchantInfo {
  id: string;
  name: string;
  status: string;
  createdAt: string;
  volumeThisMonth: number;
}

export interface ReferredAgency {
  id: string;
  name: string;
  contactEmail: string;
  tier: AgencyTier;
  status: AgencyStatus;
  merchantCount: number;
  portfolioVolume: number;
  createdAt: string;
}

export interface ResidualMonth {
  month: string;
  agencyShare: number;
  twoTierShare: number;
  volume: number;
}

export interface PayoutRecord {
  id: string;
  periodStart: string;
  totalAmount: number;
  directShare: number;
  twoTierShare: number;
  method: string;
  status: string;
  paidAt: string | null;
}

export interface AgencyDetail {
  id: string;
  name: string;
  contactEmail: string;
  contactPhone: string | null;
  referralCode: string;
  tier: AgencyTier;
  tierOverride: boolean;
  status: AgencyStatus;
  payoutEmail: string | null;
  payoutBankLast4: string | null;
  taxIdOnFile: boolean;
  referredByAgency: { id: string; name: string } | null;
  createdAt: string;
  updatedAt: string;
  merchants: MerchantInfo[];
  referredAgencies: ReferredAgency[];
  residualHistory: ResidualMonth[];
  payouts: PayoutRecord[];
}

export interface PaginationInfo {
  page: number;
  limit: number;
  total: number;
  totalPages: number;
}

export interface ListAgenciesResponse {
  agencies: AgencySummary[];
  pagination: PaginationInfo;
}

export interface ListAgenciesParams {
  page?: number;
  limit?: number;
  tier?: AgencyTier;
  status?: AgencyStatus;
  search?: string;
  sort?: string;
  order?: 'asc' | 'desc';
}

export interface UpdateAgencyPayload {
  status?: 'ACTIVE' | 'SUSPENDED';
  tier?: AgencyTier;
  tierOverride?: boolean;
}

// ─── API Functions ────────────────────────────────────────────────────

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Request failed' }));
    throw new Error((error as { error?: string }).error ?? `HTTP ${response.status}`);
  }
  return response.json() as Promise<T>;
}

export async function listAgencies(
  params: ListAgenciesParams = {}
): Promise<ListAgenciesResponse> {
  const searchParams = new URLSearchParams();
  if (params.page) searchParams.set('page', String(params.page));
  if (params.limit) searchParams.set('limit', String(params.limit));
  if (params.tier) searchParams.set('tier', params.tier);
  if (params.status) searchParams.set('status', params.status);
  if (params.search) searchParams.set('search', params.search);
  if (params.sort) searchParams.set('sort', params.sort);
  if (params.order) searchParams.set('order', params.order);

  const qs = searchParams.toString();
  const url = `${API_BASE}/agencies${qs ? `?${qs}` : ''}`;
  const res = await fetch(url);
  return handleResponse<ListAgenciesResponse>(res);
}

export async function getAgency(id: string): Promise<AgencyDetail> {
  const res = await fetch(`${API_BASE}/agencies/${id}`);
  return handleResponse<AgencyDetail>(res);
}

export async function updateAgency(
  id: string,
  payload: UpdateAgencyPayload
): Promise<AgencyDetail> {
  const res = await fetch(`${API_BASE}/agencies/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  return handleResponse<AgencyDetail>(res);
}

// ─── Formatting helpers ──────────────────────────────────────────────

export function formatCents(cents: number): string {
  return `$${(cents / 100).toLocaleString('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

export function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

export const TIER_LABELS: Record<AgencyTier, string> = {
  TIER_1: 'Tier 1',
  TIER_2: 'Tier 2',
  TIER_3: 'Tier 3',
};

export const STATUS_COLORS: Record<AgencyStatus, string> = {
  ACTIVE: 'bg-green-500/20 text-green-400',
  SUSPENDED: 'bg-red-500/20 text-red-400',
  CHURNED: 'bg-gray-500/20 text-gray-400',
};

export const TIER_COLORS: Record<AgencyTier, string> = {
  TIER_1: 'bg-blue-500/20 text-blue-400',
  TIER_2: 'bg-purple-500/20 text-purple-400',
  TIER_3: 'bg-amber-500/20 text-amber-400',
};
