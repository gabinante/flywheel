import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'

import { ExecutionTraceCard } from '@/components/execution-trace-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'
import { cn } from '@/lib/utils'

type Ticket = components['schemas']['Ticket']
type Escalation = components['schemas']['Escalation']

function Shimmer({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded bg-muted/60', className)} />
}

export function TicketInspector({
  ticketId,
  escalation,
  orgId,
  projectId,
  onReviewComplete,
}: {
  ticketId: string
  escalation?: Escalation | null
  orgId: string
  projectId: string
  onReviewComplete?: () => void
}) {
  const { client } = useAuth()
  const [ticket, setTicket] = useState<Ticket | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reviewBusy, setReviewBusy] = useState(false)
  const [reviewNotes, setReviewNotes] = useState('')
  const [reviewError, setReviewError] = useState<string | null>(null)
  const [escalationAnswer, setEscalationAnswer] = useState('')
  const [escalationBusy, setEscalationBusy] = useState(false)
  const [escalationError, setEscalationError] = useState<string | null>(null)

  const fetchTicket = useCallback(async () => {
    setLoading(true)
    const { data, error: err, response } = await client.GET('/tickets/{ticketID}', {
      params: { path: { ticketID: ticketId } },
    })
    if (!response.ok) {
      setError(formatApiError(err))
      setTicket(null)
    } else {
      setTicket(data ?? null)
      setError(null)
    }
    setLoading(false)
  }, [client, ticketId])

  useEffect(() => {
    void fetchTicket()
  }, [fetchTicket])

  const submitReview = useCallback(
    async (decision: 'approved' | 'rejected') => {
      setReviewBusy(true)
      setReviewError(null)
      const { error: err, response } = await client.POST(
        '/tickets/{ticketID}/reviews',
        {
          params: { path: { ticketID: ticketId } },
          body: { decision, notes: reviewNotes.trim() || undefined },
        },
      )
      setReviewBusy(false)
      if (!response.ok) {
        setReviewError(formatApiError(err))
        return
      }
      setReviewNotes('')
      await fetchTicket()
      onReviewComplete?.()
    },
    [client, ticketId, reviewNotes, fetchTicket, onReviewComplete],
  )

  const resolveEscalation = useCallback(async () => {
    const escalationId = escalation?.id
    const answer = escalationAnswer.trim()
    if (!escalationId || !answer) return

    setEscalationBusy(true)
    setEscalationError(null)
    const { error: err, response } = await client.POST(
      '/tickets/{ticketID}/escalations/{escalationID}/resolve',
      {
        params: { path: { ticketID: ticketId, escalationID: escalationId } },
        body: { answer },
      },
    )
    setEscalationBusy(false)

    if (!response.ok) {
      setEscalationError(formatApiError(err))
      return
    }

    setEscalationAnswer('')
    await fetchTicket()
    onReviewComplete?.()
  }, [client, escalation?.id, escalationAnswer, fetchTicket, onReviewComplete, ticketId])

  if (loading) {
    return (
      <div className="flex flex-col gap-3 p-4">
        <Shimmer className="h-6 w-48" />
        <Shimmer className="h-4 w-32" />
        <Shimmer className="h-20 w-full rounded-lg" />
        <Shimmer className="h-40 w-full rounded-lg" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-4">
        <p className="text-destructive text-sm">{error}</p>
      </div>
    )
  }

  if (!ticket) {
    return (
      <div className="p-4">
        <p className="text-muted-foreground text-sm">Ticket not found.</p>
      </div>
    )
  }

  const obj = ticket.objective
  const isAwaitingReview = ticket.state === 'awaiting_validation'
  const isAwaitingInput = ticket.state === 'awaiting_input'

  return (
    <div className="flex flex-col gap-4 p-4 animate-in fade-in duration-200">
      {/* Header */}
      <div className="flex flex-col gap-1">
        <div className="flex items-center gap-2">
          <h2 className="text-lg font-semibold tracking-tight truncate">
            {ticket.title ?? ticket.id}
          </h2>
          {ticket.state && (
            <Badge variant="secondary" className="shrink-0">
              {ticket.state}
            </Badge>
          )}
        </div>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-mono">{ticket.id}</span>
          {ticket.assigned_to && (
            <>
              <span>·</span>
              <span className="font-mono">{ticket.assigned_to}</span>
            </>
          )}
        </div>
      </div>

      {/* Human Unblock — shown before objective for prominence */}
      {isAwaitingInput ? (
        escalation?.id ? (
          <Card className="border-red-500/30 bg-red-500/5">
            <CardHeader className="pb-2">
              <div className="flex items-center gap-3">
                <span className="relative flex size-3 shrink-0">
                  <span className="absolute inline-flex size-full animate-ping rounded-full bg-red-400 opacity-75" />
                  <span className="relative inline-flex size-3 rounded-full bg-red-500" />
                </span>
                <CardTitle className="text-sm font-semibold text-red-400">
                  BLOCKED — Awaiting Your Input
                </CardTitle>
              </div>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              {escalation.question ? (
                <p className="text-base font-semibold leading-snug text-foreground">
                  {escalation.question}
                </p>
              ) : null}

              {escalation.reason ? (
                <p className="text-sm leading-relaxed text-muted-foreground">
                  <span className="font-medium text-muted-foreground/80">Context: </span>
                  {escalation.reason}
                </p>
              ) : null}

              {escalationError ? (
                <p className="text-sm text-destructive">{escalationError}</p>
              ) : null}

              <textarea
                className="border-input bg-background min-h-[110px] rounded-md border px-3 py-2 text-sm disabled:opacity-50"
                value={escalationAnswer}
                onChange={(e) => setEscalationAnswer(e.target.value)}
                disabled={escalationBusy}
                placeholder="Type your answer..."
              />
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                <p className="text-xs text-muted-foreground">
                  Your answer resumes execution.
                </p>
                <Button
                  type="button"
                  size="sm"
                  disabled={escalationBusy || !escalationAnswer.trim()}
                  onClick={() => void resolveEscalation()}
                >
                  {escalationBusy ? 'Sending...' : 'Send Answer'}
                </Button>
              </div>
            </CardContent>
          </Card>
        ) : (
          <Card className="border-red-500/30 bg-red-500/5">
            <CardContent className="flex items-start gap-3 py-4">
              <span className="relative mt-1 flex size-3 shrink-0">
                <span className="absolute inline-flex size-full animate-ping rounded-full bg-red-400 opacity-75" />
                <span className="relative inline-flex size-3 rounded-full bg-red-500" />
              </span>
              <div className="flex flex-col gap-1">
                <p className="text-sm font-semibold text-red-400">
                  BLOCKED — Awaiting Your Input
                </p>
                <p className="text-sm text-muted-foreground">
                  This ticket is waiting for human input.
                </p>
                <Link
                  to={`/orgs/${orgId}/projects/${projectId}/tickets/${ticketId}`}
                  className="text-sm text-primary hover:underline mt-1"
                >
                  Open full detail
                </Link>
              </div>
            </CardContent>
          </Card>
        )
      ) : null}

      {/* Objective */}
      {obj?.description && (
        <Card>
          <CardHeader>
            <CardTitle className="text-xs">Objective</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground whitespace-pre-wrap">
              {obj.description}
            </p>
            {obj.success_criteria && obj.success_criteria.length > 0 && (
              <ul className="mt-2 list-inside list-disc text-sm text-muted-foreground">
                {obj.success_criteria.map((c, i) => (
                  <li key={i}>{c}</li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      )}

      {/* Inline Review */}
      {isAwaitingReview && (
        <Card className="border-amber-500/20 bg-amber-500/5">
          <CardHeader>
            <CardTitle className="text-xs">Review</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            {reviewError && (
              <p className="text-destructive text-sm">{reviewError}</p>
            )}
            <textarea
              className="border-input bg-background min-h-[60px] rounded-md border px-3 py-2 text-sm disabled:opacity-50"
              value={reviewNotes}
              onChange={(e) => setReviewNotes(e.target.value)}
              disabled={reviewBusy}
              placeholder="Feedback (optional)"
              rows={3}
            />
            <div className="flex gap-2">
              <Button
                type="button"
                size="sm"
                disabled={reviewBusy}
                onClick={() => void submitReview('approved')}
              >
                Approve
              </Button>
              <Button
                type="button"
                size="sm"
                variant="destructive"
                disabled={reviewBusy}
                onClick={() => void submitReview('rejected')}
              >
                Reject
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Execution Trace */}
      <ExecutionTraceCard ticketId={ticketId} ticketState={ticket.state} />

      {/* Link to full detail */}
      <div className="flex gap-2 text-xs">
        <Link
          to={`/orgs/${orgId}/projects/${projectId}/tickets/${ticketId}`}
          className="text-primary hover:underline"
        >
          Open full detail
        </Link>
      </div>
    </div>
  )
}
