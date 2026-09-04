import { useEffect, useMemo, useState } from 'react'
import { FolderKanban } from 'lucide-react'
import { Link, useLocation, useNavigate } from 'react-router-dom'

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useAuth } from '@/contexts/use-auth'
import { useSlugResolver } from '@/contexts/slug-resolver-provider'
import { useResolvedRouteParams } from '@/hooks/use-resolved-route-params'
import type { components } from '@/lib/api/v1'
import { orgsWithIDs, resolvePreferredOrgId, setPreferredOrgId } from '@/lib/org-preferences'
import { cn } from '@/lib/utils'

type Project = components['schemas']['Project']

const ALL_PROJECTS = '__all'
const NEW_PROJECT = '__new'

/** Section of the current project route (command, tickets, code-reviews, …) so switching keeps the page. */
function currentSection(pathname: string): string {
  const m = /^\/orgs\/[^/]+\/projects\/[^/]+\/([^/]+)/.exec(pathname)
  if (!m) return 'command'
  return m[1] === 'new' ? 'command' : m[1]
}

/**
 * Project switcher for the sidebar. Flywheel is single-operator, so the org is implicit;
 * the thing you actually move between is the project (one per Linear project).
 */
export function ProjectSwitcher({ expanded }: { expanded: boolean }) {
  const { token, client } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const { orgId, projectId } = useResolvedRouteParams()
  const { orgSlug, projectSlug } = useSlugResolver()
  const [orgIdLoaded, setOrgIdLoaded] = useState<string>('')
  const [projects, setProjects] = useState<Project[] | null>(null)

  // Resolve the org to list projects for: the route's org, else the preferred/first org.
  useEffect(() => {
    if (!token) return
    let cancelled = false
    ;(async () => {
      let oid = orgId ?? ''
      if (!oid) {
        const { data, response } = await client.GET('/orgs', {})
        if (cancelled || !response.ok) return
        const orgs = orgsWithIDs(data)
        oid = resolvePreferredOrgId(orgs) ?? orgs[0]?.id ?? ''
      }
      if (!oid) {
        if (!cancelled) setProjects([])
        return
      }
      setPreferredOrgId(oid)
      const { data, response } = await client.GET('/orgs/{orgID}/projects', { params: { path: { orgID: oid } } })
      if (cancelled) return
      setOrgIdLoaded(oid)
      setProjects(response.ok && data ? data : [])
    })()
    return () => {
      cancelled = true
    }
  }, [client, token, orgId])

  const sorted = useMemo(
    () => [...(projects ?? [])].sort((a, b) => (a.name ?? '').localeCompare(b.name ?? '')),
    [projects],
  )
  const current = sorted.find((p) => p.id === projectId) ?? null
  const orgPath = orgIdLoaded ? `/orgs/${orgSlug(orgIdLoaded) ?? orgIdLoaded}` : '/orgs'

  if (!token) return null

  if (!expanded) {
    const target = current ? `${orgPath}/projects/${projectSlug(current.id ?? '') ?? current.slug ?? current.id}/command` : `${orgPath}/projects`
    return (
      <Link
        to={target}
        className="flex h-9 items-center justify-center rounded-lg text-sidebar-foreground/70 transition-colors hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
        title={current?.name ?? 'Projects'}
      >
        <FolderKanban className="size-4" />
      </Link>
    )
  }

  if (projects === null) {
    return (
      <div className="space-y-2 px-1">
        <div className="h-3 w-16 animate-pulse rounded bg-white/[0.06]" />
        <div className="h-8 animate-pulse rounded-lg bg-white/[0.06]" />
      </div>
    )
  }

  return (
    <div className="space-y-1 px-1">
      <span className="px-2 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Project</span>
      <Select
        value={current?.id ?? ''}
        onValueChange={(next) => {
          if (next === ALL_PROJECTS) {
            navigate(`${orgPath}/projects`)
            return
          }
          if (next === NEW_PROJECT) {
            navigate(`${orgPath}/projects/new`)
            return
          }
          const p = sorted.find((x) => x.id === next)
          if (!p) return
          const slug = projectSlug(p.id ?? '') ?? p.slug ?? p.id
          navigate(`${orgPath}/projects/${slug}/${currentSection(location.pathname)}`)
        }}
      >
        <SelectTrigger
          size="sm"
          className={cn(
            'h-8 w-full border-sidebar-border bg-white/[0.03] text-sidebar-foreground hover:bg-sidebar-accent/50',
            '[&_[data-slot=select-value]]:min-w-0',
          )}
          aria-label="Switch project"
        >
          <SelectValue placeholder={sorted.length ? 'Select project' : 'No projects yet'} />
        </SelectTrigger>
        <SelectContent align="start" className="min-w-56">
          {sorted.map((p) => (
            <SelectItem key={p.id} value={p.id ?? ''}>
              <span className="truncate">{p.name ?? p.slug ?? p.id}</span>
            </SelectItem>
          ))}
          <SelectItem value={ALL_PROJECTS}>
            <span className="text-muted-foreground">All projects…</span>
          </SelectItem>
          <SelectItem value={NEW_PROJECT}>
            <span className="text-muted-foreground">New project…</span>
          </SelectItem>
        </SelectContent>
      </Select>
    </div>
  )
}
