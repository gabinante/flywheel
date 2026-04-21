import { useState } from 'react'
import { Check, MessageSquare, X } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import { cn } from '@/lib/utils'

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
  const [showNotes, setShowNotes] = useState(false)

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
    <Card className="border-purple-500/20 bg-purple-500/[0.03]">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-sm">
          <div className="flex size-5 items-center justify-center rounded-full bg-purple-500/20">
            <MessageSquare className="size-3 text-purple-400" />
          </div>
          Review Required
        </CardTitle>
        <p className="text-muted-foreground text-xs">
          This ticket is awaiting your review. Approve to mark as done, or reject with feedback.
        </p>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {formError ? (
          <div className="rounded-lg border border-red-500/20 bg-red-500/[0.06] px-3 py-2">
            <p className="text-destructive text-sm">{formError}</p>
          </div>
        ) : null}

        {/* Notes toggle and textarea */}
        {!showNotes ? (
          <button
            type="button"
            className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
            onClick={() => setShowNotes(true)}
          >
            <MessageSquare className="size-3" />
            Add feedback note...
          </button>
        ) : (
          <label className="flex flex-col gap-1.5">
            <span className="text-muted-foreground text-xs font-medium">
              Feedback (optional)
            </span>
            <textarea
              className={cn(
                'min-h-[80px] rounded-lg border bg-white/[0.03] px-3 py-2.5 text-sm transition-colors',
                'border-white/[0.08] placeholder:text-muted-foreground/40',
                'focus:border-white/[0.15] focus:outline-none focus:ring-1 focus:ring-white/[0.08]',
                'disabled:opacity-50',
              )}
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              disabled={busy}
              rows={3}
              placeholder="Share your review notes..."
            />
          </label>
        )}

        {/* Action buttons - prominent and clear */}
        <div className="flex flex-wrap items-center gap-3">
          <Button
            type="button"
            size="lg"
            disabled={busy}
            onClick={() => void submit('approved')}
            className="gap-2 bg-emerald-600 text-white hover:bg-emerald-500 border-emerald-500/30"
          >
            <Check className="size-4" />
            Approve
          </Button>
          <Button
            type="button"
            variant="destructive"
            size="lg"
            disabled={busy}
            onClick={() => void submit('rejected')}
            className="gap-2"
          >
            <X className="size-4" />
            Reject
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
