import { useState, type ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

// A cache of its own for each test, which fails at once rather than retrying.
export function QueryWrapper({ children }: { children: ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { retry: false, staleTime: 0, networkMode: 'always' },
          mutations: { networkMode: 'always' },
        },
      })
  )
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}
