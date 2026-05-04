import { useQuery } from '@tanstack/react-query'

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
type Escalation = components['schemas']['Escalation']
type ChangeStreamResponse = {
  events?: ChangeStreamEvent[]
}
export type RailEscalation = {
  escalation: Escalation
  ticket: Ticket
}

interface RailData {
  activeTickets: Ticket[]
  pendingReviews: Ticket[]
  escalations: RailEscalation[]
  activityItems: ActivityItem[]
}

export function useProjectRailData(projectId: string | undefined) {
  const { client, token } = useAuth()

  const {
    data,
    isLoading: loading,
    refetch: refresh,
  } = useQuery<RailData>({
    queryKey: ['project-rail-data', projectId],
    queryFn: async (): Promise<RailData> => {
      if (!projectId || !token) {
        return { activeTickets: [], pendingReviews: [], escalations: [], activityItems: [] }
      }

      const [statusRes, reviewsRes, escalationsRes, awaitingInputRes, changeStreamRes] = await Promise.all([
        fetch('/api/dispatch/status', {
          headers: { Authorization: `Bearer ${token}` },
        }),
        client.GET('/projects/{projectID}/reviews', {
          params: { path: { projectID: projectId } },
        }),
        client.GET('/projects/{projectID}/escalations', {
          params: { path: { projectID: projectId } },
        }),
        client.GET('/projects/{projectID}/tickets', {
          params: { path: { projectID: projectId }, query: { state: 'awaiting_input' } },
        }),
        fetch(
          `/api/v1/streams/change?project_id=${encodeURIComponent(projectId)}&limit=40`,
          {
            headers: { Authorization: `Bearer ${token}` },
          },
        ),
      ])

      let status: DispatchStatus | null = null
      try {
        status = statusRes.ok ? ((await statusRes.json()) as DispatchStatus) : null
      } catch { /* non-JSON response, treat as unavailable */ }
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

      const reviews: Ticket[] = reviewsRes.response.ok
        ? (reviewsRes.data?.tickets ?? [])
        : []
      const rawEscalations: Escalation[] = escalationsRes.response.ok
        ? ((escalationsRes.data ?? []) as Escalation[])
        : []
      const awaitingInputTickets: Ticket[] = awaitingInputRes.response.ok
        ? ((awaitingInputRes.data ?? []) as Ticket[])
        : []

      let changeEvents: ChangeStreamEvent[] = []
      try {
        changeEvents = changeStreamRes.ok
          ? (((await changeStreamRes.json()) as ChangeStreamResponse).events ?? [])
          : []
      } catch { /* non-JSON response, skip activity feed */ }

      const ticketsById = new Map<string, Ticket>()
      for (const ticket of [...active, ...reviews]) {
        if (ticket.id) ticketsById.set(ticket.id, ticket)
      }

      // Ensure all escalation tickets are in the map before trace slicing
      const escalationTicketIds = rawEscalations
        .map((e) => e.ticket_id)
        .filter((id): id is string => Boolean(id))
        .filter((id) => !ticketsById.has(id))
      if (escalationTicketIds.length > 0) {
        const escResults = await Promise.all(
          escalationTicketIds.map(async (ticketId) => {
            const { data, response } = await client.GET('/tickets/{ticketID}', {
              params: { path: { ticketID: ticketId } },
            })
            if (!response.ok) return null
            const ticket = data as Ticket
            if (ticket.project_id && ticket.project_id !== projectId) return null
            return ticket
          }),
        )
        for (const ticket of escResults) {
          if (ticket?.id) ticketsById.set(ticket.id, ticket)
        }
      }

      const candidateTicketIds = [
        ...changeEvents
          .map((event) => ticketIdFromChangeEvent(event))
          .filter((ticketId): ticketId is string => Boolean(ticketId)),
        ...active.map((ticket) => ticket.id).filter((ticketId): ticketId is string => Boolean(ticketId)),
        ...reviews.map((ticket) => ticket.id).filter((ticketId): ticketId is string => Boolean(ticketId)),
        ...rawEscalations
          .map((escalation) => escalation.ticket_id)
          .filter((ticketId): ticketId is string => Boolean(ticketId)),
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
      const escalationTicketItems = rawEscalations
        .map((escalation) => {
          const ticketId = escalation.ticket_id
          if (!ticketId) return null
          const ticket = ticketsById.get(ticketId)
          if (!ticket) return null
          return { escalation, ticket }
        })
        .filter((item): item is RailEscalation => item !== null)

      // Include awaiting_input tickets that don't have a matching escalation.
      const escalationTicketIdSetRaw = new Set(
        escalationTicketItems
          .map((item) => item.ticket.id)
          .filter((ticketId): ticketId is string => Boolean(ticketId)),
      )
      for (const ticket of awaitingInputTickets) {
        if (ticket.id && !escalationTicketIdSetRaw.has(ticket.id)) {
          ticketsById.set(ticket.id, ticket)
          escalationTicketItems.push({
            escalation: {
              ticket_id: ticket.id,
              reason: 'Ticket is awaiting human input.',
            } as Escalation,
            ticket,
          })
        }
      }

      const escalationTicketIdSet = new Set(
        escalationTicketItems
          .map((item) => item.ticket.id)
          .filter((ticketId): ticketId is string => Boolean(ticketId)),
      )
      const activeTicketIdSet = new Set(
        active.map((ticket) => ticket.id).filter((ticketId): ticketId is string => Boolean(ticketId)),
      )

      return {
        activeTickets: active.filter(
          (ticket) => !ticket.id || !escalationTicketIdSet.has(ticket.id),
        ),
        pendingReviews: reviews.filter(
          (ticket) =>
            !ticket.id ||
            (!activeTicketIdSet.has(ticket.id) &&
              !escalationTicketIdSet.has(ticket.id)),
        ),
        escalations: escalationTicketItems,
        activityItems: buildActivityItems({
          changeEvents,
          traceSteps,
          ticketsById,
        }),
      }
    },
    enabled: Boolean(projectId && token),
    refetchInterval: 10_000,
    staleTime: 5_000,
  })

  return {
    activeTickets: data?.activeTickets ?? [],
    pendingReviews: data?.pendingReviews ?? [],
    escalations: data?.escalations ?? [],
    activityItems: data?.activityItems ?? [],
    loading,
    refresh,
  }
}
