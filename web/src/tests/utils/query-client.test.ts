import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import {
  clearQueryCache,
  queryClient,
  restoreQueryCache,
} from '@/utils/query-client'

const STORAGE_KEY = 'vergex.query-cache'

const saved = () => JSON.parse(localStorage.getItem(STORAGE_KEY) ?? 'null')
const savedKeys = () =>
  saved()?.state.queries.map((query: { queryKey: unknown[] }) => query.queryKey)

beforeEach(() => {
  vi.useFakeTimers()
  queryClient.clear()
  localStorage.clear()
})
afterEach(() => {
  vi.useRealTimers()
})

it('keeps only what Forecast opens on, and gives it back after a reload', () => {
  restoreQueryCache()
  queryClient.setQueryData(['forecast-strategies'], { strategies: [] })
  queryClient.setQueryData(['accounts'], [{ id: 'secret' }])
  queryClient.setQueryData(['forecast-detail', 'BTCUSDT', '1h', false], {})
  vi.advanceTimersByTime(2_000)
  expect(savedKeys()).toEqual([['forecast-strategies']])

  queryClient.clear()
  restoreQueryCache()
  expect(queryClient.getQueryData(['forecast-strategies'])).toEqual({
    strategies: [],
  })
  expect(queryClient.getQueryData(['accounts'])).toBeUndefined()
})

it('ignores what another build or a long time ago saved', () => {
  restoreQueryCache()
  queryClient.setQueryData(['forecast-strategies'], { strategies: [] })
  vi.advanceTimersByTime(2_000)
  const fresh = saved()

  queryClient.clear()
  localStorage.setItem(
    STORAGE_KEY,
    JSON.stringify({ ...fresh, build: 'another build' })
  )
  restoreQueryCache()
  expect(queryClient.getQueryData(['forecast-strategies'])).toBeUndefined()

  localStorage.setItem(
    STORAGE_KEY,
    JSON.stringify({ ...fresh, at: fresh.at - 25 * 60 * 60_000 })
  )
  restoreQueryCache()
  expect(queryClient.getQueryData(['forecast-strategies'])).toBeUndefined()
})

it('forgets everything on signing out', () => {
  restoreQueryCache()
  queryClient.setQueryData(['forecast-strategies'], { strategies: [] })
  vi.advanceTimersByTime(2_000)
  clearQueryCache()
  expect(queryClient.getQueryData(['forecast-strategies'])).toBeUndefined()
  expect(localStorage.getItem(STORAGE_KEY)).toBeNull()
})
