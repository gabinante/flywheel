import { useDispatchStatus } from '@/hooks/use-dispatch-status'
import { cn } from '@/lib/utils'

/**
 * Persistent header indicator showing dispatcher agent capacity.
 * Displays "N/M agents" with a colored dot:
 *   - Green accent when workers are active
 *   - Muted when idle (zero active workers)
 */
export function DispatchStatusIndicator() {
  const { status, loading } = useDispatchStatus()

  if (loading || !status) return null

  // Dispatcher not enabled on this server
  if (!status.enabled) return null

  const isActive = status.active_workers > 0

  return (
    <div
      className={cn(
        'flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium tabular-nums transition-colors',
        isActive
          ? 'bg-emerald-500/10 text-emerald-400'
          : 'bg-muted text-muted-foreground',
      )}
      title={
        isActive
          ? `${status.active_workers} agent${status.active_workers !== 1 ? 's' : ''} working on: ${status.active_ticket_ids.join(', ')}`
          : 'No agents active'
      }
    >
      <span
        className={cn(
          'inline-block h-1.5 w-1.5 rounded-full',
          isActive ? 'bg-emerald-400 animate-pulse' : 'bg-muted-foreground/50',
        )}
        aria-hidden
      />
      <span>
        {status.active_workers}/{status.max_workers} agents
      </span>
    </div>
  )
}
