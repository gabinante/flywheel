import { PlanMarkdown } from '@/components/plan-markdown'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { components } from '@/lib/api/v1'

type WorkStream = components['schemas']['WorkStream']

type WorkStreamSummaryCardProps = {
  stream: WorkStream | null
  /** When set, show a link-style hint (parent provides Link if needed). */
  errorMessage?: string | null
}

/** Single work stream details (e.g. ticket detail sidebar). */
export function WorkStreamSummaryCard({
  stream,
  errorMessage,
}: WorkStreamSummaryCardProps) {
  if (errorMessage) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Work stream</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-destructive text-sm">{errorMessage}</p>
        </CardContent>
      </Card>
    )
  }
  if (!stream) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Work stream</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-2">
            <Skeleton className="h-4 w-48" />
            <Skeleton className="h-3 w-32" />
            <Skeleton className="h-3 w-64" />
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center gap-2">
          <CardTitle className="text-sm">
            {stream.name ?? stream.slug ?? stream.id}
          </CardTitle>
          {stream.status ? (
            <Badge variant="outline">{stream.status}</Badge>
          ) : null}
        </div>
        {stream.slug ? (
          <CardDescription className="font-mono text-xs">
            {stream.slug}
          </CardDescription>
        ) : null}
      </CardHeader>
      <CardContent className="flex flex-col gap-2 text-sm">
        {stream.branch ? (
          <p>
            <span className="text-muted-foreground">Branch </span>
            <span className="font-mono text-xs">{stream.branch}</span>
          </p>
        ) : null}
        {stream.plan ? (
          <div className="border-border rounded-md border bg-card/50 p-2">
            <PlanMarkdown markdown={stream.plan} />
          </div>
        ) : null}
        <p className="text-muted-foreground font-mono text-xs">{stream.id}</p>
      </CardContent>
    </Card>
  )
}
