import { cn } from '@/lib/utils'
import type { components } from '@/lib/api/v1'

type TraceStep = components['schemas']['TraceStep']
export type ActivityItem = TraceStep & { ticketId: string }

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

function summarizeStep(step: TraceStep): string {
  const p = step.payload as unknown as Record<string, unknown> | undefined
  if (!p) return step.type ?? 'step'
  if ('message' in p && typeof p.message === 'string') {
    return p.message.slice(0, 80)
  }
  if ('name' in p && typeof p.name === 'string') {
    return p.name
  }
  const keys = Object.keys(p)
  if (keys.length === 0) return step.type ?? 'step'
  return keys.slice(0, 2).join(', ')
}

function stepIcon(type: string | undefined): string {
  switch (type) {
    case 'tool_call':
      return '\u2699'
    case 'observation':
      return '\u{1F441}'
    case 'thought':
      return '\u{1F4AD}'
    case 'error':
      return '\u26A0'
    default:
      return '\u2022'
  }
}

function Shimmer({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded bg-muted/60', className)} />
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
          Activity Feed
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
        Activity Feed
      </h3>
      {items.length === 0 ? (
        <p className="text-xs text-muted-foreground italic py-2">
          No recent activity.
        </p>
      ) : (
        <ul className="flex flex-col gap-0.5">
          {items.slice(0, 20).map((item, i) => (
            <li key={item.id ?? i}>
              <button
                type="button"
                onClick={() => onSelectTicket(item.ticketId)}
                className="group flex w-full items-start gap-1.5 rounded px-1.5 py-1 text-[11px] text-left transition-colors hover:bg-white/[0.04]"
              >
                <span className="mt-px shrink-0 text-muted-foreground" aria-hidden>
                  {stepIcon(item.type)}
                </span>
                <div className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-foreground/80">
                    {summarizeStep(item)}
                  </span>
                  <span className="text-[10px] text-muted-foreground">
                    <span className="font-mono">{item.ticketId.slice(0, 8)}</span>
                    {item.created_at && (
                      <span className="ml-1.5 tabular-nums">
                        {elapsed(item.created_at)}
                      </span>
                    )}
                  </span>
                </div>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
