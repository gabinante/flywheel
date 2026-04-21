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
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import { cn } from '@/lib/utils'
import type { components } from '@/lib/api/v1'

type TraceStep = components['schemas']['TraceStep']

type StepType = NonNullable<TraceStep['type']>

const STEP_TYPE_CONFIG: Record<
  StepType,
  { label: string; icon: typeof Terminal; dotColor: string; className: string }
> = {
  tool_call: {
    label: 'Tool Call',
    icon: Terminal,
    dotColor: 'bg-blue-400',
    className: 'bg-blue-500/15 text-blue-300 border-blue-500/30',
  },
  observation: {
    label: 'Observation',
    icon: Eye,
    dotColor: 'bg-emerald-400',
    className: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30',
  },
  thought: {
    label: 'Thought',
    icon: Brain,
    dotColor: 'bg-purple-400',
    className: 'bg-purple-500/15 text-purple-300 border-purple-500/30',
  },
  error: {
    label: 'Error',
    icon: AlertTriangle,
    dotColor: 'bg-red-400',
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

function formatRelativeTime(current: string, previous: string): string | null {
  try {
    const diff = new Date(current).getTime() - new Date(previous).getTime()
    if (diff < 0 || isNaN(diff)) return null
    if (diff < 1000) return '<1s'
    if (diff < 60_000) return `${Math.round(diff / 1000)}s`
    if (diff < 3600_000) return `${Math.round(diff / 60_000)}m`
    return `${Math.round(diff / 3600_000)}h`
  } catch {
    return null
  }
}

function TimelineStep({
  step,
  isLast,
  previousStep,
}: {
  step: TraceStep
  isLast: boolean
  previousStep?: TraceStep
}) {
  const [expanded, setExpanded] = useState(false)

  const stepType = step.type ?? 'thought'
  const config = STEP_TYPE_CONFIG[stepType]
  const Icon = config.icon
  const isError = stepType === 'error'

  const hasPayload =
    step.payload !== undefined &&
    step.payload !== null &&
    Object.keys(step.payload).length > 0

  const relTime =
    step.created_at && previousStep?.created_at
      ? formatRelativeTime(step.created_at, previousStep.created_at)
      : null

  return (
    <div className="group relative flex gap-3">
      {/* Timeline track */}
      <div className="flex flex-col items-center">
        {/* Dot */}
        <div
          className={cn(
            'relative z-10 mt-2.5 size-2.5 shrink-0 rounded-full ring-2 ring-background transition-all duration-200',
            config.dotColor,
            isError && 'ring-red-500/20',
          )}
        />
        {/* Vertical line */}
        {!isLast ? (
          <div className="w-px flex-1 bg-border/40" />
        ) : null}
      </div>

      {/* Content */}
      <div
        className={cn(
          'mb-3 flex-1 rounded-lg border transition-colors duration-150',
          isError
            ? 'border-red-500/20 bg-red-500/[0.04]'
            : 'border-white/[0.06] bg-white/[0.02] hover:bg-white/[0.04]',
        )}
      >
        <button
          type="button"
          className="flex w-full items-center gap-2.5 px-3 py-2 text-left"
          onClick={() => hasPayload && setExpanded(!expanded)}
          disabled={!hasPayload}
        >
          {hasPayload ? (
            expanded ? (
              <ChevronDown className="text-muted-foreground size-3 shrink-0" />
            ) : (
              <ChevronRight className="text-muted-foreground size-3 shrink-0" />
            )
          ) : (
            <span className="size-3 shrink-0" />
          )}

          <Badge
            variant="outline"
            className={cn('gap-1 px-1.5 py-0.5 text-[10px]', config.className)}
          >
            <Icon className="size-2.5" />
            {config.label}
          </Badge>

          {relTime ? (
            <span className="text-muted-foreground/50 text-[10px]">
              +{relTime}
            </span>
          ) : null}

          {step.created_at ? (
            <span className="text-muted-foreground/60 ml-auto shrink-0 font-mono text-[10px]">
              {formatTimestamp(step.created_at)}
            </span>
          ) : null}
        </button>

        {expanded && hasPayload ? (
          <div className="border-t border-white/[0.06] px-3 py-2.5">
            <pre className="max-h-64 overflow-auto whitespace-pre-wrap font-mono text-xs text-muted-foreground leading-relaxed">
              {JSON.stringify(step.payload, null, 2)}
            </pre>
          </div>
        ) : null}
      </div>
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
  const [collapsed, setCollapsed] = useState(false)
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
          <div className="flex items-center gap-2 text-muted-foreground text-sm">
            <RefreshCw className="size-3.5 animate-spin" />
            Loading trace...
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

  const stepCount = steps?.length ?? 0

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div className="flex items-center gap-2.5">
          <button
            type="button"
            className="flex items-center gap-1.5 transition-colors hover:text-foreground"
            onClick={() => setCollapsed(!collapsed)}
          >
            {collapsed ? (
              <ChevronRight className="text-muted-foreground size-3.5" />
            ) : (
              <ChevronDown className="text-muted-foreground size-3.5" />
            )}
            <CardTitle className="text-sm">Execution Trace</CardTitle>
          </button>
          {stepCount > 0 ? (
            <Badge variant="muted" className="text-[10px] tabular-nums">
              {stepCount} step{stepCount !== 1 ? 's' : ''}
            </Badge>
          ) : null}
          {isInProgress ? (
            <span className="flex items-center gap-1 text-[10px] text-emerald-400/70">
              <span className="size-1.5 animate-pulse rounded-full bg-emerald-400" />
              Live
            </span>
          ) : null}
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 gap-1.5 px-2 text-xs"
          onClick={handleRefresh}
          disabled={refreshing}
        >
          <RefreshCw
            className={cn('size-3', refreshing && 'animate-spin')}
          />
          Refresh
        </Button>
      </CardHeader>

      {!collapsed ? (
        <CardContent>
          {!steps || steps.length === 0 ? (
            <div className="flex flex-col items-center gap-2 py-6 text-center">
              <Terminal className="text-muted-foreground/30 size-8" />
              <p className="text-muted-foreground text-sm">
                No execution steps recorded yet.
              </p>
              {isInProgress ? (
                <p className="text-muted-foreground/50 text-xs">
                  Steps will appear here as the agent works.
                </p>
              ) : null}
            </div>
          ) : (
            <div className="flex flex-col">
              {steps.map((step, i) => (
                <TimelineStep
                  key={step.id ?? i}
                  step={step}
                  isLast={i === steps.length - 1}
                  previousStep={i > 0 ? steps[i - 1] : undefined}
                />
              ))}
            </div>
          )}
        </CardContent>
      ) : null}
    </Card>
  )
}
