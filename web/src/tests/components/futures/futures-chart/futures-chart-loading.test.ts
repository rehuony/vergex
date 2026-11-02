import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { WorkspaceChart } from '@/components/futures/futures-chart/futures-chart'

const series = () => ({
  setData: vi.fn(),
  update: vi.fn(),
  attachPrimitive: vi.fn(),
  applyOptions: vi.fn(),
  createPriceLine: vi.fn(),
  removePriceLine: vi.fn(),
})

// Where the test says the view is, and how many candles the chart last drew.
const view = vi.hoisted(() => ({
  range: null as { from: number; to: number } | null,
  moved: () => {},
  drawn: 0,
}))

vi.mock('@/components/market-chart/market-chart-surface', () => ({
  addProjectionSeries: () => series(),
  candleColors: () => ({}),
  colors: () => ({}),
  projectionColors: () => ({}),
  themeOptions: () => ({}),
  volumeColor: () => '',
  createMarketChart: () => ({
    chart: {
      timeScale: () => ({
        subscribeVisibleLogicalRangeChange: (moved: () => void) => {
          view.moved = moved
        },
        setVisibleLogicalRange: vi.fn(),
        getVisibleLogicalRange: () => view.range,
        getVisibleRange: () => null,
      }),
      remove: vi.fn(),
    },
    candles: {
      ...series(),
      setData: (rows: unknown[]) => {
        view.drawn = rows.length
      },
    },
    volume: series(),
  }),
}))
vi.mock('@/components/futures/futures-chart/futures-chart-surface', () => ({
  OverlayPrimitive: class {
    setShapes() {}
    update() {}
    set() {}
  },
  chartFrame: () => ({
    root: document.createElement('div'),
    host: document.createElement('div'),
  }),
}))

const candle = {
  time: 1000,
  open: '1',
  high: '2',
  low: '1',
  close: '2',
  volume: '3',
}

const feeds: { onmessage?: (event: { data: string }) => void }[] = []

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-06-01T12:30:00Z'))
  view.range = null
  feeds.length = 0
  vi.stubGlobal(
    'EventSource',
    class {
      onmessage?: (event: { data: string }) => void
      constructor() {
        feeds.push(this)
      }
      close() {}
    }
  )
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

const HOUR = 3600
const thisHour = () => Math.floor(Date.now() / 1000 / HOUR) * HOUR

// A market of hourly candles that started `bars` hours ago, and whose newest candle can be
// held back. It answers as a venue does: the newest `limit` candles before `to`.
function stubMarket(bars: number, newest = thisHour()) {
  const market = { newest, reads: [] as URLSearchParams[] }
  const oldest = thisHour() - (bars - 1) * HOUR
  vi.stubGlobal('fetch', async (path: string) => {
    if (!path.startsWith('/api/futures/candles'))
      return new Response(JSON.stringify({}))
    const query = new URLSearchParams(path.split('?')[1])
    market.reads.push(query)
    const end = Math.min(Number(query.get('to')) - 1, market.newest)
    const top = Math.floor(end / HOUR) * HOUR
    const candles = []
    for (let i = Number(query.get('limit')) - 1; i >= 0; i--) {
      const time = top - i * HOUR
      if (time >= oldest) candles.push({ ...candle, time })
    }
    return new Response(JSON.stringify({ candles }))
  })
  return market
}

async function openChart() {
  const chart = new WorkspaceChart({
    venue: 'binance',
    symbol: 'BTCUSDT',
    container: document.createElement('div'),
  })
  await chart.setContext('binance', 'BTCUSDT', '60')
  return chart
}

it('reads older candles only once the view is within a screenful of them', async () => {
  const market = stubMarket(5000)
  const chart = await openChart()
  expect(view.drawn).toBe(500)
  const reads = market.reads.length

  view.range = { from: 200, to: 328 }
  view.moved()
  await vi.advanceTimersByTimeAsync(0)
  expect(market.reads.length).toBe(reads)

  // Close enough for one page, which moves the view that far from the edge.
  view.range = { from: 100, to: 228 }
  view.moved()
  view.range = { from: 400, to: 528 }
  await vi.waitFor(() => expect(view.drawn).toBe(800))
  await vi.advanceTimersByTimeAsync(1000)
  expect(market.reads.length).toBe(reads + 1)
  chart.dispose()
})

it('keeps reading while the view is over empty space, down to the first candle', async () => {
  const market = stubMarket(6200)
  const chart = await openChart()

  // One move, and nothing touches the chart again.
  view.range = { from: -1000, to: 100 }
  view.moved()
  await vi.waitFor(() => expect(view.drawn).toBe(6200))
  await vi.advanceTimersByTimeAsync(1000)

  // That was the whole market: moving again asks for nothing.
  const reads = market.reads.length
  view.moved()
  await vi.advanceTimersByTimeAsync(1000)
  expect(market.reads.length).toBe(reads)
  chart.dispose()
})

it('fills a gap in the feed without losing the older candles it holds', async () => {
  const market = stubMarket(2000, thisHour() - 5 * HOUR)
  const chart = await openChart()
  view.range = { from: -1000, to: 100 }
  view.moved()
  await vi.waitFor(() => expect(view.drawn).toBe(1995))
  await vi.advanceTimersByTimeAsync(1000)

  // The feed comes back five candles later.
  market.newest = thisHour()
  feeds[feeds.length - 1].onmessage?.({
    data: JSON.stringify({
      type: 'candles',
      data: { candles: [{ ...candle, time: thisHour() }] },
    }),
  })
  await vi.waitFor(() => expect(view.drawn).toBe(2000))
  const read = market.reads[market.reads.length - 1]
  expect(Number(read.get('limit'))).toBeLessThan(10)
  chart.dispose()
})

it('keeps trying to read the candles, says so once, and stops when they come', async () => {
  let failures = 3
  const fetchMock = vi.fn(async (path: string) => {
    if (!path.startsWith('/api/futures/candles'))
      return new Response(JSON.stringify({}))
    if (failures-- > 0)
      return new Response(JSON.stringify({ error: 'Get "https://x": EOF' }), {
        status: 400,
      })
    return new Response(JSON.stringify({ candles: [candle] }))
  })
  vi.stubGlobal('fetch', fetchMock)
  const candleReads = () =>
    fetchMock.mock.calls.filter(([path]) =>
      path.startsWith('/api/futures/candles')
    ).length

  const onError = vi.fn()
  const available: boolean[] = []
  const chart = new WorkspaceChart({
    venue: 'binance',
    symbol: 'BTCUSDT',
    container: document.createElement('div'),
    onError,
    onAvailability: (ok) => available.push(ok),
  })
  await chart.setContext('binance', 'BTCUSDT', '60')
  expect(candleReads()).toBe(1)
  expect(onError).toHaveBeenCalledTimes(1)
  // What the exchange's error says is not for the reader.
  expect(onError.mock.calls[0][0]).not.toContain('https://')
  expect(available[available.length - 1]).toBe(false)

  // After 3 seconds, then 6, then 12.
  await vi.advanceTimersByTimeAsync(3000)
  expect(candleReads()).toBe(2)
  await vi.advanceTimersByTimeAsync(6000)
  expect(candleReads()).toBe(3)
  await vi.advanceTimersByTimeAsync(12000)
  expect(available[available.length - 1]).toBe(true)
  expect(onError).toHaveBeenCalledTimes(1)

  const reads = candleReads()
  await vi.advanceTimersByTimeAsync(60000)
  expect(candleReads()).toBe(reads)
  chart.dispose()
})

it('stops trying when the chart goes away', async () => {
  const fetchMock = vi.fn(
    async () => new Response(JSON.stringify({ error: 'down' }), { status: 400 })
  )
  vi.stubGlobal('fetch', fetchMock)
  const chart = new WorkspaceChart({
    venue: 'binance',
    symbol: 'BTCUSDT',
    container: document.createElement('div'),
  })
  await chart.setContext('binance', 'BTCUSDT', '60')
  chart.dispose()
  const calls = fetchMock.mock.calls.length
  await vi.advanceTimersByTimeAsync(60000)
  expect(fetchMock.mock.calls.length).toBe(calls)
})
