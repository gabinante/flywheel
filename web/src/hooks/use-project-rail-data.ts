import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'

import { useAPI } from '@/contexts/use-api'
import type { DispatchStatus } from '@/hooks/use-dispatch-status'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']
type Escalation = components['schemas']['Escalation']
export type RailEscalation = {
  escalation: Escalation
  ticket: Ticket
}

interface RailData {
  activeTickets: Ticket[]
  pendingReviews: Ticket[]
  escalations: RailEscalation[]
}

export function useProjectRailData(projectId: string | undefined) {
  const { client } = useAPI()

  const {
    data,
    isLoading: loading,
    refetch: refresh,
  } = useQuery<RailData>({
    queryKey: ['project-rail-data', projectId],
    queryFn: async (): Promise<RailData> => {
      if (!projectId) {
        return { activeTickets: [], pendingReviews: [], escalations: [] }
      }

      const [statusRes, reviewsRes, escalationsRes, awaitingInputRes] = await Promise.all([
        fetch('/api/dispatch/status'),
        client.GET('/projects/{projectID}/reviews', {
          params: { path: { projectID: projectId } },
        }),
        client.GET('/projects/{projectID}/escalations', {
          params: { path: { projectID: projectId } },
        }),
        client.GET('/projects/{projectID}/tickets', {
          params: { path: { projectID: projectId }, query: { state: 'awaiting_input' } },
        }),
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
      }
    },
    enabled: Boolean(projectId),
    staleTime: 5_000,
  })

  const activeTickets = useMemo(() => data?.activeTickets ?? [], [data?.activeTickets])
  const pendingReviews = useMemo(() => data?.pendingReviews ?? [], [data?.pendingReviews])
  const escalations = useMemo(() => data?.escalations ?? [], [data?.escalations])

  return { activeTickets, pendingReviews, escalations, loading, refresh }
}
