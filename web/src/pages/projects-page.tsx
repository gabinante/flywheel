import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'

import { StaggerItem, StaggerList } from '@/components/stagger-list'
import { Badge } from '@/components/ui/badge'
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ListPageSkeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']

export function ProjectsPage() {
  const { orgId } = useParams<{ orgId: string }>()
  const { client } = useAuth()
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [orgName, setOrgName] = useState<string | null>(null)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    if (!orgId) return
    let cancelled = false
    ;(async () => {
      const orgRes = await client.GET('/orgs/{orgID}', {
        params: { path: { orgID: orgId } },
      })
      if (cancelled) return
      if (orgRes.response.ok && orgRes.data) {
        setOrgName(orgRes.data.name ?? orgRes.data.slug ?? orgId)
      } else {
        setOrgName(orgId)
      }

      const { data, error, response } = await client.GET('/orgs/{orgID}/projects', {
        params: { path: { orgID: orgId } },
      })
      if (cancelled) return
      if (!response.ok) {
        setErr(formatApiError(error))
        setProjects([])
        return
      }
      setErr(null)
      setProjects(data ?? [])
    })()
    return () => {
      cancelled = true
    }
  }, [client, orgId])

  if (!orgId) {
    return <p className="text-destructive text-sm">Missing org id.</p>
  }
  if (err) {
    return <p className="text-destructive text-sm">{err}</p>
  }
  if (!projects) {
    return <ListPageSkeleton />
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <p className="text-muted-foreground text-xs">
          <Link to="/orgs" className="hover:underline">
            Organizations
          </Link>
          <span className="px-1">/</span>
        </p>
        <h1 className="text-xl font-semibold tracking-tight">
          Projects{orgName ? ` — ${orgName}` : ''}
        </h1>
      </div>
      <StaggerList className="flex flex-col gap-3">
        {projects.map((p) => (
          <StaggerItem key={p.id}>
            <Link to={`/orgs/${orgId}/projects/${p.id}`}>
              <Card className="hover:bg-muted/40">
                <CardHeader>
                  <div className="flex flex-wrap items-center gap-2">
                    <CardTitle>{p.name ?? p.slug ?? p.id}</CardTitle>
                    {p.status ? (
                      <Badge variant="outline">{p.status}</Badge>
                    ) : null}
                  </div>
                  <CardDescription className="font-mono text-xs">
                    {p.id}
                  </CardDescription>
                </CardHeader>
              </Card>
            </Link>
          </StaggerItem>
        ))}
      </StaggerList>
      {projects.length === 0 ? (
        <p className="text-muted-foreground text-sm">No projects in this org.</p>
      ) : null}
    </div>
  )
}
