import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'

import { type ActivityItem } from '@/components/command-center/activity-feed'
import { CommandCenterRail } from '@/components/command-center/command-center-rail'
import { OrchestratorConsole } from '@/components/command-center/orchestrator-console'
import { TicketInspector } from '@/components/command-center/ticket-inspector'
import { useAuth } from '@/contexts/use-auth'
import { useRightRail } from '@/contexts/use-right-rail'
import {
  buildActivityItems,
  ticketIdFromChangeEvent,
  type ChangeStreamEvent,
  type TraceActivityStep,
} from '@/lib/command-center-activity'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']
type DispatchStatus = {
  active_ticket_ids?: string[]
}
type ChangeStreamResponse = {
  events?: ChangeStreamEvent[]
}

const POLL_INTERVAL = 10_000

export function CommandCenterPage() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>()
  const { client, token } = useAuth()
  const { clearRailContent, setOpen, setRailContent } = useRightRail()

  const [activeTickets, setActiveTickets] = useState<Ticket[]>([])
  const [pendingReviews, setPendingReviews] = useState<Ticket[]>([])
  const [activityItems, setActivityItems] = useState<ActivityItem[]>([])
  const [loading, setLoading] = useState(true)
  const [selectedTicketId, setSelectedTicketId] = useState<string | null>(null)

  const hasLoaded = useRef(false)
  const ticketListRef = useRef<Ticket[]>([])

  const fetchData = useCallback(async () => {
    if (!projectId || !token) return

    try {
      const [statusRes, reviewsRes, changeStreamRes] = await Promise.all([
        fetch('/api/dispatch/status', {
          headers: { Authorization: `Bearer ${token}` },
        }),
        client.GET('/projects/{projectID}/reviews', {
          params: { path: { projectID: projectId } },
        }),
        fetch(
          `/api/v1/streams/change?project_id=${encodeURIComponent(projectId)}&limit=40`,
          {
            headers: { Authorization: `Bearer ${token}` },
          },
        ),
      ])

      const status = statusRes.ok
        ? ((await statusRes.json()) as DispatchStatus)
        : null
      const activeTicketIds = [...new Set(status?.active_ticket_ids ?? [])]

      const activeTicketResults = await Promise.all(
        activeTicketIds.map(async (ticketId) => {
          const { data, response } = await client.GET('/tickets/{ticketID}', {
            params: { path: { ticketID: ticketId } },
          })
          return response.ok ? (data as Ticket) : null
        }),
      )

      const active = activeTicketResults.filter(
        (ticket): ticket is Ticket => ticket !== null,
      )
      setActiveTickets(active)
      ticketListRef.current = active

      const reviews: Ticket[] = reviewsRes.response.ok
        ? (reviewsRes.data?.tickets ?? [])
        : []
      const activeTicketIdSet = new Set(active.map((ticket) => ticket.id).filter(Boolean))
      setPendingReviews(
        reviews.filter((ticket) => !ticket.id || !activeTicketIdSet.has(ticket.id)),
      )

      const changeEvents: ChangeStreamEvent[] = changeStreamRes.ok
        ? (((await changeStreamRes.json()) as ChangeStreamResponse).events ?? [])
        : []

      const ticketsById = new Map<string, Ticket>()
      for (const ticket of [...active, ...reviews]) {
        if (ticket.id) ticketsById.set(ticket.id, ticket)
      }

      const candidateTicketIds = [
        ...changeEvents
          .map((event) => ticketIdFromChangeEvent(event))
          .filter((ticketId): ticketId is string => Boolean(ticketId)),
        ...active.map((ticket) => ticket.id).filter((ticketId): ticketId is string => Boolean(ticketId)),
        ...reviews.map((ticket) => ticket.id).filter((ticketId): ticketId is string => Boolean(ticketId)),
      ].filter((ticketId, index, list) => list.indexOf(ticketId) === index)

      const traceTicketIds = candidateTicketIds.slice(0, 10)
      const missingTicketIds = traceTicketIds.filter((ticketId) => !ticketsById.has(ticketId))
      if (missingTicketIds.length > 0) {
        const ticketResults = await Promise.all(
          missingTicketIds.map(async (ticketId) => {
            const { data, response } = await client.GET('/tickets/{ticketID}', {
              params: { path: { ticketID: ticketId } },
            })
            return response.ok ? (data as Ticket) : null
          }),
        )
        for (const ticket of ticketResults) {
          if (ticket?.id) ticketsById.set(ticket.id, ticket)
        }
      }

      const traceResults = await Promise.all(
        traceTicketIds.map(async (ticketId) => {
          const { data, response } = await client.GET('/tickets/{ticketID}/trace', {
            params: { path: { ticketID: ticketId } },
          })
          if (!response.ok) return []
          return (data?.steps ?? []).map((step) => ({
            ...step,
            ticketId,
          }))
        }),
      )

      const traceSteps: TraceActivityStep[] = traceResults.flat()
      setActivityItems(
        buildActivityItems({
          changeEvents,
          traceSteps,
          ticketsById,
        }),
      )
    } catch {
      // silent
    } finally {
      hasLoaded.current = true
      setLoading(false)
    }
  }, [client, projectId, token])

  useEffect(() => {
    if (!projectId || !token) {
      setLoading(false)
      return
    }
    setLoading(!hasLoaded.current)
    void fetchData()
    const interval = setInterval(() => void fetchData(), POLL_INTERVAL)
    return () => clearInterval(interval)
  }, [fetchData, projectId, token])

  // Keyboard navigation
  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      // Don't intercept if user is typing in an input/textarea
      const tag = (e.target as HTMLElement).tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return

      const allTickets = [...ticketListRef.current, ...pendingReviews]
      if (allTickets.length === 0) return

      const currentIdx = selectedTicketId
        ? allTickets.findIndex((t) => t.id === selectedTicketId)
        : -1

      if (e.key === 'j') {
        e.preventDefault()
        const next = currentIdx < allTickets.length - 1 ? currentIdx + 1 : 0
        const nextTicket = allTickets[next]
        if (nextTicket?.id) setSelectedTicketId(nextTicket.id)
      } else if (e.key === 'k') {
        e.preventDefault()
        const prev = currentIdx > 0 ? currentIdx - 1 : allTickets.length - 1
        const prevTicket = allTickets[prev]
        if (prevTicket?.id) setSelectedTicketId(prevTicket.id)
      } else if (e.key === 'Escape') {
        setSelectedTicketId(null)
      }
    }

    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [selectedTicketId, pendingReviews])

  useLayoutEffect(() => {
    setOpen(true)
    return () => clearRailContent()
  }, [clearRailContent, setOpen])

  useLayoutEffect(() => {
    if (!orgId || !projectId) return
    setRailContent(
      <CommandCenterRail
        tickets={activeTickets}
        pendingReviews={pendingReviews}
        activityItems={activityItems}
        loading={loading}
        orgId={orgId}
        projectId={projectId}
        selectedTicketId={selectedTicketId}
        onSelectTicket={setSelectedTicketId}
      />,
    )
  }, [
    activeTickets,
    activityItems,
    loading,
    orgId,
    pendingReviews,
    projectId,
    selectedTicketId,
    setRailContent,
  ])

  if (!orgId || !projectId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }

  return (
    <div className="mx-auto flex max-w-[1440px] flex-col gap-4 animate-in fade-in duration-300">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">Command Center</h1>
          <p className="max-w-3xl text-sm leading-relaxed text-muted-foreground">
            Use the orchestrator to inspect scope, create or update work streams, and author tickets. Execution and review run in the queue.
          </p>
        </div>
        <span className="text-xs text-muted-foreground">
          <kbd className="rounded border border-border px-1 py-0.5 text-[10px] font-mono">j</kbd>
          /
          <kbd className="rounded border border-border px-1 py-0.5 text-[10px] font-mono">k</kbd>
          {' '}tickets
          {' '}
          <kbd className="rounded border border-border px-1 py-0.5 text-[10px] font-mono">esc</kbd>
          {' '}clear selection
        </span>
      </div>

      <OrchestratorConsole
        key={projectId}
        projectId={projectId}
        onMessageComplete={() => void fetchData()}
      />

      <div className="lg:hidden">
        <CommandCenterRail
          tickets={activeTickets}
          pendingReviews={pendingReviews}
          activityItems={activityItems}
          loading={loading}
          orgId={orgId}
          projectId={projectId}
          selectedTicketId={selectedTicketId}
          onSelectTicket={setSelectedTicketId}
        />
      </div>

      <div className="min-h-[280px] rounded-2xl border border-white/10 bg-card/60 backdrop-blur-sm">
        {selectedTicketId ? (
          <TicketInspector
            key={selectedTicketId}
            ticketId={selectedTicketId}
            orgId={orgId}
            projectId={projectId}
            onReviewComplete={() => void fetchData()}
          />
        ) : (
          <div className="flex h-full items-center justify-center p-8">
            <div className="flex max-w-xl flex-col items-center gap-2 text-center">
              <p className="text-sm text-muted-foreground">
                Select a ticket to inspect state, trace, and outputs.
              </p>
              <p className="text-xs leading-relaxed text-muted-foreground/70">
                Use the queue snapshot or activity list to switch between active tickets.
              </p>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
