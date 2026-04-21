import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  Check,
  ClipboardCopy,
  Eye,
  GitBranch,
  LayoutList,
  Plus,
  ShieldCheck,
  Ticket,
  Layers,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
} from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']
type WorkStream = components['schemas']['WorkStream']
type TicketT = components['schemas']['Ticket']

/* ---------- helpers ---------- */

/** Stat categories derived from ticket states. */
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

/** Count tickets per work-stream. Returns { total, done } per stream id. */
function streamTicketCounts(tickets: TicketT[]) {
  const m = new Map<string, { total: number; done: number }>()
  for (const t of tickets) {
    const wsId = t.work_stream_id
    if (!wsId) continue
    const entry = m.get(wsId) ?? { total: 0, done: 0 }
    entry.total++
    if (t.state === 'done') entry.done++
    m.set(wsId, entry)
  }
  return m
}

/* ---------- sub-components ---------- */

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
    <Card className="bg-white/5 backdrop-blur-md border-white/10">
      <CardContent className="flex flex-col items-center justify-center py-5 px-4">
        <span
          className={`text-3xl font-bold tabular-nums tracking-tight ${accent ?? 'text-foreground'}`}
        >
          {value}
        </span>
        <span className="text-muted-foreground text-xs font-medium mt-1">
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
      <Card className="h-full bg-white/5 backdrop-blur-md border-white/10 transition-all duration-200 hover:bg-white/10 hover:border-white/20 hover:scale-[1.02]">
        <CardContent className="flex items-start gap-4 py-5 px-5">
          <div className="shrink-0 rounded-lg bg-white/10 p-2.5 transition-colors group-hover:bg-white/15">
            <Icon className="size-5 text-foreground" />
          </div>
          <div className="flex flex-col gap-1 min-w-0">
            <span className="text-sm font-semibold tracking-tight text-foreground">
              {title}
            </span>
            <span className="text-xs text-muted-foreground leading-relaxed">
              {description}
            </span>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}

function ProgressBar({ value, max }: { value: number; max: number }) {
  const pct = max > 0 ? Math.round((value / max) * 100) : 0
  return (
    <div className="flex items-center gap-2.5 w-full">
      <div className="flex-1 h-2 rounded-full bg-white/10 overflow-hidden">
        <div
          className="h-full rounded-full bg-emerald-500/70 transition-all duration-500"
          style={{ width: `${pct}%` }}
        />
      </div>
      <span className="text-xs text-muted-foreground tabular-nums shrink-0 w-10 text-right">
        {pct}%
      </span>
    </div>
  )
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard API may be unavailable
    }
  }, [text])

  return (
    <Button
      variant="ghost"
      size="icon-xs"
      onClick={handleCopy}
      title="Copy to clipboard"
      className="shrink-0"
    >
      {copied ? (
        <Check className="size-3.5 text-emerald-400" />
      ) : (
        <ClipboardCopy className="size-3.5" />
      )}
    </Button>
  )
}

/* ---------- main page ---------- */

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

  /* --- load project --- */
  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    ;(async () => {
      const { data, error, response } = await client.GET(
        '/projects/{projectID}',
        {
          params: { path: { projectID: projectId } },
        },
      )
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

  /* --- load work streams --- */
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

  /* --- load tickets for stats --- */
  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    ;(async () => {
      const { data, response } = await client.GET(
        '/projects/{projectID}/tickets',
        {
          params: { path: { projectID: projectId } },
        },
      )
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

  /* --- guards --- */
  if (!orgId || !projectId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }
  if (err) {
    return <p className="text-destructive text-sm">{err}</p>
  }
  if (project === undefined) {
    return <p className="text-muted-foreground text-sm">Loading…</p>
  }
  if (!project) {
    return <p className="text-muted-foreground text-sm">Project not found.</p>
  }

  return (
    <div className="flex flex-col gap-8">
      {/* --- header / breadcrumbs --- */}
      <div className="flex flex-col gap-1.5">
        <p className="text-muted-foreground text-xs">
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
          {project.status ? (
            <Badge variant="outline">{project.status}</Badge>
          ) : null}
        </div>
      </div>

      {/* --- stat cards --- */}
      <section>
        <h2 className="sr-only">Project health</h2>
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
          <StatCard
            label="Total tickets"
            value={stats?.total ?? 0}
          />
          <StatCard
            label="Open"
            value={stats?.open ?? 0}
            accent="text-sky-400"
          />
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
          <StatCard
            label="Done"
            value={stats?.done ?? 0}
            accent="text-emerald-400"
          />
        </div>
      </section>

      {/* --- navigation cards --- */}
      <section>
        <h2 className="text-sm font-medium text-muted-foreground mb-3">
          Navigate
        </h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
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
        </div>
      </section>

      {/* --- work streams with progress --- */}
      <section>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-muted-foreground">
            Work Streams
          </h2>
          <div className="flex gap-2">
            <Button asChild variant="ghost" size="xs">
              <Link
                to={`/orgs/${orgId}/projects/${projectId}/work-streams`}
              >
                <LayoutList className="size-3.5" />
                All streams
              </Link>
            </Button>
            <Button asChild variant="ghost" size="xs">
              <Link
                to={`/orgs/${orgId}/projects/${projectId}/work-streams/new`}
              >
                <Plus className="size-3.5" />
                New
              </Link>
            </Button>
          </div>
        </div>

        {streamsErr ? (
          <p className="text-destructive text-sm">{streamsErr}</p>
        ) : workStreams === null ? (
          <div className="grid grid-cols-1 gap-3">
            {[1, 2].map((i) => (
              <div
                key={i}
                className="h-20 rounded-xl bg-white/5 animate-pulse"
              />
            ))}
          </div>
        ) : workStreams.length === 0 ? (
          <Card className="bg-white/5 backdrop-blur-md border-white/10">
            <CardContent className="py-8 text-center">
              <p className="text-muted-foreground text-sm">
                No work streams yet.{' '}
                <Link
                  to={`/orgs/${orgId}/projects/${projectId}/work-streams/new`}
                  className="text-foreground font-medium underline-offset-4 hover:underline"
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
                  <Card className="bg-white/5 backdrop-blur-md border-white/10 transition-all duration-200 hover:bg-white/10 hover:border-white/20">
                    <CardContent className="py-4 px-5">
                      <div className="flex flex-col gap-3">
                        <div className="flex items-center justify-between gap-3">
                          <div className="flex items-center gap-2.5 min-w-0">
                            <Layers className="size-4 text-muted-foreground shrink-0" />
                            <span className="text-sm font-medium truncate">
                              {ws.name ?? ws.slug ?? ws.id}
                            </span>
                            {ws.status ? (
                              <Badge variant="outline">{ws.status}</Badge>
                            ) : null}
                          </div>
                          <div className="flex items-center gap-3 shrink-0">
                            {counts ? (
                              <span className="text-xs text-muted-foreground tabular-nums">
                                {counts.done}/{counts.total} done
                              </span>
                            ) : null}
                            <Button
                              asChild
                              variant="outline"
                              size="xs"
                              className="opacity-0 group-hover:opacity-100 transition-opacity"
                              onClick={(e: React.MouseEvent) =>
                                e.stopPropagation()
                              }
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
                          <ProgressBar
                            value={counts.done}
                            max={counts.total}
                          />
                        ) : null}
                        {ws.branch ? (
                          <div className="flex items-center gap-1.5">
                            <GitBranch className="size-3 text-muted-foreground" />
                            <span className="text-muted-foreground font-mono text-xs truncate">
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

      {/* --- repository info --- */}
      {project.repo_url ? (
        <section>
          <h2 className="text-sm font-medium text-muted-foreground mb-3">
            Repository
          </h2>
          <Card className="bg-white/5 backdrop-blur-md border-white/10">
            <CardContent className="py-4 px-5">
              <div className="flex items-center gap-3">
                <GitBranch className="size-4 text-muted-foreground shrink-0" />
                <div className="flex flex-col gap-1 min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm break-all">
                      {project.repo_url}
                    </span>
                    <CopyButton text={project.repo_url} />
                  </div>
                  {project.default_branch ? (
                    <span className="text-xs text-muted-foreground">
                      Default branch:{' '}
                      <span className="font-mono">
                        {project.default_branch}
                      </span>
                    </span>
                  ) : null}
                </div>
              </div>
            </CardContent>
          </Card>
        </section>
      ) : null}

      {/* --- blocked tickets callout --- */}
      {stats && stats.blocked > 0 ? (
        <section>
          <Card className="bg-destructive/5 backdrop-blur-md border-destructive/20">
            <CardContent className="py-4 px-5">
              <div className="flex items-center gap-3">
                <div className="shrink-0 rounded-lg bg-destructive/10 p-2">
                  <ShieldCheck className="size-4 text-destructive" />
                </div>
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium">
                    {stats.blocked} ticket{stats.blocked === 1 ? '' : 's'}{' '}
                    need attention
                  </span>
                  <span className="text-xs text-muted-foreground">
                    Blocked, failed, or waiting for human input
                  </span>
                </div>
                <div className="ml-auto">
                  <Button asChild variant="outline" size="xs">
                    <Link
                      to={`/orgs/${orgId}/projects/${projectId}/tickets`}
                    >
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
