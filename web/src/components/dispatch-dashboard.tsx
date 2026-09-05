import { useActivityVersion } from '@/contexts/use-activity'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useAPI } from '@/contexts/use-api'
import { useDispatchStatus, type DispatchStatus } from '@/hooks/use-dispatch-status'
import { cn } from '@/lib/utils'

import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']
type TraceStep = components['schemas']['TraceStep']

/** Polling interval for active ticket details (ms). */

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Human-friendly elapsed time from an ISO date string. */
function elapsed(isoDate: string | undefined): string {
  if (!isoDate) return ''
  const diff = Date.now() - new Date(isoDate).getTime()
  if (diff < 0) return 'just now'
  const secs = Math.floor(diff / 1000)
  if (secs < 60) return `${secs}s`
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins}m`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h`
  const days = Math.floor(hours / 24)
  return `${days}d`
}

/** Map worker type to a human-readable label. */
function workerTypeLabel(type: string): string {
  switch (type) {
    case 'executor':
      return 'Executor'
    case 'planner':
      return 'Planner'
    case 'validator':
      return 'Reviewer'
    case 'deployer':
      return 'Deployer'
    case 'investigator':
      return 'Investigator'
    default:
      return type || 'Agent'
  }
}

/** Map worker type to a badge color class. */
function workerTypeBadgeClass(type: string): string {
  switch (type) {
    case 'executor':
      return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/20'
    case 'planner':
      return 'bg-blue-500/15 text-blue-400 border-blue-500/20'
    case 'validator':
      return 'bg-amber-500/15 text-amber-400 border-amber-500/20'
    case 'deployer':
      return 'bg-purple-500/15 text-purple-400 border-purple-500/20'
    case 'investigator':
      return 'bg-cyan-500/15 text-cyan-400 border-cyan-500/20'
    default:
      return 'bg-muted text-muted-foreground'
  }
}

// ---------------------------------------------------------------------------
// Shimmer skeleton
// ---------------------------------------------------------------------------

function Shimmer({ className }: { className?: string }) {
  return (
    <div
      className={cn('animate-pulse rounded bg-muted/60', className)}
    />
  )
}

// ---------------------------------------------------------------------------
// Worker card info (ticket + trace data combined)
// ---------------------------------------------------------------------------

interface WorkerInfo {
  ticket: Ticket
  workerType: string
  startedAt: string | undefined
}

// ---------------------------------------------------------------------------
// Capacity bar
// ---------------------------------------------------------------------------

function CapacityBar({
  active,
  max,
}: {
  active: number
  max: number
}) {
  const pct = max > 0 ? Math.round((active / max) * 100) : 0
  const isActive = active > 0

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center justify-between text-xs">
        <span className="text-muted-foreground">Worker capacity</span>
        <span
          className={cn(
            'font-medium tabular-nums',
            isActive ? 'text-emerald-400' : 'text-muted-foreground',
          )}
        >
          {active}/{max}
        </span>
      </div>
      <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted/60">
        <div
          className={cn(
            'h-full rounded-full transition-all duration-500 ease-out',
            isActive ? 'bg-emerald-500/70' : 'bg-muted-foreground/30',
          )}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Worker card
// ---------------------------------------------------------------------------

function WorkerCard({
  worker,
  basePath,
}: {
  worker: WorkerInfo
  basePath: string
}) {
  const { ticket, workerType, startedAt } = worker

  return (
    <Link
      to={`${basePath}/tickets/${ticket.id}`}
      className="group flex flex-col gap-2 rounded-lg border border-border bg-white/[0.03] px-3 py-2.5 transition-all hover:bg-white/[0.06] hover:border-emerald-500/20"
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <Badge
            className={cn(
              'shrink-0 text-[10px] font-medium',
              workerTypeBadgeClass(workerType),
            )}
          >
            {workerTypeLabel(workerType)}
          </Badge>
          <span className="truncate text-xs font-medium text-foreground group-hover:underline">
            {ticket.title ?? ticket.id}
          </span>
        </div>
        {startedAt && (
          <span className="shrink-0 text-[10px] tabular-nums text-muted-foreground">
            {elapsed(startedAt)}
          </span>
        )}
      </div>
      <div className="flex items-center gap-2 text-[10px] text-muted-foreground">
        <span className="font-mono">{ticket.id}</span>
        <Badge variant="outline" className="text-[10px]">
          {ticket.state}
        </Badge>
      </div>
    </Link>
  )
}

// ---------------------------------------------------------------------------
// Idle state with diagnostics
// ---------------------------------------------------------------------------

const IDLE_REASON_CONFIG: Record<string, { label: string; color: string }> = {
  all_work_complete: { label: 'All tickets resolved', color: 'text-emerald-400' },
  dispatch_disabled_project: { label: 'Dispatch disabled for this project', color: 'text-amber-400' },
  no_repo_configured: { label: 'No repository configured', color: 'text-amber-400' },
  at_capacity: { label: 'All worker slots in use', color: 'text-blue-400' },
  review_and_merge_pending: { label: 'Tickets awaiting review and merge', color: 'text-amber-400' },
  review_pending: { label: 'Tickets awaiting review', color: 'text-amber-400' },
  merge_pending: { label: 'Tickets awaiting merge', color: 'text-amber-400' },
  deps_not_met: { label: 'All queued tickets blocked by dependencies', color: 'text-amber-400' },
  awaiting_human_input: { label: 'Tickets waiting for human input', color: 'text-amber-400' },
  no_draft_tickets: { label: 'No queued tickets', color: 'text-muted-foreground' },
  idle: { label: 'No agents active', color: 'text-muted-foreground' },
}

function IdleState({ status }: { status: DispatchStatus | null }) {
  const reason = status?.idle_reason ?? 'idle'
  const config = IDLE_REASON_CONFIG[reason] ?? IDLE_REASON_CONFIG.idle
  const diag = status?.diagnostics

  // Build a summary of non-zero ticket counts.
  const parts: string[] = []
  if (diag) {
    if (diag.draft_count > 0) parts.push(`${diag.draft_count} draft`)
    if (diag.planning_count > 0) parts.push(`${diag.planning_count} planning`)
    if (diag.executing_count > 0) parts.push(`${diag.executing_count} executing`)
    if (diag.awaiting_validation_count > 0) parts.push(`${diag.awaiting_validation_count} awaiting review`)
    if (diag.validated_count > 0) parts.push(`${diag.validated_count} validated`)
    if (diag.awaiting_input_count > 0) parts.push(`${diag.awaiting_input_count} awaiting input`)
  }

  return (
    <div className="flex flex-col items-center gap-1 py-4 text-center">
      <p className={cn('text-xs font-medium', config.color)}>
        {config.label}
      </p>
      {parts.length > 0 && (
        <p className="text-[10px] text-muted-foreground/60">
          {parts.join(', ')}
        </p>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main dispatch dashboard
// ---------------------------------------------------------------------------

export function DispatchDashboard({
  projectId,
  basePath,
}: {
  projectId: string
  basePath: string
}) {
  const activityVersion = useActivityVersion('dispatch', 'tickets', 'projects', 'settings')
  const { client } = useAPI()
  const { status } = useDispatchStatus(projectId)
  const [workers, setWorkers] = useState<WorkerInfo[]>([])
  const [loading, setLoading] = useState(true)
  const hasLoaded = useRef(false)

  const fetchWorkerDetails = useCallback(async () => {
    if (!projectId) return

    try {
      // Fetch planning and executing tickets for this project
      const [planningRes, executingRes] = await Promise.all([
        client.GET('/projects/{projectID}/tickets', {
          params: { path: { projectID: projectId }, query: { state: 'planning' } },
        }),
        client.GET('/projects/{projectID}/tickets', {
          params: { path: { projectID: projectId }, query: { state: 'executing' } },
        }),
      ])

      const planning: Ticket[] = planningRes.response.ok
        ? ((planningRes.data ?? []) as Ticket[])
        : []
      const executing: Ticket[] = executingRes.response.ok
        ? ((executingRes.data ?? []) as Ticket[])
        : []
      const activeTickets = [...executing, ...planning]

      if (activeTickets.length === 0) {
        setWorkers([])
        return
      }

      // Fetch traces for active tickets to determine worker type
      const traceResults = await Promise.all(
        activeTickets.slice(0, 10).map(async (t) => {
          if (!t.id) return { ticket: t, workerType: '', startedAt: t.updated_at }
          try {
            const { data, response } = await client.GET('/tickets/{ticketID}/trace', {
              params: { path: { ticketID: t.id } },
            })
            if (!response.ok || !data?.steps?.length) {
              return { ticket: t, workerType: '', startedAt: t.updated_at }
            }

            // Find the most recent step with a worker_type
            const steps = data.steps as (TraceStep & { worker_type?: string })[]
            const withType = steps.filter((s) => s.worker_type)
            const workerType = withType.length > 0
              ? withType[withType.length - 1].worker_type ?? ''
              : ''

            // Start time is the earliest step
            const startedAt = steps.length > 0
              ? steps[0].created_at
              : t.updated_at

            return { ticket: t, workerType, startedAt }
          } catch {
            return { ticket: t, workerType: '', startedAt: t.updated_at }
          }
        }),
      )

      setWorkers(traceResults)
    } catch {
      // Silently handle network errors on background refresh
    } finally {
      hasLoaded.current = true
      setLoading(false)
    }
  }, [client, projectId])

  useEffect(() => {
    if (!projectId) {
      setLoading(false)
      return
    }

    setLoading(!hasLoaded.current)
    void fetchWorkerDetails()

  }, [fetchWorkerDetails, projectId, activityVersion])

  // Don't render if dispatch is not enabled
  if (status && !status.enabled) return null

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="space-y-1.5">
            <CardTitle className="text-sm flex items-center gap-2">
              <span>Dispatch</span>
              {status && status.active_workers > 0 && (
                <span className="inline-flex h-2 w-2 rounded-full bg-emerald-400 animate-pulse" />
              )}
            </CardTitle>
            <CardDescription>
              Agent workers and capacity for this project.
            </CardDescription>
          </div>
          {status && (
            <div
              className={cn(
                'flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium tabular-nums',
                status.active_workers > 0
                  ? 'bg-emerald-500/10 text-emerald-400'
                  : 'bg-muted text-muted-foreground',
              )}
            >
              {status.active_workers}/{status.max_workers} active
            </div>
          )}
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {/* Capacity bar */}
        {status && (
          <CapacityBar active={status.active_workers} max={status.max_workers} />
        )}

        {/* Worker cards */}
        {loading && workers.length === 0 ? (
          <div className="flex flex-col gap-2">
            <Shimmer className="h-14 w-full rounded-lg" />
            <Shimmer className="h-14 w-full rounded-lg" />
          </div>
        ) : workers.length === 0 ? (
          <IdleState status={status} />
        ) : (
          <div className="flex flex-col gap-2">
            {workers.map((w) => (
              <WorkerCard
                key={w.ticket.id}
                worker={w}
                basePath={basePath}
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
