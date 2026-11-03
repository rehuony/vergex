import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { ForecastOverlay } from '@/api/forecast'
import type { FuturesChart } from '@/api/futures'
import {
  ForecastSwitch,
  FuturesForecast,
  forecastCoin,
  useForecastSwitch,
} from '@/components/futures/futures-forecast'

const mocks = vi.hoisted(() => ({
  overlay: vi.fn(),
  setForecast: vi.fn(),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@/api', () => ({ api: { getForecastOverlay: mocks.overlay } }))

const overlay: ForecastOverlay = {
  symbol: 'BTCUSDT',
  interval: '1h',
  direction: 'down',
  display_prob: 0.36,
  calibrated: false,
  open_time: 7_200_000,
  target_time: 7_200_000 + 4 * 3_600_000,
  close: 100,
  expected_move_pct: 1.2,
  horizon: 4,
  stats: {
    judged: 160,
    samples: 40,
    moments: 40,
    hits: 30,
    misses: 10,
    hit_rate: 0.75,
    chance_rate: 0.6,
    avg_return_after_fee_pct: 0.2,
  },
}
// What the chart is told to draw from the overlay above.
const band = {
  time: 7200,
  step: 3600,
  close: 100,
  expectedMovePct: 1.2,
  horizon: 4,
  prob: 0.36,
  direction: 'down',
}

interface ShownProps {
  venue?: string
  symbol?: string
  interval?: string
  candlesReady?: boolean
  comparing?: boolean
}

function Shown(props: ShownProps) {
  const [on, toggle] = useForecastSwitch()
  return (
    <>
      <ForecastSwitch on={on} onChange={toggle} />
      <FuturesForecast
        on={on}
        chart={{
          current: {
            setForecast: mocks.setForecast,
          } as unknown as FuturesChart,
        }}
        venue="binance"
        symbol="BTCUSDT"
        interval="1h"
        scene={1}
        candlesReady
        comparing={false}
        {...props}
      />
    </>
  )
}

const show = (props: ShownProps) =>
  render(
    <MemoryRouter>
      <QueryWrapper>
        <Shown {...props} />
      </QueryWrapper>
    </MemoryRouter>
  )

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  mocks.overlay.mockResolvedValue(overlay)
})
afterEach(cleanup)

const turnOn = () =>
  fireEvent.click(screen.getByRole('switch', { name: 'Forecast' }))

it('stays off until asked, then draws the forecast and remembers', async () => {
  show({})
  expect(mocks.overlay).not.toHaveBeenCalled()
  expect(mocks.setForecast).toHaveBeenLastCalledWith(
    'binance',
    'BTCUSDT',
    '1h',
    null
  )

  turnOn()
  await waitFor(() =>
    expect(mocks.setForecast).toHaveBeenLastCalledWith(
      'binance',
      'BTCUSDT',
      '1h',
      band
    )
  )
  expect(mocks.overlay).toHaveBeenCalledWith('BTCUSDT', '1h')
  expect(screen.getByRole('link', { name: /Score 36/ })).toHaveAttribute(
    'href',
    '/forecast?symbol=BTCUSDT&interval=1h'
  )
  expect(screen.getByRole('link')).toHaveTextContent(
    'Hit rate 75.0% of 40 · by chance 60.0%'
  )
  expect(screen.getByRole('link')).toHaveAttribute(
    'title',
    'Hit rate 75.0% of 40 · by chance 60.0%'
  )
  expect(localStorage.getItem('vergex.futures.forecast')).toBe('on')
})

it('gives no percentage for a handful of calls, on a page for placing trades', async () => {
  mocks.overlay.mockResolvedValue({
    ...overlay,
    stats: {
      ...overlay.stats,
      samples: 4,
      moments: 4,
      hits: 4,
      misses: 0,
      hit_rate: 1,
    },
  })
  localStorage.setItem('vergex.futures.forecast', 'on')
  show({})
  const line = await screen.findByRole('link')
  expect(line).toHaveTextContent('4 calls, too few to read')
  expect(line.textContent).not.toContain('100.0%')
})

it('says nothing of a record when no call has been judged', async () => {
  mocks.overlay.mockResolvedValue({
    ...overlay,
    stats: {
      ...overlay.stats,
      samples: 0,
      moments: 0,
      hits: 0,
      misses: 0,
      hit_rate: null,
    },
  })
  localStorage.setItem('vergex.futures.forecast', 'on')
  show({})
  const line = await screen.findByRole('link')
  expect(line.textContent).not.toContain('calls')
  expect(line.textContent).not.toContain('Hit rate')
})

it('reads the Binance coin behind an OKX swap and says where it comes from', async () => {
  localStorage.setItem('vergex.futures.forecast', 'on')
  show({ venue: 'okx', symbol: 'BTC-USDT-SWAP' })
  await waitFor(() =>
    expect(mocks.overlay).toHaveBeenCalledWith('BTCUSDT', '1h')
  )
  expect(await screen.findByRole('link')).toHaveTextContent('Binance data')
  expect(mocks.setForecast).toHaveBeenLastCalledWith(
    'okx',
    'BTC-USDT-SWAP',
    '1h',
    band
  )
})

it('says why there is nothing to overlay, without asking the server', () => {
  localStorage.setItem('vergex.futures.forecast', 'on')
  const { unmount } = show({ interval: '5m' })
  expect(screen.getByText('Forecast covers 15m and longer')).toBeVisible()
  unmount()

  show({ venue: 'okx', symbol: 'BTC-USD-SWAP' })
  expect(screen.getByText('No forecast for this market')).toBeVisible()
  expect(mocks.overlay).not.toHaveBeenCalled()
})

it('does not ask for the forecast or draw it before the real candles are on the chart', async () => {
  localStorage.setItem('vergex.futures.forecast', 'on')
  const candlesReady = (ready: boolean) => (
    <MemoryRouter>
      <QueryWrapper>
        <Shown candlesReady={ready} />
      </QueryWrapper>
    </MemoryRouter>
  )
  const { rerender } = render(candlesReady(false))
  expect(mocks.overlay).not.toHaveBeenCalled()
  expect(mocks.setForecast).toHaveBeenLastCalledWith(
    'binance',
    'BTCUSDT',
    '1h',
    null
  )
  expect(screen.queryByRole('status')).toBeNull()
  rerender(candlesReady(true))
  await waitFor(() =>
    expect(mocks.setForecast).toHaveBeenLastCalledWith(
      'binance',
      'BTCUSDT',
      '1h',
      band
    )
  )
  expect(mocks.overlay).toHaveBeenCalledTimes(1)
})

it('passes on why a coin has no forecast yet', async () => {
  localStorage.setItem('vergex.futures.forecast', 'on')
  mocks.overlay.mockResolvedValue({
    ...overlay,
    pending: 'not_enough_history',
  })
  show({})
  expect(await screen.findByText('Not enough history')).toBeVisible()
})

it('clears the chart while plans are being compared', async () => {
  localStorage.setItem('vergex.futures.forecast', 'on')
  show({ comparing: true })
  expect(mocks.overlay).not.toHaveBeenCalled()
  expect(mocks.setForecast).toHaveBeenLastCalledWith(
    'binance',
    'BTCUSDT',
    '1h',
    null
  )
})

it('maps only USDT swaps to a coin', () => {
  expect(forecastCoin('binance', 'ETHUSDT')).toBe('ETHUSDT')
  expect(forecastCoin('okx', 'ETH-USDT-SWAP')).toBe('ETHUSDT')
  expect(forecastCoin('okx', 'ETH-USDT')).toBeNull()
})
