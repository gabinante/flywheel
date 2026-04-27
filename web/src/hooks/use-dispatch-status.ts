import { useQuery } from '@tanstack/react-query'

import { useAuth } from '@/contexts/use-auth'

/** Shape of the /api/dispatch/status response. */
export interface DispatchStatus {
  enabled: boolean
  active_workers: number
  max_workers: number
  active_ticket_ids: string[]
  timestamp: string
}

/**
 * Hook that polls the dispatch status endpoint.
 * Returns the current dispatcher state (active workers, capacity, etc.).
 */
export function useDispatchStatus() {
  const { token } = useAuth()

  const { data: status = null, isLoading: loading } = useQuery<DispatchStatus | null>({
    queryKey: ['dispatch-status'],
    queryFn: async () => {
      const res = await fetch('/api/dispatch/status', {
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
