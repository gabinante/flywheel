import { useCallback, useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { useAPI } from '@/contexts/use-api'
import { sendOrchestratorMessage } from '@/lib/api/orchestrator-client'
import type { components } from '@/lib/api/v1'
import { summarizePayload, getWorkerOutput, STEP_TYPE_META } from '@/lib/trace-utils'
import { cn } from '@/lib/utils'

type Escalation = components['schemas']['Escalation']
type TraceStep = components['schemas']['TraceStep']

interface TicketEscalationPanelProps {
  ticketId: string
  projectId: string
  onResolved?: () => void
}

function TraceContext({ steps }: { steps: TraceStep[] }) {
  const contextSteps = steps.filter((s) => !getWorkerOutput(s)).slice(-3)
  if (contextSteps.length === 0) return null
  return (
    <div className="flex flex-col gap-2">
      <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground/60">
        Recent Agent Activity
      </p>
      <div className="flex flex-col gap-1.5 rounded-lg border border-white/[0.06] bg-white/[0.02] p-3">
        {contextSteps.map((step, i) => {
          const summary = summarizePayload(step)
          const meta = STEP_TYPE_META[step.type ?? 'thought']
          return (
            <div key={step.id ?? i} className="flex items-start gap-2 text-xs">
              <span className={cn('mt-1 size-1.5 shrink-0 rounded-full', meta.dotColor)} />
              <span className="shrink-0 font-medium text-muted-foreground/60">{meta.label}</span>
              {summary && (
                <span className="line-clamp-2 text-muted-foreground/80">{summary}</span>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function TicketEscalationPanel({
  ticketId,
  projectId,
  onResolved,
}: TicketEscalationPanelProps) {
  const { client } = useAPI()
  const [escalation, setEscalation] = useState<Escalation | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [answer, setAnswer] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [traceSteps, setTraceSteps] = useState<TraceStep[]>([])
  const [traceLoaded, setTraceLoaded] = useState(false)
  const [orchestratorInput, setOrchestratorInput] = useState('')
  const [orchestratorBusy, setOrchestratorBusy] = useState(false)
  const [orchestratorError, setOrchestratorError] = useState<string | null>(null)

  // Fetch escalation record
  useEffect(() => {
    let cancelled = false
    void (async () => {
      const { data, response } = await client.GET(
        '/projects/{projectID}/escalations',
        { params: { path: { projectID: projectId } } },
      )
      if (cancelled) return
      if (!response.ok || !data) {
        setEscalation(null)
        setLoaded(true)
        return
      }
      const list = data as Escalation[]
      const match = list.find((e) => e.ticket_id === ticketId) ?? null
      setEscalation(match)
      setLoaded(true)
    })()
    return () => {
      cancelled = true
    }
  }, [client, projectId, ticketId])

  // Fetch trace steps
  useEffect(() => {
    let cancelled = false
    void (async () => {
      const { data, response } = await client.GET('/tickets/{ticketID}/trace', {
        params: { path: { ticketID: ticketId } },
      })
      if (cancelled) return
      if (response.ok && data?.steps) setTraceSteps(data.steps)
      setTraceLoaded(true)
    })()
    return () => {
      cancelled = true
    }
  }, [client, ticketId])

  const resolve = useCallback(async () => {
    if (!escalation?.id || !answer.trim()) return
    setBusy(true)
    setError(null)
    const { response } = await client.POST(
      '/tickets/{ticketID}/escalations/{escalationID}/resolve',
      {
        params: {
          path: { ticketID: ticketId, escalationID: escalation.id },
        },
        body: { answer: answer.trim() },
      },
    )
    setBusy(false)
    if (!response.ok) {
      setError('Failed to resolve escalation. Please try again.')
      return
    }
    setAnswer('')
    onResolved?.()
  }, [client, ticketId, escalation, answer, onResolved])

  const sendToOrchestrator = useCallback(async () => {
    if (!orchestratorInput.trim()) return
    setOrchestratorBusy(true)
    setOrchestratorError(null)
    const { error } = await sendOrchestratorMessage(

      projectId,
      `[Ticket ${ticketId} is blocked — awaiting_input] ${orchestratorInput.trim()}`,
    )
    setOrchestratorBusy(false)
    if (error) {
      setOrchestratorError(error)
      return
    }
    setOrchestratorInput('')
    onResolved?.()
  }, [projectId, ticketId, orchestratorInput, onResolved])

  if (!loaded) return null

  // Fallback: ticket is awaiting_input but no escalation record found
  if (!escalation) {
    return (
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
          {traceLoaded && traceSteps.length > 0 && <TraceContext steps={traceSteps} />}

          <p className="text-sm text-muted-foreground">
            This ticket needs human input to continue. Provide guidance below to
            unblock it.
          </p>

          {orchestratorError && (
            <p className="text-sm text-destructive">{orchestratorError}</p>
          )}

          <Textarea
            value={orchestratorInput}
            onChange={(e) => setOrchestratorInput(e.target.value)}
            disabled={orchestratorBusy}
            placeholder="Type your guidance..."
          />
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-xs text-muted-foreground">
              Your message will be sent to the orchestrator.
            </p>
            <Button
              type="button"
              size="sm"
              disabled={orchestratorBusy || !orchestratorInput.trim()}
              onClick={() => void sendToOrchestrator()}
            >
              {orchestratorBusy ? 'Sending...' : 'Send to Orchestrator'}
            </Button>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
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

        {traceLoaded && traceSteps.length > 0 && <TraceContext steps={traceSteps} />}

        {error ? (
          <p className="text-sm text-destructive">{error}</p>
        ) : null}

        <Textarea
          value={answer}
          onChange={(e) => setAnswer(e.target.value)}
          disabled={busy}
          placeholder="Type your answer..."
        />
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs text-muted-foreground">
            Your answer resumes execution.
          </p>
          <Button
            type="button"
            size="sm"
            disabled={busy || !answer.trim()}
            onClick={() => void resolve()}
          >
            {busy ? 'Sending...' : 'Send Answer'}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
