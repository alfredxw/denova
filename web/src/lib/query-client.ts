import { QueryClient } from '@tanstack/react-query'
import { APIError } from '@/lib/api-client'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: (failureCount, error) => {
        if (failureCount >= 1) return false
        if (!(error instanceof APIError)) return true
        return error.status === 408 || error.status === 429 || error.status >= 500
      },
      refetchOnWindowFocus: false,
    },
  },
})
