import { ActiveWorkPanel } from '@/components/command-center/active-work-panel'
import { ActivityFeed, type ActivityItem } from '@/components/command-center/activity-feed'
import { DispatchStatus } from '@/components/command-center/dispatch-status'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']

export function CommandCenterRail({
  tickets,
  pendingReviews,
  activityItems,
  loading,
  orgId,
  projectId,
  selectedTicketId,
  onSelectTicket,
}: {
  tickets: Ticket[]
  pendingReviews: Ticket[]
  activityItems: ActivityItem[]
  loading: boolean
  orgId: string
  projectId: string
  selectedTicketId: string | null
  onSelectTicket: (ticketId: string) => void
}) {
  return (
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
            tickets={tickets}
            pendingReviews={pendingReviews}
            loading={loading}
            orgId={orgId}
            projectId={projectId}
            selectedTicketId={selectedTicketId}
            onSelectTicket={onSelectTicket}
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
            onSelectTicket={onSelectTicket}
          />
        </CardContent>
      </Card>
    </div>
  )
}
