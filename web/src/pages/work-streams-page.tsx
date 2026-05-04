import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { motion, AnimatePresence } from 'framer-motion'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Switch } from '@/components/ui/switch'
import { useAuth } from '@/contexts/use-auth'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { Skeleton, WorkStreamsPageSkeleton } from '@/components/ui/skeleton'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']
type WorkStream = components['schemas']['WorkStream']
type Ticket = components['schemas']['Ticket']
type StreamStatusFilter = 'active' | 'closed' | 'all'

function parseStatusFilter(raw: string | null): StreamStatusFilter {
  if (raw === 'closed' || raw === 'all') return raw
  return 'active'
}

type TicketStats = {
  total: number
  done: number
  executing: number
  pending: number
  other: number
}

function computeStats(tickets: Ticket[]): TicketStats {
  let done = 0
  let executing = 0
  let pending = 0
  let other = 0
  for (const t of tickets) {
    const s = t.state
    if (s === 'closed') done++
    else if (s === 'executing') executing++
    else if (s === 'draft') pending++
    else other++
  }
  return { total: tickets.length, done, executing, pending, other }
}

type StreamCardProps = {
  ws: WorkStream
  basePath: string
  tickets: Ticket[]
  onToggleStatus: (ws: WorkStream, newStatus: 'active' | 'closed') => void
  toggling: boolean
}

function StreamCard({ ws, basePath, tickets, onToggleStatus, toggling }: StreamCardProps) {
  if (!ws.id) return null

  const stats = computeStats(tickets)
  const isActive = ws.status === 'active'

  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -8 }}
      transition={{ duration: 0.2 }}
    >
      <Card className="overflow-hidden bg-white/[0.03] backdrop-blur-md border-white/10 hover:bg-white/[0.05] transition-colors duration-200">
        <div className="flex flex-col gap-3 p-4">
          {/* Header row */}
          <div className="flex items-start justify-between gap-3">
            <Link
              to={`${basePath}/tickets?work_stream_id=${encodeURIComponent(ws.id)}`}
              className="group flex min-w-0 flex-1 flex-col gap-1"
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-semibold text-foreground group-hover:text-primary transition-colors duration-150">
                  {ws.name ?? ws.slug ?? ws.id}
                </span>
                {ws.slug ? (
                  <Badge variant="muted" className="font-mono text-[10px]">
                    {ws.slug}
                  </Badge>
                ) : null}
              </div>
              {ws.branch ? (
                <span className="text-muted-foreground font-mono text-xs flex items-center gap-1.5">
                  <svg className="h-3 w-3 shrink-0 opacity-60" viewBox="0 0 16 16" fill="currentColor">
                    <path d="M9.5 3.25a2.25 2.25 0 1 1 3 2.122V6A2.5 2.5 0 0 1 10 8.5H6a1 1 0 0 0-1 1v1.128a2.251 2.251 0 1 1-1.5 0V5.372a2.25 2.25 0 1 1 1.5 0v1.836A2.492 2.492 0 0 1 6 7h4a1 1 0 0 0 1-1v-.628A2.25 2.25 0 0 1 9.5 3.25Z" />
                  </svg>
                  {ws.branch}
                </span>
              ) : null}
            </Link>

            <div className="flex items-center gap-3 shrink-0">
              {/* Status toggle */}
              <div className="flex items-center gap-2">
                <span className={`text-xs ${isActive ? 'text-primary/80' : 'text-muted-foreground'}`}>
                  {isActive ? 'Active' : 'Closed'}
                </span>
                <Switch
                  checked={isActive}
                  onCheckedChange={(checked) =>
                    onToggleStatus(ws, checked ? 'active' : 'closed')
                  }
                  disabled={toggling}
                />
              </div>

              <Button asChild variant="outline" size="sm" className="border-white/10 bg-white/5 hover:bg-white/10">
                <Link
                  to={`${basePath}/work-streams/${ws.id}`}
                >
                  Manage
                </Link>
              </Button>
            </div>
          </div>

          {/* Progress section */}
          {stats.total > 0 ? (
            <div className="flex flex-col gap-2">
              <Progress value={stats.done} max={stats.total} />
              <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                <span>
                  <span className="font-medium text-foreground">{stats.done}</span>
                  <span className="opacity-60">/{stats.total}</span>
                  {' '}done
                </span>
                {stats.executing > 0 ? (
                  <span className="flex items-center gap-1">
                    <span className="h-1.5 w-1.5 rounded-full bg-amber-400/70" />
                    {stats.executing} in progress
                  </span>
                ) : null}
                {stats.pending > 0 ? (
                  <span className="flex items-center gap-1">
                    <span className="h-1.5 w-1.5 rounded-full bg-white/30" />
                    {stats.pending} draft
                  </span>
                ) : null}
                {stats.other > 0 ? (
                  <span className="flex items-center gap-1">
                    <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/50" />
                    {stats.other} other
                  </span>
                ) : null}
              </div>
            </div>
          ) : (
            <p className="text-xs text-muted-foreground/60">No tickets yet</p>
          )}
        </div>
      </Card>
    </motion.div>
  )
}

export function WorkStreamsPage() {
  const { orgId, projectId, orgSlug, projectSlug, base } = useProjectPaths()
  const { client } = useAuth()
  const [searchParams, setSearchParams] = useSearchParams()
  const statusFilter = useMemo(
    () => parseStatusFilter(searchParams.get('status')),
    [searchParams],
  )

  const [project, setProject] = useState<Project | null | undefined>(undefined)
  const [projectErr, setProjectErr] = useState<string | null>(null)
  const [streams, setStreams] = useState<WorkStream[] | null>(null)
  const [streamsErr, setStreamsErr] = useState<string | null>(null)
  const [ticketsByStream, setTicketsByStream] = useState<Record<string, Ticket[]>>({})
  const [toggling, setToggling] = useState(false)

  const setFilter = useCallback(
    (next: StreamStatusFilter) => {
      setSearchParams(
        next === 'active' ? {} : { status: next },
        { replace: true },
      )
    },
    [setSearchParams],
  )

  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    ;(async () => {
      const { data, error, response } = await client.GET('/projects/{projectID}', {
        params: { path: { projectID: projectId } },
      })
      if (cancelled) return
      if (!response.ok) {
        setProjectErr(formatApiError(error))
        setProject(null)
        return
      }
      setProjectErr(null)
      setProject(data ?? null)
    })()
    return () => {
      cancelled = true
    }
  }, [client, projectId])

  const loadStreams = useCallback(async () => {
    if (!projectId) return
    const { data, error, response } = await client.GET(
      '/projects/{projectID}/work-streams',
      {
        params: {
          path: { projectID: projectId },
          query: { status: statusFilter },
        },
      },
    )
    if (!response.ok) {
      setStreamsErr(formatApiError(error))
      setStreams([])
      return
    }
    setStreamsErr(null)
    const streamList = data ?? []
    setStreams(streamList)

    // Load tickets for each stream to show progress
    const ticketMap: Record<string, Ticket[]> = {}
    await Promise.all(
      streamList.map(async (ws) => {
        if (!ws.id) return
        const { data: tickets } = await client.GET(
          '/projects/{projectID}/tickets',
          {
            params: {
              path: { projectID: projectId },
              query: { work_stream_id: ws.id },
            },
          },
        )
        ticketMap[ws.id] = tickets ?? []
      }),
    )
    setTicketsByStream(ticketMap)
  }, [client, projectId, statusFilter])

  useEffect(() => {
    queueMicrotask(() => {
      void loadStreams()
    })
  }, [loadStreams])

  const handleToggleStatus = useCallback(
    async (ws: WorkStream, newStatus: 'active' | 'closed') => {
      if (!projectId || !ws.id) return
      setToggling(true)
      const { response } = await client.PATCH(
        '/projects/{projectID}/work-streams/{workStreamID}',
        {
          params: {
            path: { projectID: projectId, workStreamID: ws.id },
          },
          body: { status: newStatus },
        },
      )
      setToggling(false)
      if (response.ok) {
        void loadStreams()
      }
    },
    [client, projectId, loadStreams],
  )

  if (!orgId || !projectId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }
  if (projectErr) {
    return <p className="text-destructive text-sm">{projectErr}</p>
  }
  if (project === undefined) {
    return <WorkStreamsPageSkeleton />
  }
  if (!project) {
    return <p className="text-muted-foreground text-sm">Project not found.</p>
  }

  const projectLabel = project.name ?? project.slug ?? project.id ?? projectId

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <p className="text-muted-foreground text-xs">
          <OrgProjectCrumbs
            orgId={orgSlug}
            projectId={projectSlug}
            projectLabel={projectLabel}
          />
          <span className="px-1">/</span>
          <span className="text-foreground" aria-current="page">
            Work streams
          </span>
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold tracking-tight">Work streams</h1>
        </div>
        <p className="text-muted-foreground font-mono text-xs">{project.id}</p>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button asChild variant="outline" size="sm">
          <Link to={base}>Back to project</Link>
        </Button>
        <Button asChild size="sm">
          <Link to={`${base}/work-streams/new`}>
            New work stream
          </Link>
        </Button>
      </div>

      <Card className="bg-white/[0.02] backdrop-blur-md border-white/10">
        <CardHeader className="flex flex-col gap-3">
          <div className="space-y-1.5">
            <CardTitle className="text-sm">All streams in this project</CardTitle>
            <CardDescription>
              Active streams also appear on the project page. Use Closed or All to
              find archived streams.
            </CardDescription>
          </div>
          <div
            className="flex flex-wrap gap-1.5"
            role="tablist"
            aria-label="Filter by status"
          >
            {(
              [
                { value: 'active' as const, label: 'Active' },
                { value: 'closed' as const, label: 'Closed' },
                { value: 'all' as const, label: 'All' },
              ] as const
            ).map(({ value, label }) => (
              <Button
                key={value}
                type="button"
                size="sm"
                variant={statusFilter === value ? 'default' : 'ghost'}
                onClick={() => setFilter(value)}
                role="tab"
                aria-selected={statusFilter === value}
                className={
                  statusFilter === value
                    ? ''
                    : 'text-muted-foreground hover:text-foreground'
                }
              >
                {label}
              </Button>
            ))}
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {streamsErr ? (
            <p className="text-destructive text-sm">{streamsErr}</p>
          ) : null}

          {!streamsErr && streams === null ? (
            <div className="flex flex-col gap-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-14 w-full rounded-lg" />
              ))}
            </div>
          ) : !streamsErr && streams?.length === 0 ? (
            <p className="text-muted-foreground text-sm py-4 text-center">
              {statusFilter === 'active'
                ? 'No active work streams. Create one or check the Closed tab.'
                : statusFilter === 'closed'
                  ? 'No closed work streams.'
                  : 'No work streams yet.'}
            </p>
          ) : !streamsErr && streams && streams.length > 0 ? (
            <AnimatePresence mode="popLayout">
              {streams.map((ws) => (
                <StreamCard
                  key={ws.id}
                  ws={ws}
                  basePath={base}
                  tickets={ws.id ? (ticketsByStream[ws.id] ?? []) : []}
                  onToggleStatus={handleToggleStatus}
                  toggling={toggling}
                />
              ))}
            </AnimatePresence>
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}
