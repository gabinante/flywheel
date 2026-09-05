import { QueryClient } from '@tanstack/react-query'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      refetchInterval: false,
      refetchOnWindowFocus: true,
      retry: 1,
    },
  },
})
