import { useMemo } from 'react'
import { useLocation, useParams } from 'react-router-dom'

import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'

export type BreadcrumbItem = {
  label: string
  href?: string
}

/**
 * Derives breadcrumb items from the current route location and params.
 * Returns an array of { label, href? } — the last item has no href (current page).
 */
export function useRouteBreadcrumbs(): BreadcrumbItem[] {
  const location = useLocation()
  const params = useParams<{
    orgId?: string
    projectId?: string
    ticketId?: string
    workStreamId?: string
  }>()

  const projectLabel = useProjectBreadcrumbLabel(params.projectId)

  return useMemo(() => {
    const path = location.pathname

    // Home
    if (path === '/' || path === '') return []

    const crumbs: BreadcrumbItem[] = []

    // Organizations
    if (path.startsWith('/orgs')) {
      if (path === '/orgs') {
        crumbs.push({ label: 'Organizations' })
        return crumbs
      }
      crumbs.push({ label: 'Organizations', href: '/orgs' })
    }

    // Projects
    if (params.orgId && path.includes('/projects')) {
      const projectsPath = `/orgs/${params.orgId}/projects`

      if (path === projectsPath) {
        crumbs.push({ label: 'Projects' })
        return crumbs
      }
      crumbs.push({ label: 'Projects', href: projectsPath })

      if (params.projectId) {
        const projectPath = `${projectsPath}/${params.projectId}`

        // Tickets
        if (path.includes('/tickets')) {
          crumbs.push({ label: projectLabel, href: projectPath })
          const ticketsPath = `${projectPath}/tickets`
          if (params.ticketId) {
            crumbs.push({ label: 'Tickets', href: ticketsPath })
            crumbs.push({ label: params.ticketId })
          } else {
            crumbs.push({ label: 'Tickets' })
          }
          return crumbs
        }

        // Reviews
        if (path.includes('/reviews')) {
          crumbs.push({ label: projectLabel, href: projectPath })
          crumbs.push({ label: 'Reviews' })
          return crumbs
        }

        // Policies
        if (path.includes('/policies')) {
          crumbs.push({ label: projectLabel, href: projectPath })
          crumbs.push({ label: 'Policies' })
          return crumbs
        }

        // Work Streams
        if (path.includes('/work-streams')) {
          crumbs.push({ label: projectLabel, href: projectPath })
          const wsPath = `${projectPath}/work-streams`
          if (path.includes('/new')) {
            crumbs.push({ label: 'Work Streams', href: wsPath })
            crumbs.push({ label: 'New' })
          } else if (params.workStreamId) {
            crumbs.push({ label: 'Work Streams', href: wsPath })
            crumbs.push({ label: params.workStreamId })
          } else {
            crumbs.push({ label: 'Work Streams' })
          }
          return crumbs
        }

        // Project overview (no sub-path)
        crumbs.push({ label: projectLabel })
        return crumbs
      }
    }

    return crumbs
  }, [location.pathname, params.orgId, params.projectId, params.ticketId, params.workStreamId, projectLabel])
}
