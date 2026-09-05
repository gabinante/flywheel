import { useQuery } from '@tanstack/react-query'

import { useAPI } from '@/contexts/use-api'
import type { components } from '@/lib/api/v1'

type Escalation = components['schemas']['Escalation']

export function useProjectEscalations(projectId: string | undefined) {
  const { client } = useAPI()

  const {
    data: escalations = [],
    isLoading: loading,
    refetch: refresh,
  } = useQuery<Escalation[]>({
    queryKey: ['project-escalations', projectId],
    queryFn: async () => {
      const { data, response } = await client.GET('/projects/{projectID}/escalations', {
        params: { path: { projectID: projectId! } },
      })
      if (!response.ok) return []
      return (data ?? []) as Escalation[]
    },
    enabled: Boolean(projectId),
  })

  return { escalations, loading, refresh }
}
