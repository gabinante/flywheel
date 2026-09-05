import { useEffect, useState } from 'react'

import { useAPI } from '@/contexts/use-api'

/** Loads project name/slug for breadcrumb labels (shared across project-scoped pages). */
export function useProjectBreadcrumbLabel(projectId: string | undefined): string {
  const { client } = useAPI()
  const [entry, setEntry] = useState<{ id: string; label: string } | null>(null)

  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    void (async () => {
      const { data, response } = await client.GET('/projects/{projectID}', {
        params: { path: { projectID: projectId } },
      })
      if (cancelled) return
      if (!response.ok) {
        setEntry({ id: projectId, label: projectId })
        return
      }
      setEntry({
        id: projectId,
        label: data?.name ?? data?.slug ?? projectId,
      })
    })()
    return () => {
      cancelled = true
    }
  }, [client, projectId])

  if (!projectId) return 'Project'
  if (entry?.id === projectId) return entry.label
  return projectId
}
