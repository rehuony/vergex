import useSWR from 'swr'
import { api } from '@/api'
import type { AccountInfo, Position, SystemStatus } from '@/types'

const POLL = {
  refreshInterval: 15000,
  revalidateOnFocus: false,
  dedupingInterval: 10000,
  shouldRetryOnError: false,
}

// useTraderData reads what every section of the trader page shows. Account and
// positions reach the exchange, so they poll slowly and only while visible.
export function useTraderData(traderId: string) {
  const status = useSWR<SystemStatus>(
    `status-${traderId}`,
    () => api.getStatus(traderId, true),
    POLL
  )
  const account = useSWR<AccountInfo>(
    `account-${traderId}`,
    () => api.getAccount(traderId, true),
    POLL
  )
  const positions = useSWR<Position[]>(
    `positions-${traderId}`,
    () => api.getPositions(traderId, true),
    POLL
  )
  return { status, account, positions }
}
