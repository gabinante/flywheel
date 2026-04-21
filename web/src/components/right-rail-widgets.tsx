import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'

import { Badge } from '@/components/ui/badge'
import { useAuth } from '@/contexts/use-auth'

import type { components } from '@/lib/api/v1'
import { cn } from '@/lib/utils'

type Ticket = components['schemas']['Ticket']
type TraceStep = components['schemas']['TraceStep']

/** Polling interval for auto-refresh (ms). */
const POLL_INTERVAL = 30_000

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

/** State badge color mapping. */
function stateBadgeVariant(
  state: string | undefined,
): 'default' | 'secondary' | 'outline' | 'muted' {
  switch (state) {
    case 'executing':
      return 'default'
    case 'claimed':
      return 'secondary'
    case 'awaiting_review':
      return 'outline'
    default:
      return 'muted'
  }
}

/** Truncate agent ID to last segment for display. */
function shortAgent(agentId: string | undefined): string {
  if (!agentId) return 'unassigned'
  const parts = agentId.split('-')
  if (parts.length > 2) return parts.slice(-2).join('-')
  return agentId.slice(0, 12)
}

/** Summarize a trace step payload into a short label. */
function summarizeStep(step: TraceStep): string {
  const p = step.payload as unknown as Record<string, unknown> | undefined
  if (!p) return step.type ?? 'step'
  if ('message' in p && typeof p.message === 'string') {
    return p.message.slice(0, 60)
  }
  if ('name' in p && typeof p.name === 'string') {
    return p.name
  }
  const keys = Object.keys(p)
  if (keys.length === 0) return step.type ?? 'step'
  return keys.slice(0, 2).join(', ')
}

/** Step type icon character. */
function stepIcon(type: string | undefined): string {
  switch (type) {
    case 'tool_call':
      return '\u2699' // gear
    case 'observation':
      return '\u{1F441}' // eye
    case 'thought':
      return '\u{1F4AD}' // thought balloon
    case 'error':
      return '\u26A0' // warning
    default:
      return '\u2022' // bullet
  }
}

// ---------------------------------------------------------------------------
// Section header
// ---------------------------------------------------------------------------

function SectionHeader({
  title,
  count,
}: {
  title: string
  count?: number
}) {
  return (
    <div className="flex items-center justify-between">
      <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {title}
      </h3>
      {count != null && count > 0 && (
        <span className="rounded-full bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium tabular-nums text-primary">
          {count}
        </span>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Shimmer skeleton
// ---------------------------------------------------------------------------

function Shimmer({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        'animate-pulse rounded bg-muted/60',
        className,
      )}
    />
  )
}

function WidgetSkeleton() {
  return (
    <div className="flex flex-col gap-2">
      <Shimmer className="h-3 w-20" />
      <Shimmer className="h-12 w-full rounded-lg" />
      <Shimmer className="h-12 w-full rounded-lg" />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Active Tickets Widget
// ---------------------------------------------------------------------------

function ActiveTicketsWidget({
  tickets,
  loading,
  orgId,
  projectId,
}: {
  tickets: Ticket[]
  loading: boolean
  orgId: string
  projectId: string
}) {
  if (loading && tickets.length === 0) {
    return (
      <div className="flex flex-col gap-2">
        <SectionHeader title="Active" />
        <WidgetSkeleton />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-2">
      <SectionHeader title="Active" count={tickets.length} />
      {tickets.length === 0 ? (
        <p className="text-xs text-muted-foreground italic">
          No tickets in progress.
        </p>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {tickets.map((t) => (
            <li key={t.id}>
              <Link
                to={`/orgs/${orgId}/projects/${projectId}/tickets/${t.id}`}
                className="group flex flex-col gap-0.5 rounded-lg bg-white/[0.03] px-2.5 py-2 transition-colors hover:bg-white/[0.06]"
              >
                <div className="flex items-center gap-1.5">
                  <Badge
                    variant={stateBadgeVariant(t.state)}
                    className="shrink-0 text-[10px]"
                  >
                    {t.state}
                  </Badge>
                  <span className="truncate text-xs font-medium text-foreground group-hover:underline">
                    {t.title ?? t.id}
                  </span>
                </div>
                <div className="flex items-center gap-2 text-[10px] text-muted-foreground">
                  <span className="truncate font-mono">
                    {shortAgent(t.assigned_to)}
                  </span>
                  <span className="ml-auto shrink-0 tabular-nums">
                    {elapsed(t.updated_at)}
                  </span>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Pending Approvals Widget
// ---------------------------------------------------------------------------

function PendingApprovalsWidget({
  tickets,
  loading,
  orgId,
  projectId,
}: {
  tickets: Ticket[]
  loading: boolean
  orgId: string
  projectId: string
}) {
  if (loading && tickets.length === 0) {
    return (
      <div className="flex flex-col gap-2">
        <SectionHeader title="Pending Review" />
        <WidgetSkeleton />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-2">
      <SectionHeader title="Pending Review" count={tickets.length} />
      {tickets.length === 0 ? (
        <p className="text-xs text-muted-foreground italic">
          Nothing awaiting review.
        </p>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {tickets.map((t) => (
            <li key={t.id}>
              <Link
                to={`/orgs/${orgId}/projects/${projectId}/reviews`}
                className="group flex flex-col gap-0.5 rounded-lg bg-white/[0.03] px-2.5 py-2 transition-colors hover:bg-white/[0.06]"
              >
                <span className="truncate text-xs font-medium text-foreground group-hover:underline">
                  {t.title ?? t.id}
                </span>
                <div className="flex items-center gap-2 text-[10px] text-muted-foreground">
                  <span className="font-mono">{t.id}</span>
                  <span className="ml-auto shrink-0 tabular-nums">
                    {elapsed(t.updated_at)}
                  </span>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Activity Feed Widget
// ---------------------------------------------------------------------------

type ActivityItem = TraceStep & { ticketId: string }

function ActivityFeedWidget({
  items,
  loading,
  orgId,
  projectId,
}: {
  items: ActivityItem[]
  loading: boolean
  orgId: string
  projectId: string
}) {
  if (loading && items.length === 0) {
    return (
      <div className="flex flex-col gap-2">
        <SectionHeader title="Recent Activity" />
        <WidgetSkeleton />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-2">
      <SectionHeader title="Recent Activity" count={items.length > 0 ? items.length : undefined} />
      {items.length === 0 ? (
        <p className="text-xs text-muted-foreground italic">
          No recent activity.
        </p>
      ) : (
        <ul className="flex flex-col gap-1">
          {items.slice(0, 15).map((item, i) => (
            <li key={item.id ?? i}>
              <Link
                to={`/orgs/${orgId}/projects/${projectId}/tickets/${item.ticketId}`}
                className="group flex items-start gap-1.5 rounded px-1.5 py-1 text-[11px] transition-colors hover:bg-white/[0.04]"
              >
                <span className="mt-px shrink-0 text-muted-foreground" aria-hidden>
                  {stepIcon(item.type)}
                </span>
                <div className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-foreground/80">
                    {summarizeStep(item)}
                  </span>
                  <span className="text-[10px] text-muted-foreground">
                    <span className="font-mono">{item.ticketId}</span>
                    {item.created_at && (
                      <span className="ml-1.5 tabular-nums">
                        {elapsed(item.created_at)}
                      </span>
                    )}
                  </span>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main RightRailWidgets container
// ---------------------------------------------------------------------------

export function RightRailWidgets() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>()
  const { client, token } = useAuth()

  const [activeTickets, setActiveTickets] = useState<Ticket[]>([])
  const [pendingReviews, setPendingReviews] = useState<Ticket[]>([])
  const [activityItems, setActivityItems] = useState<ActivityItem[]>([])
  const [loading, setLoading] = useState(true)

  // Track whether it's the very first load vs a background refresh
  const hasLoaded = useRef(false)

  const fetchData = useCallback(async () => {
    if (!projectId || !token) return

    try {
      // Fetch claimed and executing tickets in parallel, plus pending reviews
      const [claimedRes, executingRes, reviewsRes] = await Promise.all([
        client.GET('/projects/{projectID}/tickets', {
          params: { path: { projectID: projectId }, query: { state: 'claimed' } },
        }),
        client.GET('/projects/{projectID}/tickets', {
          params: { path: { projectID: projectId }, query: { state: 'executing' } },
        }),
        client.GET('/projects/{projectID}/reviews', {
          params: { path: { projectID: projectId } },
        }),
      ])

      // Merge active tickets
      const claimed: Ticket[] = claimedRes.response.ok
        ? ((claimedRes.data ?? []) as Ticket[])
        : []
      const executing: Ticket[] = executingRes.response.ok
        ? ((executingRes.data ?? []) as Ticket[])
        : []
      const active = [...executing, ...claimed]
      setActiveTickets(active)

      // Pending reviews
      const reviews: Ticket[] = reviewsRes.response.ok
        ? (reviewsRes.data?.tickets ?? [])
        : []
      setPendingReviews(reviews)

      // Fetch traces for active tickets (limit to first 5 to avoid fan-out overload)
      const traceTargets = active.slice(0, 5)
      if (traceTargets.length > 0) {
        const traceResults = await Promise.all(
          traceTargets.map(async (t) => {
            if (!t.id) return []
            const { data, response } = await client.GET('/tickets/{ticketID}/trace', {
              params: { path: { ticketID: t.id } },
            })
            if (!response.ok) return []
            return (data?.steps ?? []).map((step) => ({
              ...step,
              ticketId: t.id!,
            }))
          }),
        )

        // Merge all steps, sort by created_at descending, take latest 20
        const allSteps: ActivityItem[] = traceResults
          .flat()
          .sort((a, b) => {
            const ta = a.created_at ? new Date(a.created_at).getTime() : 0
            const tb = b.created_at ? new Date(b.created_at).getTime() : 0
            return tb - ta
          })
          .slice(0, 20)

        setActivityItems(allSteps)
      } else {
        setActivityItems([])
      }
    } catch {
      // Silently handle network errors on background refresh
    } finally {
      hasLoaded.current = true
      setLoading(false)
    }
  }, [client, projectId, token])

  // Initial fetch + polling
  useEffect(() => {
    if (!projectId || !token) {
      setLoading(false)
      return
    }

    setLoading(!hasLoaded.current)
    void fetchData()

    const interval = setInterval(() => void fetchData(), POLL_INTERVAL)
    return () => clearInterval(interval)
  }, [fetchData, projectId, token])

  // Don't render anything if we're not in a project context
  if (!orgId || !projectId || !token) {
    return (
      <div className="flex flex-col gap-4 text-xs text-muted-foreground">
        <p className="italic">
          Navigate to a project to see dashboard widgets.
        </p>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      <ActiveTicketsWidget
        tickets={activeTickets}
        loading={loading}
        orgId={orgId}
        projectId={projectId}
      />

      <div className="border-t border-border" />

      <PendingApprovalsWidget
        tickets={pendingReviews}
        loading={loading}
        orgId={orgId}
        projectId={projectId}
      />

      <div className="border-t border-border" />

      <ActivityFeedWidget
        items={activityItems}
        loading={loading}
        orgId={orgId}
        projectId={projectId}
      />
    </div>
  )
}
