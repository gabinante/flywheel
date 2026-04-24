import { useCallback, useEffect, useRef, useState } from 'react'

import { useAuth } from '@/contexts/use-auth'
import type { DispatchStatus } from '@/hooks/use-dispatch-status'
import {
  buildActivityItems,
  ticketIdFromChangeEvent,
  type ActivityItem,
  type ChangeStreamEvent,
  type TraceActivityStep,
} from '@/lib/command-center-activity'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']
type ChangeStreamResponse = {
  events?: ChangeStreamEvent[]
}

const POLL_INTERVAL = 10_000

export function useProjectRailData(projectId: string | undefined) {
  const { client, token } = useAuth()

  const [activeTickets, setActiveTickets] = useState<Ticket[]>([])
  const [pendingReviews, setPendingReviews] = useState<Ticket[]>([])
  const [activityItems, setActivityItems] = useState<ActivityItem[]>([])
  const [loading, setLoading] = useState(true)

  const hasLoaded = useRef(false)

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
          if (!response.ok) return null
          const ticket = data as Ticket
          if (ticket.project_id && ticket.project_id !== projectId) return null
          return ticket
        }),
      )

      const active = activeTicketResults.filter(
        (ticket): ticket is Ticket => ticket !== null,
      )
      setActiveTickets(active)

      const reviews: Ticket[] = reviewsRes.response.ok
        ? (reviewsRes.data?.tickets ?? [])
        : []
      const activeTicketIdSet = new Set(
        active.map((ticket) => ticket.id).filter((ticketId): ticketId is string => Boolean(ticketId)),
      )
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
            if (!response.ok) return null
            const ticket = data as Ticket
            if (ticket.project_id && ticket.project_id !== projectId) return null
            return ticket
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
      // Silent background refresh failures should not disrupt navigation.
    } finally {
      hasLoaded.current = true
      setLoading(false)
    }
  }, [client, projectId, token])

  useEffect(() => {
    hasLoaded.current = false
    setActiveTickets([])
    setPendingReviews([])
    setActivityItems([])
    setLoading(Boolean(projectId && token))
  }, [projectId, token])

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

  return {
    activeTickets,
    pendingReviews,
    activityItems,
    loading,
    refresh: fetchData,
  }
}
