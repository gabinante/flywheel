import { useCallback, useEffect, useRef, useState } from 'react'

import { useAuth } from '@/contexts/use-auth'

/** Shape of the /api/dispatch/status response. */
export interface DispatchStatus {
  enabled: boolean
  active_workers: number
  max_workers: number
  active_ticket_ids: string[]
  timestamp: string
}

const POLL_INTERVAL = 10_000 // 10 seconds for near-real-time updates

/**
 * Hook that polls the dispatch status endpoint.
 * Returns the current dispatcher state (active workers, capacity, etc.).
 */
export function useDispatchStatus() {
  const { token } = useAuth()
  const [status, setStatus] = useState<DispatchStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const hasLoaded = useRef(false)

  const fetchStatus = useCallback(async () => {
    if (!token) return

    try {
      const res = await fetch('/api/dispatch/status', {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) return

      const data = (await res.json()) as DispatchStatus
      setStatus(data)
    } catch {
      // Silently handle network errors on background refresh
    } finally {
      hasLoaded.current = true
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    if (!token) {
      setLoading(false)
      return
    }

    setLoading(!hasLoaded.current)
    void fetchStatus()

    const interval = setInterval(() => void fetchStatus(), POLL_INTERVAL)
    return () => clearInterval(interval)
  }, [fetchStatus, token])

  return { status, loading }
}
