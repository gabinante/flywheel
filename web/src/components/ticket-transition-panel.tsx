import { useState } from 'react'
import { ArrowRight, ChevronDown } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'

type TicketTransitionPanelProps = {
  ticketId: string
  currentState: string
  onTransitioned: () => void | Promise<void>
}

type TransitionOption = {
  trigger: string
  label: string
  description: string
  targetState: string
  variant: 'default' | 'destructive' | 'outline'
  confirm?: boolean
  payload?: Record<string, unknown>
}

const STATE_LABELS: Record<string, string> = {
  draft: 'Draft',
  planning: 'Planning',
  awaiting_input: 'Awaiting Input',
  executing: 'Executing',
  awaiting_validation: 'Awaiting Validation',
  validated: 'Validated',
  closed: 'Closed',
}

function getTransitions(state: string): TransitionOption[] {
  switch (state) {
    case 'draft':
      return [
        { trigger: 'claim', label: 'Claim', description: 'Claim this ticket and begin planning', targetState: 'planning', variant: 'default' },
        { trigger: 'cancel', label: 'Cancel', description: 'Close this ticket without completing it', targetState: 'closed', variant: 'destructive', confirm: true },
      ]
    case 'planning':
      return [
        { trigger: 'start', label: 'Start execution', description: 'Move to executing state', targetState: 'executing', variant: 'default' },
        { trigger: 'cancel', label: 'Cancel', description: 'Close this ticket without completing it', targetState: 'closed', variant: 'destructive', confirm: true },
      ]
    case 'executing':
      return [
        { trigger: 'submit', label: 'Submit for review', description: 'Move to awaiting validation', targetState: 'awaiting_validation', variant: 'default' },
        { trigger: 'replan', label: 'Send back to planning', description: 'Return to planning phase', targetState: 'planning', variant: 'outline' },
        { trigger: 'fail', label: 'Mark as failed', description: 'Return to draft for retry', targetState: 'draft', variant: 'destructive', confirm: true },
        { trigger: 'cancel', label: 'Cancel', description: 'Close this ticket without completing it', targetState: 'closed', variant: 'destructive', confirm: true },
      ]
    case 'awaiting_input':
      return [
        { trigger: 'provide_input', label: 'Resume (to executing)', description: 'Unblock and return to executing', targetState: 'executing', variant: 'default', payload: { resume_state: 'executing' } },
        { trigger: 'provide_input', label: 'Resume (to planning)', description: 'Unblock and return to planning', targetState: 'planning', variant: 'outline' },
        { trigger: 'cancel', label: 'Cancel', description: 'Close this ticket without completing it', targetState: 'closed', variant: 'destructive', confirm: true },
      ]
    case 'awaiting_validation':
      return [
        { trigger: 'approve', label: 'Approve', description: 'Mark as validated', targetState: 'validated', variant: 'default' },
        { trigger: 'reject', label: 'Reject', description: 'Send back to executing for rework', targetState: 'executing', variant: 'outline' },
        { trigger: 'rollback', label: 'Rollback', description: 'Return to draft', targetState: 'draft', variant: 'destructive', confirm: true },
        { trigger: 'cancel', label: 'Cancel', description: 'Close this ticket without completing it', targetState: 'closed', variant: 'destructive', confirm: true },
      ]
    case 'validated':
      return [
        { trigger: 'close', label: 'Close', description: 'Mark as complete', targetState: 'closed', variant: 'default' },
        { trigger: 'invalidate', label: 'Invalidate', description: 'Return to planning for rework', targetState: 'planning', variant: 'outline' },
        { trigger: 'rollback', label: 'Rollback', description: 'Return to draft', targetState: 'draft', variant: 'destructive', confirm: true },
      ]
    case 'closed':
      return [
        { trigger: 'reopen', label: 'Reopen', description: 'Return to draft', targetState: 'draft', variant: 'outline' },
      ]
    default:
      return []
  }
}

export function TicketTransitionPanel({
  ticketId,
  currentState,
  onTransitioned,
}: TicketTransitionPanelProps) {
  const { client } = useAuth()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState(false)
  const [confirming, setConfirming] = useState<string | null>(null)

  const transitions = getTransitions(currentState)

  if (transitions.length === 0) return null

  async function fire(trigger: string, payload?: Record<string, unknown>) {
    setBusy(true)
    setError(null)
    const { error: apiError, response } = await client.POST(
      '/tickets/{ticketID}/transitions',
      {
        params: { path: { ticketID: ticketId } },
        body: { trigger, actor: 'human', ...(payload ? { payload: payload as Record<string, undefined> } : {}) },
      },
    )
    setBusy(false)
    setConfirming(null)
    if (!response.ok) {
      setError(formatApiError(apiError))
      return
    }
    await onTransitioned()
  }

  return (
    <Card className="border-white/[0.06] bg-white/[0.02]">
      <CardHeader className="cursor-pointer py-3" onClick={() => setExpanded(!expanded)}>
        <CardTitle className="flex items-center gap-2 text-sm">
          <ArrowRight className="size-3.5 text-muted-foreground/50" />
          <span className="flex-1">Transition state</span>
          <span className="text-xs text-muted-foreground font-normal">
            {STATE_LABELS[currentState] ?? currentState}
          </span>
          <ChevronDown className={`size-3.5 text-muted-foreground transition-transform ${expanded ? 'rotate-180' : ''}`} />
        </CardTitle>
      </CardHeader>
      {expanded ? (
        <CardContent className="flex flex-col gap-2 pt-0">
          {error ? (
            <div className="rounded-lg border border-red-500/20 bg-red-500/[0.06] px-3 py-2">
              <p className="text-destructive text-sm">{error}</p>
            </div>
          ) : null}
          {transitions.map((t) => (
            <div
              key={t.trigger}
              className="flex items-center gap-3 rounded-lg border border-white/[0.06] bg-white/[0.02] px-3 py-2"
            >
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium">{t.label}</p>
                <p className="text-xs text-muted-foreground">
                  {t.description}
                  <span className="text-muted-foreground/50"> → {STATE_LABELS[t.targetState] ?? t.targetState}</span>
                </p>
              </div>
              {t.confirm && confirming !== t.trigger ? (
                <Button
                  size="sm"
                  variant={t.variant}
                  disabled={busy}
                  onClick={() => setConfirming(t.trigger)}
                  className="shrink-0"
                >
                  {t.label}
                </Button>
              ) : t.confirm && confirming === t.trigger ? (
                <div className="flex items-center gap-1.5 shrink-0">
                  <span className="text-xs text-muted-foreground">Sure?</span>
                  <Button
                    size="sm"
                    variant="destructive"
                    disabled={busy}
                    onClick={() => void fire(t.trigger, t.payload)}
                  >
                    {busy ? 'Working…' : 'Confirm'}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    onClick={() => setConfirming(null)}
                  >
                    No
                  </Button>
                </div>
              ) : (
                <Button
                  size="sm"
                  variant={t.variant}
                  disabled={busy}
                  onClick={() => void fire(t.trigger, t.payload)}
                  className="shrink-0"
                >
                  {busy ? 'Working…' : t.label}
                </Button>
              )}
            </div>
          ))}
        </CardContent>
      ) : null}
    </Card>
  )
}
