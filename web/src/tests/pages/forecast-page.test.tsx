import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { SWRConfig } from 'swr'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type {
  ForecastDetail,
  ForecastRecord,
  ForecastSignal,
} from '@/api/forecast'
import { sortSignals } from '@/components/forecast/coin-list'
import { ForecastPage } from '@/pages/forecast-page'

const mocks = vi.hoisted(() => ({
  signals: vi.fn(),
  detail: vi.fn(),
  strategies: vi.fn(),
  activate: vi.fn(),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@/api', () => ({
  api: {
    getForecastSignals: mocks.signals,
    getForecastDetail: mocks.detail,
    getForecastStrategies: mocks.strategies,
    activateForecastStrategy: mocks.activate,
  },
}))
vi.mock('@/components/common/notify', () => ({
  notify: { success: vi.fn(), error: vi.fn() },
}))
vi.mock('@/components/market-chart/local-market-chart-view', () => ({
  LocalMarketChartView: ({ label }: { label: string }) => (
    <div role="img" aria-label={label} />
  ),
}))

const record = (patch: Partial<ForecastRecord>) =>
  ({
    id: 1,
    display_prob: 0.5,
    calibrated: false,
    direction: 'neutral',
    confidence: 10,
    expected_move_pct: 1,
    regime: 'ranging',
    open_time: 1,
    ...patch,
  }) as ForecastRecord

const signal = (
  symbol: string,
  patch: Partial<ForecastSignal> = {}
): ForecastSignal => ({
  symbol,
  interval: '1h',
  tier: 1,
  price: 100,
  record: null,
  stale: false,
  ...patch,
})

const detail = (patch: Partial<ForecastDetail> = {}) =>
  ({
    symbol: 'BTCUSDT',
    interval: '1h',
    price: 77000,
    ready: true,
    result: {
      close: 77000,
      direction: 'up',
      confidence: 41,
      expected_move_pct: 1.2,
      support: 76000,
      resistance: 78000,
      regime: 'trending',
      horizon: 4,
      has_derivatives: true,
      factors: [],
    },
    latest: record({ display_prob: 0.68, direction: 'up' }),
    records: [],
    stats: { hits: 3, misses: 1, moved: 10, rose: 6 },
    candles: [{ time: 1, open: 1, high: 1, low: 1, close: 1, volume: 1 }],
    last_closed: 1000,
    forming: null,
    projection: [],
    ...patch,
  }) as ForecastDetail

function Where() {
  const location = useLocation()
  return (
    <output aria-label="where">{location.pathname + location.search}</output>
  )
}

const show = (entry = '/forecast/BTCUSDT?interval=1h') =>
  render(
    <MemoryRouter initialEntries={[entry]}>
      <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
        <Where />
        <Routes>
          <Route path="/forecast" element={<ForecastPage />} />
          <Route path="/forecast/:symbol" element={<ForecastPage />} />
        </Routes>
      </SWRConfig>
    </MemoryRouter>
  )

const signalsOf = (signals: ForecastSignal[]) => ({
  strategy_id: 1,
  interval: '1h',
  signals,
})

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  mocks.signals.mockResolvedValue(signalsOf([]))
  mocks.detail.mockResolvedValue(detail())
  mocks.strategies.mockResolvedValue({
    active_strategy_id: 1,
    strategies: [
      { id: 1, name: 'Default' },
      { id: 2, name: 'Steeper' },
    ],
  })
})
afterEach(cleanup)

it('puts strong calls first in either direction and waiting coins last', () => {
  const sorted = sortSignals([
    signal('WAITUSDT', { pending: 'waiting' }),
    signal('WEAKUSDT', { record: record({ display_prob: 0.52 }) }),
    signal('DOWNUSDT', { record: record({ display_prob: 0.2 }) }),
    signal('UPUSDT', { record: record({ display_prob: 0.7 }) }),
  ])
  expect(sorted.map((row) => row.symbol)).toEqual([
    'DOWNUSDT',
    'UPUSDT',
    'WEAKUSDT',
    'WAITUSDT',
  ])
})

it('shows the call, and the hit rate against how often price rose anyway', async () => {
  show()
  expect(await screen.findByText('77,000')).toBeVisible()
  expect(screen.getByText('Score 68')).toBeVisible()
  expect(screen.getByText('75.0%')).toBeVisible()
  expect(screen.getByText('60.0%')).toBeVisible()
  expect(
    screen.getByRole('img', { name: 'BTCUSDT 1h candles' })
  ).toBeInTheDocument()
})

it('shows a score until a forecast is calibrated, then a probability', async () => {
  mocks.signals.mockResolvedValue(
    signalsOf([
      signal('BTCUSDT', {
        record: record({ display_prob: 0.68, direction: 'up' }),
      }),
      signal('ETHUSDT', {
        record: record({ display_prob: 0.34, calibrated: true }),
      }),
    ])
  )
  show()
  const list = (await screen.findByRole('table')) as HTMLElement
  expect(within(list).getByText('68')).toBeVisible()
  expect(within(list).getByText('34%')).toBeVisible()
})

it('says why a coin has no forecast yet', async () => {
  mocks.detail.mockResolvedValue(
    detail({ result: null, latest: null, pending: 'not_enough_history' })
  )
  show()
  expect(
    (await screen.findAllByText('Not enough history')).length
  ).toBeGreaterThan(0)
})

it('opens a coin that is not tracked when asked by name', async () => {
  mocks.signals.mockResolvedValue(signalsOf([signal('BTCUSDT')]))
  show()
  await screen.findByRole('table')
  const search = screen.getByLabelText('Search coins')
  fireEvent.change(search, { target: { value: 'new' } })
  expect(screen.getByText('Press Enter to fetch NEWUSDT.')).toBeVisible()
  fireEvent.keyDown(search, { key: 'Enter' })
  expect(screen.getByLabelText('where')).toHaveTextContent(
    '/forecast/NEWUSDT?interval=1h'
  )
})

it('asks for another interval and keeps it in the address', async () => {
  show()
  await screen.findByText('Score 68')
  fireEvent.click(screen.getByRole('button', { name: '4h' }))
  await waitFor(() =>
    expect(mocks.detail).toHaveBeenLastCalledWith('BTCUSDT', '4h', false)
  )
  expect(mocks.signals).toHaveBeenLastCalledWith('4h')
  expect(screen.getByLabelText('where')).toHaveTextContent('interval=4h')
})

it('opens the last coin looked at, or else the strongest call', async () => {
  mocks.signals.mockResolvedValue(
    signalsOf([
      signal('WAITUSDT'),
      signal('ETHUSDT', { record: record({ display_prob: 0.7 }) }),
    ])
  )
  const { unmount } = show('/forecast')
  await waitFor(() =>
    expect(screen.getByLabelText('where')).toHaveTextContent(
      '/forecast/ETHUSDT'
    )
  )
  unmount()

  show('/forecast')
  await waitFor(() =>
    expect(screen.getByLabelText('where')).toHaveTextContent(
      '/forecast/ETHUSDT'
    )
  )
})

it('says so when no coins are tracked', async () => {
  show('/forecast')
  expect(await screen.findByText('No coins are being tracked.')).toBeVisible()
  expect(screen.getByRole('link', { name: 'Choose coins' })).toHaveAttribute(
    'href',
    '/forecast/settings'
  )
  expect(screen.queryByText('Search coins')).toBeNull()
})

it('asks only for what the chart will draw', async () => {
  show()
  await screen.findByText('Score 68')
  expect(mocks.detail).toHaveBeenCalledWith('BTCUSDT', '1h', false)

  fireEvent.click(screen.getByRole('button', { name: 'Chart' }))
  fireEvent.click(await screen.findByRole('switch', { name: 'RSI 14' }))
  await waitFor(() =>
    expect(mocks.detail).toHaveBeenLastCalledWith('BTCUSDT', '1h', true)
  )
  expect(JSON.parse(localStorage.getItem('vergex.forecast.chart')!).rsi).toBe(
    true
  )
})
