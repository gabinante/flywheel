import { useMemo } from 'react'
import { useParams } from 'react-router-dom'

import { useSlugResolver } from '@/contexts/slug-resolver-provider'

/**
 * Drop-in replacement for useParams() that resolves org/project slugs to UUIDs.
 * - orgId / projectId: always UUIDs (for API calls)
 * - orgParam / projectParam: raw URL values (slug or UUID)
 * - other params (ticketId, workStreamId, etc.) pass through unchanged
 */
export function useResolvedRouteParams() {
  const params = useParams<{
    orgId?: string
    projectId?: string
    ticketId?: string
    workStreamId?: string
    entityId?: string
    section?: string
    code?: string
  }>()
  const { resolveOrgId, resolveProjectId } = useSlugResolver()

  return useMemo(() => {
    const orgId = resolveOrgId(params.orgId)
    const projectId = resolveProjectId(params.orgId, params.projectId)
    return {
      orgId,
      projectId,
      orgParam: params.orgId,
      projectParam: params.projectId,
      ticketId: params.ticketId,
      workStreamId: params.workStreamId,
      entityId: params.entityId,
      section: params.section,
      code: params.code,
    }
  }, [params, resolveOrgId, resolveProjectId])
}
