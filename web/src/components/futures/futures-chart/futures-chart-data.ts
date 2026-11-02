import { volumeColor } from '@/components/market-chart/market-chart-surface'
import type { UTCTimestamp } from 'lightweight-charts'

// Candle data for the Futures chart: timeframes, the market query, candle
// history pages and the conversions onto the chart's bar index and series.

export const resolutions: Record<string, string> = {
  '1': '1m',
  '5': '5m',
  '15': '15m',
  '60': '1h',
  '240': '4h',
  '1D': '1d',
}
export const resolutionSeconds: Record<string, number> = {
  '1': 60,
  '5': 300,
  '15': 900,
  '60': 3600,
  '240': 14400,
  '1D': 86400,
}

export interface CandleRow {
  time: number
  open: string
  high: string
  low: string
  close: string
  volume: string
}
export interface CandleResponse {
  candles?: CandleRow[]
  stale?: boolean
  mode?: string
}

export function instrument(name: string) {
  const at = name.indexOf(':'),
    venue = name.slice(0, at),
    symbol = name.slice(at + 1)
  if (
    !venue ||
    !symbol ||
    !/^[A-Z0-9\p{Script=Han}-]{1,40}$/u.test(symbol) ||
    !['binance', 'okx'].includes(venue.toLowerCase())
  )
    throw new Error('Invalid symbol format')
  return { venue: venue.toLowerCase(), symbol }
}
export const ticker = (venue: string, symbol: string) =>
  venue.toUpperCase() + ':' + symbol

// chartQuery builds the candle/feed query for "VENUE:SYMBOL". Invalid venues
// and path-like symbols throw before any request is made.
export function chartQuery(
  name: string,
  resolution: string,
  extra: Record<string, string> = {}
) {
  const { venue, symbol } = instrument(name)
  return new URLSearchParams({
    venue,
    symbol,
    interval: resolutions[resolution],
    ...extra,
  }).toString()
}

// logicalAt maps a unix time onto the bar index scale. Times between bars are
// interpolated; times outside the loaded data extrapolate by the bar step so
// drawings that start in history or end in the future keep their geometry.
export function logicalAt(times: number[], step: number, time: number) {
  const n = times.length
  if (!n) return null
  if (time <= times[0]) return (time - times[0]) / step
  if (time >= times[n - 1]) return n - 1 + (time - times[n - 1]) / step
  let lo = 0,
    hi = n - 1
  while (hi - lo > 1) {
    const mid = (lo + hi) >> 1
    if (times[mid] <= time) lo = mid
    else hi = mid
  }
  return lo + (time - times[lo]) / (times[hi] - times[lo])
}

export const toBar = (c: CandleRow) => ({
  time: c.time as UTCTimestamp,
  open: Number(c.open),
  high: Number(c.high),
  low: Number(c.low),
  close: Number(c.close),
})
export const toVolume = (c: CandleRow) => ({
  time: c.time as UTCTimestamp,
  value: Number(c.volume),
  color: volumeColor(Number(c.close) >= Number(c.open)),
})

export async function fetchJSON<T>(path: string): Promise<T> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), 20000)
  try {
    const response = await fetch(path, {
      credentials: 'same-origin',
      signal: controller.signal,
    })
    const data = await response.json()
    if (!response.ok)
      throw new Error(data.error || 'Market data request failed')
    return data as T
  } catch (e) {
    if (controller.signal.aborted)
      throw new Error(
        'Market chart timed out. Check the connection and try again.'
      )
    throw e
  } finally {
    clearTimeout(timer)
  }
}

// fetchHistory reads up to count candles before `to`. The candle endpoint
// caps pages at 300; older history spans several pages.
export async function fetchHistory(
  query: (extra: Record<string, string>) => string,
  to: number,
  count: number
) {
  const rows = new Map<number, CandleRow>()
  let cursor = Math.min(to, Math.floor(Date.now() / 1000) + 1)
  let exhausted = false
  let metadata: CandleResponse = {}
  for (let page = 0; page < 16 && rows.size < count; page++) {
    const data = await fetchJSON<CandleResponse>(
      '/api/futures/candles?' +
        query({
          limit: String(Math.min(300, count - rows.size)),
          to: String(cursor),
        })
    )
    if (!page) metadata = data
    const batch = (data.candles || [])
      .filter((r) => r.time < cursor)
      .sort((a, b) => a.time - b.time)
    if (!batch.length) {
      exhausted = true
      break
    }
    for (const row of batch) rows.set(row.time, row)
    cursor = batch[0].time
  }
  return {
    ...metadata,
    candles: [...rows.values()].sort((a, b) => a.time - b.time),
    exhausted,
  }
}
