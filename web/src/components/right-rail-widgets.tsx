import { CommandCenterRail } from '@/components/command-center/command-center-rail'
import { useAuth } from '@/contexts/use-auth'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useResolvedRouteParams } from '@/hooks/use-resolved-route-params'
import { useProjectRailData } from '@/hooks/use-project-rail-data'

const noop = () => {}

export function RightRailWidgets() {
  const { ticketId } = useResolvedRouteParams()
  const { orgId, projectId, orgSlug, projectSlug } = useProjectPaths()
  const { token } = useAuth()
  const { activeTickets, pendingReviews, escalations, activityItems, loading } =
    useProjectRailData(projectId)

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
      escalations={escalations}
      activityItems={activityItems}
      loading={loading}
      orgId={orgSlug}
      projectId={projectSlug}
      selectedTicketId={ticketId ?? null}
      onSelectTicket={noop}
    />
  )
}
