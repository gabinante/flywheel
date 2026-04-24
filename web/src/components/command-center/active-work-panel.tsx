import { Link } from 'react-router-dom'

import { Badge } from '@/components/ui/badge'
import type { components } from '@/lib/api/v1'
import { cn } from '@/lib/utils'

type Ticket = components['schemas']['Ticket']

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
  return `${Math.floor(hours / 24)}d`
}

function stateBadgeVariant(
  state: string | undefined,
): 'default' | 'secondary' | 'outline' | 'muted' {
  switch (state) {
    case 'executing':
      return 'default'
    case 'planning':
    case 'claimed':
      return 'secondary'
    case 'awaiting_validation':
    case 'awaiting_review':
    case 'validated':
      return 'outline'
    default:
      return 'muted'
  }
}

function shortAgent(agentId: string | undefined): string {
  if (!agentId) return 'unassigned'
  const parts = agentId.split('-')
  if (parts.length > 2) return parts.slice(-2).join('-')
  return agentId.slice(0, 12)
}

function Shimmer({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded bg-muted/60', className)} />
}

export function ActiveWorkPanel({
  tickets,
  pendingReviews,
  loading,
  orgId,
  projectId,
  selectedTicketId,
  onSelectTicket,
}: {
  tickets: Ticket[]
  pendingReviews: Ticket[]
  loading: boolean
  orgId: string
  projectId: string
  selectedTicketId: string | null
  onSelectTicket: (ticketId: string) => void
}) {
  if (loading && tickets.length === 0 && pendingReviews.length === 0) {
    return (
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <Shimmer className="h-3 w-24" />
          <Shimmer className="h-14 w-full rounded-lg" />
          <Shimmer className="h-14 w-full rounded-lg" />
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-5">
      {/* Active Work */}
      <div className="flex flex-col gap-1.5">
        <div className="flex items-center justify-between">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Active Work
          </h3>
          {tickets.length > 0 && (
            <span className="rounded-full bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium tabular-nums text-primary">
              {tickets.length}
            </span>
          )}
        </div>
        {tickets.length === 0 ? (
          <p className="text-xs text-muted-foreground italic py-2">
            No tickets in progress.
          </p>
        ) : (
          <ul className="flex flex-col gap-1">
            {tickets.map((t) => (
              <li key={t.id}>
                <button
                  type="button"
                  onClick={() => t.id && onSelectTicket(t.id)}
                  className={cn(
                    'group flex w-full flex-col gap-0.5 rounded-lg px-2.5 py-2 text-left transition-colors',
                    selectedTicketId === t.id
                      ? 'bg-primary/10 ring-1 ring-primary/30'
                      : 'bg-white/[0.03] hover:bg-white/[0.06]',
                  )}
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
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* Pending Review */}
      <div className="flex flex-col gap-1.5">
        <div className="flex items-center justify-between">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Pending Review
          </h3>
          {pendingReviews.length > 0 && (
            <span className="rounded-full bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium tabular-nums text-amber-400">
              {pendingReviews.length}
            </span>
          )}
        </div>
        {pendingReviews.length === 0 ? (
          <p className="text-xs text-muted-foreground italic py-2">
            Nothing awaiting review.
          </p>
        ) : (
          <ul className="flex flex-col gap-1">
            {pendingReviews.map((t) => (
              <li key={t.id}>
                <button
                  type="button"
                  onClick={() => t.id && onSelectTicket(t.id)}
                  className={cn(
                    'group flex w-full flex-col gap-0.5 rounded-lg px-2.5 py-2 text-left transition-colors',
                    selectedTicketId === t.id
                      ? 'bg-primary/10 ring-1 ring-primary/30'
                      : 'bg-white/[0.03] hover:bg-white/[0.06]',
                  )}
                >
                  <span className="truncate text-xs font-medium text-foreground group-hover:underline">
                    {t.title ?? t.id}
                  </span>
                  <div className="flex items-center gap-2 text-[10px] text-muted-foreground">
                    <Badge variant="outline" className="text-[10px]">
                      awaiting review
                    </Badge>
                    <span className="ml-auto shrink-0 tabular-nums">
                      {elapsed(t.updated_at)}
                    </span>
                  </div>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* Link to full views */}
      <div className="flex gap-2 text-xs">
        <Link
          to={`/orgs/${orgId}/projects/${projectId}/tickets`}
          className="text-primary hover:underline"
        >
          All tickets
        </Link>
        <Link
          to={`/orgs/${orgId}/projects/${projectId}/reviews`}
          className="text-primary hover:underline"
        >
          Review queue
        </Link>
      </div>
    </div>
  )
}
