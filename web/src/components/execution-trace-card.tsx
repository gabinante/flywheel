import { useCallback, useEffect, useRef, useState } from 'react'
import {
  AlertTriangle,
  Brain,
  ChevronDown,
  ChevronRight,
  Eye,
  Loader2,
  RefreshCw,
  Terminal,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import { cn } from '@/lib/utils'
import {
  getWorkerOutput,
  summarizePayload,
} from '@/lib/trace-utils'
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
const IN_PROGRESS_STATES = new Set(['planning', 'executing'])
const LIVE_POLL_INTERVAL_MS = 1500
const PAGE_SIZE = 200

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
  const summary = summarizePayload(step)

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
          className="flex w-full items-start gap-2.5 px-3 py-2 text-left"
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

          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-2.5">
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
            </div>
            {summary ? (
              <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-muted-foreground/80">
                {summary}
              </p>
            ) : null}
          </div>

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

function WorkerLogConsole({
  steps,
  totalCount,
  isLive,
  loadingOlder,
  hasMore,
  onLoadOlder,
}: {
  steps: TraceStep[]
  totalCount: number
  isLive: boolean
  loadingOlder: boolean
  hasMore: boolean
  onLoadOlder: () => void
}) {
  const scrollerRef = useRef<HTMLDivElement | null>(null)
  const sentinelRef = useRef<HTMLDivElement | null>(null)
  const entries = steps
    .map((step, index) => {
      const output = getWorkerOutput(step)
      if (!output) return null
      return {
        id: step.id ?? `worker-log-${index}`,
        createdAt: step.created_at,
        stream: output.stream,
        text: output.text,
      }
    })
    .filter((entry): entry is NonNullable<typeof entry> => entry !== null)

  // Auto-scroll to bottom when live and new entries arrive.
  useEffect(() => {
    if (!isLive || !scrollerRef.current) return
    scrollerRef.current.scrollTop = scrollerRef.current.scrollHeight
  }, [entries.length, isLive])

  // IntersectionObserver to load older entries when scrolling to top.
  useEffect(() => {
    if (!sentinelRef.current || !hasMore) return
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && !loadingOlder) {
          onLoadOlder()
        }
      },
      { root: scrollerRef.current, threshold: 0.1 },
    )
    observer.observe(sentinelRef.current)
    return () => observer.disconnect()
  }, [hasMore, loadingOlder, onLoadOlder])

  return (
    <div className="overflow-hidden rounded-xl border border-white/[0.08] bg-black/20">
      <div className="flex items-center justify-between border-b border-white/[0.06] px-3 py-2">
        <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-[0.24em] text-foreground/55">
          <Terminal className="size-3.5" />
          Worker Logs
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="muted" className="text-[10px] tabular-nums">
            {totalCount > entries.length
              ? `${entries.length} / ${totalCount.toLocaleString()}`
              : `${entries.length}`}{' '}
            line{entries.length !== 1 ? 's' : ''}
          </Badge>
          {isLive ? (
            <span className="flex items-center gap-1 text-[10px] text-emerald-400/70">
              <span className="size-1.5 animate-pulse rounded-full bg-emerald-400" />
              Tailing
            </span>
          ) : null}
        </div>
      </div>

      {entries.length === 0 ? (
        <div className="flex flex-col items-center gap-2 px-4 py-6 text-center">
          <Terminal className="size-8 text-muted-foreground/25" />
          <p className="text-sm text-muted-foreground">No worker output yet.</p>
          {isLive ? (
            <p className="text-xs text-muted-foreground/60">
              Logs will appear here while the worker runs.
            </p>
          ) : null}
        </div>
      ) : (
        <div
          ref={scrollerRef}
          className="max-h-80 overflow-auto px-3 py-3 font-mono text-xs leading-6"
        >
          <div className="flex flex-col gap-1.5">
            {/* Sentinel for loading older entries */}
            {hasMore ? (
              <div ref={sentinelRef} className="flex justify-center py-1">
                {loadingOlder ? (
                  <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground/60">
                    <Loader2 className="size-3 animate-spin" />
                    Loading older entries…
                  </span>
                ) : (
                  <span className="text-[10px] text-muted-foreground/40">
                    ↑ Scroll for older
                  </span>
                )}
              </div>
            ) : null}
            {entries.map((entry) => (
              <div key={entry.id} className="flex items-start gap-3">
                <span
                  className={cn(
                    'mt-0.5 shrink-0 rounded px-1.5 py-0.5 text-[10px] uppercase tracking-[0.24em]',
                    entry.stream === 'stderr'
                      ? 'bg-red-500/15 text-red-300'
                      : 'bg-emerald-500/15 text-emerald-300',
                  )}
                >
                  {entry.stream === 'stderr' ? 'ERR' : 'OUT'}
                </span>
                <span className="min-w-0 flex-1 whitespace-pre-wrap break-words text-foreground/80">
                  {entry.text || ' '}
                </span>
                {entry.createdAt ? (
                  <span className="shrink-0 text-[10px] text-muted-foreground/45">
                    {formatTimestamp(entry.createdAt)}
                  </span>
                ) : null}
              </div>
            ))}
          </div>
        </div>
      )}
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
  // All steps loaded so far, in chronological order (oldest first).
  const [steps, setSteps] = useState<TraceStep[] | null>(null)
  const [totalCount, setTotalCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingOlder, setLoadingOlder] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  // Track how many steps we've fetched so far (offset for next older-page fetch).
  const fetchedCountRef = useRef(0)

  const isInProgress = ticketState
    ? IN_PROGRESS_STATES.has(ticketState)
    : false

  // Fetch the latest page (offset=0). Used for initial load and live polling.
  const fetchLatestPage = useCallback(
    async (opts?: { silent?: boolean }) => {
      if (!opts?.silent) setLoading(true)
      const { data, error, response } = await client.GET(
        '/tickets/{ticketID}/trace',
        {
          params: {
            path: { ticketID: ticketId },
            query: { limit: PAGE_SIZE, offset: 0 },
          },
        },
      )
      if (!response.ok) {
        if (response.status === 404) {
          setSteps([])
          setTotalCount(0)
          setErr(null)
        } else {
          setErr(formatApiError(error))
        }
      } else {
        const newSteps = (data?.steps ?? []).slice().reverse() // API returns DESC, we want ASC
        const total = data?.total_count ?? newSteps.length
        setTotalCount(total)
        setErr(null)

        setSteps((prev) => {
          if (!prev || prev.length === 0) {
            // First load — just use the new steps.
            fetchedCountRef.current = newSteps.length
            return newSteps
          }
          // Merge: keep older prepended steps, replace the tail with fresh data.
          const olderCount = prev.length - fetchedCountRef.current
          const older = olderCount > 0 ? prev.slice(0, olderCount) : []
          fetchedCountRef.current = newSteps.length
          return [...older, ...newSteps]
        })
      }
      setLoading(false)
      setRefreshing(false)
    },
    [client, ticketId],
  )

  // Fetch an older page and prepend.
  const fetchOlderPage = useCallback(async () => {
    if (loadingOlder) return
    setLoadingOlder(true)

    const currentTotal = totalCount
    const currentLoaded = steps?.length ?? 0
    if (currentLoaded >= currentTotal) {
      setLoadingOlder(false)
      return
    }

    // We need to fetch the next chunk of older steps.
    // The API returns DESC order. offset=0 is newest. We want the next older batch.
    // currentLoaded steps are already in memory. Fetch from offset=currentLoaded.
    const { data, response } = await client.GET(
      '/tickets/{ticketID}/trace',
      {
        params: {
          path: { ticketID: ticketId },
          query: { limit: PAGE_SIZE, offset: currentLoaded },
        },
      },
    )
    if (response.ok && data?.steps) {
      const olderSteps = data.steps.slice().reverse() // DESC → ASC
      if (olderSteps.length > 0) {
        setSteps((prev) => [...olderSteps, ...(prev ?? [])])
        if (data.total_count != null) {
          setTotalCount(data.total_count)
        }
      }
    }
    setLoadingOlder(false)
  }, [client, ticketId, totalCount, steps?.length, loadingOlder])

  // Initial fetch.
  useEffect(() => {
    let cancelled = false
    void (async () => {
      await fetchLatestPage()
      if (cancelled) return
    })()
    return () => {
      cancelled = true
    }
  }, [fetchLatestPage])

  // Poll more aggressively while the worker is active so the log view feels live.
  useEffect(() => {
    if (isInProgress) {
      intervalRef.current = setInterval(() => {
        void fetchLatestPage({ silent: true })
      }, LIVE_POLL_INTERVAL_MS)
    }
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current)
        intervalRef.current = null
      }
    }
  }, [isInProgress, fetchLatestPage])

  const handleRefresh = () => {
    setRefreshing(true)
    void fetchLatestPage({ silent: true })
  }

  const hasMore = (steps?.length ?? 0) < totalCount

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

  const stepCount = totalCount || (steps?.length ?? 0)
  const workerLogSteps = (steps ?? []).filter((step) => getWorkerOutput(step))
  const timelineSteps = (steps ?? []).filter((step) => !getWorkerOutput(step))

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
            <div className="flex flex-col gap-4">
              {workerLogSteps.length > 0 || isInProgress ? (
                <WorkerLogConsole
                  steps={workerLogSteps}
                  totalCount={stepCount}
                  isLive={isInProgress}
                  loadingOlder={loadingOlder}
                  hasMore={hasMore}
                  onLoadOlder={fetchOlderPage}
                />
              ) : null}

              {timelineSteps.length > 0 ? (
                <div className="flex flex-col">
                  {hasMore ? (
                    <div className="flex justify-center pb-2">
                      {loadingOlder ? (
                        <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground/60">
                          <Loader2 className="size-3 animate-spin" />
                          Loading older steps…
                        </span>
                      ) : (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-6 text-[10px] text-muted-foreground/60"
                          onClick={fetchOlderPage}
                        >
                          Load older steps ({totalCount - (steps?.length ?? 0)} remaining)
                        </Button>
                      )}
                    </div>
                  ) : null}
                  {timelineSteps.map((step, i) => (
                    <TimelineStep
                      key={step.id ?? i}
                      step={step}
                      isLast={i === timelineSteps.length - 1}
                      previousStep={i > 0 ? timelineSteps[i - 1] : undefined}
                    />
                  ))}
                </div>
              ) : null}
            </div>
          )}
        </CardContent>
      ) : null}
    </Card>
  )
}
