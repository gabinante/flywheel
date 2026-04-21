import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { StaggerItem, StaggerList } from '@/components/stagger-list'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { Skeleton, WorkStreamsPageSkeleton } from '@/components/ui/skeleton'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']
type WorkStream = components['schemas']['WorkStream']
type StreamStatusFilter = 'active' | 'closed' | 'all'

function parseStatusFilter(raw: string | null): StreamStatusFilter {
  if (raw === 'closed' || raw === 'all') return raw
  return 'active'
}

export function WorkStreamsPage() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>()
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
    setStreams(data ?? [])
  }, [client, projectId, statusFilter])

  useEffect(() => {
    queueMicrotask(() => {
      void loadStreams()
    })
  }, [loadStreams])

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
            orgId={orgId}
            projectId={projectId}
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
          <Link to={`/orgs/${orgId}/projects/${projectId}`}>Back to project</Link>
        </Button>
        <Button asChild size="sm">
          <Link to={`/orgs/${orgId}/projects/${projectId}/work-streams/new`}>
            New work stream
          </Link>
        </Button>
      </div>

      <Card>
        <CardHeader className="flex flex-col gap-3">
          <div className="space-y-1.5">
            <CardTitle className="text-sm">All streams in this project</CardTitle>
            <CardDescription>
              Active streams also appear on the project page. Use Closed or All to
              find archived streams.
            </CardDescription>
          </div>
          <div
            className="flex flex-wrap gap-2"
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
                variant={statusFilter === value ? 'default' : 'outline'}
                onClick={() => setFilter(value)}
                role="tab"
                aria-selected={statusFilter === value}
              >
                {label}
              </Button>
            ))}
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
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
            <p className="text-muted-foreground text-sm">
              {statusFilter === 'active'
                ? 'No active work streams. Create one or check the Closed tab.'
                : statusFilter === 'closed'
                  ? 'No closed work streams.'
                  : 'No work streams yet.'}
            </p>
          ) : !streamsErr && streams && streams.length > 0 ? (
            <StaggerList className="flex flex-col gap-2">
              {streams.map((ws) => {
                if (!ws.id) return null
                return (
                  <StaggerItem
                    key={ws.id}
                    className="border-border flex flex-wrap items-stretch gap-2 rounded-lg border p-2 transition-all duration-200 hover:border-primary/20"
                  >
                    <Link
                      to={`/orgs/${orgId}/projects/${projectId}/tickets?work_stream_id=${encodeURIComponent(ws.id)}`}
                      className="hover:bg-muted/40 flex min-w-[200px] flex-1 flex-col justify-center gap-1 rounded-md px-2 py-1 transition-colors"
                    >
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-sm font-medium">
                          {ws.name ?? ws.slug ?? ws.id}
                        </span>
                        {ws.status ? (
                          <Badge variant="outline">{ws.status}</Badge>
                        ) : null}
                      </div>
                      {ws.branch ? (
                        <span className="text-muted-foreground font-mono text-xs">
                          {ws.branch}
                        </span>
                      ) : null}
                    </Link>
                    <Button asChild variant="outline" size="sm" className="self-center">
                      <Link
                        to={`/orgs/${orgId}/projects/${projectId}/work-streams/${ws.id}`}
                      >
                        Manage
                      </Link>
                    </Button>
                  </StaggerItem>
                )
              })}
            </StaggerList>
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}
