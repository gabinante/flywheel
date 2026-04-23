import { useEffect, useState } from 'react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type TransitionHistory = components['schemas']['TransitionHistory']
type StateTransitionEntry = components['schemas']['StateTransitionEntry']

type TicketTimelineProps = {
  ticketId: string
  currentState: string | undefined
  createdAt: string | undefined
}

/** Format a timestamp relative to now or as a short datetime. */
function formatTimestamp(iso: string): string {
  const date = new Date(iso)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60000)
  if (diffMins < 1) return 'just now'
  if (diffMins < 60) return `${diffMins}m ago`
  const diffHours = Math.floor(diffMins / 60)
  if (diffHours < 24) return `${diffHours}h ago`
  const diffDays = Math.floor(diffHours / 24)
  if (diffDays < 7) return `${diffDays}d ago`
  return date.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

/** Friendly display name for a trigger. */
function triggerLabel(trigger: string): string {
  const labels: Record<string, string> = {
    spec: 'Specced',
    plan: 'Planned',
    claim: 'Claimed',
    start: 'Started',
    submit: 'Submitted',
    validate: 'Validated',
    approve: 'Approved',
    reject: 'Rejected',
    deploy: 'Deploying',
    observe: 'Observing',
    close: 'Closed',
    cancel: 'Cancelled',
    escalate: 'Escalated',
    request_input: 'Input requested',
    provide_input: 'Input provided',
    replan: 'Replanned',
    invalidate: 'Invalidated',
    lease_expired: 'Lease expired',
    fail: 'Failed',
    reopen: 'Reopened',
  }
  return labels[trigger] ?? trigger
}

/** Color classes for the timeline dot based on trigger/state. */
function dotColor(trigger: string, isActive: boolean): string {
  if (isActive) return 'bg-green-400 ring-green-400/30 ring-4'
  const colors: Record<string, string> = {
    reject: 'bg-red-400',
    fail: 'bg-red-400',
    lease_expired: 'bg-amber-400',
    escalate: 'bg-amber-400',
    cancel: 'bg-zinc-500',
  }
  return colors[trigger] ?? 'bg-green-600'
}

/** Actor badge display. */
function ActorBadge({
  actorId,
  actorType,
}: {
  actorId: string | undefined
  actorType: string | undefined
}) {
  if (!actorId && !actorType) return null
  const typeLabel = actorType === 'human' ? 'Human' : actorType === 'agent' ? 'Agent' : 'System'
  const displayId = actorId && actorId !== '' && actorId !== 'api' && actorId !== 'system'
    ? actorId.length > 12 ? `${actorId.slice(0, 8)}...` : actorId
    : null
  return (
    <span className="text-muted-foreground text-[10px] font-medium uppercase tracking-wider">
      {typeLabel}
      {displayId ? <span className="ml-1 font-mono lowercase opacity-70">{displayId}</span> : null}
    </span>
  )
}

export function TicketTimeline({ ticketId, currentState, createdAt }: TicketTimelineProps) {
  const { client } = useAuth()
  const [history, setHistory] = useState<TransitionHistory | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!ticketId) return
    let cancelled = false
    setLoading(true)
    void (async () => {
      const { data, error: apiErr, response } = await client.GET(
        '/tickets/{ticketID}/transitions',
        { params: { path: { ticketID: ticketId } } },
      )
      if (cancelled) return
      setLoading(false)
      if (!response.ok) {
        setError(formatApiError(apiErr))
        return
      }
      setError(null)
      setHistory(data ?? null)
    })()
    return () => { cancelled = true }
  }, [client, ticketId])

  const transitions: StateTransitionEntry[] = history?.transitions ?? []
  const hasTransitions = transitions.length > 0

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">State Timeline</CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="flex flex-col gap-3">
            {[1, 2, 3].map((i) => (
              <div key={i} className="flex items-center gap-3">
                <div className="bg-muted h-3 w-3 animate-pulse rounded-full" />
                <div className="bg-muted h-3 w-24 animate-pulse rounded" />
              </div>
            ))}
          </div>
        ) : error ? (
          <p className="text-destructive text-sm">{error}</p>
        ) : (
          <div className="relative flex flex-col">
            {/* Created / initial state entry */}
            <TimelineEntry
              label="Created"
              fromState=""
              toState="draft"
              timestamp={createdAt}
              isFirst
              isActive={!hasTransitions && currentState === 'draft'}
              trigger="created"
            />

            {/* Transition entries */}
            {transitions.map((tr, i) => {
              const isLast = i === transitions.length - 1
              const isActive = isLast && tr.to_state === currentState
              return (
                <TimelineEntry
                  key={tr.id ?? i}
                  label={triggerLabel(tr.trigger ?? '')}
                  fromState={tr.from_state ?? ''}
                  toState={tr.to_state ?? ''}
                  timestamp={tr.created_at}
                  actorId={tr.actor_id}
                  actorType={tr.actor_type}
                  trigger={tr.trigger ?? ''}
                  isActive={isActive}
                />
              )
            })}

            {/* Current state indicator if no transitions yet */}
            {!hasTransitions && currentState && currentState !== 'draft' ? (
              <TimelineEntry
                label={currentState}
                fromState="draft"
                toState={currentState}
                isActive
                trigger=""
              />
            ) : null}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

type TimelineEntryProps = {
  label: string
  fromState: string
  toState: string
  timestamp?: string | undefined
  actorId?: string | undefined
  actorType?: string | undefined
  trigger: string
  isFirst?: boolean
  isActive?: boolean
}

function TimelineEntry({
  label,
  toState,
  timestamp,
  actorId,
  actorType,
  trigger,
  isFirst,
  isActive,
}: TimelineEntryProps) {
  return (
    <div className="group relative flex items-start gap-3 pb-4 last:pb-0">
      {/* Vertical line */}
      <div className="absolute top-3 left-[5px] bottom-0 w-px bg-border group-last:hidden" />

      {/* Dot */}
      <div
        className={`relative z-10 mt-1 h-[11px] w-[11px] shrink-0 rounded-full ${dotColor(trigger, !!isActive)}`}
      />

      {/* Content */}
      <div className="flex flex-col gap-0.5 min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className={`text-sm font-medium ${isActive ? 'text-green-400' : 'text-foreground'}`}>
            {label}
          </span>
          {toState ? (
            <span className="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[10px] font-mono">
              {toState}
            </span>
          ) : null}
          {isActive ? (
            <span className="text-green-400 text-[10px] font-semibold uppercase tracking-wider">
              current
            </span>
          ) : null}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {timestamp ? (
            <span className="text-muted-foreground text-xs">
              {formatTimestamp(timestamp)}
            </span>
          ) : isFirst ? (
            <span className="text-muted-foreground text-xs">initial state</span>
          ) : null}
          <ActorBadge actorId={actorId} actorType={actorType} />
        </div>
      </div>
    </div>
  )
}
