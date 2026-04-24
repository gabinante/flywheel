import { useNavigate, useParams } from 'react-router-dom'

import { CommandCenterRail } from '@/components/command-center/command-center-rail'
import { useAuth } from '@/contexts/use-auth'
import { useProjectRailData } from '@/hooks/use-project-rail-data'

export function RightRailWidgets() {
  const { orgId, projectId, ticketId } = useParams<{
    orgId: string
    projectId: string
    ticketId: string
  }>()
  const navigate = useNavigate()
  const { token } = useAuth()
  const { activeTickets, pendingReviews, activityItems, loading } = useProjectRailData(projectId)

  if (!orgId || !projectId || !token) {
    return (
      <div className="flex flex-col gap-4 text-xs text-muted-foreground">
        <p className="italic">Navigate to a project to see queue and activity.</p>
      </div>
    )
  }

  return (
    <CommandCenterRail
      tickets={activeTickets}
      pendingReviews={pendingReviews}
      activityItems={activityItems}
      loading={loading}
      orgId={orgId}
      projectId={projectId}
      selectedTicketId={ticketId ?? null}
      onSelectTicket={(nextTicketId) =>
        navigate(`/orgs/${orgId}/projects/${projectId}/tickets/${nextTicketId}`)
      }
    />
  )
}
