import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  Eye,
  GitBranch,
  Layers,
  LayoutList,
  PieChart,
  Plus,
  Save,
  ShieldCheck,
  Ticket,
} from 'lucide-react'

import { DispatchDashboard } from '@/components/dispatch-dashboard'
import { ProjectDispatchRoutingCard } from '@/components/project-dispatch-routing-card'
import { ProjectIntegrationsSection } from '@/components/project-integrations-section'
import { WorkflowTimelineEditor } from '@/components/workflow-timeline-editor'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { ProjectPageSkeleton, Skeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']
type WorkStream = components['schemas']['WorkStream']
type TicketT = components['schemas']['Ticket']

function computeStats(tickets: TicketT[]) {
  let total = 0
  let open = 0
  let inReview = 0
  let done = 0
  let executing = 0
  let blocked = 0

  for (const t of tickets) {
    total++
    switch (t.state) {
      case 'done':
        done++
        break
      case 'awaiting_review':
        inReview++
        break
      case 'executing':
      case 'claimed':
        executing++
        break
      case 'blocked':
      case 'needs_human':
      case 'failed':
        blocked++
        break
      default:
        open++
        break
    }
  }

  return { total, open, inReview, done, executing, blocked }
}

function streamTicketCounts(tickets: TicketT[]) {
  const counts = new Map<string, { total: number; done: number }>()
  for (const t of tickets) {
    const workStreamID = t.work_stream_id
    if (!workStreamID) continue
    const entry = counts.get(workStreamID) ?? { total: 0, done: 0 }
    entry.total++
    if (t.state === 'done') entry.done++
    counts.set(workStreamID, entry)
  }
  return counts
}

function StatCard({
  label,
  value,
  accent,
}: {
  label: string
  value: number
  accent?: string
}) {
  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardContent className="flex flex-col items-center justify-center px-4 py-5">
        <span
          className={`text-3xl font-bold tracking-tight tabular-nums ${accent ?? 'text-foreground'}`}
        >
          {value}
        </span>
        <span className="mt-1 text-xs font-medium text-muted-foreground">
          {label}
        </span>
      </CardContent>
    </Card>
  )
}

function NavCard({
  to,
  icon: Icon,
  title,
  description,
}: {
  to: string
  icon: React.ComponentType<{ className?: string }>
  title: string
  description: string
}) {
  return (
    <Link to={to} className="group">
      <Card className="h-full border-white/10 bg-white/5 backdrop-blur-md transition-all duration-200 hover:scale-[1.02] hover:border-white/20 hover:bg-white/10">
        <CardContent className="flex items-start gap-4 px-5 py-5">
          <div className="shrink-0 rounded-lg bg-white/10 p-2.5 transition-colors group-hover:bg-white/15">
            <Icon className="size-5 text-foreground" />
          </div>
          <div className="flex min-w-0 flex-col gap-1">
            <span className="text-sm font-semibold tracking-tight text-foreground">
              {title}
            </span>
            <span className="text-xs leading-relaxed text-muted-foreground">
              {description}
            </span>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}

function ProgressBar({ value, max }: { value: number; max: number }) {
  const percent = max > 0 ? Math.round((value / max) * 100) : 0
  return (
    <div className="flex w-full items-center gap-2.5">
      <div className="h-2 flex-1 overflow-hidden rounded-full bg-white/10">
        <div
          className="h-full rounded-full bg-emerald-500/70 transition-all duration-500"
          style={{ width: `${percent}%` }}
        />
      </div>
      <span className="w-10 shrink-0 text-right text-xs tabular-nums text-muted-foreground">
        {percent}%
      </span>
    </div>
  )
}


function RepositoryCard({
  projectId,
  project,
  onProjectChange,
}: {
  projectId: string
  project: Project
  onProjectChange: (project: Project) => void
}) {
  const { client } = useAuth()
  const [repoUrl, setRepoUrl] = useState(project.repo_url ?? '')
  const [defaultBranch, setDefaultBranch] = useState(project.default_branch ?? '')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [savedAt, setSavedAt] = useState<number | null>(null)

  useEffect(() => {
    setRepoUrl(project.repo_url ?? '')
    setDefaultBranch(project.default_branch ?? '')
  }, [project.repo_url, project.default_branch])

  async function saveRepo() {
    setSaving(true)
    setError(null)
    setSavedAt(null)
    const { data, error: apiError, response } = await client.PATCH(
      '/projects/{projectID}',
      {
        params: { path: { projectID: projectId } },
        body: { repo_url: repoUrl || undefined, default_branch: defaultBranch || undefined },
      },
    )
    if (!response.ok || !data) {
      setError(formatApiError(apiError))
      setSaving(false)
      return
    }
    onProjectChange(data)
    setSavedAt(Date.now())
    setSaving(false)
  }

  return (
    <section>
      <h2 className="mb-3 text-sm font-medium text-muted-foreground">
        Repository
      </h2>
      <Card className="border-white/10 bg-white/5 backdrop-blur-md">
        <CardContent className="px-5 py-4">
          <div className="flex flex-col gap-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="space-y-1.5">
                <span className="text-xs font-medium text-muted-foreground">
                  Repository URL
                </span>
                <Input
                  value={repoUrl}
                  onChange={(e) => setRepoUrl(e.target.value)}
                  placeholder="https://github.com/org/repo.git"
                />
              </label>
              <label className="space-y-1.5">
                <span className="text-xs font-medium text-muted-foreground">
                  Default branch
                </span>
                <Input
                  value={defaultBranch}
                  onChange={(e) => setDefaultBranch(e.target.value)}
                  placeholder="main"
                />
              </label>
            </div>
            <div className="flex items-center gap-2">
              <Button size="xs" onClick={() => void saveRepo()} disabled={saving}>
                <Save className="size-3.5" />
                {saving ? 'Saving…' : 'Save'}
              </Button>
              {savedAt ? (
                <span className="text-xs text-emerald-400">Saved</span>
              ) : null}
              {error ? (
                <span className="text-xs text-destructive">{error}</span>
              ) : null}
            </div>
          </div>
        </CardContent>
      </Card>
    </section>
  )
}

export function ProjectPage() {
  const { orgId, projectId } = useParams<{
    orgId: string
    projectId: string
  }>()
  const { client } = useAuth()
  const [project, setProject] = useState<Project | null | undefined>(undefined)
  const [workStreams, setWorkStreams] = useState<WorkStream[] | null>(null)
  const [tickets, setTickets] = useState<TicketT[] | null>(null)
  const [streamsErr, setStreamsErr] = useState<string | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [dispatchToggling, setDispatchToggling] = useState(false)

  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    ;(async () => {
      const { data, error, response } = await client.GET('/projects/{projectID}', {
        params: { path: { projectID: projectId } },
      })
      if (cancelled) return
      if (!response.ok) {
        setErr(formatApiError(error))
        setProject(null)
        return
      }
      setErr(null)
      setProject(data ?? null)
    })()
    return () => {
      cancelled = true
    }
  }, [client, projectId])

  const loadWorkStreams = useCallback(async () => {
    if (!projectId) return
    const { data, error, response } = await client.GET(
      '/projects/{projectID}/work-streams',
      {
        params: {
          path: { projectID: projectId },
        },
      },
    )
    if (!response.ok) {
      setStreamsErr(formatApiError(error))
      setWorkStreams([])
      return
    }
    setStreamsErr(null)
    setWorkStreams(data ?? [])
  }, [client, projectId])

  useEffect(() => {
    queueMicrotask(() => {
      void loadWorkStreams()
    })
  }, [loadWorkStreams])

  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    ;(async () => {
      const { data, response } = await client.GET('/projects/{projectID}/tickets', {
        params: { path: { projectID: projectId } },
      })
      if (cancelled) return
      setTickets(response.ok ? (data ?? []) : [])
    })()
    return () => {
      cancelled = true
    }
  }, [client, projectId])

  const stats = useMemo(
    () => (tickets ? computeStats(tickets) : null),
    [tickets],
  )
  const streamCounts = useMemo(
    () => (tickets ? streamTicketCounts(tickets) : null),
    [tickets],
  )

  const toggleDispatch = useCallback(async () => {
    if (!projectId || !project) return
    setDispatchToggling(true)
    const nextValue = !(project.dispatch_enabled !== false)
    const { response } = await client.PATCH('/projects/{projectID}', {
      params: { path: { projectID: projectId } },
      body: { dispatch_enabled: nextValue },
    })
    if (response.ok) {
      setProject((prev) =>
        prev ? { ...prev, dispatch_enabled: nextValue } : prev,
      )
    }
    setDispatchToggling(false)
  }, [client, project, projectId])

  if (!orgId || !projectId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }
  if (err) {
    return <p className="text-destructive text-sm">{err}</p>
  }
  if (project === undefined) {
    return <ProjectPageSkeleton />
  }
  if (!project) {
    return <p className="text-muted-foreground text-sm">Project not found.</p>
  }

  const dispatchOn = project.dispatch_enabled !== false

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-1.5">
        <p className="text-xs text-muted-foreground">
          <Link to="/orgs" className="hover:underline">
            Organizations
          </Link>
          <span className="px-1">/</span>
          <Link to={`/orgs/${orgId}/projects`} className="hover:underline">
            Projects
          </Link>
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-2xl font-semibold tracking-tight">
            {project.name ?? project.slug ?? project.id}
          </h1>
          {project.status ? <Badge variant="outline">{project.status}</Badge> : null}
        </div>
      </div>

      <section>
        <h2 className="sr-only">Project health</h2>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          <StatCard label="Total tickets" value={stats?.total ?? 0} />
          <StatCard label="Open" value={stats?.open ?? 0} accent="text-sky-400" />
          <StatCard
            label="In progress"
            value={stats?.executing ?? 0}
            accent="text-amber-400"
          />
          <StatCard
            label="In review"
            value={stats?.inReview ?? 0}
            accent="text-violet-400"
          />
          <StatCard label="Done" value={stats?.done ?? 0} accent="text-emerald-400" />
        </div>
      </section>

      <Card className="border-white/10 bg-white/5 backdrop-blur-md">
        <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="space-y-1.5">
            <CardTitle className="text-sm">Agent dispatch</CardTitle>
            <CardDescription>
              {dispatchOn
                ? 'Agents will automatically pick up pending tickets for this project.'
                : 'Dispatch is paused — agents will not pick up new tickets.'}
            </CardDescription>
          </div>
          <Button
            variant={dispatchOn ? 'default' : 'outline'}
            size="sm"
            className={
              dispatchOn
                ? 'shrink-0 bg-emerald-600 hover:bg-emerald-700'
                : 'shrink-0'
            }
            disabled={dispatchToggling}
            onClick={toggleDispatch}
          >
            {dispatchToggling
              ? 'Saving…'
              : dispatchOn
                ? 'Dispatch enabled'
                : 'Dispatch disabled'}
          </Button>
        </CardHeader>
      </Card>

      <WorkflowTimelineEditor projectId={projectId} />

      <ProjectDispatchRoutingCard
        projectId={projectId}
        project={project}
        onProjectChange={setProject}
      />

      <ProjectIntegrationsSection
        projectId={projectId}
        project={project}
      />

      <section>
        <h2 className="mb-3 text-sm font-medium text-muted-foreground">
          Navigate
        </h2>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <NavCard
            to={`/orgs/${orgId}/projects/${projectId}/tickets`}
            icon={Ticket}
            title="Tickets"
            description="View and manage all tickets in this project"
          />
          <NavCard
            to={`/orgs/${orgId}/projects/${projectId}/reviews`}
            icon={Eye}
            title="Pending Reviews"
            description="Review tickets awaiting approval"
          />
          <NavCard
            to={`/orgs/${orgId}/projects/${projectId}/policies`}
            icon={ShieldCheck}
            title="Policy Health"
            description="Monitor policy compliance and gates"
          />
          <NavCard
            to={`/orgs/${orgId}/projects/${projectId}/usage`}
            icon={PieChart}
            title="Usage"
            description="Inspect token, cost, and API usage trends"
          />
        </div>
      </section>

      <section>
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-medium text-muted-foreground">
            Work Streams
          </h2>
          <div className="flex gap-2">
            <Button asChild variant="ghost" size="xs">
              <Link to={`/orgs/${orgId}/projects/${projectId}/work-streams`}>
                <LayoutList className="size-3.5" />
                All streams
              </Link>
            </Button>
            <Button asChild variant="ghost" size="xs">
              <Link to={`/orgs/${orgId}/projects/${projectId}/work-streams/new`}>
                <Plus className="size-3.5" />
                New
              </Link>
            </Button>
          </div>
        </div>

        {streamsErr ? (
          <p className="text-destructive text-sm">{streamsErr}</p>
        ) : workStreams === null ? (
          <div className="flex flex-col gap-2">
            {Array.from({ length: 3 }).map((_, index) => (
              <Skeleton key={index} className="h-20 w-full rounded-xl" />
            ))}
          </div>
        ) : workStreams.length === 0 ? (
          <Card className="border-white/10 bg-white/5 backdrop-blur-md">
            <CardContent className="py-8 text-center">
              <p className="text-sm text-muted-foreground">
                No work streams yet.{' '}
                <Link
                  to={`/orgs/${orgId}/projects/${projectId}/work-streams/new`}
                  className="font-medium text-foreground underline-offset-4 hover:underline"
                >
                  Create one
                </Link>{' '}
                to organize your tickets.
              </p>
            </CardContent>
          </Card>
        ) : (
          <div className="grid grid-cols-1 gap-3">
            {workStreams.map((ws) => {
              if (!ws.id) return null
              const counts = streamCounts?.get(ws.id)
              return (
                <Link
                  key={ws.id}
                  to={`/orgs/${orgId}/projects/${projectId}/tickets?work_stream_id=${encodeURIComponent(ws.id)}`}
                  className="group"
                >
                  <Card className="border-white/10 bg-white/5 backdrop-blur-md transition-all duration-200 hover:border-white/20 hover:bg-white/10">
                    <CardContent className="px-5 py-4">
                      <div className="flex flex-col gap-3">
                        <div className="flex items-center justify-between gap-3">
                          <div className="flex min-w-0 items-center gap-2.5">
                            <Layers className="size-4 shrink-0 text-muted-foreground" />
                            <span className="truncate text-sm font-medium">
                              {ws.name ?? ws.slug ?? ws.id}
                            </span>
                            {ws.status ? (
                              <Badge variant="outline">{ws.status}</Badge>
                            ) : null}
                          </div>
                          <div className="flex shrink-0 items-center gap-3">
                            {counts ? (
                              <span className="text-xs tabular-nums text-muted-foreground">
                                {counts.done}/{counts.total} done
                              </span>
                            ) : null}
                            <Button
                              asChild
                              variant="outline"
                              size="xs"
                              className="opacity-0 transition-opacity group-hover:opacity-100"
                              onClick={(event: React.MouseEvent) => event.stopPropagation()}
                            >
                              <Link
                                to={`/orgs/${orgId}/projects/${projectId}/work-streams/${ws.id}`}
                              >
                                Manage
                              </Link>
                            </Button>
                          </div>
                        </div>
                        {counts && counts.total > 0 ? (
                          <ProgressBar value={counts.done} max={counts.total} />
                        ) : null}
                        {ws.branch ? (
                          <div className="flex items-center gap-1.5">
                            <GitBranch className="size-3 text-muted-foreground" />
                            <span className="truncate font-mono text-xs text-muted-foreground">
                              {ws.branch}
                            </span>
                          </div>
                        ) : null}
                      </div>
                    </CardContent>
                  </Card>
                </Link>
              )
            })}
          </div>
        )}
      </section>

      <DispatchDashboard orgId={orgId} projectId={projectId} />

      <RepositoryCard
        projectId={projectId}
        project={project}
        onProjectChange={setProject}
      />

      {stats && stats.blocked > 0 ? (
        <section>
          <Card className="border-destructive/20 bg-destructive/5 backdrop-blur-md">
            <CardContent className="px-5 py-4">
              <div className="flex items-center gap-3">
                <div className="shrink-0 rounded-lg bg-destructive/10 p-2">
                  <ShieldCheck className="size-4 text-destructive" />
                </div>
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium">
                    {stats.blocked} ticket{stats.blocked === 1 ? '' : 's'} need
                    attention
                  </span>
                  <span className="text-xs text-muted-foreground">
                    Blocked, failed, or waiting for human input
                  </span>
                </div>
                <div className="ml-auto">
                  <Button asChild variant="outline" size="xs">
                    <Link to={`/orgs/${orgId}/projects/${projectId}/tickets`}>
                      View tickets
                    </Link>
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        </section>
      ) : null}
    </div>
  )
}
