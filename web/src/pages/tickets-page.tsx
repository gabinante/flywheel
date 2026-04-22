import { useEffect, useMemo, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { StaggerItem, StaggerList } from '@/components/stagger-list'
import { Badge } from '@/components/ui/badge'
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { TicketsPageSkeleton } from '@/components/ui/skeleton'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']
type TicketState = NonNullable<Ticket['state']>
type WorkStream = components['schemas']['WorkStream']

const ALL_STATES = [
  'pending',
  'claimed',
  'executing',
  'awaiting_review',
  'done',
  'blocked',
  'needs_human',
  'failed',
] as const satisfies readonly TicketState[]

type CategoryId =
  | 'open'
  | 'all'
  | 'backlog'
  | 'in_progress'
  | 'awaiting_review'
  | 'done'
  | 'blocked'

/** Everything except `done` — default “open work” view. */
const OPEN_STATES = ALL_STATES.filter((s) => s !== 'done')

const CATEGORY_OPTIONS: { id: CategoryId; label: string }[] = [
  { id: 'open', label: 'Open' },
  { id: 'all', label: 'All tickets' },
  { id: 'backlog', label: 'Backlog' },
  { id: 'in_progress', label: 'In progress' },
  { id: 'awaiting_review', label: 'Awaiting review' },
  { id: 'done', label: 'Done' },
  { id: 'blocked', label: 'Blocked / needs input' },
]

/** States included in each category; `all` means no category filter. */
function statesInCategory(cat: CategoryId): readonly TicketState[] | null {
  switch (cat) {
    case 'open':
      return OPEN_STATES
    case 'all':
      return null
    case 'backlog':
      return ['pending']
    case 'in_progress':
      return ['claimed', 'executing']
    case 'awaiting_review':
      return ['awaiting_review']
    case 'done':
      return ['done']
    case 'blocked':
      return ['blocked', 'needs_human', 'failed']
    default:
      return null
  }
}

function filterTickets(
  all: Ticket[],
  category: CategoryId,
  specificState: TicketState | '',
): Ticket[] {
  const inCategory = statesInCategory(category)
  let list = all
  if (inCategory) {
    const set = new Set(inCategory)
    list = list.filter((t) => t.state && set.has(t.state))
  }
  if (specificState) {
    list = list.filter((t) => t.state === specificState)
  }
  return list
}

export function TicketsPage() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const { client } = useAuth()
  const workStreamFilter = searchParams.get('work_stream_id') ?? ''
  const [allTickets, setAllTickets] = useState<Ticket[] | null>(null)
  const [streams, setStreams] = useState<WorkStream[] | null>(null)
  const [streamsErr, setStreamsErr] = useState<string | null>(null)
  const [category, setCategory] = useState<CategoryId>('open')
  const [specificState, setSpecificState] = useState<TicketState | ''>('')
  const [err, setErr] = useState<string | null>(null)
  const projectLabel = useProjectBreadcrumbLabel(projectId)

  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    ;(async () => {
      const { data, error, response } = await client.GET(
        '/projects/{projectID}/work-streams',
        {
          params: {
            path: { projectID: projectId },
            query: { status: 'all' },
          },
        },
      )
      if (cancelled) return
      if (!response.ok) {
        setStreamsErr(formatApiError(error))
        setStreams([])
        return
      }
      setStreamsErr(null)
      setStreams(data ?? [])
    })()
    return () => {
      cancelled = true
    }
  }, [client, projectId])

  useEffect(() => {
    if (!streams || workStreamFilter === '') return
    const ok = streams.some((s) => s.id === workStreamFilter)
    if (!ok) {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev)
          next.delete('work_stream_id')
          return next
        },
        { replace: true },
      )
    }
  }, [streams, workStreamFilter, setSearchParams])

  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    ;(async () => {
      const { data, error, response } = await client.GET('/projects/{projectID}/tickets', {
        params: {
          path: { projectID: projectId },
          query: workStreamFilter
            ? { work_stream_id: workStreamFilter }
            : {},
        },
      })
      if (cancelled) return
      if (!response.ok) {
        setErr(formatApiError(error))
        setAllTickets([])
        return
      }
      setErr(null)
      setAllTickets(data ?? [])
    })()
    return () => {
      cancelled = true
    }
  }, [client, projectId, workStreamFilter])

  const refineOptions = useMemo((): readonly TicketState[] => {
    if (category === 'all') return ALL_STATES
    const allowed = statesInCategory(category)
    return allowed ?? ALL_STATES
  }, [category])

  const tickets = useMemo(() => {
    if (!allTickets) return null
    return filterTickets(allTickets, category, specificState)
  }, [allTickets, category, specificState])

  const streamLabelById = useMemo(() => {
    const m = new Map<string, string>()
    for (const s of streams ?? []) {
      if (s.id) m.set(s.id, s.name ?? s.slug ?? s.id)
    }
    return m
  }, [streams])

  const ticketTitleById = useMemo(() => {
    const m = new Map<string, string>()
    for (const t of allTickets ?? []) {
      if (t.id) m.set(t.id, t.title?.trim() || t.id)
    }
    return m
  }, [allTickets])

  function setWorkStreamFilter(id: string) {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        if (id) next.set('work_stream_id', id)
        else next.delete('work_stream_id')
        return next
      },
      { replace: true },
    )
  }

  if (!orgId || !projectId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }
  if (err) {
    return <p className="text-destructive text-sm">{err}</p>
  }
  if (!tickets) {
    return <TicketsPageSkeleton />
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <p className="text-muted-foreground text-xs">
          <OrgProjectCrumbs
            orgId={orgId}
            projectId={projectId}
            projectLabel={projectLabel}
          />
          {workStreamFilter ? (
            <>
              <span className="px-1">/</span>
              <Link
                to={`/orgs/${orgId}/projects/${projectId}/tickets?work_stream_id=${encodeURIComponent(workStreamFilter)}`}
                className="hover:underline"
              >
                {streamLabelById.get(workStreamFilter) ?? 'Work stream'}
              </Link>
            </>
          ) : null}
          <span className="px-1">/</span>
          <span className="text-foreground" aria-current="page">
            Tickets
          </span>
        </p>
        <h1 className="text-xl font-semibold tracking-tight">Tickets</h1>
      </div>

      {streamsErr ? (
        <p className="text-destructive text-sm">{streamsErr}</p>
      ) : null}

      <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-end">
        <div className="flex flex-col gap-1">
          <label className="text-muted-foreground text-sm" htmlFor="work-stream-filter">
            Work stream
          </label>
          <select
            id="work-stream-filter"
            className="border-input bg-background h-8 min-w-[12rem] rounded-md border px-2 text-sm"
            value={workStreamFilter}
            onChange={(e) => setWorkStreamFilter(e.target.value)}
            disabled={!streams}
          >
            <option value="">All work streams</option>
            {(streams ?? []).filter((s) => s.id).map((s) => (
              <option key={s.id} value={s.id}>
                {s.name ?? s.slug ?? s.id}
              </option>
            ))}
          </select>
        </div>
        <div className="flex flex-col gap-1">
          <label className="text-muted-foreground text-sm" htmlFor="category-filter">
            View
          </label>
          <select
            id="category-filter"
            className="border-input bg-background h-8 min-w-[12rem] rounded-md border px-2 text-sm"
            value={category}
            onChange={(e) => {
              const next = e.target.value as CategoryId
              setCategory(next)
              const allowed = statesInCategory(next)
              setSpecificState((prev) => {
                if (!prev) return ''
                if (allowed && !allowed.includes(prev)) return ''
                return prev
              })
            }}
          >
            {CATEGORY_OPTIONS.map((o) => (
              <option key={o.id} value={o.id}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
        <div className="flex flex-col gap-1">
          <label className="text-muted-foreground text-sm" htmlFor="state-refine">
            Refine by state
          </label>
          <select
            id="state-refine"
            className="border-input bg-background h-8 min-w-[12rem] rounded-md border px-2 text-sm"
            value={specificState}
            onChange={(e) =>
              setSpecificState(e.target.value as TicketState | '')
            }
          >
            <option value="">
              {category === 'all' ? 'Any state' : 'Any in this view'}
            </option>
            {refineOptions.map((s) => (
              <option key={s} value={s}>
                {s.replace(/_/g, ' ')}
              </option>
            ))}
          </select>
        </div>
      </div>

      {workStreamFilter ? (
        <p className="text-muted-foreground max-w-2xl text-sm">
          Only tickets assigned to this work stream are listed. Tickets created without a work stream (or assigned to
          another) are hidden here—choose <span className="font-medium">All work streams</span> to see every ticket in
          the project.
        </p>
      ) : null}

      <StaggerList className="flex flex-col gap-3">
        {tickets.map((t) => (
          <StaggerItem key={t.id}>
            <Link
              to={`/orgs/${orgId}/projects/${projectId}/tickets/${t.id}`}
            >
              <Card className="transition-colors hover:bg-white/[0.06]">
                <CardHeader>
                  <div className="flex flex-wrap items-center gap-2">
                    <CardTitle className="text-base">{t.title ?? t.id}</CardTitle>
                    {t.state ? <Badge variant="secondary">{t.state}</Badge> : null}
                    {t.type ? (
                      <Badge variant="outline">{t.type}</Badge>
                    ) : null}
                    {t.work_stream_id ? (
                      <Badge variant="outline" title={t.work_stream_id}>
                        {streamLabelById.get(t.work_stream_id) ?? 'Stream'}
                      </Badge>
                    ) : null}
                    {t.depends_on && t.depends_on.length > 0 ? (
                      <Badge
                        variant="outline"
                        title={t.depends_on
                          .map((id) => ticketTitleById.get(id) ?? id)
                          .join(', ')}
                      >
                        {t.depends_on.length} dep
                        {t.depends_on.length === 1 ? '' : 's'}
                      </Badge>
                    ) : null}
                  </div>
                  <CardDescription className="font-mono text-xs">
                    {t.id}
                  </CardDescription>
                </CardHeader>
              </Card>
            </Link>
          </StaggerItem>
        ))}
      </StaggerList>
      {tickets.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {allTickets?.length === 0
            ? workStreamFilter
              ? 'No tickets in this work stream.'
              : 'No tickets in this project yet.'
            : category === 'open' &&
                allTickets?.every((t) => t.state === 'done')
              ? 'No open tickets in this work stream (the Open view hides completed work). Switch View to “All tickets” or “Done” above.'
              : 'No tickets match this view. Try “All tickets” or change the filters above.'}
        </p>
      ) : null}
    </div>
  )
}
