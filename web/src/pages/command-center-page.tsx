import { useCallback, useEffect, useMemo, useState } from 'react'

import { ActiveWorkPanel } from '@/components/command-center/active-work-panel'
import { OrchestratorConsole } from '@/components/command-center/orchestrator-console'
import { TicketInspector } from '@/components/command-center/ticket-inspector'
import { Badge } from '@/components/ui/badge'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useProjectRailData } from '@/hooks/use-project-rail-data'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']

export function CommandCenterPage() {
  const { orgId, projectId } = useProjectPaths()
  const {
    activeTickets,
    pendingReviews,
    escalations,
    loading,
    refresh,
  } = useProjectRailData(projectId)
  const [selection, setSelection] = useState<{ projectId: string | undefined; id: string | null | undefined }>({ projectId, id: undefined })
  if (selection.projectId !== projectId) setSelection({ projectId, id: undefined })
  const selectedTicketId = selection.projectId === projectId && selection.id !== undefined ? selection.id : escalations[0]?.ticket.id ?? null
  const setSelectedTicketId = useCallback((id: string | null) => setSelection({ projectId, id }), [projectId])
  const navigableTickets = useMemo(() => {
    const seen = new Set<string>()
    const ordered: Ticket[] = []
    for (const ticket of [
      ...escalations.map((item) => item.ticket),
      ...activeTickets,
      ...pendingReviews,
    ]) {
      if (!ticket.id || seen.has(ticket.id)) continue
      seen.add(ticket.id)
      ordered.push(ticket)
    }
    return ordered
  }, [activeTickets, escalations, pendingReviews])
  const selectedEscalation =
    escalations.find((item) => item.ticket.id === selectedTicketId) ?? null

  // Keyboard navigation
  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      // Don't intercept if user is typing in an input/textarea
      const tag = (e.target as HTMLElement).tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return

      if (navigableTickets.length === 0) return

      const currentIdx = selectedTicketId
        ? navigableTickets.findIndex((t) => t.id === selectedTicketId)
        : -1

      if (e.key === 'j') {
        e.preventDefault()
        const next = currentIdx < navigableTickets.length - 1 ? currentIdx + 1 : 0
        const nextTicket = navigableTickets[next]
        if (nextTicket?.id) setSelectedTicketId(nextTicket.id)
      } else if (e.key === 'k') {
        e.preventDefault()
        const prev = currentIdx > 0 ? currentIdx - 1 : navigableTickets.length - 1
        const prevTicket = navigableTickets[prev]
        if (prevTicket?.id) setSelectedTicketId(prevTicket.id)
      } else if (e.key === 'Escape') {
        setSelectedTicketId(null)
      }
    }

    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [navigableTickets, selectedTicketId, setSelectedTicketId])

  if (!orgId || !projectId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }

  return (
    <div className="mx-auto flex max-w-[1440px] flex-col gap-4 animate-in fade-in duration-300">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div className="space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">Command Center</h1>
            {escalations.length > 0 ? (
              <Badge className="border-red-500/30 bg-red-500/15 text-red-400">
                {escalations.length} awaiting input
              </Badge>
            ) : null}
          </div>
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
        onMessageComplete={() => void refresh()}
      />

      <details className="rounded-xl border border-white/10 bg-card/60 p-4">
        <summary className="mb-3 cursor-pointer text-sm font-medium">Project work</summary>
        <ActiveWorkPanel
          tickets={activeTickets}
          pendingReviews={pendingReviews}
          escalations={escalations}
          loading={loading}
          orgId={orgId}
          projectId={projectId}
          selectedTicketId={selectedTicketId}
          onSelectTicket={setSelectedTicketId}
        />
      </details>

      <div className="min-h-[280px] rounded-2xl border border-white/10 bg-card/60 backdrop-blur-sm">
        {selectedTicketId ? (
          <TicketInspector
            key={selectedTicketId}
            ticketId={selectedTicketId}
            escalation={selectedEscalation?.escalation ?? null}
            orgId={orgId}
            projectId={projectId}
            onReviewComplete={() => void refresh()}
          />
        ) : (
          <div className="flex h-full items-center justify-center p-8">
            <div className="flex max-w-xl flex-col items-center gap-2 text-center">
              <p className="text-sm text-muted-foreground">
                Select a ticket to inspect state, trace, and outputs.
              </p>
              <p className="text-xs leading-relaxed text-muted-foreground/70">
                Open Project work to inspect tickets here, or use the global tray to open a ticket or session.
              </p>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
