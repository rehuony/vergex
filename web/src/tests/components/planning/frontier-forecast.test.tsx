import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { SWRConfig } from 'swr'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { ForecastOverlay } from '@/api/forecast'
import type { PlanningChart } from '@/api/planning'
import {
  FrontierForecast,
  forecastCoin,
} from '@/components/planning/frontier-forecast'

const mocks = vi.hoisted(() => ({
  overlay: vi.fn(),
  setForecast: vi.fn(),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@/api', () => ({ api: { getForecastOverlay: mocks.overlay } }))

const rows = [{ time: 2, open: 1, high: 2, low: 1, close: 2, volume: 0 }]
const overlay: ForecastOverlay = {
  symbol: 'BTCUSDT',
  interval: '1h',
  direction: 'down',
  display_prob: 0.36,
  calibrated: false,
  stats: { hits: 3, misses: 1, moved: 10, rose: 6 },
  projection: rows,
}

const show = (props: {
  venue?: string
  symbol?: string
  interval?: string
  comparing?: boolean
}) =>
  render(
    <MemoryRouter>
      <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
        <FrontierForecast
          chart={{
            current: {
              setForecast: mocks.setForecast,
            } as unknown as PlanningChart,
          }}
          venue="binance"
          symbol="BTCUSDT"
          interval="1h"
          scene={1}
          comparing={false}
          hovering={false}
          {...props}
        />
      </SWRConfig>
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
    []
  )

  turnOn()
  await waitFor(() =>
    expect(mocks.setForecast).toHaveBeenLastCalledWith(
      'binance',
      'BTCUSDT',
      '1h',
      rows
    )
  )
  expect(mocks.overlay).toHaveBeenCalledWith('BTCUSDT', '1h')
  expect(screen.getByRole('link', { name: /Score 36/ })).toHaveAttribute(
    'href',
    '/forecast/BTCUSDT?interval=1h'
  )
  expect(screen.getByText(/Hit rate 75\.0%/)).toBeVisible()
  expect(screen.getByText(/rose anyway 60\.0%/)).toBeVisible()
  expect(screen.getByText('Simulated candles are not prices.')).toBeVisible()
  expect(localStorage.getItem('vergex.frontier.forecast')).toBe('on')
})

it('reads the Binance coin behind an OKX swap and says where it comes from', async () => {
  localStorage.setItem('vergex.frontier.forecast', 'on')
  show({ venue: 'okx', symbol: 'BTC-USDT-SWAP' })
  await waitFor(() =>
    expect(mocks.overlay).toHaveBeenCalledWith('BTCUSDT', '1h')
  )
  expect(
    await screen.findByText(
      'Simulated candles are not prices. From Binance data.'
    )
  ).toBeVisible()
  expect(mocks.setForecast).toHaveBeenLastCalledWith(
    'okx',
    'BTC-USDT-SWAP',
    '1h',
    rows
  )
})

it('says why there is nothing to overlay, without asking the server', () => {
  localStorage.setItem('vergex.frontier.forecast', 'on')
  const { unmount } = show({ interval: '5m' })
  expect(screen.getByText('Forecast covers 15m and longer.')).toBeVisible()
  unmount()

  show({ venue: 'okx', symbol: 'BTC-USD-SWAP' })
  expect(screen.getByText('No forecast for this market.')).toBeVisible()
  expect(mocks.overlay).not.toHaveBeenCalled()
})

it('passes on why a coin has no forecast yet', async () => {
  localStorage.setItem('vergex.frontier.forecast', 'on')
  mocks.overlay.mockResolvedValue({
    ...overlay,
    pending: 'not_enough_history',
    projection: [],
  })
  show({})
  expect(await screen.findByText('Not enough history')).toBeVisible()
})

it('clears the chart while plans are being compared', async () => {
  localStorage.setItem('vergex.frontier.forecast', 'on')
  show({ comparing: true })
  expect(mocks.overlay).not.toHaveBeenCalled()
  expect(mocks.setForecast).toHaveBeenLastCalledWith(
    'binance',
    'BTCUSDT',
    '1h',
    []
  )
})

it('maps only USDT swaps to a coin', () => {
  expect(forecastCoin('binance', 'ETHUSDT')).toBe('ETHUSDT')
  expect(forecastCoin('okx', 'ETH-USDT-SWAP')).toBe('ETHUSDT')
  expect(forecastCoin('okx', 'ETH-USDT')).toBeNull()
})
