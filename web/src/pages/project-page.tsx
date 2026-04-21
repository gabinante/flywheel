import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'

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
import { ProjectPageSkeleton, Skeleton } from '@/components/ui/skeleton'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']
type WorkStream = components['schemas']['WorkStream']

export function ProjectPage() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>()
  const { client } = useAuth()
  const [project, setProject] = useState<Project | null | undefined>(undefined)
  const [workStreams, setWorkStreams] = useState<WorkStream[] | null>(null)
  const [streamsErr, setStreamsErr] = useState<string | null>(null)
  const [err, setErr] = useState<string | null>(null)

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

  useEffect(() => {
    queueMicrotask(() => {
      void loadWorkStreams()
    })
  }, [loadWorkStreams])

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

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <p className="text-muted-foreground text-xs">
          <Link to="/orgs" className="hover:underline">
            Organizations
          </Link>
          <span className="px-1">/</span>
          <Link to={`/orgs/${orgId}/projects`} className="hover:underline">
            Projects
          </Link>
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold tracking-tight">
            {project.name ?? project.slug ?? project.id}
          </h1>
          {project.status ? (
            <Badge variant="outline">{project.status}</Badge>
          ) : null}
        </div>
        <p className="text-muted-foreground font-mono text-xs">{project.id}</p>
      </div>

      <div className="flex flex-wrap gap-2">
        <Button asChild>
          <Link to={`/orgs/${orgId}/projects/${projectId}/tickets`}>
            Tickets
          </Link>
        </Button>
        <Button asChild variant="secondary">
          <Link to={`/orgs/${orgId}/projects/${projectId}/reviews`}>
            Pending reviews
          </Link>
        </Button>
        <Button asChild variant="secondary">
          <Link to={`/orgs/${orgId}/projects/${projectId}/policies`}>
            Policy Health
          </Link>
        </Button>
      </div>

      <Card>
        <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="space-y-1.5">
            <CardTitle className="text-sm">Work streams</CardTitle>
            <CardDescription>
              Active streams at a glance. Open{' '}
              <Link
                to={`/orgs/${orgId}/projects/${projectId}/work-streams`}
                className="text-foreground font-medium underline-offset-4 hover:underline"
              >
                all work streams
              </Link>{' '}
              for closed streams and full management.
            </CardDescription>
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            <Button asChild variant="outline" size="sm">
              <Link to={`/orgs/${orgId}/projects/${projectId}/work-streams`}>
                All streams
              </Link>
            </Button>
            <Button asChild size="sm">
              <Link
                to={`/orgs/${orgId}/projects/${projectId}/work-streams/new`}
              >
                New work stream
              </Link>
            </Button>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {streamsErr ? (
            <p className="text-destructive text-sm">{streamsErr}</p>
          ) : null}

          {!streamsErr && workStreams === null ? (
            <div className="flex flex-col gap-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-14 w-full rounded-lg" />
              ))}
            </div>
          ) : !streamsErr && workStreams?.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No work streams yet — use{' '}
              <span className="text-foreground font-medium">New work stream</span>{' '}
              to add one.
            </p>
          ) : !streamsErr && workStreams && workStreams.length > 0 ? (
            <ul className="flex flex-col gap-2">
              {workStreams.map((ws) => {
                if (!ws.id) return null
                return (
                  <li
                    key={ws.id}
                    className="border-border flex flex-wrap items-stretch gap-2 rounded-lg border p-2"
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
                  </li>
                )
              })}
            </ul>
          ) : null}
        </CardContent>
      </Card>

      {project.repo_url ? (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Repository</CardTitle>
            <CardDescription className="font-mono text-xs break-all">
              {project.repo_url}
            </CardDescription>
          </CardHeader>
        </Card>
      ) : null}
    </div>
  )
}
