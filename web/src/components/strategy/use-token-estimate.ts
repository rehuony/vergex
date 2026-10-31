import { useEffect, useState } from 'react'
import useSWR from 'swr'
import { api } from '@/api'
import type { StrategyConfig } from '@/types'

// The prompt size the server estimates for a config being edited. Requests
// wait for a pause in typing, and the last answer stays while the next loads.
export function useTokenEstimate(config: StrategyConfig | null) {
  const serialized = config ? JSON.stringify(config) : null
  const [settled, setSettled] = useState(serialized)
  useEffect(() => {
    const timer = window.setTimeout(() => setSettled(serialized), 500)
    return () => window.clearTimeout(timer)
  }, [serialized])

  const { data } = useSWR(
    settled ? ['strategy-estimate', settled] : null,
    ([, body]) => api.estimateStrategyTokens(JSON.parse(body)),
    {
      keepPreviousData: true,
      revalidateOnFocus: false,
      shouldRetryOnError: false,
    }
  )
  return config ? data : undefined
}
