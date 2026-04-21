import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  ClipboardList,
  Code,
  Bug,
  Lightbulb,
  FileText,
  Wrench,
  HelpCircle,
  GitBranch,
  Clock,
  Play,
  Eye,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  PauseCircle,
  Inbox,
  ArrowRight,
  Search,
} from 'lucide-react'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { Badge } from '@/components/ui/badge'
import { Card, CardHeader, CardTitle } from '@/components/ui/card'
import { StyledSelect } from '@/components/ui/styled-select'
import { useAuth } from '@/contexts/use-auth'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { formatApiError } from '@/lib/api/client'
import { cn } from '@/lib/utils'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']
type TicketState = NonNullable<Ticket['state']>
type WorkStream = components['schemas']['WorkStream']

// ---------------------------------------------------------------------------
// Constants & helpers
// ---------------------------------------------------------------------------

const ALL_STATES = [
  'draft',
  'specced',
  'planning',
  'executing',
  'awaiting_validation',
  'validated',
  'deploying',
  'observing',
  'closed',
  'awaiting_input',
  // Legacy states kept for compatibility
  'pending',
  'claimed',
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

const CATEGORY_OPTIONS: { id: CategoryId; label: string }[] = [
  { id: 'open', label: 'Open' },
  { id: 'all', label: 'All tickets' },
  { id: 'backlog', label: 'Backlog' },
  { id: 'in_progress', label: 'In progress' },
  { id: 'awaiting_review', label: 'Awaiting review' },
  { id: 'done', label: 'Done' },
  { id: 'blocked', label: 'Blocked / needs input' },
]

const DONE_STATES: TicketState[] = ['done', 'closed']

/** Everything except done states — default "open work" view. */
const OPEN_STATES = ALL_STATES.filter((s) => !DONE_STATES.includes(s))

function statesInCategory(cat: CategoryId): readonly TicketState[] | null {
  switch (cat) {
    case 'open':
      return OPEN_STATES
    case 'all':
      return null
    case 'backlog':
      return ['pending', 'draft', 'specced']
    case 'in_progress':
      return ['claimed', 'executing', 'planning', 'deploying', 'observing']
    case 'awaiting_review':
      return ['awaiting_review', 'awaiting_validation', 'validated']
    case 'done':
      return ['done', 'closed']
    case 'blocked':
      return ['blocked', 'needs_human', 'failed', 'awaiting_input']
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

// ---------------------------------------------------------------------------
// State color system (semantic)
// ---------------------------------------------------------------------------

type StateColorKey =
  | 'green'   // done/closed/validated
  | 'amber'   // review/awaiting
  | 'red'     // blocked/failed
  | 'blue'    // executing/in-progress
  | 'purple'  // planning
  | 'gray'    // draft/pending

function stateColorKey(state: TicketState | undefined): StateColorKey {
  if (!state) return 'gray'
  switch (state) {
    case 'done':
    case 'closed':
    case 'validated':
      return 'green'
    case 'awaiting_review':
    case 'awaiting_validation':
    case 'awaiting_input':
    case 'observing':
      return 'amber'
    case 'blocked':
    case 'needs_human':
    case 'failed':
      return 'red'
    case 'executing':
    case 'claimed':
    case 'deploying':
      return 'blue'
    case 'planning':
    case 'specced':
      return 'purple'
    case 'pending':
    case 'draft':
    default:
      return 'gray'
  }
}

const STATE_BORDER_CLASSES: Record<StateColorKey, string> = {
  green: 'border-l-green-500',
  amber: 'border-l-amber-500',
  red: 'border-l-red-500',
  blue: 'border-l-blue-500',
  purple: 'border-l-purple-500',
  gray: 'border-l-white/20',
}

const STATE_BADGE_CLASSES: Record<StateColorKey, string> = {
  green: 'bg-green-500/15 text-green-400 border-green-500/30',
  amber: 'bg-amber-500/15 text-amber-400 border-amber-500/30',
  red: 'bg-red-500/15 text-red-400 border-red-500/30',
  blue: 'bg-blue-500/15 text-blue-400 border-blue-500/30',
  purple: 'bg-purple-500/15 text-purple-400 border-purple-500/30',
  gray: 'bg-white/5 text-muted-foreground border-white/10',
}

// ---------------------------------------------------------------------------
// State icons
// ---------------------------------------------------------------------------

function StateIcon({ state, className }: { state: TicketState | undefined; className?: string }) {
  const cls = cn('size-4', className)
  switch (state) {
    case 'draft':
    case 'pending':
      return <Clock className={cls} />
    case 'specced':
      return <FileText className={cls} />
    case 'planning':
      return <Lightbulb className={cls} />
    case 'executing':
    case 'claimed':
      return <Play className={cls} />
    case 'awaiting_review':
    case 'awaiting_validation':
      return <Eye className={cls} />
    case 'validated':
    case 'done':
    case 'closed':
      return <CheckCircle2 className={cls} />
    case 'deploying':
      return <ArrowRight className={cls} />
    case 'observing':
      return <Search className={cls} />
    case 'blocked':
    case 'failed':
      return <XCircle className={cls} />
    case 'needs_human':
    case 'awaiting_input':
      return <AlertTriangle className={cls} />
    default:
      return <PauseCircle className={cls} />
  }
}

// ---------------------------------------------------------------------------
// Ticket type icons
// ---------------------------------------------------------------------------

function TypeIcon({ type, className }: { type: string | undefined; className?: string }) {
  const cls = cn('size-3.5', className)
  switch (type) {
    case 'task':
      return <ClipboardList className={cls} />
    case 'feature':
      return <Lightbulb className={cls} />
    case 'bug':
      return <Bug className={cls} />
    case 'chore':
      return <Wrench className={cls} />
    case 'investigation':
    case 'spike':
      return <Search className={cls} />
    case 'refactor':
      return <Code className={cls} />
    case 'docs':
    case 'documentation':
      return <FileText className={cls} />
    default:
      return <HelpCircle className={cls} />
  }
}

// ---------------------------------------------------------------------------
// Empty state component
// ---------------------------------------------------------------------------

function EmptyState({
  title,
  description,
  icon: Icon,
}: {
  title: string
  description: string
  icon: React.ElementType
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-4 rounded-xl border border-white/5 bg-white/[0.02] py-16 px-8 text-center">
      <div className="flex size-14 items-center justify-center rounded-full bg-white/5">
        <Icon className="size-7 text-muted-foreground" />
      </div>
      <div className="flex flex-col gap-1.5">
        <h3 className="text-sm font-medium text-foreground">{title}</h3>
        <p className="max-w-sm text-sm text-muted-foreground">{description}</p>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Ticket card component
// ---------------------------------------------------------------------------

interface TicketCardProps {
  ticket: Ticket
  orgId: string
  projectId: string
  streamLabel?: string
  depLabels: Map<string, string>
  isSelected: boolean
  onMouseEnter?: () => void
}

function TicketCard({
  ticket: t,
  orgId,
  projectId,
  streamLabel,
  depLabels,
  isSelected,
  onMouseEnter,
}: TicketCardProps) {
  const colorKey = stateColorKey(t.state ?? undefined)
  const stateLabel = t.state?.replace(/_/g, ' ') ?? 'unknown'

  return (
    <Link
      to={`/orgs/${orgId}/projects/${projectId}/tickets/${t.id}`}
      data-ticket-id={t.id}
      onMouseEnter={onMouseEnter}
    >
      <Card
        className={cn(
          'relative overflow-hidden border-l-[3px] transition-all duration-200',
          STATE_BORDER_CLASSES[colorKey],
          isSelected
            ? 'ring-2 ring-green-500/40 bg-white/[0.08]'
            : 'hover:bg-white/[0.06]',
        )}
      >
        <CardHeader className="py-3">
          <div className="flex items-start gap-3">
            {/* State icon */}
            <div
              className={cn(
                'mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md',
                STATE_BADGE_CLASSES[colorKey],
              )}
            >
              <StateIcon state={t.state ?? undefined} />
            </div>

            {/* Content */}
            <div className="flex min-w-0 flex-1 flex-col gap-1.5">
              <div className="flex items-center gap-2">
                <CardTitle className="truncate text-sm">{t.title ?? t.id}</CardTitle>
              </div>

              <div className="flex flex-wrap items-center gap-1.5">
                {/* State badge */}
                <span
                  className={cn(
                    'inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-medium capitalize',
                    STATE_BADGE_CLASSES[colorKey],
                  )}
                >
                  {stateLabel}
                </span>

                {/* Type badge */}
                {t.type ? (
                  <span className="inline-flex items-center gap-1 rounded-md border border-white/10 bg-white/5 px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
                    <TypeIcon type={t.type} className="size-3" />
                    {t.type}
                  </span>
                ) : null}

                {/* Stream badge */}
                {streamLabel ? (
                  <span className="inline-flex items-center gap-1 rounded-md border border-white/10 bg-white/5 px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
                    {streamLabel}
                  </span>
                ) : null}

                {/* Dependencies badge */}
                {t.depends_on && t.depends_on.filter(Boolean).length > 0 ? (
                  <span
                    className="inline-flex items-center gap-1 rounded-md border border-white/10 bg-white/5 px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground"
                    title={t.depends_on
                      .filter(Boolean)
                      .map((id) => depLabels.get(id) ?? id)
                      .join(', ')}
                  >
                    <GitBranch className="size-3" />
                    {t.depends_on.filter(Boolean).length} dep
                    {t.depends_on.filter(Boolean).length === 1 ? '' : 's'}
                  </span>
                ) : null}

                {/* Priority badge */}
                {t.priority !== undefined && t.priority !== null ? (
                  <span
                    className={cn(
                      'inline-flex items-center rounded-md border px-1.5 py-0.5 text-[11px] font-medium',
                      t.priority === 0
                        ? 'border-red-500/30 bg-red-500/15 text-red-400'
                        : t.priority === 1
                          ? 'border-amber-500/30 bg-amber-500/15 text-amber-400'
                          : 'border-white/10 bg-white/5 text-muted-foreground',
                    )}
                  >
                    P{t.priority}
                  </span>
                ) : null}
              </div>

              {/* Ticket ID */}
              <span className="font-mono text-[11px] text-muted-foreground/60">{t.id}</span>
            </div>
          </div>
        </CardHeader>
      </Card>
    </Link>
  )
}

// ---------------------------------------------------------------------------
// Main page component
// ---------------------------------------------------------------------------

export function TicketsPage() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const navigate = useNavigate()
  const { client } = useAuth()
  const workStreamFilter = searchParams.get('work_stream_id') ?? ''
  const [allTickets, setAllTickets] = useState<Ticket[] | null>(null)
  const [streams, setStreams] = useState<WorkStream[] | null>(null)
  const [streamsErr, setStreamsErr] = useState<string | null>(null)
  const [category, setCategory] = useState<CategoryId>('open')
  const [specificState, setSpecificState] = useState<TicketState | ''>('')
  const [err, setErr] = useState<string | null>(null)
  const [selectedIndex, setSelectedIndex] = useState(-1)
  const listRef = useRef<HTMLUListElement>(null)
  const projectLabel = useProjectBreadcrumbLabel(projectId)

  // ---- Fetch work streams ----
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

  // ---- Validate work stream filter ----
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

  // ---- Fetch tickets ----
  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    ;(async () => {
      const { data, error, response } = await client.GET('/projects/{projectID}/tickets', {
        params: {
          path: { projectID: projectId },
          query: workStreamFilter ? { work_stream_id: workStreamFilter } : {},
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

  // ---- Derived data ----
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

  // ---- Filter helpers ----
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

  // ---- Select options for styled selects ----
  const workStreamOptions = useMemo(() => {
    const opts = [{ value: '', label: 'All work streams' }]
    for (const s of streams ?? []) {
      if (s.id) {
        opts.push({ value: s.id, label: s.name ?? s.slug ?? s.id })
      }
    }
    return opts
  }, [streams])

  const categoryOptions = useMemo(
    () => CATEGORY_OPTIONS.map((o) => ({ value: o.id, label: o.label })),
    [],
  )

  const refineSelectOptions = useMemo(() => {
    const opts = [
      {
        value: '',
        label: category === 'all' ? 'Any state' : 'Any in this view',
      },
    ]
    for (const s of refineOptions) {
      opts.push({ value: s, label: s.replace(/_/g, ' ') })
    }
    return opts
  }, [category, refineOptions])

  // ---- Keyboard navigation ----
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (!tickets || tickets.length === 0) return

      // Don't capture keystrokes when a select/input is focused
      const tag = (e.target as HTMLElement)?.tagName?.toLowerCase()
      if (tag === 'input' || tag === 'select' || tag === 'textarea') return
      // Don't capture when inside a radix select portal
      if ((e.target as HTMLElement)?.closest('[data-radix-select-content]')) return

      switch (e.key) {
        case 'j': {
          e.preventDefault()
          setSelectedIndex((prev) => {
            const next = Math.min(prev + 1, tickets.length - 1)
            scrollToIndex(next)
            return next
          })
          break
        }
        case 'k': {
          e.preventDefault()
          setSelectedIndex((prev) => {
            const next = Math.max(prev - 1, 0)
            scrollToIndex(next)
            return next
          })
          break
        }
        case 'Enter': {
          if (selectedIndex >= 0 && selectedIndex < tickets.length) {
            e.preventDefault()
            const t = tickets[selectedIndex]
            if (t.id && orgId && projectId) {
              navigate(`/orgs/${orgId}/projects/${projectId}/tickets/${t.id}`)
            }
          }
          break
        }
      }
    },
    [tickets, selectedIndex, orgId, projectId, navigate],
  )

  function scrollToIndex(index: number) {
    const list = listRef.current
    if (!list) return
    const items = list.querySelectorAll('[data-ticket-id]')
    items[index]?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  }

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [handleKeyDown])

  // Reset selection on filter change
  useEffect(() => {
    setSelectedIndex(-1)
  }, [category, specificState, workStreamFilter])

  // ---- Guard clauses ----
  if (!orgId || !projectId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }
  if (err) {
    return <p className="text-destructive text-sm">{err}</p>
  }
  if (!tickets) {
    return (
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <p className="text-muted-foreground text-xs">
            <OrgProjectCrumbs orgId={orgId} projectId={projectId} projectLabel={projectLabel} />
            <span className="px-1">/</span>
            <span className="text-foreground" aria-current="page">Tickets</span>
          </p>
          <h1 className="text-xl font-semibold tracking-tight">Tickets</h1>
        </div>
        {/* Skeleton loading state */}
        <div className="flex gap-3">
          {[1, 2, 3].map((i) => (
            <div key={i} className="h-8 w-40 animate-pulse rounded-lg bg-white/5" />
          ))}
        </div>
        <div className="flex flex-col gap-3">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="h-20 animate-pulse rounded-xl border border-white/5 bg-white/[0.02]" />
          ))}
        </div>
      </div>
    )
  }

  // ---- Render ----
  return (
    <div className="flex flex-col gap-5">
      {/* Header */}
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
        <div className="flex items-baseline gap-3">
          <h1 className="text-xl font-semibold tracking-tight">Tickets</h1>
          {tickets.length > 0 ? (
            <span className="text-sm text-muted-foreground">{tickets.length} ticket{tickets.length === 1 ? '' : 's'}</span>
          ) : null}
        </div>
      </div>

      {streamsErr ? (
        <p className="text-destructive text-sm">{streamsErr}</p>
      ) : null}

      {/* Filter bar */}
      <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-end">
        <div className="flex flex-col gap-1.5">
          <label className="text-muted-foreground text-xs font-medium" htmlFor="work-stream-filter">
            Work stream
          </label>
          <StyledSelect
            id="work-stream-filter"
            value={workStreamFilter}
            onValueChange={setWorkStreamFilter}
            options={workStreamOptions}
            placeholder="All work streams"
            disabled={!streams}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <label className="text-muted-foreground text-xs font-medium" htmlFor="category-filter">
            View
          </label>
          <StyledSelect
            id="category-filter"
            value={category}
            onValueChange={(val) => {
              const next = val as CategoryId
              setCategory(next)
              const allowed = statesInCategory(next)
              setSpecificState((prev) => {
                if (!prev) return ''
                if (allowed && !(allowed as readonly string[]).includes(prev)) return ''
                return prev
              })
            }}
            options={categoryOptions}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <label className="text-muted-foreground text-xs font-medium" htmlFor="state-refine">
            Refine by state
          </label>
          <StyledSelect
            id="state-refine"
            value={specificState}
            onValueChange={(val) => setSpecificState(val as TicketState | '')}
            options={refineSelectOptions}
          />
        </div>
      </div>

      {workStreamFilter ? (
        <p className="text-muted-foreground max-w-2xl text-sm">
          Only tickets assigned to this work stream are listed. Choose{' '}
          <span className="font-medium text-foreground/80">All work streams</span> to see every
          ticket.
        </p>
      ) : null}

      {/* Keyboard hint */}
      {tickets.length > 0 ? (
        <div className="flex items-center gap-2 text-[11px] text-muted-foreground/50">
          <span className="rounded border border-white/10 bg-white/5 px-1.5 py-0.5 font-mono">j</span>
          <span className="rounded border border-white/10 bg-white/5 px-1.5 py-0.5 font-mono">k</span>
          <span>to navigate</span>
          <span className="rounded border border-white/10 bg-white/5 px-1.5 py-0.5 font-mono">↵</span>
          <span>to open</span>
        </div>
      ) : null}

      {/* Ticket list */}
      {tickets.length > 0 ? (
        <ul ref={listRef} className="flex flex-col gap-2" role="listbox" aria-label="Tickets">
          {tickets.map((t, idx) => (
            <li key={t.id} role="option" aria-selected={idx === selectedIndex}>
              <TicketCard
                ticket={t}
                orgId={orgId}
                projectId={projectId}
                streamLabel={t.work_stream_id ? streamLabelById.get(t.work_stream_id) : undefined}
                depLabels={ticketTitleById}
                isSelected={idx === selectedIndex}
                onMouseEnter={() => setSelectedIndex(idx)}
              />
            </li>
          ))}
        </ul>
      ) : (
        <>
          {allTickets?.length === 0 ? (
            workStreamFilter ? (
              <EmptyState
                icon={Inbox}
                title="No tickets yet"
                description="This work stream doesn't have any tickets. Create one to get started."
              />
            ) : (
              <EmptyState
                icon={Inbox}
                title="No tickets in this project"
                description="Create your first ticket to start tracking work."
              />
            )
          ) : category === 'open' && allTickets?.every((t) => DONE_STATES.includes(t.state as TicketState)) ? (
            <EmptyState
              icon={CheckCircle2}
              title="All caught up!"
              description='No open tickets — all work is done. Switch View to "All tickets" or "Done" to see completed work.'
            />
          ) : (
            <EmptyState
              icon={Search}
              title="No matching tickets"
              description={'Try "All tickets" or change the filters above to find what you\u2019re looking for.'}
            />
          )}
        </>
      )}
    </div>
  )
}
