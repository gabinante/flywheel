import { useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
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
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="reopen-note">Note (optional)</Label>
          <Textarea
            id="reopen-note"
            className="min-h-[72px]"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            disabled={busy}
            rows={3}
            placeholder="Why reopen…"
          />
        </div>
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
