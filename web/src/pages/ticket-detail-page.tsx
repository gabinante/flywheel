import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'

import { ExecutionTraceCard } from '@/components/execution-trace-card'
import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { ReviewQueueCelebration } from '@/components/review-queue-celebration'
import { TicketOutputsCard } from '@/components/ticket-outputs'
import { TicketRelationshipsCard } from '@/components/ticket-relationships-card'
import { TicketReopenPanel } from '@/components/ticket-reopen-panel'
import { TicketReviewPanel } from '@/components/ticket-review-panel'
import { WorkStreamSummaryCard } from '@/components/work-stream-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']
type WorkStream = components['schemas']['WorkStream']

type ReviewBanner =
  | { kind: 'reopened' }
  | { kind: 'next-in-stream'; nextTicketId: string; decision: 'approved' | 'rejected' }
  | { kind: 'more-elsewhere'; decision: 'approved' | 'rejected' }
  | { kind: 'project-empty'; decision: 'approved' | 'rejected' }
  | { kind: 'simple'; decision: 'approved' | 'rejected' }
  | { kind: 'followup-error'; message: string; decision: 'approved' | 'rejected' }

export function TicketDetailPage() {
  const { orgId, projectId, ticketId } = useParams<{
    orgId: string
    projectId: string
    ticketId: string
  }>()
  const { client } = useAuth()
  const [ticket, setTicket] = useState<Ticket | null | undefined>(undefined)
  const [workStream, setWorkStream] = useState<
    WorkStream | null | undefined
  >(undefined)
  const [workStreamErr, setWorkStreamErr] = useState<string | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [reviewBanner, setReviewBanner] = useState<ReviewBanner | null>(null)
  const [projectTickets, setProjectTickets] = useState<Ticket[] | null>(null)
  const [projectTicketsErr, setProjectTicketsErr] = useState<string | null>(
    null,
  )
  const projectLabel = useProjectBreadcrumbLabel(projectId)

  const reloadTicket = useCallback(async () => {
    if (!ticketId) return
    const { data, error, response } = await client.GET('/tickets/{ticketID}', {
      params: { path: { ticketID: ticketId } },
    })
    if (!response.ok) {
      setErr(formatApiError(error))
      return
    }
    setErr(null)
    setTicket(data ?? null)
  }, [client, ticketId])

  useEffect(() => {
    if (!ticketId) return
    let cancelled = false
    ;(async () => {
      const { data, error, response } = await client.GET('/tickets/{ticketID}', {
        params: { path: { ticketID: ticketId } },
      })
      if (cancelled) return
      if (!response.ok) {
        setErr(formatApiError(error))
        setTicket(null)
        return
      }
      setErr(null)
      setTicket(data ?? null)
    })()
    return () => {
      cancelled = true
    }
  }, [client, ticketId])

  useEffect(() => {
    queueMicrotask(() => {
      setReviewBanner(null)
    })
  }, [ticketId])

  const handleAfterReview = useCallback(
    async (decision: 'approved' | 'rejected') => {
      const streamKey = ticket?.work_stream_id ?? ''
      await reloadTicket()

      if (!projectId) {
        setReviewBanner({ kind: 'simple', decision })
        return
      }

      const { data, error, response } = await client.GET(
        '/projects/{projectID}/reviews',
        { params: { path: { projectID: projectId } } },
      )
      if (!response.ok) {
        setReviewBanner({
          kind: 'followup-error',
          message: formatApiError(error),
          decision,
        })
        return
      }

      const pending = data?.tickets ?? []
      const next = pending.find((t) => (t.work_stream_id ?? '') === streamKey)
      if (next?.id) {
        setReviewBanner({
          kind: 'next-in-stream',
          nextTicketId: next.id,
          decision,
        })
        return
      }
      if (pending.length === 0) {
        setReviewBanner({ kind: 'project-empty', decision })
        return
      }
      setReviewBanner({ kind: 'more-elsewhere', decision })
    },
    [ticket?.work_stream_id, projectId, client, reloadTicket],
  )

  const handleAfterReopen = useCallback(async () => {
    await reloadTicket()
    setReviewBanner({ kind: 'reopened' })
  }, [reloadTicket])

  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    setProjectTickets(null)
    setProjectTicketsErr(null)
    void (async () => {
      const { data, error, response } = await client.GET(
        '/projects/{projectID}/tickets',
        { params: { path: { projectID: projectId } } },
      )
      if (cancelled) return
      if (!response.ok) {
        setProjectTicketsErr(formatApiError(error))
        setProjectTickets([])
        return
      }
      setProjectTicketsErr(null)
      setProjectTickets(data ?? [])
    })()
    return () => {
      cancelled = true
    }
  }, [client, projectId])

  useEffect(() => {
    if (!projectId || !ticket?.work_stream_id) {
      queueMicrotask(() => {
        setWorkStream(undefined)
        setWorkStreamErr(null)
      })
      return
    }
    let cancelled = false
    const wsId = ticket.work_stream_id
    void (async () => {
      setWorkStream(undefined)
      setWorkStreamErr(null)
      const { data, error, response } = await client.GET(
        '/projects/{projectID}/work-streams/{workStreamID}',
        {
          params: {
            path: { projectID: projectId, workStreamID: wsId },
          },
        },
      )
      if (cancelled) return
      if (!response.ok) {
        setWorkStreamErr(formatApiError(error))
        setWorkStream(null)
        return
      }
      setWorkStream(data ?? null)
    })()
    return () => {
      cancelled = true
    }
  }, [client, projectId, ticket?.work_stream_id])

  if (!orgId || !projectId || !ticketId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }
  if (err) {
    return <p className="text-destructive text-sm">{err}</p>
  }
  if (ticket === undefined) {
    return <p className="text-muted-foreground text-sm">Loading…</p>
  }
  if (!ticket) {
    return <p className="text-muted-foreground text-sm">Ticket not found.</p>
  }

  const obj = ticket.objective

  const allTicketsHref = `/orgs/${orgId}/projects/${projectId}/tickets`
  const streamTicketsHref =
    ticket.work_stream_id && projectId
      ? `${allTicketsHref}?work_stream_id=${encodeURIComponent(ticket.work_stream_id)}`
      : null

  const workStreamCrumbLabel =
    workStream?.name ?? workStream?.slug ?? workStream?.id ?? null

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <p className="text-muted-foreground text-xs">
          <OrgProjectCrumbs
            orgId={orgId}
            projectId={projectId}
            projectLabel={projectLabel}
          />
          {streamTicketsHref ? (
            <>
              <span className="px-1">/</span>
              <Link to={streamTicketsHref} className="hover:underline">
                {workStreamCrumbLabel ?? 'Work stream'}
              </Link>
            </>
          ) : null}
          <span className="px-1">/</span>
          <Link to={allTicketsHref} className="hover:underline">
            Tickets
          </Link>
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold tracking-tight">
            {ticket.title ?? ticket.id}
          </h1>
          {ticket.state ? (
            <Badge variant="secondary">{ticket.state}</Badge>
          ) : null}
        </div>
        <p className="text-muted-foreground font-mono text-xs">{ticket.id}</p>
      </div>

      {obj?.description ? (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Objective</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-muted-foreground whitespace-pre-wrap text-sm">
              {obj.description}
            </p>
            {obj.success_criteria && obj.success_criteria.length > 0 ? (
              <ul className="mt-3 list-inside list-disc text-sm">
                {obj.success_criteria.map((c, i) => (
                  <li key={i}>{c}</li>
                ))}
              </ul>
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      <TicketRelationshipsCard
        orgId={orgId}
        projectId={projectId}
        ticket={ticket}
        projectTickets={projectTickets}
        projectTicketsError={projectTicketsErr}
      />

      {ticket.work_stream_id ? (
        workStreamErr ? (
          <WorkStreamSummaryCard stream={null} errorMessage={workStreamErr} />
        ) : (
          <div className="flex flex-col gap-2">
            <WorkStreamSummaryCard stream={workStream ?? null} />
            {workStream?.id ? (
              <div className="flex flex-wrap gap-3 text-sm">
                <Link
                  className="text-primary hover:underline"
                  to={`/orgs/${orgId}/projects/${projectId}/work-streams/${workStream.id}`}
                >
                  Manage work stream
                </Link>
                <Link
                  className="text-primary hover:underline"
                  to={`/orgs/${orgId}/projects/${projectId}/tickets?work_stream_id=${encodeURIComponent(workStream.id)}`}
                >
                  View tickets in this stream
                </Link>
              </div>
            ) : null}
          </div>
        )
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Work stream</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-muted-foreground text-sm">
              This ticket is not associated with a work stream.
            </p>
          </CardContent>
        </Card>
      )}

      <TicketOutputsCard outputs={ticket.outputs} />

      <ExecutionTraceCard ticketId={ticketId} ticketState={ticket.state} />

      {reviewBanner?.kind === 'reopened' ? (
        <Card className="border-primary/20 bg-primary/[0.07] backdrop-blur-sm">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Back in review queue</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3 text-sm">
            <p className="text-muted-foreground">
              This ticket is awaiting review again. Use the review panel below
              to approve or reject, or open the full pending list.
            </p>
            <div className="flex flex-wrap gap-2">
              {projectId ? (
                <Button asChild size="sm" variant="outline" className="w-fit">
                  <Link
                    to={`/orgs/${orgId}/projects/${projectId}/reviews`}
                  >
                    Pending reviews
                  </Link>
                </Button>
              ) : null}
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setReviewBanner(null)}
              >
                Dismiss
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : reviewBanner?.kind === 'project-empty' ? (
        <ReviewQueueCelebration
          onDismiss={() => setReviewBanner(null)}
          extraLead={
            reviewBanner.decision === 'approved'
              ? 'This ticket is marked done.'
              : undefined
          }
        />
      ) : reviewBanner ? (
        <Card
          className={
            reviewBanner.decision === 'rejected' &&
            (reviewBanner.kind === 'simple' || reviewBanner.kind === 'followup-error')
              ? 'border-destructive/20 bg-destructive/[0.07] backdrop-blur-sm'
              : 'border-primary/20 bg-primary/[0.07] backdrop-blur-sm'
          }
        >
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">
              {reviewBanner.decision === 'rejected' ? 'Rejected' : 'Review submitted'}
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3 text-sm">
            {reviewBanner.kind === 'next-in-stream' ? (
              <>
                <p className="text-muted-foreground">
                  {reviewBanner.decision === 'approved'
                    ? 'This ticket is marked done. Another ticket in the same work stream is still awaiting review.'
                    : 'Your review was recorded. Another ticket in the same work stream is still awaiting review.'}
                </p>
                <Button asChild size="sm" className="w-fit">
                  <Link
                    to={`/orgs/${orgId}/projects/${projectId}/tickets/${reviewBanner.nextTicketId}`}
                  >
                    Next in this stream
                  </Link>
                </Button>
              </>
            ) : null}
            {reviewBanner.kind === 'more-elsewhere' ? (
              <>
                <p className="text-muted-foreground">
                  {reviewBanner.decision === 'approved'
                    ? 'This ticket is marked done. No other reviews in this work stream, but this project still has pending reviews.'
                    : 'No other reviews in this work stream, but this project still has pending reviews.'}
                </p>
                <Button asChild variant="outline" size="sm" className="w-fit">
                  <Link
                    to={`/orgs/${orgId}/projects/${projectId}/reviews`}
                  >
                    Open pending reviews
                  </Link>
                </Button>
              </>
            ) : null}
            {reviewBanner.kind === 'simple' ? (
              <p className="text-muted-foreground">
                {reviewBanner.decision === 'approved'
                  ? 'This ticket is marked done.'
                  : 'Your review was recorded.'}
              </p>
            ) : null}
            {reviewBanner.kind === 'followup-error' ? (
              <>
                <p className="text-muted-foreground">
                  {reviewBanner.decision === 'approved'
                    ? 'This ticket is marked done.'
                    : 'Your review was recorded.'}
                </p>
                <p className="text-destructive text-sm">{reviewBanner.message}</p>
              </>
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      {ticket.state === 'awaiting_review' && ticketId ? (
        <TicketReviewPanel ticketId={ticketId} onReviewed={handleAfterReview} />
      ) : null}

      {ticket.state === 'done' && ticketId ? (
        <TicketReopenPanel ticketId={ticketId} onReopened={handleAfterReopen} />
      ) : null}
    </div>
  )
}
