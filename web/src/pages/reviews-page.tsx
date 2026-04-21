import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'

import { KeyboardShortcutHelp } from '@/components/keyboard-shortcut-help'
import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { ReviewQueueCelebration } from '@/components/review-queue-celebration'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { FlywheelClient } from '@/contexts/auth-context'
import { useAuth } from '@/contexts/use-auth'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']
type TraceStep = components['schemas']['TraceStep']

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
          <span className="bg-muted rounded-md px-2 py-0.5">
            {counts.tool_call} tool call{counts.tool_call !== 1 ? 's' : ''}
          </span>
        )}
        {counts.observation > 0 && (
          <span className="bg-muted rounded-md px-2 py-0.5">
            {counts.observation} observation{counts.observation !== 1 ? 's' : ''}
          </span>
        )}
        {counts.thought > 0 && (
          <span className="bg-muted rounded-md px-2 py-0.5">
            {counts.thought} thought{counts.thought !== 1 ? 's' : ''}
          </span>
        )}
        {counts.error > 0 && (
          <span className="bg-destructive/10 text-destructive rounded-md px-2 py-0.5">
            {counts.error} error{counts.error !== 1 ? 's' : ''}
          </span>
        )}
      </div>
      {/* Show last few steps as summary */}
      <ul className="flex flex-col gap-0.5">
        {steps.slice(-3).map((step, i) => (
          <li key={step.id ?? i} className="text-muted-foreground truncate text-xs">
            <span className="text-foreground/60 font-mono">{step.type}</span>
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

  return (
    <div className="flex flex-col gap-1">
      {summary && (
        <p className="text-muted-foreground text-sm">{summary}</p>
      )}
      {prUrl && (
        <a
          href={prUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="text-primary text-xs hover:underline"
        >
          {prUrl}
        </a>
      )}
      {!summary && !prUrl && (
        <pre className="bg-muted max-h-24 overflow-auto rounded-md p-2 font-mono text-xs">
          {JSON.stringify(o, null, 2)}
        </pre>
      )}
    </div>
  )
}

export function ReviewsPage() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>()
  const { client } = useAuth()
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
  const [traceCache, setTraceCache] = useState<Record<string, TraceStep[] | null>>({})
  const itemRefs = useRef<(HTMLLIElement | null)[]>([])
  const notesRefs = useRef<(HTMLTextAreaElement | null)[]>([])

  // Load pending reviews
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
      setErr(null)
      setTickets(r.tickets)
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

  // Auto-expand the active ticket when navigating
  useEffect(() => {
    if (!tickets || tickets.length === 0) return
    const ticket = tickets[activeIndex]
    if (ticket?.id && !expandedIds.has(ticket.id)) {
      setExpandedIds((prev) => {
        const next = new Set(prev)
        next.add(ticket.id!)
        return next
      })
      void loadTraceForTicket(ticket.id)
    }
  }, [activeIndex, tickets, expandedIds, loadTraceForTicket])

  // Submit review
  async function submitReview(ticketId: string, decision: 'approved' | 'rejected' | 'reopened') {
    if (!projectId || tickets === null) return
    const countBefore = tickets.length
    setBusyId(ticketId)
    setMessage(null)
    setJustEmptiedQueue(false)
    const { error, response } = await client.POST('/tickets/{ticketID}/reviews', {
      params: { path: { ticketID: ticketId } },
      body: {
        decision,
        notes: notesById[ticketId]?.trim() || undefined,
      },
    })
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
    // Remove from trace cache
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
    setTickets(r.tickets)

    if (r.tickets.length === 0 && countBefore > 0) {
      setMessage(null)
      setJustEmptiedQueue(true)
      return
    }
    // Keep active index in bounds
    if (activeIndex >= r.tickets.length) {
      setActiveIndex(Math.max(0, r.tickets.length - 1))
    }
  }

  // Keyboard handler
  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      // Don't capture when help overlay is open (it handles its own keys)
      if (showHelp) return

      // Don't capture shortcuts when typing in a textarea/input (except Escape)
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
          setActiveIndex((prev) => Math.min(prev + 1, tickets.length - 1))
          break
        }
        case 'k':
        case 'ArrowUp': {
          e.preventDefault()
          setActiveIndex((prev) => Math.max(prev - 1, 0))
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
  }, [tickets, activeIndex, busyId, showHelp, toggleExpanded])

  // Scroll active item into view
  useEffect(() => {
    const el = itemRefs.current[activeIndex]
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    }
  }, [activeIndex])

  if (!orgId || !projectId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }
  if (err) {
    return <p className="text-destructive text-sm">{err}</p>
  }
  if (!tickets) {
    return <p className="text-muted-foreground text-sm">Loading…</p>
  }

  return (
    <div className="flex flex-col gap-4">
      <KeyboardShortcutHelp open={showHelp} onClose={() => setShowHelp(false)} />

      {/* Header */}
      <div className="flex flex-col gap-1">
        <p className="text-muted-foreground text-xs">
          <OrgProjectCrumbs
            orgId={orgId}
            projectId={projectId}
            projectLabel={projectLabel}
          />
          <span className="px-1">/</span>
          <span className="text-foreground" aria-current="page">
            Pending reviews
          </span>
        </p>
        <div className="flex items-center justify-between gap-4">
          <h1 className="text-xl font-semibold tracking-tight">Pending reviews</h1>
          <div className="flex items-center gap-3">
            {tickets.length > 0 && (
              <span className="text-muted-foreground text-sm tabular-nums">
                {activeIndex + 1} / {tickets.length}
              </span>
            )}
            <button
              type="button"
              onClick={() => setShowHelp(true)}
              className="text-muted-foreground hover:text-foreground rounded-md border border-border px-2 py-0.5 text-xs transition-colors"
              aria-label="Show keyboard shortcuts"
            >
              <kbd className="font-mono">?</kbd> shortcuts
            </button>
          </div>
        </div>
      </div>

      {/* Status messages */}
      {message && (
        <p
          className={
            message.startsWith('Rejected') || message.startsWith('Reopened')
              ? 'text-muted-foreground text-sm'
              : message.includes('failed') || message.includes('Request')
                ? 'text-destructive text-sm'
                : 'text-primary text-sm'
          }
          role="status"
        >
          {message}
        </p>
      )}

      {/* Ticket list */}
      <ul className="flex flex-col gap-3" role="listbox" aria-label="Review queue">
        {tickets.map((t, idx) => {
          const isActive = idx === activeIndex
          const isExpanded = expandedIds.has(t.id ?? '')
          const trace = traceCache[t.id ?? '']

          return (
            <li
              key={t.id}
              ref={(el) => { itemRefs.current[idx] = el }}
              role="option"
              aria-selected={isActive}
              className={`scroll-mt-4 rounded-lg transition-all duration-150 ${
                isActive
                  ? 'ring-primary/50 ring-2 ring-offset-1 ring-offset-background'
                  : ''
              }`}
            >
              <Card
                className={`transition-colors duration-150 ${
                  isActive ? 'border-primary/30 bg-primary/[0.03]' : ''
                }`}
              >
                <CardHeader
                  className="cursor-pointer select-none"
                  onClick={() => {
                    setActiveIndex(idx)
                    if (t.id) toggleExpanded(t.id)
                  }}
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <span
                      className={`inline-flex size-5 items-center justify-center rounded-full text-xs font-medium transition-colors ${
                        isActive
                          ? 'bg-primary text-primary-foreground'
                          : 'bg-muted text-muted-foreground'
                      }`}
                    >
                      {idx + 1}
                    </span>
                    <CardTitle className="text-base">{t.title ?? t.id}</CardTitle>
                    {t.state && <Badge variant="secondary">{t.state}</Badge>}
                    <span className="text-muted-foreground ml-auto font-mono text-xs">
                      {t.id}
                    </span>
                  </div>
                </CardHeader>

                {/* Expanded inline details */}
                {isExpanded && (
                  <CardContent className="flex flex-col gap-4 border-t border-border pt-4">
                    {/* Objective */}
                    {t.objective?.description && (
                      <div>
                        <h3 className="text-foreground mb-1 text-xs font-medium uppercase tracking-wide">
                          Objective
                        </h3>
                        <p className="text-muted-foreground whitespace-pre-wrap text-sm">
                          {t.objective.description}
                        </p>
                        {t.objective.success_criteria &&
                          t.objective.success_criteria.length > 0 && (
                            <ul className="text-muted-foreground mt-2 list-inside list-disc text-sm">
                              {t.objective.success_criteria.map((c, i) => (
                                <li key={i}>{c}</li>
                              ))}
                            </ul>
                          )}
                      </div>
                    )}

                    {/* Outputs */}
                    {t.outputs &&
                      typeof t.outputs === 'object' &&
                      Object.keys(t.outputs).length > 0 && (
                        <div>
                          <h3 className="text-foreground mb-1 text-xs font-medium uppercase tracking-wide">
                            Outputs
                          </h3>
                          <OutputsSummary outputs={t.outputs} />
                        </div>
                      )}

                    {/* Trace summary */}
                    <div>
                      <h3 className="text-foreground mb-1 text-xs font-medium uppercase tracking-wide">
                        Execution trace
                      </h3>
                      {trace === undefined ? (
                        <p className="text-muted-foreground text-xs italic">
                          Loading trace…
                        </p>
                      ) : (
                        <TraceSummary steps={trace} />
                      )}
                    </div>

                    {/* Notes */}
                    <label className="flex flex-col gap-1 text-sm">
                      <span className="text-muted-foreground text-xs">
                        Review notes{' '}
                        <kbd className="bg-muted rounded border border-border px-1 font-mono text-[10px]">
                          n
                        </kbd>
                      </span>
                      <textarea
                        ref={(el) => { notesRefs.current[idx] = el }}
                        className="border-input bg-background min-h-[60px] rounded-md border px-3 py-2 text-sm focus:ring-1 focus:ring-primary/50 focus:outline-none"
                        value={notesById[t.id ?? ''] ?? ''}
                        onChange={(e) =>
                          setNotesById((prev) => ({
                            ...prev,
                            [t.id ?? '']: e.target.value,
                          }))
                        }
                        placeholder="Optional feedback…"
                        rows={2}
                      />
                    </label>

                    {/* Action buttons */}
                    <div className="flex flex-wrap items-center gap-2">
                      <Button
                        size="sm"
                        disabled={busyId === t.id}
                        onClick={() => t.id && submitReview(t.id, 'approved')}
                      >
                        <span className="mr-1.5 font-mono text-xs opacity-60">a</span>
                        Approve
                      </Button>
                      <Button
                        size="sm"
                        variant="destructive"
                        disabled={busyId === t.id}
                        onClick={() => t.id && submitReview(t.id, 'rejected')}
                      >
                        <span className="mr-1.5 font-mono text-xs opacity-60">r</span>
                        Reject
                      </Button>
                      <Button
                        size="sm"
                        variant="secondary"
                        disabled={busyId === t.id}
                        onClick={() => t.id && submitReview(t.id, 'reopened')}
                      >
                        <span className="mr-1.5 font-mono text-xs opacity-60">o</span>
                        Reopen
                      </Button>
                      <Link
                        to={`/orgs/${orgId}/projects/${projectId}/tickets/${t.id}`}
                        className="text-primary ml-auto text-xs hover:underline"
                      >
                        Full detail →
                      </Link>
                    </div>
                  </CardContent>
                )}
              </Card>
            </li>
          )
        })}
      </ul>

      {/* Empty states */}
      {tickets.length === 0 && justEmptiedQueue && (
        <ReviewQueueCelebration onDismiss={() => setJustEmptiedQueue(false)} />
      )}

      {tickets.length === 0 && !justEmptiedQueue && (
        <p className="text-muted-foreground text-sm">No tickets awaiting review.</p>
      )}

      {/* Keyboard hint footer */}
      {tickets.length > 0 && (
        <p className="text-muted-foreground text-center text-xs">
          <kbd className="font-mono">j</kbd>/<kbd className="font-mono">k</kbd> navigate
          {' · '}
          <kbd className="font-mono">a</kbd> approve
          {' · '}
          <kbd className="font-mono">r</kbd> reject
          {' · '}
          <kbd className="font-mono">n</kbd> notes
          {' · '}
          <kbd className="font-mono">?</kbd> help
        </p>
      )}
    </div>
  )
}
