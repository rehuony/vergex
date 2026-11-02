import { queryOptions } from '@tanstack/react-query'
import { api } from '@/api'
import { pollEvery } from '@/utils/poll'

// What the Auto trading tabs read, with what they do with it kept in the components.
export const tradersQuery = queryOptions({
  queryKey: ['traders'],
  queryFn: () => api.getTraders(true),
  refetchInterval: 5000,
})

export const traderMetricsQuery = queryOptions({
  queryKey: ['trader-metrics'],
  queryFn: () => api.getTraderMetrics(true),
  ...pollEvery(30000),
})
