import { useMemo } from 'react'

import { useSlugResolver } from '@/contexts/slug-resolver-provider'
import { useResolvedRouteParams } from '@/hooks/use-resolved-route-params'

/**
 * Returns slug-based path builders for the current org/project.
 * - orgId / projectId: UUIDs for API calls
 * - orgSlug / projectSlug: slugs (or UUID fallback) for URL construction
 * - base: `/orgs/{orgSlug}/projects/{projectSlug}` or empty string
 */
export function useProjectPaths() {
  const { orgId, projectId } = useResolvedRouteParams()
  const { orgSlug, projectSlug } = useSlugResolver()

  return useMemo(() => {
    const oSlug = orgId ? (orgSlug(orgId) ?? orgId) : ''
    const pSlug = projectId ? (projectSlug(projectId) ?? projectId) : ''
    const base = oSlug && pSlug ? `/orgs/${oSlug}/projects/${pSlug}` : ''
    return { orgId, projectId, orgSlug: oSlug, projectSlug: pSlug, base }
  }, [orgId, projectId, orgSlug, projectSlug])
}
