import { Link } from 'react-router-dom'
import { ArrowDown, ArrowUp, GitBranch } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']

function ticketMap(tickets: Ticket[]): Map<string, Ticket> {
  const m = new Map<string, Ticket>()
  for (const t of tickets) {
    if (t.id) m.set(t.id, t)
  }
  return m
}

const STATE_STYLES: Record<string, { className: string; label: string }> = {
  draft: {
    className: 'bg-zinc-500/15 text-zinc-400 border-zinc-500/30',
    label: 'Draft',
  },
  planning: {
    className: 'bg-blue-500/15 text-blue-300 border-blue-500/30',
    label: 'Planning',
  },
  executing: {
    className: 'bg-amber-500/15 text-amber-300 border-amber-500/30',
    label: 'Executing',
  },
  awaiting_input: {
    className: 'bg-orange-500/15 text-orange-300 border-orange-500/30',
    label: 'Awaiting Input',
  },
  awaiting_validation: {
    className: 'bg-purple-500/15 text-purple-300 border-purple-500/30',
    label: 'Validation',
  },
  validated: {
    className: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30',
    label: 'Validated',
  },
  closed: {
    className: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30',
    label: 'Closed',
  },
}

function RelationshipCard({
  orgId,
  projectId,
  ticket,
  direction,
}: {
  orgId: string
  projectId: string
  ticket: Ticket
  direction: 'depends_on' | 'blocks'
}) {
  const state = ticket.state ?? 'draft'
  const stateStyle = STATE_STYLES[state] ?? STATE_STYLES.draft

  return (
    <Link
      to={`/orgs/${orgId}/projects/${projectId}/tickets/${encodeURIComponent(ticket.id ?? '')}`}
      className={cn(
        'group flex items-start gap-3 rounded-lg border px-3 py-2.5 transition-all duration-150',
        'border-white/[0.06] bg-white/[0.02] hover:bg-white/[0.05] hover:border-white/[0.1]',
      )}
    >
      {/* Direction indicator */}
      <div className="mt-0.5 shrink-0">
        {direction === 'depends_on' ? (
          <ArrowDown className="size-3.5 text-muted-foreground/50" />
        ) : (
          <ArrowUp className="size-3.5 text-muted-foreground/50" />
        )}
      </div>

      {/* Content */}
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="text-sm font-medium text-foreground group-hover:text-foreground/90 truncate">
          {ticket.title?.trim() || ticket.id}
        </span>
        <div className="flex items-center gap-2">
          <span className="font-mono text-[10px] text-muted-foreground/50">
            {ticket.id}
          </span>
          <Badge
            variant="outline"
            className={cn('px-1.5 py-0 text-[9px]', stateStyle.className)}
          >
            {stateStyle.label}
          </Badge>
        </div>
      </div>
    </Link>
  )
}

function UnresolvedCard({
  orgId,
  projectId,
  id,
}: {
  orgId: string
  projectId: string
  id: string
}) {
  return (
    <Link
      to={`/orgs/${orgId}/projects/${projectId}/tickets/${encodeURIComponent(id)}`}
      className={cn(
        'group flex items-center gap-3 rounded-lg border px-3 py-2.5 transition-all duration-150',
        'border-white/[0.06] bg-white/[0.02] hover:bg-white/[0.05] hover:border-white/[0.1]',
      )}
    >
      <ArrowDown className="size-3.5 shrink-0 text-muted-foreground/50" />
      <span className="font-mono text-sm text-muted-foreground group-hover:text-foreground/80">
        {id}
      </span>
    </Link>
  )
}

export type TicketRelationshipsCardProps = {
  orgId: string
  projectId: string
  ticket: Ticket
  /** Resolved project ticket list; `null` means still loading. */
  projectTickets: Ticket[] | null
  projectTicketsError: string | null
}

/** Depends on + Blocks (reverse) for a single ticket. */
export function TicketRelationshipsCard({
  orgId,
  projectId,
  ticket,
  projectTickets,
  projectTicketsError,
}: TicketRelationshipsCardProps) {
  const dependsOn = ticket.depends_on ?? []
  const hasDeps = dependsOn.length > 0

  if (projectTickets === null && !projectTicketsError) {
    return (
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <GitBranch className="text-muted-foreground size-4" />
            <CardTitle className="text-sm">Relationships</CardTitle>
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-2">
            <Skeleton className="h-3 w-full" />
            <Skeleton className="h-3 w-3/4" />
          </div>
        </CardContent>
      </Card>
    )
  }

  const list = projectTickets ?? []
  const byId = ticketMap(list)
  const selfId = ticket.id
  const blocks = selfId
    ? list.filter(
        (u) =>
          u.id &&
          u.id !== selfId &&
          (u.depends_on ?? []).includes(selfId),
      )
    : []
  const hasBlocks = blocks.length > 0

  if (!hasDeps && !hasBlocks) {
    if (projectTicketsError) {
      return (
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <GitBranch className="text-muted-foreground size-4" />
              <CardTitle className="text-sm">Relationships</CardTitle>
            </div>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 text-sm">
            <p className="text-destructive">{projectTicketsError}</p>
          </CardContent>
        </Card>
      )
    }
    return null
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <GitBranch className="text-muted-foreground size-4" />
          <CardTitle className="text-sm">Relationships</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">
        {projectTicketsError ? (
          <p className="text-destructive text-sm">{projectTicketsError}</p>
        ) : null}

        {hasDeps ? (
          <div className="flex flex-col gap-2">
            <p className="flex items-center gap-1.5 text-muted-foreground text-[11px] font-medium tracking-wide uppercase">
              <ArrowDown className="size-3" />
              Depends on
            </p>
            <div className="flex flex-col gap-1.5">
              {dependsOn.map((id) => {
                const other = byId.get(id)
                if (other) {
                  return (
                    <RelationshipCard
                      key={id}
                      orgId={orgId}
                      projectId={projectId}
                      ticket={other}
                      direction="depends_on"
                    />
                  )
                }
                return (
                  <UnresolvedCard
                    key={id}
                    orgId={orgId}
                    projectId={projectId}
                    id={id}
                  />
                )
              })}
            </div>
          </div>
        ) : null}

        {hasBlocks ? (
          <div className="flex flex-col gap-2">
            <p className="flex items-center gap-1.5 text-muted-foreground text-[11px] font-medium tracking-wide uppercase">
              <ArrowUp className="size-3" />
              Blocks
            </p>
            <div className="flex flex-col gap-1.5">
              {blocks.map((t) => (
                <RelationshipCard
                  key={t.id}
                  orgId={orgId}
                  projectId={projectId}
                  ticket={t}
                  direction="blocks"
                />
              ))}
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
