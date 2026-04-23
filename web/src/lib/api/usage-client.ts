export type UsageSummary = {
  calls: number
  input_tokens: number
  output_tokens: number
  total_tokens: number
  cost_millicent: number
  cost_dollars: number
}

export type UsageDailyPoint = UsageSummary & {
  date: string
}

export type UsageBreakdown = UsageSummary & {
  key: string
  label: string
  share: number
}

export type UsageBudgetStatus = {
  spent_millicent: number
  remaining_millicent: number
  used_fraction: number
  projected_millicent: number
  over_budget: boolean
  warning: boolean
}

export type UsageResponse = {
  project_id: string
  estimated: boolean
  window: {
    days: number
    from: string
    to: string
  }
  summary: UsageSummary
  daily: UsageDailyPoint[]
  by_task_type: UsageBreakdown[]
  by_worker_group: UsageBreakdown[]
  by_api: UsageBreakdown[]
  by_provider: UsageBreakdown[]
  by_model: UsageBreakdown[]
  budget_status?: UsageBudgetStatus | null
}

export async function getProjectUsage(
  token: string | null,
  projectId: string,
  days: number,
): Promise<{ data: UsageResponse | null; error: string | null }> {
  const headers: Record<string, string> = {}
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }

  try {
    const res = await fetch(
      `/api/projects/${encodeURIComponent(projectId)}/usage?days=${encodeURIComponent(String(days))}`,
      { headers },
    )
    if (!res.ok) {
      const body = await res.json().catch(() => null)
      const message = (body as { error?: string } | null)?.error ?? `HTTP ${res.status}`
      return { data: null, error: message }
    }
    const data = (await res.json()) as UsageResponse
    return { data, error: null }
  } catch (error) {
    return { data: null, error: String(error) }
  }
}
