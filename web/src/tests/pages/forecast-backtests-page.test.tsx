import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { BacktestMetrics, BacktestRun } from '@/api/forecast'
import { ForecastBacktestsPage } from '@/pages/forecast-backtests-page'

const mocks = vi.hoisted(() => ({
  strategies: vi.fn(),
  runs: vi.fn(),
  run: vi.fn(),
  start: vi.fn(),
  cancel: vi.fn(),
  rename: vi.fn(),
  remove: vi.fn(),
  confirm: vi.fn(),
  settings: vi.fn(),
  status: vi.fn(),
  coverage: vi.fn(),
  backfill: vi.fn(),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@/api', () => ({
  api: {
    getForecastStrategies: mocks.strategies,
    getForecastBacktests: mocks.runs,
    getForecastBacktest: mocks.run,
    startForecastBacktest: mocks.start,
    cancelForecastBacktest: mocks.cancel,
    renameForecastBacktest: mocks.rename,
    deleteForecastBacktest: mocks.remove,
    getMarketDataSettings: mocks.settings,
    getMarketDataStatus: mocks.status,
    getHistoryCoverage: mocks.coverage,
    startBackfill: mocks.backfill,
  },
}))
vi.mock('@/components/common/notify', () => ({
  notify: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
  confirmToast: mocks.confirm,
}))

const metrics: BacktestMetrics = {
  samples: 100,
  signals: 80,
  hits: 40,
  misses: 40,
  hit_rate: 0.5,
  chance_rate: 0.51,
  baseline_up: 0.5,
  brier: 0.25,
  brier_candles: 0.26,
  baseline_brier: 0.25,
  lift: -0.1,
  avg_return_pct: 0,
  avg_return_after_fee_pct: -0.1,
  move_coverage: 0.8,
  derivative_coverage: 0.12,
}

const run = (patch: Partial<BacktestRun>): BacktestRun => ({
  id: 1,
  strategy_id: 'strategy-1',
  name: '',
  options: { symbols: ['BTCUSDT'], intervals: ['1h'], from: 1, to: 2 },
  status: 'finished',
  done: 1,
  total: 1,
  created_at: 1,
  ...patch,
})

const show = (entry = '/forecast/backtests') =>
  render(
    <MemoryRouter initialEntries={[entry]}>
      <QueryWrapper>
        <ForecastBacktestsPage />
      </QueryWrapper>
    </MemoryRouter>
  )

const rail = () => screen.getByRole('navigation', { name: 'Runs' })

beforeEach(() => {
  vi.clearAllMocks()
  mocks.strategies.mockResolvedValue({
    active_strategy_id: 'strategy-1',
    default_config: {},
    strategies: [
      { id: 'strategy-1', name: 'Default', retired: false },
      { id: 'strategy-0', name: 'Old', retired: true },
    ],
  })
  mocks.settings.mockResolvedValue({
    enabled: true,
    watchlist: ['BTCUSDT'],
    top_n: 0,
  })
  mocks.status.mockResolvedValue({ backfill: { running: false } })
  mocks.coverage.mockResolvedValue({ coverage: [] })
  mocks.runs.mockResolvedValue({ backtests: [] })
  mocks.start.mockResolvedValue(run({ id: 9, status: 'running' }))
})
afterEach(cleanup)

it('opens the form when there is no run yet, and starts one', async () => {
  show()
  const start = await screen.findByRole('button', { name: 'Run backtest' })
  await waitFor(() => expect(start).toBeEnabled())
  fireEvent.click(start)
  await waitFor(() => expect(mocks.start).toHaveBeenCalled())
  const request = mocks.start.mock.calls[0][0]
  expect(request.strategy_id).toBe('strategy-1')
  expect(request.symbols).toEqual(['BTCUSDT'])
  expect(request.to).toBeGreaterThan(request.from)
})

it('offers no download when no history is short', async () => {
  show()
  await screen.findByRole('button', { name: 'Run backtest' })
  await waitFor(() => expect(mocks.coverage).toHaveBeenCalled())
  expect(screen.queryByRole('button', { name: 'Download history' })).toBeNull()
  expect(screen.queryByText('Download derivatives')).toBeNull()
})

it('asks for derivatives with the history unless turned off', async () => {
  mocks.backfill.mockResolvedValue(undefined)
  mocks.coverage.mockResolvedValue({
    coverage: [
      { symbol: 'BTCUSDT', interval: '1h', candles: 0, first: 0, last: 0 },
    ],
  })
  show()
  const download = await screen.findByRole('button', {
    name: 'Download history',
  })
  await waitFor(() => expect(screen.getByText('BTCUSDT')).toBeVisible())
  fireEvent.click(download)
  await waitFor(() => expect(mocks.backfill).toHaveBeenCalled())
  expect(mocks.backfill.mock.calls[0][0].derivatives).toBe(true)
  expect(mocks.backfill.mock.calls[0][0].months).toBeGreaterThanOrEqual(12)
})

it('takes a custom range and refuses one that runs backwards', async () => {
  show()
  const from = await screen.findByLabelText('From')
  fireEvent.change(from, { target: { value: '2026-05-01' } })
  fireEvent.change(screen.getByLabelText('To'), {
    target: { value: '2026-02-01' },
  })
  expect(screen.getByText('The range must end after it starts')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Run backtest' })).toBeDisabled()

  fireEvent.change(screen.getByLabelText('To'), {
    target: { value: '2026-06-01' },
  })
  const start = screen.getByRole('button', { name: 'Run backtest' })
  await waitFor(() => expect(start).toBeEnabled())
  fireEvent.click(start)
  await waitFor(() => expect(mocks.start).toHaveBeenCalled())
  expect(mocks.start.mock.calls[0][0].from).toBe(
    new Date('2026-05-01T00:00:00').getTime()
  )
})

it('compares runs in the list and reads the opened one whole', async () => {
  mocks.runs.mockResolvedValue({
    backtests: [run({ id: 2, summary: metrics })],
  })
  mocks.run.mockResolvedValue(
    run({
      id: 2,
      report: {
        fee_pct: 0.1,
        overall: metrics,
        by_interval: { '1h': metrics },
        by_regime: { trending: metrics },
        by_symbol: { BTCUSDT: metrics },
        factors: [
          {
            name: 'trend',
            trending: { samples: 50, agree_rate: 0.55, correlation: 0.1 },
            ranging: { samples: 50, agree_rate: null, correlation: null },
          },
        ],
        reliability: [],
        first_time: 1,
        last_time: 2,
      },
    })
  )
  show()
  expect(await within(rail()).findByText(/50\.0% \/ 51\.0%/)).toBeVisible()
  expect(await screen.findByText('12.0%')).toBeVisible()
  expect(mocks.run).toHaveBeenCalledWith(2)
  expect(screen.getByText('55.0%')).toBeVisible()
  expect(screen.getByText('+0.100')).toBeVisible()
})

it('stops a running run, and says why one failed', async () => {
  mocks.runs.mockResolvedValue({
    backtests: [
      run({ id: 3, status: 'running', done: 1, total: 4 }),
      run({ id: 2, status: 'failed', error: 'canceled' }),
      run({ id: 1, status: 'failed', error: 'out of memory' }),
    ],
  })
  mocks.cancel.mockResolvedValue(undefined)
  show()
  expect(await screen.findByText('1 of 4 series')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Stop' }))
  await waitFor(() => expect(mocks.cancel).toHaveBeenCalledWith(3))

  const entries = within(rail()).getAllByTitle('BTC · 1h')
  fireEvent.click(entries[1])
  expect(await screen.findByText('This run was stopped')).toBeVisible()
  fireEvent.click(entries[2])
  expect(
    await screen.findByText('This run failed: out of memory')
  ).toBeVisible()
  expect(mocks.run).not.toHaveBeenCalled()
})

it('says when a finished run had nothing to test, rather than judging it', async () => {
  const empty = { ...metrics, samples: 0, signals: 0, hit_rate: null }
  mocks.runs.mockResolvedValue({
    backtests: [run({ id: 2, summary: empty })],
  })
  mocks.run.mockResolvedValue(
    run({
      id: 2,
      report: {
        fee_pct: 0.1,
        overall: empty,
        by_interval: {},
        by_regime: {},
        by_symbol: {},
        reliability: null,
        first_time: 0,
        last_time: 0,
        skipped: ['BTCUSDT 1h: not enough history'],
      },
    })
  )
  show()
  expect(await screen.findByText(/Nothing could be tested/)).toBeVisible()
  expect(screen.getByText(/not enough history/)).toBeVisible()
})

it('names a run in place, and an empty name goes back to the strategy', async () => {
  mocks.runs.mockResolvedValue({ backtests: [run({ id: 2, name: 'Holdout' })] })
  mocks.run.mockResolvedValue(run({ id: 2 }))
  mocks.rename.mockResolvedValue(undefined)
  show()
  expect(await within(rail()).findByText('Holdout')).toBeVisible()

  fireEvent.click(screen.getByRole('button', { name: 'Rename' }))
  const field = screen.getByLabelText('Name')
  fireEvent.change(field, { target: { value: 'Holdout 2' } })
  fireEvent.keyDown(field, { key: 'Escape' })
  expect(mocks.rename).not.toHaveBeenCalled()
  expect(within(rail()).getByText('Holdout')).toBeVisible()

  fireEvent.click(screen.getByRole('button', { name: 'Rename' }))
  fireEvent.change(screen.getByLabelText('Name'), {
    target: { value: 'Holdout 3' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Keep the old name' }))
  expect(mocks.rename).not.toHaveBeenCalled()
  expect(within(rail()).getByText('Holdout')).toBeVisible()

  fireEvent.click(screen.getByRole('button', { name: 'Rename' }))
  const again = screen.getByLabelText('Name')
  fireEvent.change(again, { target: { value: '' } })
  fireEvent.keyDown(again, { key: 'Enter' })
  await waitFor(() => expect(mocks.rename).toHaveBeenCalledWith(2, ''))
  expect(await within(rail()).findByText('Default')).toBeVisible()
})

it('runs one again from what it tested, and deletes one after asking', async () => {
  const options = {
    symbols: ['ETHUSDT', 'SOLUSDT'],
    intervals: ['4h'],
    from: new Date('2026-03-01T00:00:00').getTime(),
    to: new Date('2026-04-01T00:00:00').getTime(),
  }
  mocks.runs.mockResolvedValue({ backtests: [run({ id: 2, options })] })
  mocks.run.mockResolvedValue(run({ id: 2, options }))
  mocks.confirm.mockResolvedValue(false)
  mocks.remove.mockResolvedValue(undefined)
  show()

  fireEvent.click(await screen.findByRole('button', { name: 'Delete run' }))
  await waitFor(() => expect(mocks.confirm).toHaveBeenCalled())
  expect(mocks.remove).not.toHaveBeenCalled()
  mocks.confirm.mockResolvedValue(true)
  fireEvent.click(screen.getByRole('button', { name: 'Delete run' }))
  await waitFor(() => expect(mocks.remove).toHaveBeenCalledWith(2))

  fireEvent.click(screen.getByRole('button', { name: 'Run again' }))
  const start = await screen.findByRole('button', { name: 'Run backtest' })
  expect(screen.getByLabelText('From')).toHaveValue('2026-03-01')
  expect(screen.getByLabelText('To')).toHaveValue('2026-04-01')
  await waitFor(() => expect(start).toBeEnabled())
  fireEvent.click(start)
  await waitFor(() => expect(mocks.start).toHaveBeenCalled())
  expect(mocks.start.mock.calls[0][0].symbols).toEqual(['ETHUSDT', 'SOLUSDT'])
  expect(mocks.start.mock.calls[0][0].intervals).toEqual(['4h'])
})

it('says what a run really tested when history fell short of what it asked for', async () => {
  const asked = {
    symbols: ['BTCUSDT'],
    intervals: ['1h'],
    from: new Date('2024-11-02T00:00:00').getTime(),
    to: new Date('2026-11-02T00:00:00').getTime(),
  }
  mocks.runs.mockResolvedValue({
    backtests: [
      run({
        id: 3,
        options: asked,
        tested_from: new Date('2026-10-28T12:00:00').getTime(),
        tested_to: new Date('2026-10-31T12:00:00').getTime(),
      }),
    ],
  })
  show()
  expect(await screen.findByText('Short')).toBeVisible()
  expect(screen.getByText(/Oct 28, 2026 – Oct 31, 2026/)).toBeVisible()
})

it('says nothing more when a run tested about what it asked for', async () => {
  const from = new Date('2026-01-01T00:00:00').getTime()
  const to = new Date('2026-07-01T00:00:00').getTime()
  mocks.runs.mockResolvedValue({
    backtests: [
      run({
        id: 3,
        options: { symbols: ['BTCUSDT'], intervals: ['1h'], from, to },
        tested_from: from + 86400000,
        tested_to: to - 86400000,
      }),
    ],
  })
  show()
  await screen.findByText(/BTC · 1h/)
  expect(screen.queryByText('Short')).toBeNull()
})

it('says a run was not found rather than showing another', async () => {
  mocks.runs.mockResolvedValue({ backtests: [run({ id: 3 })] })
  show('/forecast/backtests?run=999')
  expect(await screen.findByText(/Run 999 was not found/)).toBeVisible()
})

it("says a restart stopped a run, in the reader's language", async () => {
  mocks.runs.mockResolvedValue({
    backtests: [
      run({ id: 3, status: 'failed', error: 'interrupted by a restart' }),
    ],
  })
  show()
  expect(await screen.findByText(/A restart stopped this run/)).toBeVisible()
})
