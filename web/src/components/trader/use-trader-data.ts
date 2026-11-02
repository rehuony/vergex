import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { pollEvery } from '@/utils/poll'

const POLL = {
  ...pollEvery(15000),
  refetchOnWindowFocus: false,
  staleTime: 10000,
}

// useTraderData reads what every section of the trader page shows. Account and
// positions reach the exchange, so they poll slowly and only while visible.
export function useTraderData(traderId: string) {
  const status = useQuery({
    queryKey: ['trader-status', traderId],
    queryFn: () => api.getStatus(traderId, true),
    ...POLL,
  })
  const account = useQuery({
    queryKey: ['trader-account', traderId],
    queryFn: () => api.getAccount(traderId, true),
    ...POLL,
  })
  const positions = useQuery({
    queryKey: ['trader-positions', traderId],
    queryFn: () => api.getPositions(traderId, true),
    ...POLL,
  })
  return { status, account, positions }
}
