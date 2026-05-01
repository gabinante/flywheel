import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  Calendar,
  Clock,
  Hash,
  Target,
  User,
} from 'lucide-react'

import { ExecutionTraceCard } from '@/components/execution-trace-card'
import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { ReviewQueueCelebration } from '@/components/review-queue-celebration'
import { TicketLifecycle } from '@/components/ticket-lifecycle'
import { TicketOutputsCard } from '@/components/ticket-outputs'
import { TicketRelationshipsCard } from '@/components/ticket-relationships-card'
import { TicketReopenPanel } from '@/components/ticket-reopen-panel'
import { TicketEscalationPanel } from '@/components/ticket-escalation-panel'
import { TicketReviewPanel } from '@/components/ticket-review-panel'
import { TicketTimeline } from '@/components/ticket-timeline'
import { WorkStreamSummaryCard } from '@/components/work-stream-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { DetailPageSkeleton } from '@/components/ui/skeleton'
import { formatApiError } from '@/lib/api/client'
import { cn } from '@/lib/utils'
import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']
type WorkStream = components['schemas']['WorkStream']
type WorkflowPhase = components['schemas']['WorkflowPhase']

type WorkflowPositionData = {
  phases: WorkflowPhase[]
  currentPhaseId?: string
}
type ProjectTicketsState = {
  projectId: string | null
  tickets: Ticket[] | null
  error: string | null
}

type ReviewBanner =
  | { kind: 'reopened' }
  | { kind: 'next-in-stream'; nextTicketId: string; decision: 'approved' | 'rejected' }
  | { kind: 'more-elsewhere'; decision: 'approved' | 'rejected' }
  | { kind: 'project-empty'; decision: 'approved' | 'rejected' }
  | { kind: 'simple'; decision: 'approved' | 'rejected' }
  | { kind: 'followup-error'; message: string; decision: 'approved' | 'rejected' }

const TYPE_STYLES: Record<string, string> = {
  task: 'bg-blue-500/15 text-blue-300 border-blue-500/30',
  bug: 'bg-red-500/15 text-red-300 border-red-500/30',
  spike: 'bg-amber-500/15 text-amber-300 border-amber-500/30',
  review: 'bg-purple-500/15 text-purple-300 border-purple-500/30',
}

const PRIORITY_LABELS: Record<number, { label: string; className: string }> = {
  0: { label: 'P0 Critical', className: 'text-red-400' },
  1: { label: 'P1 High', className: 'text-orange-400' },
  2: { label: 'P2 Normal', className: 'text-muted-foreground' },
  3: { label: 'P3 Low', className: 'text-muted-foreground/60' },
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return iso
  }
}

export function TicketDetailPage() {
  const { orgId, projectId, ticketId } = useParams<{
    orgId: string
    projectId: string
    ticketId: string
  }>()
  const { client, token } = useAuth()
  const [ticket, setTicket] = useState<Ticket | null | undefined>(undefined)
  const [workflowPos, setWorkflowPos] = useState<WorkflowPositionData | null>(null)
  const [workStream, setWorkStream] = useState<
    WorkStream | null | undefined
  >(undefined)
  const [workStreamErr, setWorkStreamErr] = useState<string | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [reviewBanner, setReviewBanner] = useState<ReviewBanner | null>(null)
  const [projectTicketsState, setProjectTicketsState] = useState<ProjectTicketsState>({
    projectId: null,
    tickets: null,
    error: null,
  })
  const projectLabel = useProjectBreadcrumbLabel(projectId)
  const projectTickets =
    projectTicketsState.projectId === projectId
      ? projectTicketsState.tickets
      : null
  const projectTicketsErr =
    projectTicketsState.projectId === projectId
      ? projectTicketsState.error
      : null

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

  // Fetch workflow position when ticket has a workflow
  useEffect(() => {
    if (!ticket?.workflow_id || !ticketId) {
      setWorkflowPos(null)
      return
    }
    let cancelled = false
    ;(async () => {
      const headers: Record<string, string> = { 'Content-Type': 'application/json' }
      if (token) headers['Authorization'] = `Bearer ${token}`
      const resp = await fetch(`/api/v1/tickets/${ticketId}/workflow`, { headers })
      if (cancelled || !resp.ok) return
      const data = await resp.json().catch(() => null)
      if (cancelled || !data?.position) return
      const pos = data.position
      setWorkflowPos({
        phases: pos.phases ?? [],
        currentPhaseId: pos.current_phase?.id,
      })
    })()
    return () => { cancelled = true }
  }, [ticket?.workflow_id, ticket?.workflow_phase, ticketId, token])

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
    const currentProjectId = projectId
    void (async () => {
      const { data, error, response } = await client.GET(
        '/projects/{projectID}/tickets',
        { params: { path: { projectID: currentProjectId } } },
      )
      if (cancelled) return
      if (!response.ok) {
        setProjectTicketsState({
          projectId: currentProjectId,
          tickets: [],
          error: formatApiError(error),
        })
        return
      }
      setProjectTicketsState({
        projectId: currentProjectId,
        tickets: data ?? [],
        error: null,
      })
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
    return <DetailPageSkeleton />
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

  const ticketType = ticket.type ?? 'task'
  const typeStyle = TYPE_STYLES[ticketType] ?? TYPE_STYLES.task
  const priority = PRIORITY_LABELS[ticket.priority ?? 2] ?? PRIORITY_LABELS[2]

  return (
    <div className="flex flex-col gap-6">
      {/* ── Header section ─────────────────────────────── */}
      <div className="flex flex-col gap-3">
        {/* Breadcrumbs */}
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

        {/* Title row */}
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center gap-2.5">
            <h1 className="text-xl font-semibold tracking-tight">
              {ticket.title ?? ticket.id}
            </h1>
            <Badge
              variant="outline"
              className={cn('text-[10px] uppercase', typeStyle)}
            >
              {ticketType}
            </Badge>
          </div>

          {/* Meta row */}
          <div className="flex flex-wrap items-center gap-4 text-xs text-muted-foreground">
            <span className="flex items-center gap-1 font-mono">
              <Hash className="size-3" />
              {ticket.id}
            </span>
            <span className={cn('flex items-center gap-1', priority.className)}>
              <Target className="size-3" />
              {priority.label}
            </span>
            {ticket.assigned_to ? (
              <span className="flex items-center gap-1">
                <User className="size-3" />
                {ticket.assigned_to}
              </span>
            ) : null}
            {ticket.created_at ? (
              <span className="flex items-center gap-1">
                <Calendar className="size-3" />
                {formatDate(ticket.created_at)}
              </span>
            ) : null}
            {ticket.updated_at && ticket.updated_at !== ticket.created_at ? (
              <span className="flex items-center gap-1 text-muted-foreground/60">
                <Clock className="size-3" />
                Updated {formatDate(ticket.updated_at)}
              </span>
            ) : null}
          </div>
        </div>

        {/* Lifecycle state visualization */}
        <TicketLifecycle
          currentState={ticket.state}
          workflowPhases={workflowPos?.phases}
          currentPhaseId={workflowPos?.currentPhaseId}
        />
      </div>

      {/* ── Escalation panel (awaiting_input state) ──────── */}
      {ticket.state === 'awaiting_input' && ticketId && projectId ? (
        <TicketEscalationPanel
          ticketId={ticketId}
          projectId={projectId}
          onResolved={() => void reloadTicket()}
        />
      ) : null}

      {/* ── Objective section ──────────────────────────── */}
      {obj?.description ? (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-sm">
              <Target className="size-4 text-muted-foreground" />
              Objective
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <p className="text-muted-foreground whitespace-pre-wrap text-sm leading-relaxed">
              {obj.description}
            </p>
            {obj.success_criteria && obj.success_criteria.length > 0 ? (
              <div className="flex flex-col gap-1.5">
                <p className="text-[11px] font-medium tracking-wide uppercase text-muted-foreground/60">
                  Success Criteria
                </p>
                <ul className="flex flex-col gap-1">
                  {obj.success_criteria.map((c, i) => (
                    <li
                      key={i}
                      className="flex items-start gap-2 text-sm"
                    >
                      <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-emerald-500/40" />
                      <span className="text-muted-foreground">{c}</span>
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            {obj.acceptance_test ? (
              <div className="flex flex-col gap-1.5">
                <p className="text-[11px] font-medium tracking-wide uppercase text-muted-foreground/60">
                  Acceptance Test
                </p>
                <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] px-3 py-2">
                  <code className="font-mono text-xs text-muted-foreground">
                    {obj.acceptance_test}
                  </code>
                </div>
              </div>
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      <TicketTimeline
        ticketId={ticketId}
        currentState={ticket.state}
        createdAt={ticket.created_at}
      />

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

      {/* ── Review panel (awaiting_validation state) ───── */}
      {ticket.state === 'awaiting_validation' && ticketId ? (
        <TicketReviewPanel ticketId={ticketId} onReviewed={handleAfterReview} />
      ) : null}

      {/* ── Relationships ──────────────────────────────── */}
      <TicketRelationshipsCard
        orgId={orgId}
        projectId={projectId}
        ticket={ticket}
        projectTickets={projectTickets}
        projectTicketsError={projectTicketsErr}
      />

      {/* ── Work stream ────────────────────────────────── */}
      {ticket.work_stream_id ? (
        workStreamErr ? (
          <WorkStreamSummaryCard stream={null} errorMessage={workStreamErr} />
        ) : (
          <div className="flex flex-col gap-2">
            <WorkStreamSummaryCard stream={workStream ?? null} />
            {workStream?.id ? (
              <div className="flex flex-wrap gap-3 text-sm">
                <Link
                  className="text-primary hover:underline transition-colors"
                  to={`/orgs/${orgId}/projects/${projectId}/work-streams/${workStream.id}`}
                >
                  Manage work stream
                </Link>
                <Link
                  className="text-primary hover:underline transition-colors"
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

      {/* ── Outputs ────────────────────────────────────── */}
      <TicketOutputsCard outputs={ticket.outputs} />

      {/* ── Execution trace ────────────────────────────── */}
      <ExecutionTraceCard ticketId={ticketId} ticketState={ticket.state} />

      {/* ── Reopen panel (closed state) ─────────────────── */}
      {ticket.state === 'closed' && ticketId ? (
        <TicketReopenPanel ticketId={ticketId} onReopened={handleAfterReopen} />
      ) : null}
    </div>
  )
}
