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
        subscribeVisibleLogicalRangeChange: vi.fn(),
        setVisibleLogicalRange: vi.fn(),
        getVisibleLogicalRange: () => null,
        getVisibleRange: () => null,
      }),
      remove: vi.fn(),
    },
    candles: series(),
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

beforeEach(() => {
  vi.useFakeTimers()
  vi.stubGlobal(
    'EventSource',
    class {
      close() {}
    }
  )
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
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
