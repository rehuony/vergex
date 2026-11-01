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
import type { ForecastConfig } from '@/api/forecast'
import { ForecastStrategiesPage } from '@/pages/forecast-strategies-page'

const mocks = vi.hoisted(() => ({
  strategies: vi.fn(),
  remove: vi.fn(),
  confirm: vi.fn(),
  create: vi.fn(),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@/api', () => ({
  api: {
    getForecastStrategies: mocks.strategies,
    createForecastStrategy: mocks.create,
    deleteForecastStrategy: mocks.remove,
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

const show = () =>
  render(
    <MemoryRouter>
      <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
        <ForecastStrategiesPage />
      </SWRConfig>
    </MemoryRouter>
  )

beforeEach(() => {
  vi.clearAllMocks()
  mocks.strategies.mockResolvedValue({
    active_strategy_id: 'strategy-2',
    default_config: config,
    strategies: [
      {
        id: 'strategy-2',
        name: 'Current',
        engine_version: 1,
        config,
        created_at: 1,
      },
      {
        id: 'strategy-1',
        name: 'First',
        engine_version: 1,
        config,
        created_at: 1,
      },
    ],
  })
  mocks.create.mockResolvedValue({ id: 'strategy-3' })
})
afterEach(cleanup)

it('saves an edit as a new strategy, never over the old one', async () => {
  show()
  const save = await screen.findByRole('button', {
    name: 'Save as new strategy',
  })
  expect(save).toBeDisabled()

  fireEvent.change(screen.getByLabelText('Score slope'), {
    target: { value: '5' },
  })
  expect(save).toBeDisabled()
  expect(screen.getByText('Name the new strategy.')).toBeVisible()
  fireEvent.change(screen.getByLabelText('Name'), {
    target: { value: 'Steeper' },
  })
  expect(save).toBeEnabled()
  fireEvent.click(save)

  await waitFor(() => expect(mocks.create).toHaveBeenCalled())
  const [saved, name] = mocks.create.mock.calls[0]
  expect(saved.slope).toBe(5)
  expect(name).toBe('Steeper')
})

it('deletes only a strategy that is not in use', async () => {
  mocks.confirm.mockResolvedValue(true)
  mocks.remove.mockResolvedValue(undefined)
  show()
  const remove = await screen.findByRole('button', { name: 'Delete strategy' })
  expect(remove).toBeDisabled()

  fireEvent.click(screen.getByRole('button', { name: /First/ }))
  await waitFor(() => expect(remove).toBeEnabled())
  fireEvent.click(remove)
  await waitFor(() => expect(mocks.remove).toHaveBeenCalledWith('strategy-1'))
})

it('will not save with every factor off', async () => {
  show()
  fireEvent.click(await screen.findByRole('switch', { name: 'Trend' }))
  expect(screen.getByText('Turn on at least one factor.')).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Save as new strategy' })
  ).toBeDisabled()
})
