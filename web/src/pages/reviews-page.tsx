import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import {
  CheckCircle2,
  ChevronDown,
  ExternalLink,
  GitPullRequest,
  Layers,
  XCircle,
  RotateCcw,
} from 'lucide-react'

import { KeyboardShortcutHelp } from '@/components/keyboard-shortcut-help'
import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { ReviewQueueCelebration } from '@/components/review-queue-celebration'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { FlywheelClient } from '@/contexts/api-context'
import { useAPI } from '@/contexts/use-api'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { ReviewsPageSkeleton, Skeleton } from '@/components/ui/skeleton'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']
type TraceStep = components['schemas']['TraceStep']

/* ------------------------------------------------------------------ */
/* Data helpers                                                        */
/* ------------------------------------------------------------------ */

async function loadPendingReviews(
  client: FlywheelClient,
  projectId: string,
): Promise<
  { ok: true; tickets: Ticket[] } | { ok: false; message: string }
> {
  const { data, error, response } = await client.GET(
    '/projects/{projectID}/reviews',
    { params: { path: { projectID: projectId } } },
  )
  if (!response.ok) {
    return { ok: false, message: formatApiError(error) }
  }
  return { ok: true, tickets: data?.tickets ?? [] }
}

async function loadTrace(
  client: FlywheelClient,
  ticketId: string,
): Promise<TraceStep[] | null> {
  const { data, response } = await client.GET('/tickets/{ticketID}/trace', {
    params: { path: { ticketID: ticketId } },
  })
  if (!response.ok) return null
  return data?.steps ?? []
}

/* ------------------------------------------------------------------ */
/* Priority helpers                                                    */
/* ------------------------------------------------------------------ */

const PRIORITY_META: Record<
  number,
  { label: string; color: string; ring: string; bg: string }
> = {
  0: {
    label: 'P0 Critical',
    color: 'text-red-400',
    ring: 'ring-red-500/40',
    bg: 'bg-red-500/10',
  },
  1: {
    label: 'P1 High',
    color: 'text-amber-400',
    ring: 'ring-amber-500/40',
    bg: 'bg-amber-500/10',
  },
  2: {
    label: 'P2 Normal',
    color: 'text-emerald-400',
    ring: 'ring-emerald-500/30',
    bg: 'bg-emerald-500/8',
  },
  3: {
    label: 'P3 Low',
    color: 'text-slate-400',
    ring: 'ring-slate-500/20',
    bg: 'bg-slate-500/5',
  },
}

function getPriorityMeta(priority: number | undefined) {
  return PRIORITY_META[priority ?? 2] ?? PRIORITY_META[2]
}

/** Sort tickets by priority (P0 first) then by creation date (oldest first). */
function sortByPriority(tickets: Ticket[]): Ticket[] {
  return [...tickets].sort((a, b) => {
    const pa = a.priority ?? 2
    const pb = b.priority ?? 2
    if (pa !== pb) return pa - pb
    // same priority — earlier created first
    const ta = a.created_at ?? ''
    const tb = b.created_at ?? ''
    return ta.localeCompare(tb)
  })
}

/* ------------------------------------------------------------------ */
/* Subcomponents                                                       */
/* ------------------------------------------------------------------ */

function TraceSummary({ steps }: { steps: TraceStep[] | null }) {
  if (!steps || steps.length === 0) {
    return (
      <p className="text-muted-foreground text-xs italic">
        No execution trace available.
      </p>
    )
  }
  const counts = { tool_call: 0, observation: 0, thought: 0, error: 0 }
  for (const s of steps) {
    if (s.type && s.type in counts) {
      counts[s.type as keyof typeof counts]++
    }
  }
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex flex-wrap gap-2 text-xs">
        {counts.tool_call > 0 && (
          <span className="rounded-md bg-white/5 px-2 py-0.5 text-emerald-300/80">
            {counts.tool_call} tool call{counts.tool_call !== 1 ? 's' : ''}
          </span>
        )}
        {counts.observation > 0 && (
          <span className="rounded-md bg-white/5 px-2 py-0.5 text-blue-300/80">
            {counts.observation} observation
            {counts.observation !== 1 ? 's' : ''}
          </span>
        )}
        {counts.thought > 0 && (
          <span className="rounded-md bg-white/5 px-2 py-0.5 text-purple-300/80">
            {counts.thought} thought{counts.thought !== 1 ? 's' : ''}
          </span>
        )}
        {counts.error > 0 && (
          <span className="rounded-md bg-red-500/10 px-2 py-0.5 text-red-400">
            {counts.error} error{counts.error !== 1 ? 's' : ''}
          </span>
        )}
      </div>
      <ul className="flex flex-col gap-0.5">
        {steps.slice(-3).map((step, i) => (
          <li
            key={step.id ?? i}
            className="text-muted-foreground truncate text-xs"
          >
            <span className="font-mono text-foreground/60">{step.type}</span>
            {step.payload && Object.keys(step.payload).length > 0 ? (
              <span className="ml-1.5 opacity-70">
                {summarizePayload(step.payload)}
              </span>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  )
}

function summarizePayload(payload: Record<string, never>): string {
  const p = payload as unknown as Record<string, unknown>
  if ('message' in p && typeof p.message === 'string') {
    return p.message.slice(0, 80)
  }
  if ('name' in p && typeof p.name === 'string') {
    return p.name
  }
  const keys = Object.keys(p)
  if (keys.length === 0) return ''
  return keys.slice(0, 3).join(', ')
}

function OutputsSummary({ outputs }: { outputs: unknown }) {
  if (!outputs || typeof outputs !== 'object') return null
  const o = outputs as Record<string, unknown>
  const keys = Object.keys(o)
  if (keys.length === 0) return null

  const summary = typeof o.summary === 'string' ? o.summary.trim() : null
  const prUrl = typeof o.pr_url === 'string' ? o.pr_url : null
  const filesChanged = Array.isArray(o.files_changed) ? o.files_changed : null

  return (
    <div className="flex flex-col gap-2">
      {summary && (
        <p className="text-sm leading-relaxed text-foreground/80">{summary}</p>
      )}
      {prUrl && (
        <a
          href={prUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="group inline-flex items-center gap-2 rounded-lg bg-white/5 px-3 py-2 text-sm text-emerald-400 backdrop-blur-sm transition-colors hover:bg-white/10 hover:text-emerald-300"
        >
          <GitPullRequest className="size-4" />
          <span className="truncate">{prUrl.replace('https://github.com/', '')}</span>
          <ExternalLink className="ml-auto size-3 opacity-50 transition-opacity group-hover:opacity-100" />
        </a>
      )}
      {filesChanged && filesChanged.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {filesChanged.length} file{filesChanged.length !== 1 ? 's' : ''} changed
        </p>
      )}
      {!summary && !prUrl && (
        <pre className="max-h-24 overflow-auto rounded-lg bg-white/5 p-3 font-mono text-xs text-foreground/70 backdrop-blur-sm">
          {JSON.stringify(o, null, 2)}
        </pre>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* Queue Depth Indicator                                               */
/* ------------------------------------------------------------------ */

function QueueDepthIndicator({
  total,
  currentIndex,
}: {
  total: number
  currentIndex: number
}) {
  if (total === 0) return null

  return (
    <div className="flex items-center gap-3">
      <Layers className="size-4 text-emerald-400/70" />
      <div className="flex items-center gap-2">
        {/* Progress dots */}
        <div className="flex gap-1">
          {Array.from({ length: Math.min(total, 10) }, (_, i) => (
            <motion.div
              key={i}
              className={`size-2 rounded-full transition-colors duration-200 ${
                i === currentIndex
                  ? 'bg-emerald-400'
                  : i < currentIndex
                    ? 'bg-emerald-400/30'
                    : 'bg-white/10'
              }`}
              initial={{ scale: 0 }}
              animate={{ scale: 1 }}
              transition={{ delay: i * 0.03, type: 'spring', stiffness: 500 }}
            />
          ))}
          {total > 10 && (
            <span className="ml-1 text-xs text-muted-foreground">
              +{total - 10}
            </span>
          )}
        </div>
        <span className="min-w-[3ch] text-right font-mono text-sm tabular-nums text-foreground/80">
          {currentIndex + 1}/{total}
        </span>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* Kbd helper                                                          */
/* ------------------------------------------------------------------ */

function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded border border-white/10 bg-white/5 px-1 font-mono text-[10px] text-foreground/50">
      {children}
    </kbd>
  )
}

/* ------------------------------------------------------------------ */
/* Main page                                                           */
/* ------------------------------------------------------------------ */

export function ReviewsPage() {
  const { orgId, projectId, orgSlug, projectSlug, base } = useProjectPaths()
  const { client } = useAPI()
  const [tickets, setTickets] = useState<Ticket[] | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [notesById, setNotesById] = useState<Record<string, string>>({})
  const [busyId, setBusyId] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [justEmptiedQueue, setJustEmptiedQueue] = useState(false)
  const [showHelp, setShowHelp] = useState(false)
  const projectLabel = useProjectBreadcrumbLabel(projectId)

  // Keyboard navigation state
  const [activeIndex, setActiveIndex] = useState(0)
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())
  const [traceCache, setTraceCache] = useState<
    Record<string, TraceStep[] | null>
  >({})
  const itemRefs = useRef<(HTMLLIElement | null)[]>([])
  const notesRefs = useRef<(HTMLTextAreaElement | null)[]>([])

  // Load pending reviews (sorted by priority)
  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    void (async () => {
      const r = await loadPendingReviews(client, projectId)
      if (cancelled) return
      if (!r.ok) {
        setErr(r.message)
        setTickets([])
        return
      }
      const sorted = sortByPriority(r.tickets)
      setErr(null)
      setTickets(sorted)
      if (sorted.length > 0) {
        setActiveIndex(0)
        setExpandedIds(new Set(sorted[0]?.id ? [sorted[0].id] : []))
        if (sorted[0]?.id) {
          void loadTrace(client, sorted[0].id).then((steps) => {
            if (cancelled || !sorted[0]?.id) return
            setTraceCache((prev) => ({ ...prev, [sorted[0].id!]: steps }))
          })
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [client, projectId])
  // Load trace when a ticket is expanded
  const loadTraceForTicket = useCallback(
    async (ticketId: string) => {
      if (traceCache[ticketId] !== undefined) return
      setTraceCache((prev) => ({ ...prev, [ticketId]: null }))
      const steps = await loadTrace(client, ticketId)
      setTraceCache((prev) => ({ ...prev, [ticketId]: steps }))
    },
    [client, traceCache],
  )

  // Expand the active ticket and load trace
  const toggleExpanded = useCallback(
    (ticketId: string) => {
      setExpandedIds((prev) => {
        const next = new Set(prev)
        if (next.has(ticketId)) {
          next.delete(ticketId)
        } else {
          next.add(ticketId)
          void loadTraceForTicket(ticketId)
        }
        return next
      })
    },
    [loadTraceForTicket],
  )

  const activateIndex = useCallback(
    (nextIndex: number, sourceTickets: Ticket[] | null = tickets) => {
      if (!sourceTickets || sourceTickets.length === 0) {
        setActiveIndex(0)
        return
      }
      const clamped = Math.max(0, Math.min(nextIndex, sourceTickets.length - 1))
      setActiveIndex(clamped)
      const ticket = sourceTickets[clamped]
      if (ticket?.id) {
        setExpandedIds((prev) => {
          if (prev.has(ticket.id!)) return prev
          const next = new Set(prev)
          next.add(ticket.id!)
          return next
        })
        void loadTraceForTicket(ticket.id)
      }
    },
    [tickets, loadTraceForTicket],
  )

  // Submit review
  const submitReview = useCallback(async (
    ticketId: string,
    decision: 'approved' | 'rejected' | 'reopened',
  ) => {
    if (!projectId || tickets === null) return
    const countBefore = tickets.length
    setBusyId(ticketId)
    setMessage(null)
    setJustEmptiedQueue(false)
    const { error, response } = await client.POST(
      '/tickets/{ticketID}/reviews',
      {
        params: { path: { ticketID: ticketId } },
        body: {
          decision,
          notes: notesById[ticketId]?.trim() || undefined,
        },
      },
    )
    setBusyId(null)
    if (!response.ok) {
      setMessage(formatApiError(error))
      return
    }
    if (decision === 'rejected') {
      setMessage(`Rejected ${ticketId}.`)
    } else if (decision === 'reopened') {
      setMessage(`Reopened ${ticketId}.`)
    } else {
      setMessage(`Approved ${ticketId}.`)
    }
    setNotesById((prev) => {
      const next = { ...prev }
      delete next[ticketId]
      return next
    })
    setTraceCache((prev) => {
      const next = { ...prev }
      delete next[ticketId]
      return next
    })
    const r = await loadPendingReviews(client, projectId)
    if (!r.ok) {
      setMessage(r.message)
      return
    }
    const sorted = sortByPriority(r.tickets)
    setTickets(sorted)

    if (sorted.length === 0 && countBefore > 0) {
      setMessage(null)
      setJustEmptiedQueue(true)
      return
    }
    if (sorted.length > 0) {
      activateIndex(activeIndex, sorted)
    } else {
      setActiveIndex(0)
    }
  }, [projectId, tickets, client, notesById, activeIndex, activateIndex])

  // Keyboard handler
  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      if (showHelp) return

      const target = e.target as HTMLElement
      const isInput =
        target.tagName === 'TEXTAREA' ||
        target.tagName === 'INPUT' ||
        target.isContentEditable
      if (isInput && e.key !== 'Escape') return

      if (!tickets || tickets.length === 0) {
        if (e.key === '?') {
          e.preventDefault()
          setShowHelp(true)
        }
        return
      }

      switch (e.key) {
        case 'j':
        case 'ArrowDown': {
          e.preventDefault()
          activateIndex(activeIndex + 1)
          break
        }
        case 'k':
        case 'ArrowUp': {
          e.preventDefault()
          activateIndex(activeIndex - 1)
          break
        }
        case 'a': {
          e.preventDefault()
          const ticket = tickets[activeIndex]
          if (ticket?.id && !busyId) {
            void submitReview(ticket.id, 'approved')
          }
          break
        }
        case 'r': {
          e.preventDefault()
          const ticket = tickets[activeIndex]
          if (ticket?.id && !busyId) {
            void submitReview(ticket.id, 'rejected')
          }
          break
        }
        case 'o': {
          e.preventDefault()
          const ticket = tickets[activeIndex]
          if (ticket?.id && !busyId) {
            void submitReview(ticket.id, 'reopened')
          }
          break
        }
        case 'n': {
          e.preventDefault()
          const ta = notesRefs.current[activeIndex]
          if (ta) {
            ta.focus()
          }
          break
        }
        case 'Enter': {
          e.preventDefault()
          const ticket = tickets[activeIndex]
          if (ticket?.id) {
            toggleExpanded(ticket.id)
          }
          break
        }
        case '?': {
          e.preventDefault()
          setShowHelp(true)
          break
        }
        case 'Escape': {
          if (isInput) {
            ;(target as HTMLElement).blur()
          }
          break
        }
      }
    }

    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [tickets, activeIndex, busyId, showHelp, submitReview, toggleExpanded, activateIndex])

  // Scroll active item into view
  useEffect(() => {
    const el = itemRefs.current[activeIndex]
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    }
  }, [activeIndex])

  if (!orgId || !projectId) {
    return <p className="text-sm text-destructive">Missing route params.</p>
  }
  if (err) {
    return <p className="text-sm text-destructive">{err}</p>
  }
  if (!tickets) {
    return <ReviewsPageSkeleton />
  }

  return (
    <motion.div
      className="flex flex-col gap-6"
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3 }}
    >
      <KeyboardShortcutHelp
        open={showHelp}
        onClose={() => setShowHelp(false)}
      />

      {/* ---- Header ---- */}
      <div className="flex flex-col gap-3">
        <p className="text-xs text-muted-foreground">
          <OrgProjectCrumbs
            orgId={orgSlug}
            projectId={projectSlug}
            projectLabel={projectLabel}
          />
          <span className="px-1">/</span>
          <span className="text-foreground" aria-current="page">
            Reviews
          </span>
        </p>

        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-semibold tracking-tight">
              Review queue
            </h1>
            {tickets.length > 0 && (
              <Badge
                variant="outline"
                className="border-emerald-500/30 bg-emerald-500/10 text-emerald-400"
              >
                {tickets.length} pending
              </Badge>
            )}
          </div>

          <div className="flex items-center gap-3">
            <QueueDepthIndicator
              total={tickets.length}
              currentIndex={activeIndex}
            />
            <button
              type="button"
              onClick={() => setShowHelp(true)}
              className="rounded-lg border border-white/10 bg-white/5 px-2.5 py-1 text-xs text-muted-foreground backdrop-blur-sm transition-colors hover:bg-white/10 hover:text-foreground"
              aria-label="Show keyboard shortcuts"
            >
              <Kbd>?</Kbd>
              <span className="ml-1.5">shortcuts</span>
            </button>
          </div>
        </div>
      </div>

      {/* ---- Status messages ---- */}
      <AnimatePresence>
        {message && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={{ opacity: 0, height: 0 }}
            className="overflow-hidden"
          >
            <p
              className={`rounded-lg px-4 py-2 text-sm backdrop-blur-sm ${
                message.includes('Approved')
                  ? 'border border-emerald-500/20 bg-emerald-500/10 text-emerald-400'
                  : message.includes('Rejected')
                    ? 'border border-red-500/20 bg-red-500/10 text-red-400'
                    : message.includes('Reopened')
                    ? 'border border-amber-500/20 bg-amber-500/10 text-amber-400'
                      : 'border border-destructive/20 bg-destructive/10 text-destructive'
              }`}
              role="status"
            >
              {message}
            </p>
          </motion.div>
        )}
      </AnimatePresence>

      {/* ---- Ticket stack ---- */}
      <ul className="flex flex-col gap-3" role="listbox" aria-label="Review queue">
        <AnimatePresence mode="popLayout">
          {tickets.map((t, idx) => {
            const isActive = idx === activeIndex
            const isExpanded = expandedIds.has(t.id ?? '')
            const trace = traceCache[t.id ?? '']
            const pMeta = getPriorityMeta(t.priority)

            return (
              <motion.li
                key={t.id}
                ref={(el) => {
                  itemRefs.current[idx] = el
                }}
                role="option"
                aria-selected={isActive}
                layout
                initial={{ opacity: 0, y: 20, scale: 0.97 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                exit={{ opacity: 0, x: -80, scale: 0.95 }}
                transition={{
                  type: 'spring',
                  stiffness: 400,
                  damping: 30,
                  delay: idx * 0.04,
                }}
                className="scroll-mt-4"
              >
                <div
                  className={`group relative overflow-hidden rounded-xl border backdrop-blur-md transition-all duration-200 ${
                    isActive
                      ? `border-emerald-500/30 bg-white/[0.06] ${pMeta.ring} ring-2 ring-offset-1 ring-offset-background`
                      : 'border-white/10 bg-white/[0.03] hover:border-white/15 hover:bg-white/[0.05]'
                  }`}
                >
                  <div
                    className={`absolute left-0 top-0 h-full w-1 rounded-l-xl transition-opacity ${
                      isActive ? 'opacity-100' : 'opacity-50'
                    } ${
                      (t.priority ?? 2) === 0
                        ? 'bg-red-500'
                        : (t.priority ?? 2) === 1
                          ? 'bg-amber-500'
                          : (t.priority ?? 2) === 3
                            ? 'bg-slate-500'
                            : 'bg-emerald-500'
                    }`}
                  />

                  <div
                    className="cursor-pointer select-none px-5 py-4 pl-6"
                    onClick={() => {
                      setActiveIndex(idx)
                      if (t.id) toggleExpanded(t.id)
                    }}
                  >
                    <div className="flex items-center gap-3">
                      <span
                        className={`inline-flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-medium transition-colors ${
                          isActive
                            ? 'bg-emerald-500/20 text-emerald-400'
                            : 'bg-white/5 text-muted-foreground'
                        }`}
                      >
                        {idx + 1}
                      </span>

                      <div className="flex min-w-0 flex-1 flex-col gap-1">
                        <div className="flex items-center gap-2">
                          <span className="truncate text-sm font-medium text-foreground">
                            {t.title ?? t.id}
                          </span>
                        </div>
                        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                          <span className="font-mono">{t.id}</span>
                          {t.state && (
                            <Badge variant="muted" className="text-[10px]">
                              {t.state}
                            </Badge>
                          )}
                          <span className={pMeta.color}>{pMeta.label}</span>
                        </div>
                      </div>

                      <motion.div
                        animate={{ rotate: isExpanded ? 180 : 0 }}
                        transition={{ duration: 0.2 }}
                      >
                        <ChevronDown className="size-4 text-muted-foreground" />
                      </motion.div>
                    </div>
                  </div>

                  <AnimatePresence>
                    {isExpanded && (
                      <motion.div
                        initial={{ height: 0, opacity: 0 }}
                        animate={{ height: 'auto', opacity: 1 }}
                        exit={{ height: 0, opacity: 0 }}
                        transition={{ duration: 0.25, ease: 'easeInOut' }}
                        className="overflow-hidden"
                      >
                        <div className="flex flex-col gap-5 border-t border-white/5 px-5 py-5 pl-6">
                          {t.objective?.description && (
                            <div>
                              <h3 className="mb-1.5 text-[10px] font-medium uppercase tracking-widest text-emerald-400/60">
                                Objective
                              </h3>
                              <p className="whitespace-pre-wrap text-sm leading-relaxed text-foreground/80">
                                {t.objective.description}
                              </p>
                              {t.objective.success_criteria &&
                                t.objective.success_criteria.length > 0 && (
                                  <ul className="mt-2 flex flex-col gap-1">
                                    {t.objective.success_criteria.map((c, i) => (
                                      <li
                                        key={i}
                                        className="flex items-start gap-2 text-sm text-muted-foreground"
                                      >
                                        <span className="mt-1.5 block size-1 shrink-0 rounded-full bg-emerald-500/50" />
                                        {c}
                                      </li>
                                    ))}
                                  </ul>
                                )}
                            </div>
                          )}

                          {t.outputs &&
                            typeof t.outputs === 'object' &&
                            Object.keys(t.outputs).length > 0 && (
                              <div>
                                <h3 className="mb-1.5 text-[10px] font-medium uppercase tracking-widest text-emerald-400/60">
                                  Outputs
                                </h3>
                                <OutputsSummary outputs={t.outputs} />
                              </div>
                            )}

                          <div>
                            <h3 className="mb-1.5 text-[10px] font-medium uppercase tracking-widest text-emerald-400/60">
                              Execution trace
                            </h3>
                            {trace === undefined ? (
                              <div className="flex flex-col gap-1.5">
                                <Skeleton className="h-3 w-full" />
                                <Skeleton className="h-3 w-3/4" />
                              </div>
                            ) : (
                              <TraceSummary steps={trace} />
                            )}
                          </div>

                          <label className="flex flex-col gap-1.5">
                            <span className="flex items-center gap-2 text-xs text-muted-foreground">
                              Review notes
                              <Kbd>n</Kbd>
                            </span>
                            <textarea
                              ref={(el) => {
                                notesRefs.current[idx] = el
                              }}
                              className="min-h-[56px] rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground/50 backdrop-blur-sm transition-colors focus:border-emerald-500/30 focus:outline-none focus:ring-1 focus:ring-emerald-500/20"
                              value={notesById[t.id ?? ''] ?? ''}
                              onChange={(e) =>
                                setNotesById((prev) => ({
                                  ...prev,
                                  [t.id ?? '']: e.target.value,
                                }))
                              }
                              placeholder="Optional feedback..."
                              rows={2}
                            />
                          </label>

                          <div className="flex flex-wrap items-center gap-2">
                            <Button
                              size="sm"
                              disabled={busyId === t.id}
                              onClick={() => t.id && submitReview(t.id, 'approved')}
                              className="border-emerald-500/30 bg-emerald-500/15 text-emerald-400 hover:bg-emerald-500/25 focus-visible:ring-emerald-500/30"
                            >
                              <CheckCircle2 className="mr-1 size-3.5" />
                              Approve
                              <Kbd>a</Kbd>
                            </Button>
                            <Button
                              size="sm"
                              variant="destructive"
                              disabled={busyId === t.id}
                              onClick={() => t.id && submitReview(t.id, 'rejected')}
                            >
                              <XCircle className="mr-1 size-3.5" />
                              Reject
                              <Kbd>r</Kbd>
                            </Button>
                            <Button
                              size="sm"
                              variant="secondary"
                              disabled={busyId === t.id}
                              onClick={() => t.id && submitReview(t.id, 'reopened')}
                              className="border border-white/10 bg-white/5 text-foreground/80 hover:bg-white/10"
                            >
                              <RotateCcw className="mr-1 size-3.5" />
                              Reopen
                              <Kbd>o</Kbd>
                            </Button>
                            <Link
                              to={`${base}/tickets/${t.id}`}
                              className="ml-auto flex items-center gap-1 text-xs text-emerald-400/70 transition-colors hover:text-emerald-400"
                            >
                              Full detail
                              <ExternalLink className="size-3" />
                            </Link>
                          </div>
                        </div>
                      </motion.div>
                    )}
                  </AnimatePresence>
                </div>
              </motion.li>
            )
          })}
        </AnimatePresence>
      </ul>

      {/* ---- Empty states ---- */}
      {tickets.length === 0 && justEmptiedQueue && (
        <ReviewQueueCelebration
          onDismiss={() => setJustEmptiedQueue(false)}
        />
      )}

      {tickets.length === 0 && !justEmptiedQueue && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          className="flex flex-col items-center gap-3 rounded-xl border border-white/5 bg-white/[0.02] py-16 text-center backdrop-blur-sm"
        >
          <CheckCircle2 className="size-10 text-emerald-500/30" />
          <p className="text-sm text-muted-foreground">
            No tickets awaiting review.
          </p>
        </motion.div>
      )}

      {/* ---- Keyboard hint footer ---- */}
      {tickets.length > 0 && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.3 }}
          className="flex items-center justify-center gap-4 py-2 text-xs text-muted-foreground/60"
        >
          <span className="flex items-center gap-1">
            <Kbd>j</Kbd>
            <Kbd>k</Kbd>
            navigate
          </span>
          <span className="flex items-center gap-1">
            <Kbd>a</Kbd>
            approve
          </span>
          <span className="flex items-center gap-1">
            <Kbd>r</Kbd>
            reject
          </span>
          <span className="flex items-center gap-1">
            <Kbd>o</Kbd>
            reopen
          </span>
          <span className="flex items-center gap-1">
            <Kbd>n</Kbd>
            notes
          </span>
          <span className="flex items-center gap-1">
            <Kbd>?</Kbd>
            help
          </span>
        </motion.div>
      )}
    </motion.div>
  )
}
