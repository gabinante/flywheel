import { useQuery } from '@tanstack/react-query'

import { useAuth } from '@/contexts/use-auth'
import type { components } from '@/lib/api/v1'

type Escalation = components['schemas']['Escalation']

export function useProjectEscalations(projectId: string | undefined) {
  const { client, token } = useAuth()

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
    enabled: Boolean(projectId && token),
    refetchInterval: 10_000,
  })

  return { escalations, loading, refresh }
}
