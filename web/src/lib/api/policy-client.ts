/**
 * Thin policy API client. Policy endpoints are registered outside the OpenAPI spec
 * (via PoliciesHandler.RegisterRoutes), so we use raw fetch with the auth token.
 * Once the spec is regenerated, these can migrate to the typed openapi-fetch client.
 */

// --- Types mirroring the Go policy package ---

export type PolicyCondition = {
  field: string
  operator: string
  value: unknown
  label?: string
}

export type PolicyRules = {
  auto_approve_conditions?: PolicyCondition[]
  review_conditions?: PolicyCondition[]
  block_conditions?: PolicyCondition[]
}

export type Policy = {
  id: string
  project_id: string
  name: string
  description: string
  rules: PolicyRules
  enabled: boolean
  min_sample: number
  created_at: string
  updated_at: string
}

export type PolicyDecision = {
  id: string
  policy_id: string
  ticket_id: string
  decision: 'auto_approved' | 'required_review' | 'blocked'
  outcome?: 'success' | 'rollback' | 'incident' | null
  reason: string
  decided_at: string
  outcome_at?: string | null
}

export type PolicyMetrics = {
  policy_id: string
  total_decisions: number
  auto_approval_rate: number
  rollback_rate: number
  incident_rate: number
  success_rate: number
  pending_outcomes: number
  sample_size_sufficient: boolean
}

export type PolicyChangeEvent = {
  id: string
  policy_id: string
  actor_id: string
  change_type: string
  prev_rules?: PolicyRules | null
  new_rules?: PolicyRules | null
  notes: string
  created_at: string
}

export type PolicyProposal = {
  id: string
  policy_id: string
  proposal_type: 'broaden' | 'review'
  suggestion: Record<string, unknown>
  statistics: Record<string, unknown>
  status: 'pending' | 'accepted' | 'rejected' | 'dismissed'
  created_at: string
  resolved_at?: string | null
  resolved_by?: string
}

export type PolicyHealth = {
  policy: Policy
  metrics: PolicyMetrics
  proposals: PolicyProposal[]
  history: PolicyChangeEvent[]
}

export type SimulationResult = {
  policy_id: string
  candidate_rules: PolicyRules
  tickets_simulated: number
  results: Array<{
    ticket_id: string
    current_decision: string
    new_decision: string
    changed: boolean
  }>
  summary: {
    total_tickets: number
    would_auto_approve: number
    would_require_review: number
    would_block: number
    changed_decisions: number
    change_rate: number
  }
}

// --- API helpers ---

async function policyFetch<T>(
  path: string,
  token: string | null,
  init?: RequestInit,
): Promise<{ data: T | null; error: string | null }> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  try {
    const res = await fetch(path, { ...init, headers: { ...headers, ...init?.headers } })
    if (!res.ok) {
      const body = await res.json().catch(() => null)
      const msg = (body as { error?: string })?.error ?? `HTTP ${res.status}`
      return { data: null, error: msg }
    }
    if (res.status === 204) {
      return { data: null, error: null }
    }
    const data = await res.json()
    return { data: data as T, error: null }
  } catch (e) {
    return { data: null, error: String(e) }
  }
}

/** Get health view for all policies in a project. */
export async function getProjectPolicyHealth(
  token: string | null,
  projectId: string,
): Promise<{ data: PolicyHealth[] | null; error: string | null }> {
  const res = await policyFetch<{ policies: PolicyHealth[] }>(
    `/projects/${encodeURIComponent(projectId)}/policies/health`,
    token,
  )
  if (res.error) return { data: null, error: res.error }
  return { data: res.data?.policies ?? [], error: null }
}

/** Run calibration for a project (generates proposals). */
export async function runCalibration(
  token: string | null,
  projectId: string,
): Promise<{ data: PolicyProposal[] | null; error: string | null }> {
  const res = await policyFetch<{ proposals: PolicyProposal[] }>(
    `/projects/${encodeURIComponent(projectId)}/policies/calibrate`,
    token,
    { method: 'POST' },
  )
  if (res.error) return { data: null, error: res.error }
  return { data: res.data?.proposals ?? [], error: null }
}

/** Resolve a proposal (accept, reject, dismiss). */
export async function resolveProposal(
  token: string | null,
  proposalId: string,
  status: 'accepted' | 'rejected' | 'dismissed',
): Promise<{ error: string | null }> {
  const res = await policyFetch<null>(
    `/proposals/${encodeURIComponent(proposalId)}/resolve`,
    token,
    { method: 'POST', body: JSON.stringify({ status }) },
  )
  return { error: res.error }
}

/** Simulate rule changes against historical tickets. */
export async function simulateRuleChange(
  token: string | null,
  policyId: string,
  candidateRules: PolicyRules,
  days: number,
): Promise<{ data: SimulationResult | null; error: string | null }> {
  return policyFetch<SimulationResult>(
    `/policies/${encodeURIComponent(policyId)}/simulate`,
    token,
    { method: 'POST', body: JSON.stringify({ candidate_rules: candidateRules, days }) },
  )
}

/** Get health for a single policy. */
export async function getPolicyHealth(
  token: string | null,
  policyId: string,
): Promise<{ data: PolicyHealth | null; error: string | null }> {
  return policyFetch<PolicyHealth>(
    `/policies/${encodeURIComponent(policyId)}/health`,
    token,
  )
}

/** Get decisions for a policy. */
export async function getPolicyDecisions(
  token: string | null,
  policyId: string,
): Promise<{ data: PolicyDecision[] | null; error: string | null }> {
  const res = await policyFetch<{ decisions: PolicyDecision[] }>(
    `/policies/${encodeURIComponent(policyId)}/decisions`,
    token,
  )
  if (res.error) return { data: null, error: res.error }
  return { data: res.data?.decisions ?? [], error: null }
}

/** Get change events (edit history) for a policy. */
export async function getPolicyChangeEvents(
  token: string | null,
  policyId: string,
): Promise<{ data: PolicyChangeEvent[] | null; error: string | null }> {
  const res = await policyFetch<{ events: PolicyChangeEvent[] }>(
    `/policies/${encodeURIComponent(policyId)}/history`,
    token,
  )
  if (res.error) return { data: null, error: res.error }
  return { data: res.data?.events ?? [], error: null }
}
