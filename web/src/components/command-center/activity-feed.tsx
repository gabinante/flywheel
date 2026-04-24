import {
  ClipboardCheck,
  GitCommitHorizontal,
  GitPullRequest,
  Rocket,
  Ticket,
  Workflow,
} from 'lucide-react'

import type { ActivityItem } from '@/lib/command-center-activity'
import { cn } from '@/lib/utils'

export type { ActivityItem } from '@/lib/command-center-activity'

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

function Shimmer({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded bg-muted/60', className)} />
}

function ActivityIcon({ kind }: { kind: ActivityItem['kind'] }) {
  const className = 'mt-0.5 size-3.5 shrink-0 text-muted-foreground'

  switch (kind) {
    case 'pr':
      return <GitPullRequest className={className} />
    case 'git':
      return <GitCommitHorizontal className={className} />
    case 'deploy':
      return <Rocket className={className} />
    case 'review':
      return <ClipboardCheck className={className} />
    case 'plan':
    case 'work_stream':
      return <Workflow className={className} />
    default:
      return <Ticket className={className} />
  }
}

export function ActivityFeed({
  items,
  loading,
  onSelectTicket,
}: {
  items: ActivityItem[]
  loading: boolean
  onSelectTicket: (ticketId: string) => void
}) {
  if (loading && items.length === 0) {
    return (
      <div className="flex flex-col gap-2">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Recent Activity
        </h3>
        <Shimmer className="h-4 w-full" />
        <Shimmer className="h-4 w-3/4" />
        <Shimmer className="h-4 w-5/6" />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-1.5">
      <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        Recent Activity
      </h3>
      {items.length === 0 ? (
        <p className="py-2 text-xs italic text-muted-foreground">
          No recent activity.
        </p>
      ) : (
        <ul className="flex flex-col gap-1">
          {items.slice(0, 20).map((item) => {
            const content = (
              <>
                <ActivityIcon kind={item.kind} />
                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="truncate text-[11px] font-medium text-foreground/90">
                    {item.title}
                  </span>
                  {item.detail && (
                    <span className="line-clamp-2 text-[10px] leading-relaxed text-muted-foreground">
                      {item.detail}
                    </span>
                  )}
                  <span className="text-[10px] text-muted-foreground">
                    {item.ticketId && (
                      <span className="font-mono">{item.ticketId.slice(0, 8)}</span>
                    )}
                    {item.ticketId && item.timestamp && <span className="mx-1">·</span>}
                    {item.timestamp && (
                      <span className="tabular-nums">{elapsed(item.timestamp)}</span>
                    )}
                  </span>
                </div>
              </>
            )

            return (
              <li key={item.id}>
                {item.ticketId ? (
                  <button
                    type="button"
                    onClick={() => onSelectTicket(item.ticketId!)}
                    className="group flex w-full items-start gap-2 rounded-lg px-1.5 py-1.5 text-left transition-colors hover:bg-white/[0.04]"
                  >
                    {content}
                  </button>
                ) : (
                  <div className="flex items-start gap-2 rounded-lg px-1.5 py-1.5">
                    {content}
                  </div>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
