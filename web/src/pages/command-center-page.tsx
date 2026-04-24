import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'

import { type ActivityItem } from '@/components/command-center/activity-feed'
import { CommandCenterRail } from '@/components/command-center/command-center-rail'
import { OrchestratorConsole } from '@/components/command-center/orchestrator-console'
import { TicketInspector } from '@/components/command-center/ticket-inspector'
import { useAuth } from '@/contexts/use-auth'
import { useRightRail } from '@/contexts/use-right-rail'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']
type DispatchStatus = {
  active_ticket_ids?: string[]
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
      const [statusRes, reviewsRes] = await Promise.all([
        fetch('/api/dispatch/status', {
          headers: { Authorization: `Bearer ${token}` },
        }),
        client.GET('/projects/{projectID}/reviews', {
          params: { path: { projectID: projectId } },
        }),
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

      // Fetch traces for active tickets
      const traceTargets = active.slice(0, 5)
      if (traceTargets.length > 0) {
        const traceResults = await Promise.all(
          traceTargets.map(async (t) => {
            if (!t.id) return []
            const { data, response } = await client.GET('/tickets/{ticketID}/trace', {
              params: { path: { ticketID: t.id } },
            })
            if (!response.ok) return []
            return (data?.steps ?? []).map((step) => ({
              ...step,
              ticketId: t.id!,
            }))
          }),
        )

        const allSteps: ActivityItem[] = traceResults
          .flat()
          .sort((a, b) => {
            const ta = a.created_at ? new Date(a.created_at).getTime() : 0
            const tb = b.created_at ? new Date(b.created_at).getTime() : 0
            return tb - ta
          })
          .slice(0, 25)

        setActivityItems(allSteps)
      } else {
        setActivityItems([])
      }
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
