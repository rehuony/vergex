import { queryOptions } from '@tanstack/react-query'
import { api } from '@/api'
import type { ForecastInterval } from '@/api/forecast'

// What Forecast reads, in one place, because pages read it and ForecastFrame reads it
// ahead of them.

export const strategiesQuery = queryOptions({
  queryKey: ['forecast-strategies'],
  queryFn: () => api.getForecastStrategies(),
  refetchOnWindowFocus: false,
})

export const performanceQuery = (strategyId: string) =>
  queryOptions({
    queryKey: ['forecast-performance', strategyId],
    queryFn: () => api.getForecastPerformance(strategyId),
    refetchInterval: 60000,
  })

export const backtestsQuery = queryOptions({
  queryKey: ['forecast-backtests'],
  queryFn: () => api.getForecastBacktests(),
})

export const marketSettingsQuery = queryOptions({
  queryKey: ['market-data-settings'],
  queryFn: () => api.getMarketDataSettings(),
  refetchOnWindowFocus: false,
})

export const marketStatusQuery = queryOptions({
  queryKey: ['market-data-status'],
  queryFn: () => api.getMarketDataStatus(),
})

export const forecastSettingsQuery = queryOptions({
  queryKey: ['forecast-settings'],
  queryFn: () => api.getForecastSettings(),
  refetchOnWindowFocus: false,
})

export const detailQuery = (
  symbol: string,
  interval: ForecastInterval,
  withIndicators: boolean
) =>
  queryOptions({
    queryKey: ['forecast-detail', symbol, interval, withIndicators],
    queryFn: () => api.getForecastDetail(symbol, interval, withIndicators),
  })
