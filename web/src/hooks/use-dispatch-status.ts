import { useQuery } from '@tanstack/react-query'

import { useAuth } from '@/contexts/use-auth'

/** Ticket count breakdown by state. */
export interface StatusDiagnostics {
  draft_count: number
  planning_count: number
  executing_count: number
  awaiting_validation_count: number
  validated_count: number
  awaiting_input_count: number
  closed_count: number
}

/** Shape of the /api/dispatch/status response. */
export interface DispatchStatus {
  enabled: boolean
  active_workers: number
  max_workers: number
  active_ticket_ids: string[]
  timestamp: string
  idle_reason?: string
  diagnostics?: StatusDiagnostics
}

/**
 * Hook that polls the dispatch status endpoint.
 * Returns the current dispatcher state (active workers, capacity, etc.).
 * Optionally scoped to a specific project.
 */
export function useDispatchStatus(projectId?: string) {
  const { token } = useAuth()

  const { data: status = null, isLoading: loading } = useQuery<DispatchStatus | null>({
    queryKey: ['dispatch-status', projectId ?? ''],
    queryFn: async () => {
      const url = projectId
        ? `/api/dispatch/status?project_id=${encodeURIComponent(projectId)}`
        : '/api/dispatch/status'
      const res = await fetch(url, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) return null
      return (await res.json()) as DispatchStatus
    },
    enabled: Boolean(token),
    refetchInterval: 10_000,
  })

  return { status, loading }
}
