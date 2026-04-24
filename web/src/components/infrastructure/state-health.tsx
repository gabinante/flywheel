import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import {
  Activity,
  AlertTriangle,
  ArrowLeftRight,
  Clock,
  Database,
  Layers,
  Server,
  Wifi,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'

/* ------------------------------------------------------------------ */
/*  Types (matching api/rest/stateindex.go response shapes)            */
/* ------------------------------------------------------------------ */

interface StateSummary {
  project_id: string
  total_resources: number
  resources_by_type: Record<string, number>
  resources_by_env: Record<string, number>
  stale_count: number
  drift_count: number
  unattributed_count: number
  last_full_scan?: string
  staleness_distribution: Record<string, number>
}

interface ObservedResource {
  id: string
  project_id: string
  resource_type: string
  environment: string
  name: string
  external_id?: string
  provider?: string
  region?: string
  properties: Record<string, unknown>
  declared_state?: Record<string, unknown>
  observed_at: string
  source: string
  created_at: string
  updated_at: string
}

interface StalenessReport {
  project_id: string
  threshold_seconds: number
  stale_count: number
  total_count: number
  stale_resources: ObservedResource[]
}

interface DriftItem {
  resource_id: string
  resource_name: string
  resource_type: string
  environment: string
  declared: Record<string, unknown>
  observed: Record<string, unknown>
  differences: Record<string, { declared: unknown; observed: unknown; type: string }>
}

interface DriftReport {
  project_id: string
  drift_count: number
  drifts: DriftItem[] | null
}

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

/** Authenticated fetch for state-index endpoints (outside OpenAPI spec). */
async function stateApiFetch(
  path: string,
  token: string | null,
): Promise<Response> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  if (token) headers['Authorization'] = `Bearer ${token}`
  return fetch(path, { headers })
}

/** Format resource type for display (e.g. "loadbalancer" -> "Load Balancer"). */
function formatResourceType(type: string): string {
  const map: Record<string, string> = {
    container: 'Containers',
    service: 'Services',
    database: 'Databases',
    loadbalancer: 'Load Balancers',
    bucket: 'Buckets',
    vm: 'VMs',
    network: 'Networks',
    dns: 'DNS Records',
    queue: 'Queues',
    function: 'Functions',
    custom: 'Custom',
  }
  return map[type] ?? type.charAt(0).toUpperCase() + type.slice(1)
}

/** Format a relative duration from an ISO timestamp. */
function formatStaleness(observedAt: string): string {
  const now = Date.now()
  const observed = new Date(observedAt).getTime()
  const diffMs = now - observed

  if (diffMs < 0) return 'just now'

  const seconds = Math.floor(diffMs / 1000)
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

/** Format ISO timestamp for display. */
function formatTimestamp(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

/** Get an icon for a resource type. */
function resourceTypeIcon(type: string) {
  switch (type) {
    case 'database':
      return Database
    case 'service':
      return Server
    case 'network':
    case 'dns':
      return Wifi
    default:
      return Layers
  }
}

/* ------------------------------------------------------------------ */
/*  Skeleton cards                                                     */
/* ------------------------------------------------------------------ */

function StateHealthCardSkeleton({ lines = 3 }: { lines?: number }) {
  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="border-b border-white/10 pb-4">
        <Skeleton className="h-4 w-2/5" />
        <Skeleton className="h-3 w-3/5" />
      </CardHeader>
      <CardContent className="pt-4">
        <div className="flex flex-col gap-3">
          {Array.from({ length: lines }).map((_, i) => (
            <Skeleton
              key={i}
              className="h-4"
              style={{ width: `${60 + Math.random() * 40}%` }}
            />
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

/* ------------------------------------------------------------------ */
/*  Summary Card                                                       */
/* ------------------------------------------------------------------ */

function SummaryCard({
  summary,
  loading,
  error,
}: {
  summary: StateSummary | null
  loading: boolean
  error: string | null
}) {
  if (loading) return <StateHealthCardSkeleton lines={4} />

  if (error) {
    return (
      <Card className="border-white/10 bg-white/5 backdrop-blur-md">
        <CardHeader className="border-b border-white/10 pb-4">
          <CardTitle className="flex items-center gap-2 text-sm">
            <Activity className="size-4 text-muted-foreground" />
            Summary
          </CardTitle>
        </CardHeader>
        <CardContent className="py-6 text-center">
          <p className="text-xs text-destructive">{error}</p>
        </CardContent>
      </Card>
    )
  }

  if (!summary || summary.total_resources === 0) {
    return (
      <Card className="border-white/10 bg-white/5 backdrop-blur-md">
        <CardHeader className="border-b border-white/10 pb-4">
          <CardTitle className="flex items-center gap-2 text-sm">
            <Activity className="size-4 text-muted-foreground" />
            Summary
          </CardTitle>
          <CardDescription>
            Resource overview and type breakdown.
          </CardDescription>
        </CardHeader>
        <CardContent className="py-8 text-center">
          <Layers className="mx-auto mb-2 size-8 text-muted-foreground/40" />
          <p className="text-sm text-muted-foreground">
            No resources observed yet.
          </p>
          <p className="mt-1 text-xs text-muted-foreground/60">
            Resources will appear here once state ingestion is configured.
          </p>
        </CardContent>
      </Card>
    )
  }

  const typeEntries = Object.entries(summary.resources_by_type).sort(
    ([, a], [, b]) => b - a,
  )

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="border-b border-white/10 pb-4">
        <CardTitle className="flex items-center gap-2 text-sm">
          <Activity className="size-4 text-green-400" />
          Summary
        </CardTitle>
        <CardDescription>
          Resource overview and type breakdown.
        </CardDescription>
      </CardHeader>
      <CardContent className="pt-4">
        <div className="mb-4 flex items-baseline gap-2">
          <span className="text-3xl font-semibold tabular-nums tracking-tight text-foreground">
            {summary.total_resources}
          </span>
          <span className="text-xs text-muted-foreground">
            resources observed
          </span>
        </div>

        {summary.last_full_scan && (
          <p className="mb-3 flex items-center gap-1.5 text-xs text-muted-foreground">
            <Clock className="size-3" />
            Last scan: {formatTimestamp(summary.last_full_scan)}
          </p>
        )}

        <div className="flex flex-col gap-1.5">
          {typeEntries.map(([type, count]) => {
            const Icon = resourceTypeIcon(type)
            return (
              <div
                key={type}
                className="flex items-center justify-between rounded-md bg-white/[0.03] px-2.5 py-1.5 text-xs"
              >
                <span className="flex items-center gap-2 text-muted-foreground">
                  <Icon className="size-3.5" />
                  {formatResourceType(type)}
                </span>
                <span className="font-medium tabular-nums text-foreground">
                  {count}
                </span>
              </div>
            )
          })}
        </div>
      </CardContent>
    </Card>
  )
}

/* ------------------------------------------------------------------ */
/*  Staleness Card                                                     */
/* ------------------------------------------------------------------ */

function StalenessCard({
  report,
  loading,
  error,
}: {
  report: StalenessReport | null
  loading: boolean
  error: string | null
}) {
  if (loading) return <StateHealthCardSkeleton lines={3} />

  if (error) {
    return (
      <Card className="border-white/10 bg-white/5 backdrop-blur-md">
        <CardHeader className="border-b border-white/10 pb-4">
          <CardTitle className="flex items-center gap-2 text-sm">
            <Clock className="size-4 text-muted-foreground" />
            Staleness
          </CardTitle>
        </CardHeader>
        <CardContent className="py-6 text-center">
          <p className="text-xs text-destructive">{error}</p>
        </CardContent>
      </Card>
    )
  }

  const staleResources = report?.stale_resources ?? []
  const isFresh = report !== null && staleResources.length === 0

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="border-b border-white/10 pb-4">
        <div className="flex items-center justify-between">
          <CardTitle className="flex items-center gap-2 text-sm">
            <Clock className="size-4 text-amber-400" />
            Staleness
          </CardTitle>
          {report && (
            <Badge
              variant="outline"
              className={
                isFresh
                  ? 'border-green-500/30 text-green-400'
                  : 'border-amber-500/30 text-amber-400'
              }
            >
              {isFresh ? 'All fresh' : `${staleResources.length} stale`}
            </Badge>
          )}
        </div>
        <CardDescription>
          Resources not observed within the threshold.
        </CardDescription>
      </CardHeader>
      <CardContent className="pt-4">
        {!report ? (
          <div className="py-4 text-center">
            <Clock className="mx-auto mb-2 size-8 text-muted-foreground/40" />
            <p className="text-sm text-muted-foreground">
              No staleness data available.
            </p>
          </div>
        ) : isFresh ? (
          <div className="py-4 text-center">
            <div className="mx-auto mb-2 flex size-10 items-center justify-center rounded-full bg-green-500/10">
              <Clock className="size-5 text-green-400" />
            </div>
            <p className="text-sm text-green-300/90">
              All resources are fresh.
            </p>
            <p className="mt-1 text-xs text-muted-foreground/60">
              {report.total_count} resource{report.total_count !== 1 ? 's' : ''}{' '}
              observed within threshold.
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {staleResources.map((resource) => (
              <div
                key={resource.id}
                className="flex items-center gap-3 rounded-md border border-amber-500/10 bg-amber-500/[0.04] px-3 py-2"
              >
                <AlertTriangle className="size-3.5 shrink-0 text-amber-400" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs font-medium text-foreground">
                    {resource.name}
                  </p>
                  <p className="text-[10px] text-muted-foreground">
                    <span className="capitalize">
                      {resource.resource_type}
                    </span>
                    {resource.environment && (
                      <>
                        {' '}
                        &middot; {resource.environment}
                      </>
                    )}
                  </p>
                </div>
                <span className="shrink-0 text-[10px] tabular-nums text-amber-400/80">
                  {formatStaleness(resource.observed_at)}
                </span>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

/* ------------------------------------------------------------------ */
/*  Drift Card                                                         */
/* ------------------------------------------------------------------ */

function DriftCard({
  report,
  loading,
  error,
}: {
  report: DriftReport | null
  loading: boolean
  error: string | null
}) {
  if (loading) return <StateHealthCardSkeleton lines={3} />

  if (error) {
    return (
      <Card className="border-white/10 bg-white/5 backdrop-blur-md">
        <CardHeader className="border-b border-white/10 pb-4">
          <CardTitle className="flex items-center gap-2 text-sm">
            <ArrowLeftRight className="size-4 text-muted-foreground" />
            Drift
          </CardTitle>
        </CardHeader>
        <CardContent className="py-6 text-center">
          <p className="text-xs text-destructive">{error}</p>
        </CardContent>
      </Card>
    )
  }

  const drifts = report?.drifts ?? []
  const hasDrift = drifts.length > 0

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="border-b border-white/10 pb-4">
        <div className="flex items-center justify-between">
          <CardTitle className="flex items-center gap-2 text-sm">
            <ArrowLeftRight className="size-4 text-red-400" />
            Drift
          </CardTitle>
          {report && (
            <Badge
              variant="outline"
              className={
                hasDrift
                  ? 'border-red-500/30 text-red-400'
                  : 'border-green-500/30 text-green-400'
              }
            >
              {hasDrift ? `${drifts.length} drifted` : 'No drift'}
            </Badge>
          )}
        </div>
        <CardDescription>
          Resources where observed state differs from declared intent.
        </CardDescription>
      </CardHeader>
      <CardContent className="pt-4">
        {!report ? (
          <div className="py-4 text-center">
            <ArrowLeftRight className="mx-auto mb-2 size-8 text-muted-foreground/40" />
            <p className="text-sm text-muted-foreground">
              No drift data available.
            </p>
          </div>
        ) : !hasDrift ? (
          <div className="py-4 text-center">
            <div className="mx-auto mb-2 flex size-10 items-center justify-center rounded-full bg-green-500/10">
              <ArrowLeftRight className="size-5 text-green-400" />
            </div>
            <p className="text-sm text-green-300/90">No drift detected.</p>
            <p className="mt-1 text-xs text-muted-foreground/60">
              Observed state matches declared intent.
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {drifts.map((item) => {
              const diffKeys = Object.keys(item.differences)
              const diffSummary =
                diffKeys.length === 1
                  ? `${diffKeys[0]} changed`
                  : `${diffKeys.length} properties differ`

              return (
                <div
                  key={item.resource_id}
                  className="flex items-start gap-3 rounded-md border border-red-500/10 bg-red-500/[0.04] px-3 py-2"
                >
                  <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-red-400" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-xs font-medium text-foreground">
                      {item.resource_name}
                    </p>
                    <p className="text-[10px] text-muted-foreground">
                      <span className="capitalize">{item.resource_type}</span>
                      {item.environment && (
                        <>
                          {' '}
                          &middot; {item.environment}
                        </>
                      )}
                    </p>
                    <p className="mt-1 text-[10px] text-red-400/80">
                      {diffSummary}
                    </p>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

/* ------------------------------------------------------------------ */
/*  Main Component                                                     */
/* ------------------------------------------------------------------ */

export function StateHealth() {
  const { projectId } = useParams<{ projectId: string }>()
  const { token } = useAuth()

  const [summary, setSummary] = useState<StateSummary | null>(null)
  const [summaryLoading, setSummaryLoading] = useState(true)
  const [summaryError, setSummaryError] = useState<string | null>(null)

  const [staleness, setStaleness] = useState<StalenessReport | null>(null)
  const [stalenessLoading, setStalenessLoading] = useState(true)
  const [stalenessError, setStalenessError] = useState<string | null>(null)

  const [drift, setDrift] = useState<DriftReport | null>(null)
  const [driftLoading, setDriftLoading] = useState(true)
  const [driftError, setDriftError] = useState<string | null>(null)

  useEffect(() => {
    if (!projectId) return

    let cancelled = false

    // Fetch all three endpoints in parallel
    const fetchSummary = async () => {
      try {
        setSummaryLoading(true)
        const res = await stateApiFetch(
          `/api/v1/state/summary?project_id=${encodeURIComponent(projectId)}`,
          token,
        )
        if (cancelled) return
        if (!res.ok) {
          setSummaryError(`Failed to load summary: ${res.statusText}`)
          return
        }
        const data: StateSummary = await res.json()
        setSummary(data)
        setSummaryError(null)
      } catch (e) {
        if (!cancelled) setSummaryError(String(e))
      } finally {
        if (!cancelled) setSummaryLoading(false)
      }
    }

    const fetchStaleness = async () => {
      try {
        setStalenessLoading(true)
        const res = await stateApiFetch(
          `/api/v1/state/staleness?project_id=${encodeURIComponent(projectId)}`,
          token,
        )
        if (cancelled) return
        if (!res.ok) {
          setStalenessError(`Failed to load staleness: ${res.statusText}`)
          return
        }
        const data: StalenessReport = await res.json()
        setStaleness(data)
        setStalenessError(null)
      } catch (e) {
        if (!cancelled) setStalenessError(String(e))
      } finally {
        if (!cancelled) setStalenessLoading(false)
      }
    }

    const fetchDrift = async () => {
      try {
        setDriftLoading(true)
        const res = await stateApiFetch(
          `/api/v1/state/drift?project_id=${encodeURIComponent(projectId)}`,
          token,
        )
        if (cancelled) return
        if (!res.ok) {
          setDriftError(`Failed to load drift: ${res.statusText}`)
          return
        }
        const data: DriftReport = await res.json()
        setDrift(data)
        setDriftError(null)
      } catch (e) {
        if (!cancelled) setDriftError(String(e))
      } finally {
        if (!cancelled) setDriftLoading(false)
      }
    }

    void fetchSummary()
    void fetchStaleness()
    void fetchDrift()

    return () => {
      cancelled = true
    }
  }, [projectId, token])

  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      <SummaryCard
        summary={summary}
        loading={summaryLoading}
        error={summaryError}
      />
      <StalenessCard
        report={staleness}
        loading={stalenessLoading}
        error={stalenessError}
      />
      <DriftCard report={drift} loading={driftLoading} error={driftError} />
    </div>
  )
}
