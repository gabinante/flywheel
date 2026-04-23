import { Link } from 'react-router-dom'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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

export type TicketRelationshipsCardProps = {
  orgId: string
  projectId: string
  ticket: Ticket
  /** Resolved project ticket list; `null` means still loading. */
  projectTickets: Ticket[] | null
  projectTicketsError: string | null
}

function DepRow({
  orgId,
  projectId,
  id,
  label,
}: {
  orgId: string
  projectId: string
  id: string
  label: string
}) {
  return (
    <li>
      <Link
        className="text-primary hover:underline"
        to={`/orgs/${orgId}/projects/${projectId}/tickets/${encodeURIComponent(id)}`}
      >
        {label}
      </Link>
      <span className="text-muted-foreground font-mono text-xs"> · {id}</span>
    </li>
  )
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
          <CardTitle className="text-sm">Dependencies</CardTitle>
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
            <CardTitle className="text-sm">Dependencies</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 text-sm">
            <p className="text-destructive">{projectTicketsError}</p>
            <p className="text-muted-foreground text-sm">
              Could not load project tickets to verify blocking relationships.
            </p>
          </CardContent>
        </Card>
      )
    }
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Dependencies</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-muted-foreground text-sm">No ticket dependencies.</p>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Dependencies</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4 text-sm">
        {projectTicketsError ? (
          <p className="text-destructive text-sm">{projectTicketsError}</p>
        ) : null}
        {hasDeps ? (
          <div className="flex flex-col gap-2">
            <p className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
              Depends on
            </p>
            <ul className="list-inside list-disc space-y-1">
              {dependsOn.map((id) => {
                const other = byId.get(id)
                const label = other?.title?.trim() || id
                return (
                  <DepRow
                    key={id}
                    orgId={orgId}
                    projectId={projectId}
                    id={id}
                    label={label}
                  />
                )
              })}
            </ul>
          </div>
        ) : (
          <p className="text-muted-foreground text-sm">
            This ticket does not depend on other tickets.
          </p>
        )}
        {hasBlocks ? (
          <div className="flex flex-col gap-2">
            <p className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
              Blocks
            </p>
            <p className="text-muted-foreground text-xs">
              Other tickets that list this ticket in &quot;depends on&quot;:
            </p>
            <ul className="list-inside list-disc space-y-1">
              {blocks.map((t) => (
                <DepRow
                  key={t.id}
                  orgId={orgId}
                  projectId={projectId}
                  id={t.id!}
                  label={t.title?.trim() || t.id!}
                />
              ))}
            </ul>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
