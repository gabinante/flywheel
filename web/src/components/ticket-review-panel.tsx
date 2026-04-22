import { useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'

type TicketReviewPanelProps = {
  ticketId: string
  onReviewed: (decision: 'approved' | 'rejected') => void | Promise<void>
}

export function TicketReviewPanel({
  ticketId,
  onReviewed,
}: TicketReviewPanelProps) {
  const { client } = useAuth()
  const [notes, setNotes] = useState('')
  const [busy, setBusy] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)

  async function submit(decision: 'approved' | 'rejected') {
    setBusy(true)
    setFormError(null)
    const { error, response } = await client.POST('/tickets/{ticketID}/reviews', {
      params: { path: { ticketID: ticketId } },
      body: {
        decision,
        notes: notes.trim() || undefined,
      },
    })
    setBusy(false)
    if (!response.ok) {
      setFormError(formatApiError(error))
      return
    }
    setNotes('')
    await onReviewed(decision)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Review</CardTitle>
        <CardDescription>
          Approve or reject this ticket. Optional notes are stored with the
          review.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {formError ? (
          <p className="text-destructive text-sm">{formError}</p>
        ) : null}
        <label className="flex flex-col gap-1 text-sm">
          <span className="text-muted-foreground">Feedback (optional)</span>
          <textarea
            className="min-h-[88px] rounded-xl border border-white/10 bg-white/[0.04] px-3 py-2 text-sm backdrop-blur-sm focus:border-primary/40 focus:outline-none disabled:opacity-50"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            disabled={busy}
            rows={4}
          />
        </label>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            disabled={busy}
            onClick={() => void submit('approved')}
          >
            Approve
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={busy}
            onClick={() => void submit('rejected')}
          >
            Reject
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
