import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'

import { ActiveWorkPanel } from '@/components/command-center/active-work-panel'
import { ActivityFeed, type ActivityItem } from '@/components/command-center/activity-feed'
import { DispatchStatus } from '@/components/command-center/dispatch-status'
import { OrchestratorConsole } from '@/components/command-center/orchestrator-console'
import { TicketInspector } from '@/components/command-center/ticket-inspector'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']

const POLL_INTERVAL = 10_000

export function CommandCenterPage() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>()
  const { client, token } = useAuth()

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
      const [claimedRes, executingRes, reviewsRes] = await Promise.all([
        client.GET('/projects/{projectID}/tickets', {
          params: { path: { projectID: projectId }, query: { state: 'claimed' } },
        }),
        client.GET('/projects/{projectID}/tickets', {
          params: { path: { projectID: projectId }, query: { state: 'executing' } },
        }),
        client.GET('/projects/{projectID}/reviews', {
          params: { path: { projectID: projectId } },
        }),
      ])

      const claimed: Ticket[] = claimedRes.response.ok
        ? ((claimedRes.data ?? []) as Ticket[])
        : []
      const executing: Ticket[] = executingRes.response.ok
        ? ((executingRes.data ?? []) as Ticket[])
        : []
      const active = [...executing, ...claimed]
      setActiveTickets(active)
      ticketListRef.current = active

      const reviews: Ticket[] = reviewsRes.response.ok
        ? (reviewsRes.data?.tickets ?? [])
        : []
      setPendingReviews(reviews)

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

  if (!orgId || !projectId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }

  return (
    <div className="mx-auto flex max-w-[1440px] flex-col gap-4 animate-in fade-in duration-300">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">Command Center</h1>
          <p className="max-w-3xl text-sm leading-relaxed text-muted-foreground">
            Chat with the orchestrator here. It should decompose the initiative into work streams and tickets while the event-driven dispatch system fans execution out to smaller workers in the background.
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

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.45fr)_390px]">
        <OrchestratorConsole
          key={projectId}
          projectId={projectId}
          onMessageComplete={() => void fetchData()}
        />

        <div className="flex flex-col gap-4">
          <DispatchStatus />

          <Card>
            <CardHeader className="border-b border-white/10 pb-4">
              <CardTitle className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                Queue Snapshot
              </CardTitle>
            </CardHeader>
            <CardContent className="pt-5">
              <ActiveWorkPanel
                tickets={activeTickets}
                pendingReviews={pendingReviews}
                loading={loading}
                orgId={orgId}
                projectId={projectId}
                selectedTicketId={selectedTicketId}
                onSelectTicket={setSelectedTicketId}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="border-b border-white/10 pb-4">
              <CardTitle className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                Live Activity
              </CardTitle>
            </CardHeader>
            <CardContent className="pt-5">
              <ActivityFeed
                items={activityItems}
                loading={loading}
                onSelectTicket={setSelectedTicketId}
              />
            </CardContent>
          </Card>
        </div>
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
                Select a ticket to inspect execution details.
              </p>
              <p className="text-xs leading-relaxed text-muted-foreground/70">
                The chat stays primary. Ticket detail stays one click away from the queue snapshot or the live activity feed.
              </p>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
