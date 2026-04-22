import { useCallback, useEffect, useState } from 'react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'

type DispatchStatusData = {
  enabled: boolean
  active_workers: number
  max_workers: number
  active_ticket_ids: string[]
  timestamp: string
}

const POLL_INTERVAL = 10_000

export function DispatchStatus() {
  const [status, setStatus] = useState<DispatchStatusData | null>(null)
  const [error, setError] = useState<string | null>(null)

  const fetchStatus = useCallback(async () => {
    try {
      const res = await fetch('/api/dispatch/status')
      if (!res.ok) {
        setError('Dispatcher unavailable')
        return
      }
      const data = (await res.json()) as DispatchStatusData
      setStatus(data)
      setError(null)
    } catch {
      setError('Failed to reach dispatcher')
    }
  }, [])

  useEffect(() => {
    void fetchStatus()
    const interval = setInterval(() => void fetchStatus(), POLL_INTERVAL)
    return () => clearInterval(interval)
  }, [fetchStatus])

  const active = status?.active_workers ?? 0
  const max = status?.max_workers ?? 0
  const pct = max > 0 ? (active / max) * 100 : 0

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-xs uppercase tracking-wide text-muted-foreground">
          Dispatch Status
        </CardTitle>
      </CardHeader>
      <CardContent>
        {error ? (
          <p className="text-xs text-muted-foreground italic">{error}</p>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="flex items-baseline justify-between">
              <span className="text-2xl font-semibold tabular-nums text-foreground">
                {active}
                <span className="text-sm font-normal text-muted-foreground">
                  /{max}
                </span>
              </span>
              <span className="text-xs text-muted-foreground">workers active</span>
            </div>
            {/* Capacity bar */}
            <div className="h-2 w-full overflow-hidden rounded-full bg-muted/60">
              <div
                className={cn(
                  'h-full rounded-full transition-all duration-500',
                  pct > 80 ? 'bg-amber-500' : 'bg-primary',
                )}
                style={{ width: `${Math.min(pct, 100)}%` }}
              />
            </div>
            {status?.active_ticket_ids && status.active_ticket_ids.length > 0 && (
              <p className="text-[10px] text-muted-foreground">
                {status.active_ticket_ids.length} ticket
                {status.active_ticket_ids.length !== 1 ? 's' : ''} in flight
              </p>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
