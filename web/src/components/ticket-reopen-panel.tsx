import { useState } from 'react'
import { RotateCcw } from 'lucide-react'

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

type TicketReopenPanelProps = {
  ticketId: string
  onReopened: () => void | Promise<void>
}

/** Shown when a ticket is **done**: move it back to **awaiting_review** (same API as review; decision reopened). */
export function TicketReopenPanel({
  ticketId,
  onReopened,
}: TicketReopenPanelProps) {
  const { client } = useAuth()
  const [notes, setNotes] = useState('')
  const [busy, setBusy] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)

  async function submit() {
    setBusy(true)
    setFormError(null)
    const { error, response } = await client.POST('/tickets/{ticketID}/reviews', {
      params: { path: { ticketID: ticketId } },
      body: {
        decision: 'reopened',
        notes: notes.trim() || undefined,
      },
    })
    setBusy(false)
    if (!response.ok) {
      setFormError(formatApiError(error))
      return
    }
    setNotes('')
    await onReopened()
  }

  if (!showForm) {
    return (
      <div className="flex items-center gap-3 rounded-lg border border-white/[0.06] bg-white/[0.02] px-4 py-3">
        <RotateCcw className="size-3.5 text-muted-foreground/50" />
        <span className="text-sm text-muted-foreground">
          This ticket is complete.
        </span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="ml-auto gap-1.5 text-xs"
          onClick={() => setShowForm(true)}
        >
          <RotateCcw className="size-3" />
          Reopen
        </Button>
      </div>
    )
  }

  return (
    <Card className="border-amber-500/15 bg-amber-500/[0.02]">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-sm">
          <RotateCcw className="size-4 text-amber-400" />
          Reopen for review
        </CardTitle>
        <p className="text-muted-foreground text-xs">
          Send this ticket back to the review queue. Outputs are preserved.
        </p>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {formError ? (
          <div className="rounded-lg border border-red-500/20 bg-red-500/[0.06] px-3 py-2">
            <p className="text-destructive text-sm">{formError}</p>
          </div>
        ) : null}
        <label className="flex flex-col gap-1.5">
          <span className="text-muted-foreground text-xs font-medium">
            Reason (optional)
          </span>
          <textarea
            className={cn(
              'min-h-[64px] rounded-lg border bg-white/[0.03] px-3 py-2.5 text-sm transition-colors',
              'border-white/[0.08] placeholder:text-muted-foreground/40',
              'focus:border-white/[0.15] focus:outline-none focus:ring-1 focus:ring-white/[0.08]',
              'disabled:opacity-50',
            )}
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            disabled={busy}
            rows={2}
            placeholder="Why reopen..."
          />
        </label>
        <div className="flex gap-2">
          <Button
            type="button"
            variant="secondary"
            disabled={busy}
            onClick={() => void submit()}
            className="gap-1.5"
          >
            <RotateCcw className="size-3.5" />
            Reopen for review
          </Button>
          <Button
            type="button"
            variant="ghost"
            disabled={busy}
            onClick={() => setShowForm(false)}
          >
            Cancel
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
