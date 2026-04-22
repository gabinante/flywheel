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

function Shimmer({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded bg-muted/60', className)} />
}

export function TicketInspector({
  ticketId,
  orgId,
  projectId,
  onReviewComplete,
}: {
  ticketId: string
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
      {ticket.state === 'awaiting_review' && (
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
