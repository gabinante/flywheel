import { useDispatchStatus } from '@/hooks/use-dispatch-status'
import { cn } from '@/lib/utils'

/** Human-friendly labels for idle reason codes. */
const IDLE_REASON_LABELS: Record<string, string> = {
  all_work_complete: 'All tickets resolved',
  dispatch_disabled_project: 'Dispatch disabled',
  no_repo_configured: 'No repository configured',
  at_capacity: 'All worker slots in use',
  review_and_merge_pending: 'Tickets awaiting review and merge',
  review_pending: 'Tickets awaiting review',
  merge_pending: 'Tickets awaiting merge',
  deps_not_met: 'All queued tickets blocked by dependencies',
  awaiting_human_input: 'Tickets waiting for human input',
  no_draft_tickets: 'No queued tickets',
  idle: 'No agents active',
}

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

  const tooltip = isActive
    ? `${status.active_workers} agent${status.active_workers !== 1 ? 's' : ''} working on: ${status.active_ticket_ids.join(', ')}`
    : IDLE_REASON_LABELS[status.idle_reason ?? 'idle'] ?? 'No agents active'

  return (
    <div
      className={cn(
        'flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium tabular-nums transition-colors',
        isActive
          ? 'bg-emerald-500/10 text-emerald-400'
          : 'bg-muted text-muted-foreground',
      )}
      title={tooltip}
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
