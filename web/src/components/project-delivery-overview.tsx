import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  AlertCircle,
  ExternalLink,
  GitPullRequest,
  RefreshCcw,
  Rocket,
  Server,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

type IntegrationStatus = 'ok' | 'unconfigured' | 'unsupported' | 'degraded'

type PullRequest = {
  number: number
  title: string
  url: string
  state: string
  draft: boolean
  author?: string
  head_ref?: string
  base_ref?: string
  updated_at: string
}

type PullRequestOverview = {
  project_id: string
  status: IntegrationStatus
  provider?: string
  repository?: string
  message?: string
  pull_requests: PullRequest[]
}

type PipelineResource = {
  id: string
  name: string
  type: string
  provider?: string
  region?: string
  status?: string
  version?: string
  observed_at: string
}

type PipelineDeployment = {
  service: string
  version: string
  source: string
  observed_at: string
}

type PipelineEnvironment = {
  id?: string
  name: string
  slug: string
  infrastructure?: string
  data_tenancy?: string
  integration_mode?: string
  is_default: boolean
  deployments: PipelineDeployment[]
  resources: PipelineResource[]
}

type PipelineOverview = {
  project_id: string
  status: IntegrationStatus
  provider?: string
  message?: string
  refreshed_at?: string
  environments: PipelineEnvironment[]
}

type LoadState<T> = {
  data: T | null
  loading: boolean
  error: string | null
}

async function fetchJSON<T>(
  path: string,
  token: string | null,
): Promise<{ ok: true; data: T } | { ok: false; error: string }> {
  const response = await fetch(path, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  })
  const raw = (await response.json().catch(() => null)) as
    | Record<string, unknown>
    | null
  if (!response.ok) {
    const message =
      raw && typeof raw.error === 'string' ? raw.error : 'Request failed'
    return { ok: false, error: message }
  }
  return { ok: true, data: raw as T }
}

function relativeTime(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const deltaMs = Date.now() - date.getTime()
  const deltaMinutes = Math.round(deltaMs / 60000)
  if (Math.abs(deltaMinutes) < 60) {
    const minutes = Math.max(1, Math.abs(deltaMinutes))
    return `${minutes}m ${deltaMinutes >= 0 ? 'ago' : 'from now'}`
  }
  const deltaHours = Math.round(deltaMinutes / 60)
  if (Math.abs(deltaHours) < 48) {
    const hours = Math.max(1, Math.abs(deltaHours))
    return `${hours}h ${deltaHours >= 0 ? 'ago' : 'from now'}`
  }
  return date.toLocaleString()
}

function statusTone(status: IntegrationStatus) {
  switch (status) {
    case 'ok':
      return 'text-emerald-400'
    case 'degraded':
      return 'text-amber-400'
    case 'unsupported':
      return 'text-sky-400'
    default:
      return 'text-muted-foreground'
  }
}

function PullRequestsPanel({
  state,
}: {
  state: LoadState<PullRequestOverview>
}) {
  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="space-y-2">
        <div className="flex items-center gap-2">
          <GitPullRequest className="size-4 text-muted-foreground" />
          <CardTitle className="text-sm">Open Pull Requests</CardTitle>
        </div>
        <CardDescription>
          Live repository state, not Flywheel ticket outputs.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {state.loading ? (
          <div className="space-y-2">
            {Array.from({ length: 3 }).map((_, index) => (
              <Skeleton key={index} className="h-14 w-full rounded-lg" />
            ))}
          </div>
        ) : state.error ? (
          <div className="rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-3 text-sm text-destructive">
            {state.error}
          </div>
        ) : state.data ? (
          <>
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              {state.data.provider ? (
                <Badge variant="outline">{state.data.provider}</Badge>
              ) : null}
              {state.data.repository ? (
                <span className="font-mono">{state.data.repository}</span>
              ) : null}
              <span className={statusTone(state.data.status)}>
                {state.data.status}
              </span>
            </div>

            {state.data.message ? (
              <div className="flex items-start gap-2 rounded-lg border border-white/10 bg-white/5 px-3 py-3 text-sm text-muted-foreground">
                <AlertCircle className="mt-0.5 size-4 shrink-0" />
                <span>{state.data.message}</span>
              </div>
            ) : null}

            {state.data.pull_requests.length === 0 ? (
              <div className="rounded-lg border border-white/10 bg-white/5 px-3 py-6 text-sm text-muted-foreground">
                No open pull requests.
              </div>
            ) : (
              <div className="space-y-2">
                {state.data.pull_requests.slice(0, 8).map((pr) => (
                  <a
                    key={pr.number}
                    href={pr.url}
                    target="_blank"
                    rel="noreferrer"
                    className="flex flex-col gap-2 rounded-lg border border-white/10 bg-white/5 px-3 py-3 transition-colors hover:border-white/20 hover:bg-white/10"
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-xs text-emerald-400">
                            #{pr.number}
                          </span>
                          {pr.draft ? (
                            <Badge variant="outline">draft</Badge>
                          ) : null}
                        </div>
                        <div className="truncate text-sm font-medium">
                          {pr.title}
                        </div>
                      </div>
                      <ExternalLink className="size-4 shrink-0 text-muted-foreground" />
                    </div>
                    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                      {pr.author ? <span>@{pr.author}</span> : null}
                      {pr.head_ref ? (
                        <span className="font-mono">
                          {pr.head_ref}
                          {pr.base_ref ? ` -> ${pr.base_ref}` : ''}
                        </span>
                      ) : null}
                      <span>{relativeTime(pr.updated_at)}</span>
                    </div>
                  </a>
                ))}
              </div>
            )}
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}

function PipelinePanel({
  state,
  refreshing,
  onRefresh,
}: {
  state: LoadState<PipelineOverview>
  refreshing: boolean
  onRefresh: () => void
}) {
  const environments = state.data?.environments ?? []

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <Rocket className="size-4 text-muted-foreground" />
            <CardTitle className="text-sm">Deployment Pipeline</CardTitle>
          </div>
          <CardDescription>
            Environment rollout, live versions, and observed infrastructure.
          </CardDescription>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="xs"
          onClick={onRefresh}
          disabled={refreshing}
        >
          <RefreshCcw
            className={`size-3.5 ${refreshing ? 'animate-spin' : ''}`}
          />
          Refresh
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        {state.loading ? (
          <div className="space-y-2">
            {Array.from({ length: 3 }).map((_, index) => (
              <Skeleton key={index} className="h-28 w-full rounded-lg" />
            ))}
          </div>
        ) : state.error ? (
          <div className="rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-3 text-sm text-destructive">
            {state.error}
          </div>
        ) : state.data ? (
          <>
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              {state.data.provider ? (
                <Badge variant="outline">{state.data.provider}</Badge>
              ) : null}
              <span className={statusTone(state.data.status)}>
                {state.data.status}
              </span>
              {state.data.refreshed_at ? (
                <span>Updated {relativeTime(state.data.refreshed_at)}</span>
              ) : null}
            </div>

            {state.data.message ? (
              <div className="flex items-start gap-2 rounded-lg border border-white/10 bg-white/5 px-3 py-3 text-sm text-muted-foreground">
                <AlertCircle className="mt-0.5 size-4 shrink-0" />
                <span>{state.data.message}</span>
              </div>
            ) : null}

            {environments.length === 0 ? (
              <div className="rounded-lg border border-white/10 bg-white/5 px-3 py-6 text-sm text-muted-foreground">
                No environments found for this project.
              </div>
            ) : (
              <div className="space-y-3">
                {environments.map((env) => (
                  <div
                    key={env.id ?? env.slug}
                    className="rounded-lg border border-white/10 bg-white/5 px-4 py-4"
                  >
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="space-y-1">
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-medium">{env.name}</span>
                          {env.is_default ? (
                            <Badge variant="outline">default</Badge>
                          ) : null}
                        </div>
                        <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                          {env.infrastructure ? (
                            <Badge variant="outline">{env.infrastructure}</Badge>
                          ) : null}
                          {env.data_tenancy ? (
                            <Badge variant="outline">{env.data_tenancy}</Badge>
                          ) : null}
                          {env.integration_mode ? (
                            <Badge variant="outline">{env.integration_mode}</Badge>
                          ) : null}
                        </div>
                      </div>
                      <span className="font-mono text-xs text-muted-foreground">
                        {env.slug}
                      </span>
                    </div>

                    <div className="mt-4 grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
                      <div className="space-y-2">
                        <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                          Deployments
                        </div>
                        {env.deployments.length === 0 ? (
                          <div className="rounded-lg border border-dashed border-white/10 px-3 py-4 text-sm text-muted-foreground">
                            No observed deployments.
                          </div>
                        ) : (
                          <div className="space-y-2">
                            {env.deployments.map((deployment) => (
                              <div
                                key={`${env.slug}:${deployment.service}`}
                                className="rounded-lg border border-white/10 px-3 py-3"
                              >
                                <div className="flex items-center justify-between gap-3">
                                  <span className="text-sm font-medium">
                                    {deployment.service}
                                  </span>
                                  <span className="text-xs text-muted-foreground">
                                    {relativeTime(deployment.observed_at)}
                                  </span>
                                </div>
                                <div className="mt-1 break-all font-mono text-xs text-emerald-300">
                                  {deployment.version}
                                </div>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>

                      <div className="space-y-2">
                        <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                          <Server className="size-3.5" />
                          Infrastructure
                        </div>
                        {env.resources.length === 0 ? (
                          <div className="rounded-lg border border-dashed border-white/10 px-3 py-4 text-sm text-muted-foreground">
                            No observed resources.
                          </div>
                        ) : (
                          <div className="space-y-2">
                            {env.resources.slice(0, 6).map((resource) => (
                              <div
                                key={resource.id}
                                className="rounded-lg border border-white/10 px-3 py-3"
                              >
                                <div className="flex flex-wrap items-center justify-between gap-2">
                                  <div>
                                    <div className="text-sm font-medium">
                                      {resource.name}
                                    </div>
                                    <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                                      <span>{resource.type}</span>
                                      {resource.region ? (
                                        <span>{resource.region}</span>
                                      ) : null}
                                      {resource.status ? (
                                        <span>{resource.status}</span>
                                      ) : null}
                                    </div>
                                  </div>
                                  <span className="text-xs text-muted-foreground">
                                    {relativeTime(resource.observed_at)}
                                  </span>
                                </div>
                                {resource.version ? (
                                  <div className="mt-1 break-all font-mono text-xs text-sky-300">
                                    {resource.version}
                                  </div>
                                ) : null}
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}

export function ProjectDeliveryOverview({
  projectId,
  token,
  refreshKey = 0,
}: {
  projectId: string
  token: string | null
  refreshKey?: number
}) {
  const [pullRequests, setPullRequests] = useState<LoadState<PullRequestOverview>>(
    {
      data: null,
      loading: true,
      error: null,
    },
  )
  const [pipeline, setPipeline] = useState<LoadState<PipelineOverview>>({
    data: null,
    loading: true,
    error: null,
  })
  const [refreshTick, setRefreshTick] = useState(0)

  const refresh = useCallback(() => {
    setRefreshTick((value) => value + 1)
  }, [])

  useEffect(() => {
    let cancelled = false
    setPullRequests((prev) => ({ ...prev, loading: true, error: null }))
    void fetchJSON<PullRequestOverview>(
      `/projects/${projectId}/pull-requests`,
      token,
    ).then((result) => {
      if (cancelled) return
      if (!result.ok) {
        setPullRequests({ data: null, loading: false, error: result.error })
        return
      }
      setPullRequests({ data: result.data, loading: false, error: null })
    })
    return () => {
      cancelled = true
    }
  }, [projectId, token, refreshKey, refreshTick])

  useEffect(() => {
    let cancelled = false
    setPipeline((prev) => ({ ...prev, loading: true, error: null }))
    void fetchJSON<PipelineOverview>(
      `/projects/${projectId}/pipeline?refresh=true`,
      token,
    ).then((result) => {
      if (cancelled) return
      if (!result.ok) {
        setPipeline({ data: null, loading: false, error: result.error })
        return
      }
      setPipeline({ data: result.data, loading: false, error: null })
    })
    return () => {
      cancelled = true
    }
  }, [projectId, token, refreshKey, refreshTick])

  const refreshing = useMemo(
    () => refreshTick > 0 && (pullRequests.loading || pipeline.loading),
    [pipeline.loading, pullRequests.loading, refreshTick],
  )

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium text-muted-foreground">SDLC</h2>
      </div>
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
        <PullRequestsPanel state={pullRequests} />
        <PipelinePanel
          state={pipeline}
          refreshing={refreshing}
          onRefresh={refresh}
        />
      </div>
    </section>
  )
}
