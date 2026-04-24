import { useCallback, useEffect, useMemo, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { ChevronRight, RefreshCw, Settings, Container, Layers } from 'lucide-react'
import { motion } from 'framer-motion'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'

// ── Types ────────────────────────────────────────────────────────────────────

/** A single deployment entry from the catalog deployment matrix. */
interface DeploymentEntry {
  service_id: string
  service_name: string
  environment_id: string
  environment_name: string
  version: string
  source: string
  observed_at: string
}

/** A single observed resource from the state index. */
interface StateResource {
  id: string
  project_id: string
  resource_type: string
  name: string
  environment: string
  observed_state: Record<string, unknown>
  declared_state?: Record<string, unknown>
  last_observed: string
  source: string
}

/** A pipeline environment stage (grouped from deployment entries). */
interface PipelineStage {
  environmentId: string
  environmentName: string
  deployments: DeploymentEntry[]
  resourceCount: number
}

// ── Fetch helpers ────────────────────────────────────────────────────────────

async function fetchDeployments(
  token: string | null,
  projectId: string,
): Promise<{ data: DeploymentEntry[]; error: string | null }> {
  const headers: Record<string, string> = {}
  if (token) headers.Authorization = `Bearer ${token}`

  try {
    const res = await fetch(
      `/api/v1/catalog/deployments?project_id=${encodeURIComponent(projectId)}`,
      { headers },
    )
    if (!res.ok) {
      const body = await res.json().catch(() => null) as { error?: string } | null
      return { data: [], error: body?.error ?? `HTTP ${res.status}` }
    }
    const json = await res.json() as { deployments?: DeploymentEntry[] }
    return { data: json.deployments ?? [], error: null }
  } catch (err) {
    return { data: [], error: String(err) }
  }
}

async function fetchResources(
  token: string | null,
  projectId: string,
): Promise<{ data: StateResource[]; error: string | null }> {
  const headers: Record<string, string> = {}
  if (token) headers.Authorization = `Bearer ${token}`

  try {
    const res = await fetch(
      `/api/v1/state/resources?project_id=${encodeURIComponent(projectId)}&limit=200`,
      { headers },
    )
    if (!res.ok) {
      // Non-critical — resources are supplementary
      return { data: [], error: null }
    }
    const json = await res.json() as StateResource[]
    return { data: Array.isArray(json) ? json : [], error: null }
  } catch {
    return { data: [], error: null }
  }
}

// ── Animation variants ───────────────────────────────────────────────────────

const ease = [0.25, 0.1, 0.25, 1] as const

const stageVariants = {
  hidden: { opacity: 0, x: 24 },
  visible: (i: number) => ({
    opacity: 1,
    x: 0,
    transition: {
      delay: i * 0.12,
      duration: 0.35,
      ease,
    },
  }),
}

const connectorVariants = {
  hidden: { opacity: 0, scaleX: 0 },
  visible: (i: number) => ({
    opacity: 1,
    scaleX: 1,
    transition: {
      delay: i * 0.12 + 0.06,
      duration: 0.25,
      ease,
    },
  }),
}

const verticalConnectorVariants = {
  hidden: { opacity: 0, scaleY: 0 },
  visible: (i: number) => ({
    opacity: 1,
    scaleY: 1,
    transition: {
      delay: i * 0.12 + 0.06,
      duration: 0.25,
      ease,
    },
  }),
}

// ── Helpers ──────────────────────────────────────────────────────────────────

function formatTimestamp(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  const now = new Date()
  const diffMs = now.getTime() - d.getTime()
  const diffMin = Math.floor(diffMs / 60_000)
  if (diffMin < 1) return 'just now'
  if (diffMin < 60) return `${diffMin}m ago`
  const diffHr = Math.floor(diffMin / 60)
  if (diffHr < 24) return `${diffHr}h ago`
  const diffDay = Math.floor(diffHr / 24)
  return `${diffDay}d ago`
}

/** Derive a sort order for environments based on common naming patterns. */
function envSortKey(name: string): number {
  const n = name.toLowerCase()
  if (n.includes('dev') || n.includes('development') || n.includes('local')) return 0
  if (n.includes('test') || n.includes('testing')) return 1
  if (n.includes('stag') || n.includes('staging') || n.includes('uat')) return 2
  if (n.includes('pre') || n.includes('preprod')) return 3
  if (n.includes('prod') || n.includes('production') || n.includes('live')) return 4
  return 3 // unknown envs go near the end
}

// ── Sub-components ───────────────────────────────────────────────────────────

function PipelineStageSkeleton() {
  return (
    <div className="flex min-w-[260px] flex-col gap-3 rounded-xl border border-white/10 bg-white/[0.03] p-4 backdrop-blur-md">
      <Skeleton className="h-5 w-24" />
      <Skeleton className="h-3 w-16" />
      <div className="space-y-2 pt-2">
        <Skeleton className="h-10 w-full rounded-lg" />
        <Skeleton className="h-10 w-full rounded-lg" />
      </div>
    </div>
  )
}

function PipelineStageCard({
  stage,
  index,
}: {
  stage: PipelineStage
  index: number
}) {
  return (
    <motion.div
      custom={index}
      variants={stageVariants}
      initial="hidden"
      animate="visible"
      className="flex min-w-[260px] max-w-[340px] flex-1 flex-col gap-3 rounded-xl border border-white/10 bg-white/5 p-4 backdrop-blur-md transition-colors hover:border-white/20"
    >
      {/* Environment header */}
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold text-foreground">
            {stage.environmentName}
          </h3>
          <p className="text-xs text-muted-foreground">{stage.environmentId}</p>
        </div>
        {stage.resourceCount > 0 && (
          <div className="flex shrink-0 items-center gap-1 text-xs text-muted-foreground" title={`${stage.resourceCount} resource${stage.resourceCount === 1 ? '' : 's'}`}>
            <Layers className="size-3" />
            <span>{stage.resourceCount}</span>
          </div>
        )}
      </div>

      {/* Metadata badges — show source info */}
      {stage.deployments.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {[...new Set(stage.deployments.map((d) => d.source))].map((src) => (
            <Badge key={src} variant="outline" className="text-[10px]">
              {src}
            </Badge>
          ))}
        </div>
      )}

      {/* Deployments list */}
      {stage.deployments.length > 0 ? (
        <div className="flex flex-col gap-1.5">
          {stage.deployments.map((dep) => (
            <div
              key={`${dep.service_id}-${dep.version}`}
              className="flex items-center justify-between gap-2 rounded-lg border border-white/[0.06] bg-white/[0.03] px-3 py-2"
            >
              <div className="flex min-w-0 items-center gap-2">
                <Container className="size-3.5 shrink-0 text-muted-foreground" />
                <span className="truncate text-xs font-medium text-foreground">
                  {dep.service_name}
                </span>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <code className="rounded bg-emerald-500/10 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-emerald-400">
                  {dep.version}
                </code>
                <span className="text-[10px] text-muted-foreground" title={dep.observed_at}>
                  {formatTimestamp(dep.observed_at)}
                </span>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <p className="py-2 text-center text-xs text-muted-foreground">
          No deployments observed
        </p>
      )}
    </motion.div>
  )
}

function HorizontalConnector({ index }: { index: number }) {
  return (
    <motion.div
      custom={index}
      variants={connectorVariants}
      initial="hidden"
      animate="visible"
      className="hidden items-center lg:flex"
      style={{ originX: 0 }}
    >
      <div className="h-px w-8 bg-gradient-to-r from-white/20 to-white/10" />
      <ChevronRight className="size-4 -ml-1 text-white/20" />
    </motion.div>
  )
}

function VerticalConnector({ index }: { index: number }) {
  return (
    <motion.div
      custom={index}
      variants={verticalConnectorVariants}
      initial="hidden"
      animate="visible"
      className="flex flex-col items-center lg:hidden"
      style={{ originY: 0 }}
    >
      <div className="h-6 w-px bg-gradient-to-b from-white/20 to-white/10" />
      <ChevronRight className="size-4 -mt-1 rotate-90 text-white/20" />
    </motion.div>
  )
}

// ── Main component ───────────────────────────────────────────────────────────

export function PipelineView() {
  const { projectId, orgId } = useParams<{ orgId: string; projectId: string }>()
  const { token } = useAuth()

  const [deployments, setDeployments] = useState<DeploymentEntry[]>([])
  const [resources, setResources] = useState<StateResource[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const loadData = useCallback(
    async (isRefresh = false) => {
      if (!projectId || !token) {
        setLoading(false)
        return
      }

      if (isRefresh) setRefreshing(true)
      else setLoading(true)

      const [depResult, resResult] = await Promise.all([
        fetchDeployments(token, projectId),
        fetchResources(token, projectId),
      ])

      setDeployments(depResult.data)
      setResources(resResult.data)
      setError(depResult.error)
      setLoading(false)
      setRefreshing(false)
    },
    [projectId, token],
  )

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      await loadData()
      if (cancelled) return
    })()
    return () => {
      cancelled = true
    }
  }, [loadData])

  // Group deployments by environment into pipeline stages
  const stages = useMemo<PipelineStage[]>(() => {
    const envMap = new Map<string, PipelineStage>()

    for (const dep of deployments) {
      const key = dep.environment_id || dep.environment_name
      if (!envMap.has(key)) {
        envMap.set(key, {
          environmentId: dep.environment_id,
          environmentName: dep.environment_name,
          deployments: [],
          resourceCount: 0,
        })
      }
      envMap.get(key)!.deployments.push(dep)
    }

    // Count resources per environment
    for (const res of resources) {
      const key = res.environment
      if (key && envMap.has(key)) {
        envMap.get(key)!.resourceCount++
      }
    }

    // Sort stages by environment naming convention (dev → staging → prod)
    return Array.from(envMap.values()).sort(
      (a, b) => envSortKey(a.environmentName) - envSortKey(b.environmentName),
    )
  }, [deployments, resources])

  const settingsPath = orgId && projectId
    ? `/#/orgs/${orgId}/projects/${projectId}/settings`
    : '#'

  // ── Loading state ──────────────────────────────────────────────────────────

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-7 w-20 rounded-md" />
        </div>
        <div className="flex gap-4 overflow-x-auto pb-2">
          <PipelineStageSkeleton />
          <PipelineStageSkeleton />
          <PipelineStageSkeleton />
        </div>
      </div>
    )
  }

  // ── Error state ────────────────────────────────────────────────────────────

  if (error) {
    return (
      <div className="rounded-xl border border-destructive/30 bg-destructive/5 px-5 py-4">
        <p className="text-sm text-destructive">{error}</p>
      </div>
    )
  }

  // ── Empty state ────────────────────────────────────────────────────────────

  if (stages.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-white/10 bg-white/[0.02] px-6 py-12 text-center">
        <Container className="size-8 text-muted-foreground/50" />
        <div className="space-y-1">
          <p className="text-sm font-medium text-muted-foreground">
            No pipeline data
          </p>
          <p className="text-xs text-muted-foreground/70">
            Configure infrastructure integration in project settings to see
            deployment stages.
          </p>
        </div>
        <Link to={settingsPath}>
          <Button variant="outline" size="sm" className="mt-2 gap-1.5">
            <Settings className="size-3.5" />
            Project settings
          </Button>
        </Link>
      </div>
    )
  }

  // ── Pipeline flow ──────────────────────────────────────────────────────────

  return (
    <div className="space-y-4">
      {/* Header with refresh */}
      <div className="flex items-center justify-between">
        <p className="text-xs text-muted-foreground">
          {stages.length} environment{stages.length === 1 ? '' : 's'} &middot;{' '}
          {deployments.length} deployment{deployments.length === 1 ? '' : 's'}
        </p>
        <Button
          variant="outline"
          size="sm"
          disabled={refreshing}
          onClick={() => loadData(true)}
          className="gap-1.5"
        >
          <RefreshCw
            className={`size-3.5 ${refreshing ? 'animate-spin' : ''}`}
          />
          {refreshing ? 'Refreshing...' : 'Refresh'}
        </Button>
      </div>

      {/* Horizontal pipeline flow (lg+) */}
      <div className="hidden items-stretch gap-0 overflow-x-auto pb-2 lg:flex">
        {stages.map((stage, i) => (
          <div key={stage.environmentId} className="flex items-stretch">
            <PipelineStageCard stage={stage} index={i} />
            {i < stages.length - 1 && <HorizontalConnector index={i} />}
          </div>
        ))}
      </div>

      {/* Vertical pipeline flow (below lg) */}
      <div className="flex flex-col items-stretch gap-0 lg:hidden">
        {stages.map((stage, i) => (
          <div key={stage.environmentId} className="flex flex-col items-center">
            <div className="w-full max-w-[400px]">
              <PipelineStageCard stage={stage} index={i} />
            </div>
            {i < stages.length - 1 && <VerticalConnector index={i} />}
          </div>
        ))}
      </div>
    </div>
  )
}
