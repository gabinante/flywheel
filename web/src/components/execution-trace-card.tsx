import { useCallback, useEffect, useRef, useState } from 'react'
import {
  AlertTriangle,
  Brain,
  ChevronDown,
  ChevronRight,
  Eye,
  RefreshCw,
  Terminal,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type TraceStep = components['schemas']['TraceStep']

type StepType = NonNullable<TraceStep['type']>

const STEP_TYPE_CONFIG: Record<
  StepType,
  { label: string; icon: typeof Terminal; className: string }
> = {
  tool_call: {
    label: 'Tool Call',
    icon: Terminal,
    className: 'bg-blue-500/15 text-blue-300 border-blue-500/30',
  },
  observation: {
    label: 'Observation',
    icon: Eye,
    className: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30',
  },
  thought: {
    label: 'Thought',
    icon: Brain,
    className: 'bg-purple-500/15 text-purple-300 border-purple-500/30',
  },
  error: {
    label: 'Error',
    icon: AlertTriangle,
    className: 'bg-red-500/15 text-red-300 border-red-500/30',
  },
}

/** States where the ticket is still being worked on and trace may grow. */
const IN_PROGRESS_STATES = new Set(['claimed', 'executing'])

function formatTimestamp(iso: string): string {
  try {
    const d = new Date(iso)
    return d.toLocaleString(undefined, {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  } catch {
    return iso
  }
}

function TraceStepRow({ step }: { step: TraceStep }) {
  const [expanded, setExpanded] = useState(false)

  const stepType = step.type ?? 'thought'
  const config = STEP_TYPE_CONFIG[stepType]
  const Icon = config.icon
  const isError = stepType === 'error'

  const hasPayload =
    step.payload !== undefined &&
    step.payload !== null &&
    Object.keys(step.payload).length > 0

  return (
    <div
      className={`rounded-lg border transition-colors ${
        isError
          ? 'border-red-500/30 bg-red-500/5'
          : 'border-border/50 bg-muted/20 hover:bg-muted/40'
      }`}
    >
      <button
        type="button"
        className="flex w-full items-center gap-3 px-3 py-2.5 text-left"
        onClick={() => hasPayload && setExpanded(!expanded)}
        disabled={!hasPayload}
      >
        {hasPayload ? (
          expanded ? (
            <ChevronDown className="text-muted-foreground size-3.5 shrink-0" />
          ) : (
            <ChevronRight className="text-muted-foreground size-3.5 shrink-0" />
          )
        ) : (
          <span className="size-3.5 shrink-0" />
        )}

        <Badge
          variant="outline"
          className={`gap-1 px-1.5 py-0.5 text-[10px] ${config.className}`}
        >
          <Icon className="size-2.5" />
          {config.label}
        </Badge>

        {step.created_at ? (
          <span className="text-muted-foreground ml-auto shrink-0 font-mono text-[10px]">
            {formatTimestamp(step.created_at)}
          </span>
        ) : null}
      </button>

      {expanded && hasPayload ? (
        <div className="border-t border-border/30 px-3 py-2.5">
          <pre className="max-h-64 overflow-auto whitespace-pre-wrap font-mono text-xs text-muted-foreground">
            {JSON.stringify(step.payload, null, 2)}
          </pre>
        </div>
      ) : null}
    </div>
  )
}

type ExecutionTraceCardProps = {
  ticketId: string
  ticketState?: string
}

export function ExecutionTraceCard({
  ticketId,
  ticketState,
}: ExecutionTraceCardProps) {
  const { client } = useAuth()
  const [steps, setSteps] = useState<TraceStep[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const isInProgress = ticketState
    ? IN_PROGRESS_STATES.has(ticketState)
    : false

  const fetchTrace = useCallback(
    async (opts?: { silent?: boolean }) => {
      if (!opts?.silent) setLoading(true)
      const { data, error, response } = await client.GET(
        '/tickets/{ticketID}/trace',
        { params: { path: { ticketID: ticketId } } },
      )
      if (!response.ok) {
        if (response.status === 404) {
          setSteps([])
          setErr(null)
        } else {
          setErr(formatApiError(error))
        }
      } else {
        setSteps(data?.steps ?? [])
        setErr(null)
      }
      setLoading(false)
      setRefreshing(false)
    },
    [client, ticketId],
  )

  useEffect(() => {
    let cancelled = false
    void (async () => {
      await fetchTrace()
      if (cancelled) return
    })()
    return () => {
      cancelled = true
    }
  }, [fetchTrace])

  // Auto-refresh every 10s when in-progress
  useEffect(() => {
    if (isInProgress) {
      intervalRef.current = setInterval(() => {
        void fetchTrace({ silent: true })
      }, 10_000)
    }
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current)
        intervalRef.current = null
      }
    }
  }, [isInProgress, fetchTrace])

  const handleRefresh = () => {
    setRefreshing(true)
    void fetchTrace({ silent: true })
  }

  if (loading && steps === null) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Execution Trace</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-1.5">
            <Skeleton className="h-3 w-24" />
            <Skeleton className="h-10 w-full rounded-lg" />
            <Skeleton className="h-10 w-full rounded-lg" />
            <Skeleton className="h-10 w-full rounded-lg" />
          </div>
        </CardContent>
      </Card>
    )
  }

  if (err) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Execution Trace</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-destructive text-sm">{err}</p>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle className="text-sm">Execution Trace</CardTitle>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 gap-1.5 px-2 text-xs"
          onClick={handleRefresh}
          disabled={refreshing}
        >
          <RefreshCw
            className={`size-3 ${refreshing ? 'animate-spin' : ''}`}
          />
          Refresh
        </Button>
      </CardHeader>
      <CardContent>
        {!steps || steps.length === 0 ? (
          <div className="flex flex-col items-center gap-2 py-6 text-center">
            <Terminal className="text-muted-foreground/50 size-8" />
            <p className="text-muted-foreground text-sm">
              No execution steps recorded yet.
            </p>
            {isInProgress ? (
              <p className="text-muted-foreground/70 text-xs">
                Steps will appear here as the agent works.
              </p>
            ) : null}
          </div>
        ) : (
          <div className="flex flex-col gap-1.5">
            <p className="text-muted-foreground mb-1 text-xs">
              {steps.length} step{steps.length !== 1 ? 's' : ''}
              {isInProgress ? ' · auto-refreshing' : ''}
            </p>
            {steps.map((step, i) => (
              <TraceStepRow key={step.id ?? i} step={step} />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
