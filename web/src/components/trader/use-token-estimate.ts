import { useEffect, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import type { AgentConfig } from '@/types'

// The prompt size the server estimates for a config being edited. Requests
// wait for a pause in typing, and the last answer stays while the next loads.
export function useTokenEstimate(config: AgentConfig | null) {
  const serialized = config ? JSON.stringify(config) : null
  const [settled, setSettled] = useState(serialized)
  useEffect(() => {
    const timer = window.setTimeout(() => setSettled(serialized), 500)
    return () => window.clearTimeout(timer)
  }, [serialized])

  const { data } = useQuery({
    queryKey: ['token-estimate', settled],
    queryFn: () => api.estimateTokens(JSON.parse(settled as string)),
    enabled: Boolean(settled),
    placeholderData: keepPreviousData,
    refetchOnWindowFocus: false,
    retry: false,
  })
  return config ? data : undefined
}
