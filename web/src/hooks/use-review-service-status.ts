import { useQuery } from '@tanstack/react-query'
import { useAPI } from '@/contexts/use-api'
import { formatApiError } from '@/lib/api/client'

export function useReviewServiceStatus() {
  const { client } = useAPI()
  return useQuery({
    queryKey: ['code-review-status'],
    queryFn: async ({ signal }) => {
      const { data, error, response } = await client.GET('/code-reviews/status', { signal })
      if (!response.ok || !data) throw new Error(formatApiError(error))
      return data
    },
    refetchInterval: 3_000,
  })
}
