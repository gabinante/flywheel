import { useMemo } from 'react'
import { useLocation } from 'react-router-dom'

import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { useResolvedRouteParams } from '@/hooks/use-resolved-route-params'

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
  const { projectId, orgParam, projectParam, ticketId, workStreamId } = useResolvedRouteParams()

  // Pass resolved UUID to the label hook so it can call the API
  const projectLabel = useProjectBreadcrumbLabel(projectId)

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

    // Use raw URL params for href construction (they already contain slugs)
    if (orgParam && path.includes('/projects')) {
      const projectsPath = `/orgs/${orgParam}/projects`

      if (path === projectsPath) {
        crumbs.push({ label: 'Projects' })
        return crumbs
      }
      crumbs.push({ label: 'Projects', href: projectsPath })

      if (projectParam) {
        const projectPath = `${projectsPath}/${projectParam}`

        // Tickets
        if (path.includes('/tickets')) {
          crumbs.push({ label: projectLabel, href: projectPath })
          const ticketsPath = `${projectPath}/tickets`
          if (ticketId) {
            crumbs.push({ label: 'Tickets', href: ticketsPath })
            crumbs.push({ label: ticketId })
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
          } else if (workStreamId) {
            crumbs.push({ label: 'Work Streams', href: wsPath })
            crumbs.push({ label: workStreamId })
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
  }, [location.pathname, orgParam, projectParam, ticketId, workStreamId, projectLabel])
}
