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

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Reopen for review</CardTitle>
        <CardDescription>
          This ticket is done. Send it back to the review queue if it was
          approved by mistake or needs another pass. Outputs are kept.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {formError ? (
          <p className="text-destructive text-sm">{formError}</p>
        ) : null}
        <label className="flex flex-col gap-1 text-sm">
          <span className="text-muted-foreground">Note (optional)</span>
          <textarea
            className="min-h-[72px] rounded-xl border border-white/10 bg-white/[0.04] px-3 py-2 text-sm backdrop-blur-sm focus:border-primary/40 focus:outline-none disabled:opacity-50"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            disabled={busy}
            rows={3}
            placeholder="Why reopen…"
          />
        </label>
        <Button
          type="button"
          variant="secondary"
          disabled={busy}
          onClick={() => void submit()}
        >
          Reopen for review
        </Button>
      </CardContent>
    </Card>
  )
}
