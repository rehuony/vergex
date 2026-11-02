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
import type { ForecastConfig } from '@/api/forecast'
import { ForecastStrategiesPage } from '@/pages/forecast-strategies-page'

const mocks = vi.hoisted(() => ({
  strategies: vi.fn(),
  remove: vi.fn(),
  confirm: vi.fn(),
  create: vi.fn(),
  activate: vi.fn(),
  deactivate: vi.fn(),
  revive: vi.fn(),
  rename: vi.fn(),
  performance: vi.fn(),
  backtests: vi.fn(),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@/api', () => ({
  api: {
    getForecastStrategies: mocks.strategies,
    createForecastStrategy: mocks.create,
    deleteForecastStrategy: mocks.remove,
    activateForecastStrategy: mocks.activate,
    deactivateForecastStrategy: mocks.deactivate,
    reviveForecastStrategy: mocks.revive,
    renameForecastStrategy: mocks.rename,
    getForecastPerformance: mocks.performance,
    getForecastBacktests: mocks.backtests,
  },
}))
vi.mock('@/components/common/notify', () => ({
  notify: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
  confirmToast: mocks.confirm,
}))

const config: ForecastConfig = {
  scorer: 'weighted',
  factors: { trend: { enabled: true, trending: 1, ranging: 0.2 } },
  adx_threshold: 25,
  slope: 3,
  direction_threshold: 0.05,
  horizons: { '15m': 4, '1h': 4, '4h': 4, '1d': 4 },
}

const stats = (samples: number) => ({
  judged: samples * 4,
  samples,
  hits: samples,
  misses: 0,
  hit_rate: samples ? 0.62 : null,
  chance_rate: samples ? 0.51 : null,
  avg_return_after_fee_pct: samples ? 0.3 : null,
})

const show = () =>
  render(
    <MemoryRouter>
      <QueryWrapper>
        <ForecastStrategiesPage />
      </QueryWrapper>
    </MemoryRouter>
  )

const rail = () => screen.getByRole('navigation', { name: 'Strategies' })

beforeEach(() => {
  vi.clearAllMocks()
  sessionStorage.clear()
  mocks.strategies.mockResolvedValue({
    active_strategy_id: 'strategy-2',
    default_config: {
      ...config,
      factors: {
        ...config.factors,
        funding: { enabled: true, trending: 0.4, ranging: 0.5 },
      },
    },
    strategies: [
      {
        id: 'strategy-2',
        name: 'Current',
        retired: false,
        config,
        created_at: 1,
      },
      {
        id: 'strategy-1',
        name: 'First',
        retired: false,
        config,
        created_at: 1,
      },
      {
        id: 'strategy-0',
        name: 'Old',
        retired: true,
        config,
        created_at: 1,
      },
    ],
  })
  mocks.create.mockResolvedValue({ id: 'strategy-3', name: 'Steeper' })
  mocks.activate.mockResolvedValue(undefined)
  mocks.backtests.mockResolvedValue({ backtests: [] })
  mocks.performance.mockResolvedValue({
    overall: stats(40),
    by_interval: { '1h': stats(40) },
    by_regime: { trending: stats(40) },
    by_direction: { up: stats(40) },
    by_confidence: { '1': stats(40) },
    calibration: [{ interval: '1h', moments: 12, needed: 200, active: false }],
  })
})
afterEach(cleanup)

it('reads a saved strategy as plain values, and saves new rules as a new strategy', async () => {
  show()
  fireEvent.click(await screen.findByRole('tab', { name: 'Rules' }))
  expect(screen.queryByLabelText('Score slope')).toBeNull()

  fireEvent.click(within(rail()).getByRole('button', { name: 'New strategy' }))
  fireEvent.change(await screen.findByLabelText('Score slope'), {
    target: { value: '5' },
  })
  const save = screen.getByRole('button', { name: 'Save strategy' })
  fireEvent.click(save)
  expect(screen.getByText('Name the new strategy.')).toBeVisible()
  expect(mocks.create).not.toHaveBeenCalled()

  fireEvent.change(screen.getByLabelText('Name'), {
    target: { value: 'Steeper' },
  })
  fireEvent.click(save)

  await waitFor(() => expect(mocks.create).toHaveBeenCalled())
  const [saved, name] = mocks.create.mock.calls[0]
  expect(saved.slope).toBe(5)
  expect(name).toBe('Steeper')
  // A factor the code knows and the old rules lack can be turned on, and starts off.
  expect(saved.factors.funding.enabled).toBe(false)
})

it('asks before leaving a changed draft, and not before an untouched one', async () => {
  show()
  await screen.findByRole('tab', { name: 'Rules' })
  fireEvent.click(within(rail()).getByRole('button', { name: 'New strategy' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.getByRole('tab', { name: 'Rules' })).toBeVisible()
  )
  expect(mocks.confirm).not.toHaveBeenCalled()

  fireEvent.click(within(rail()).getByRole('button', { name: 'New strategy' }))
  fireEvent.change(await screen.findByLabelText('Score slope'), {
    target: { value: '5' },
  })
  mocks.confirm.mockResolvedValue(false)
  fireEvent.click(screen.getByRole('button', { name: /First/ }))
  await waitFor(() => expect(mocks.confirm).toHaveBeenCalled())
  expect(screen.getByLabelText('Score slope')).toHaveValue(5)
})

it('stops the strategy in use with one press', async () => {
  mocks.deactivate.mockResolvedValue(undefined)
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Stop using' }))
  await waitFor(() =>
    expect(mocks.deactivate).toHaveBeenCalledWith('strategy-2')
  )
  expect(mocks.confirm).not.toHaveBeenCalled()
})

it('uses and deletes only what it may', async () => {
  mocks.confirm.mockResolvedValue(true)
  mocks.remove.mockResolvedValue(undefined)
  show()
  const remove = await screen.findByRole('button', { name: 'Delete strategy' })
  expect(remove).toBeDisabled()
  expect(screen.queryByRole('button', { name: 'Use' })).toBeNull()

  fireEvent.click(screen.getByRole('button', { name: /First/ }))
  await waitFor(() => expect(remove).toBeEnabled())
  fireEvent.click(screen.getByRole('button', { name: 'Use' }))
  await waitFor(() => expect(mocks.activate).toHaveBeenCalledWith('strategy-1'))
  fireEvent.click(remove)
  await waitFor(() => expect(mocks.remove).toHaveBeenCalledWith('strategy-1'))
})

it('asks before using a retired strategy again, and offers no backtest of it', async () => {
  mocks.confirm.mockResolvedValueOnce(false).mockResolvedValueOnce(true)
  mocks.revive.mockResolvedValue(undefined)
  show()
  fireEvent.click(await screen.findByRole('button', { name: /Old/ }))
  await screen.findByRole('button', { name: 'Delete strategy' })
  await waitFor(() =>
    expect(screen.queryByRole('link', { name: 'Backtest' })).toBeNull()
  )

  fireEvent.click(screen.getByRole('button', { name: 'Use' }))
  await waitFor(() => expect(mocks.confirm).toHaveBeenCalledTimes(1))
  expect(mocks.revive).not.toHaveBeenCalled()

  fireEvent.click(screen.getByRole('button', { name: 'Use' }))
  await waitFor(() => expect(mocks.revive).toHaveBeenCalledWith('strategy-0'))
  expect(mocks.activate).not.toHaveBeenCalled()
})

it('opens on a strategy when none is in use', async () => {
  mocks.strategies.mockResolvedValue({
    ...(await mocks.strategies()),
    active_strategy_id: '',
  })
  show()
  expect(await screen.findByRole('button', { name: 'Use' })).toBeVisible()
})

it('will not save with every factor off', async () => {
  show()
  await screen.findByRole('tab', { name: 'Rules' })
  fireEvent.click(within(rail()).getByRole('button', { name: 'New strategy' }))
  fireEvent.click(await screen.findByRole('switch', { name: 'Trend' }))
  fireEvent.change(screen.getByLabelText('Name'), {
    target: { value: 'Nothing' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save strategy' }))
  expect(screen.getByText('Turn on at least one factor.')).toBeVisible()
  expect(mocks.create).not.toHaveBeenCalled()
})

it('shows how a strategy has done live, and how far it is from a curve', async () => {
  show()
  expect(await screen.findByText('12 / 200')).toBeVisible()
  expect(screen.getByText('160 judged')).toBeVisible()
  expect(screen.getAllByText('62.0%').length).toBeGreaterThan(1)
  expect(screen.getAllByText('51.0%').length).toBeGreaterThan(1)
  expect(rail()).toBeVisible()
})

it('says when no calls have been judged, with dashes for what is unknown', async () => {
  mocks.performance.mockResolvedValue({
    overall: {
      judged: 0,
      samples: 0,
      hits: 0,
      misses: 0,
      hit_rate: null,
      chance_rate: null,
      avg_return_after_fee_pct: null,
    },
    by_interval: {},
    by_regime: {},
    by_direction: {},
    by_confidence: {},
    calibration: [],
  })
  show()
  expect(
    await screen.findByText('No calls have been judged yet.')
  ).toBeVisible()
  expect(screen.getAllByText('—').length).toBeGreaterThan(3)
  expect(screen.queryByText('NaN')).toBeNull()
})

it('renames a strategy in place, and refuses a name already taken', async () => {
  mocks.rename.mockResolvedValue(undefined)
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Rename' }))
  const field = screen.getByLabelText('Name')
  expect(field).toHaveValue('Current')
  fireEvent.change(field, { target: { value: 'First' } })
  fireEvent.keyDown(field, { key: 'Enter' })
  expect(mocks.rename).not.toHaveBeenCalled()
  expect(field).toHaveAttribute('aria-invalid', 'true')

  fireEvent.change(field, { target: { value: 'Renamed' } })
  expect(field).toHaveAttribute('aria-invalid', 'false')
  fireEvent.click(screen.getByRole('button', { name: 'Save name' }))
  await waitFor(() =>
    expect(mocks.rename).toHaveBeenCalledWith('strategy-2', 'Renamed')
  )
  expect(await within(rail()).findByText('Renamed')).toBeVisible()
  expect(screen.queryByLabelText('Name')).toBeNull()
})

it('saves a new name on leaving the field, unless it is taken', async () => {
  mocks.rename.mockResolvedValue(undefined)
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Rename' }))
  const field = screen.getByLabelText('Name')
  fireEvent.change(field, { target: { value: 'First' } })
  fireEvent.blur(field)
  expect(mocks.rename).not.toHaveBeenCalled()
  expect(screen.queryByLabelText('Name')).toBeNull()

  fireEvent.click(screen.getByRole('button', { name: 'Rename' }))
  const again = screen.getByLabelText('Name')
  fireEvent.change(again, { target: { value: 'Renamed' } })
  fireEvent.blur(again)
  await waitFor(() =>
    expect(mocks.rename).toHaveBeenCalledWith('strategy-2', 'Renamed')
  )
})

it('keeps a draft after leaving the page', async () => {
  const { unmount } = show()
  await screen.findByRole('tab', { name: 'Rules' })
  fireEvent.click(within(rail()).getByRole('button', { name: 'New strategy' }))
  fireEvent.change(await screen.findByLabelText('Name'), {
    target: { value: 'Half done' },
  })
  unmount()

  show()
  expect(await screen.findByLabelText('Name')).toHaveValue('Half done')
})
