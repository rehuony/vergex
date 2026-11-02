import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type {
  ForecastCoin,
  ForecastDetail,
  ForecastSignal,
} from '@/api/forecast'
import { sortCoins } from '@/components/forecast/coin-list'
import { ForecastPage } from '@/pages/forecast-page'

const mocks = vi.hoisted(() => ({
  coins: vi.fn(),
  detail: vi.fn(),
  strategies: vi.fn(),
  activate: vi.fn(),
  market: vi.fn(),
  saveMarket: vi.fn(),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@/api', () => ({
  api: {
    getForecastCoins: mocks.coins,
    getForecastDetail: mocks.detail,
    getForecastStrategies: mocks.strategies,
    activateForecastStrategy: mocks.activate,
    getMarketDataSettings: mocks.market,
    saveMarketDataSettings: mocks.saveMarket,
  },
}))
vi.mock('@/components/common/notify', () => ({
  notify: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}))
vi.mock('@/components/market-chart/local-market-chart-view', () => ({
  LocalMarketChartView: ({ label }: { label: string }) => (
    <div role="img" aria-label={label} />
  ),
}))

const signal = (patch: Partial<ForecastSignal>) =>
  ({
    id: 1,
    display_prob: 0.5,
    calibrated: false,
    direction: 'neutral',
    confidence: 10,
    expected_move_pct: 1,
    regime: 'ranging',
    horizon: 4,
    interval: '1h',
    open_time: 1,
    support: 76000,
    resistance: 78000,
    ...patch,
  }) as ForecastSignal

const coin = (
  symbol: string,
  patch: Partial<ForecastCoin> = {}
): ForecastCoin => ({
  symbol,
  interval: '1h',
  tier: 1,
  price: 100,
  change_24h_pct: null,
  signal: null,
  stale: false,
  ...patch,
})

const detail = (patch: Partial<ForecastDetail> = {}) =>
  ({
    symbol: 'BTCUSDT',
    interval: '1h',
    price: 77000,
    ready: true,
    stale: false,
    result: {
      close: 77000,
      direction: 'up',
      confidence: 41,
      expected_move_pct: 1.2,
      support: 76000,
      resistance: 78000,
      regime: 'trending',
      horizon: 4,
      adx: 20,
      has_derivatives: true,
      derivatives: { funding: null, oi_change_pct: null, long_short_z: null },
      factors: [],
      missing: [],
    },
    latest: signal({ display_prob: 0.68, direction: 'up' }),
    signals: [],
    stats: {
      judged: 12,
      samples: 4,
      moments: 4,
      hits: 3,
      misses: 1,
      hit_rate: 0.75,
      chance_rate: 0.6,
      avg_return_after_fee_pct: 0.2,
    },
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

const show = (entry = '/forecast?symbol=BTCUSDT&interval=1h') =>
  render(
    <MemoryRouter initialEntries={[entry]}>
      <QueryWrapper>
        <Where />
        <Routes>
          <Route path="/forecast" element={<ForecastPage />} />
        </Routes>
      </QueryWrapper>
    </MemoryRouter>
  )

const coinsOf = (coins: ForecastCoin[]) => ({
  strategy_id: 'strategy-1',
  interval: '1h',
  coins,
})

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  mocks.market.mockResolvedValue({
    enabled: true,
    watchlist: ['ETHUSDT'],
    top_n: 200,
  })
  mocks.saveMarket.mockImplementation(async (settings) => settings)
  mocks.coins.mockResolvedValue(coinsOf([]))
  mocks.detail.mockResolvedValue(detail())
  mocks.strategies.mockResolvedValue({
    active_strategy_id: 'strategy-1',
    strategies: [
      { id: 'strategy-1', name: 'Default' },
      { id: 'strategy-2', name: 'Steeper' },
    ],
  })
})
afterEach(cleanup)

it('puts strong calls first in either direction and waiting coins last', () => {
  const sorted = sortCoins([
    coin('WAITUSDT', { pending: 'waiting' }),
    coin('WEAKUSDT', { signal: signal({ display_prob: 0.52 }) }),
    coin('DOWNUSDT', { signal: signal({ display_prob: 0.2 }) }),
    coin('UPUSDT', { signal: signal({ display_prob: 0.7 }) }),
  ])
  expect(sorted.map((row) => row.symbol)).toEqual([
    'DOWNUSDT',
    'UPUSDT',
    'WEAKUSDT',
    'WAITUSDT',
  ])
})

it('shows the call, and the hit rate against what chance would hit', async () => {
  show()
  expect(await screen.findByText('77,000')).toBeVisible()
  expect(screen.getByText('Score 68')).toBeVisible()
  // Four calls are too few to read, so they stay grey.
  expect(screen.getByText('75.0%').className).toContain('text-vergex-muted')
  expect(screen.getByText('60.0%')).toBeVisible()
  expect(
    screen.getByRole('img', { name: 'BTCUSDT 1h candles' })
  ).toBeInTheDocument()
})

it('shows a score until a forecast is calibrated, then a probability', async () => {
  mocks.coins.mockResolvedValue(
    coinsOf([
      coin('BTCUSDT', {
        signal: signal({ display_prob: 0.68, direction: 'up' }),
      }),
      coin('ETHUSDT', {
        signal: signal({ display_prob: 0.34, calibrated: true }),
      }),
    ])
  )
  show()
  const list = (await screen.findByRole('table')) as HTMLElement
  expect(within(list).getByText('68')).toBeVisible()
  expect(within(list).getByText('34%')).toBeVisible()
})

it('adds a starred coin to the watchlist and keeps the rest of the settings', async () => {
  show()
  await screen.findByText('Score 68')
  const star = await screen.findByRole('button', {
    name: 'Add BTC to the watchlist',
  })
  await waitFor(() => expect(mocks.market).toHaveBeenCalled())
  await waitFor(() => {
    fireEvent.click(star)
    expect(mocks.saveMarket).toHaveBeenCalledWith({
      enabled: true,
      watchlist: ['ETHUSDT', 'BTCUSDT'],
      top_n: 200,
    })
  })
})

it('shows what the factors read, and those with no reading', async () => {
  mocks.detail.mockResolvedValue(
    detail({
      result: {
        ...detail().result!,
        adx: 31,
        derivatives: {
          funding: 0.0001,
          oi_change_pct: 2.5,
          long_short_z: null,
        },
        factors: [
          { name: 'trend', value: 0.4, weight: 1.5, contribution: 0.6 },
        ],
        missing: ['long_short'],
      },
    })
  )
  show()
  expect(await screen.findByText('0.0100%')).toBeVisible()
  expect(screen.getByText('+2.50%')).toBeVisible()
  expect(screen.getByText('Long/short ratio')).toBeVisible()
  expect(screen.getAllByText('No reading').length).toBeGreaterThan(0)
})

it('chooses the strategy in use from a list that leaves out retired ones', async () => {
  mocks.strategies.mockResolvedValue({
    active_strategy_id: 'strategy-1',
    strategies: [
      { id: 'strategy-1', name: 'Default', created_at: 1, retired: false },
      { id: 'strategy-2', name: 'Steeper', created_at: 1, retired: false },
      { id: 'strategy-0', name: 'Old', created_at: 1, retired: true },
    ],
  })
  mocks.activate.mockResolvedValue(undefined)
  show()
  fireEvent.click(await screen.findByRole('button', { name: /Strategy/ }))
  expect(await screen.findByRole('button', { name: /Steeper/ })).toBeVisible()
  expect(screen.queryByRole('button', { name: /Old/ })).toBeNull()
  expect(
    screen.getByRole('link', { name: 'Manage strategies' })
  ).toHaveAttribute('href', '/forecast/strategies')
  fireEvent.click(screen.getByRole('button', { name: /Steeper/ }))
  await waitFor(() => expect(mocks.activate).toHaveBeenCalledWith('strategy-2'))
})

it('moves through the coin list with the keyboard', async () => {
  mocks.coins.mockResolvedValue(
    coinsOf([coin('BTCUSDT'), coin('ETHUSDT'), coin('SOLUSDT')])
  )
  show()
  await screen.findByRole('table')
  const search = screen.getByLabelText('Search coins')
  fireEvent.keyDown(search, { key: 'ArrowDown' })
  fireEvent.keyDown(search, { key: 'Enter' })
  expect(screen.getByLabelText('where')).toHaveTextContent(
    '/forecast?symbol=ETHUSDT'
  )

  fireEvent.change(search, { target: { value: 'sol' } })
  fireEvent.keyDown(search, { key: 'Escape' })
  expect(search).toHaveValue('')
})

it('remembers the interval for the next visit', async () => {
  show('/forecast?symbol=BTCUSDT&interval=4h')
  await waitFor(() =>
    expect(localStorage.getItem('vergex.forecast.interval')).toBe('4h')
  )
  cleanup()
  show('/forecast?symbol=BTCUSDT')
  await waitFor(() => expect(mocks.coins).toHaveBeenLastCalledWith('4h'))
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
  mocks.coins.mockResolvedValue(coinsOf([coin('BTCUSDT')]))
  show()
  await screen.findByRole('table')
  const search = screen.getByLabelText('Search coins')
  fireEvent.change(search, { target: { value: 'new' } })
  expect(screen.getByText('Press Enter to fetch NEWUSDT')).toBeVisible()
  fireEvent.keyDown(search, { key: 'Enter' })
  expect(screen.getByLabelText('where')).toHaveTextContent(
    '/forecast?symbol=NEWUSDT&interval=1h'
  )
})

it('asks for another interval and keeps it in the address', async () => {
  show()
  await screen.findByText('Score 68')
  fireEvent.click(screen.getByRole('button', { name: '4h' }))
  await waitFor(() =>
    expect(mocks.detail).toHaveBeenLastCalledWith('BTCUSDT', '4h', false)
  )
  expect(mocks.coins).toHaveBeenLastCalledWith('4h')
  expect(screen.getByLabelText('where')).toHaveTextContent(
    '/forecast?symbol=BTCUSDT&interval=4h'
  )
})

it('opens the last coin looked at, or else the strongest call', async () => {
  mocks.coins.mockResolvedValue(
    coinsOf([
      coin('WAITUSDT'),
      coin('ETHUSDT', { signal: signal({ display_prob: 0.7 }) }),
    ])
  )
  const { unmount } = show('/forecast')
  await waitFor(() =>
    expect(mocks.detail).toHaveBeenLastCalledWith('ETHUSDT', '1h', false)
  )
  expect(screen.getByLabelText('where')).toHaveTextContent(/^\/forecast$/)
  unmount()

  vi.clearAllMocks()
  mocks.coins.mockResolvedValue(coinsOf([coin('BTCUSDT')]))
  show('/forecast')
  await waitFor(() =>
    expect(mocks.detail).toHaveBeenLastCalledWith('ETHUSDT', '1h', false)
  )
})

it('says so when no strategy is in use, and reads no coin', async () => {
  mocks.coins.mockResolvedValue({
    strategy_id: '',
    interval: '1h',
    coins: [],
  })
  show()
  expect(await screen.findByText(/No strategy is in use/)).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'Choose a strategy' })
  ).toHaveAttribute('href', '/forecast/strategies')
})

it('says so when no coins are tracked', async () => {
  show('/forecast')
  expect(await screen.findByText('No coins are being tracked')).toBeVisible()
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

it('leaves the move of a call not yet judged blank, and readings the coin has none of', async () => {
  mocks.detail.mockResolvedValue(
    detail({
      signals: [
        signal({ id: 1, status: 'open' }),
        signal({
          id: 2,
          open_time: 3600000,
          status: 'verified',
          outcome: 'hit',
          actual_move_pct: 1.5,
        }),
      ],
    })
  )
  show()
  const open = await screen.findByText('Open')
  expect(open.closest('tr')).toHaveTextContent('—')
  expect(open.closest('tr')).not.toHaveTextContent('0.00%')
  expect(screen.getByText('+1.50%')).toBeVisible()
  expect(screen.getByText('Funding').parentElement).toHaveTextContent('—')
})
