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
  versions: vi.fn(),
  create: vi.fn(),
  activate: vi.fn(),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@/api', () => ({
  api: {
    getForecastVersions: mocks.versions,
    createForecastVersion: mocks.create,
    activateForecastVersion: mocks.activate,
  },
}))
vi.mock('@/components/common/notify', () => ({
  notify: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
  confirmToast: vi.fn(),
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
  mocks.versions.mockResolvedValue({
    active_version_id: 2,
    default_config: config,
    versions: [
      { id: 2, note: 'current', engine_version: 1, config, created_at: 1 },
      { id: 1, note: 'first', engine_version: 1, config, created_at: 1 },
    ],
  })
  mocks.create.mockResolvedValue({ id: 3 })
})
afterEach(cleanup)

it('saves an edit as a new version, never over the old one', async () => {
  show()
  const save = await screen.findByRole('button', {
    name: 'Save as new version',
  })
  expect(save).toBeDisabled()

  fireEvent.change(screen.getByLabelText('Score slope'), {
    target: { value: '5' },
  })
  expect(save).toBeEnabled()
  fireEvent.change(screen.getByLabelText('Note'), {
    target: { value: 'steeper' },
  })
  fireEvent.click(save)

  await waitFor(() => expect(mocks.create).toHaveBeenCalled())
  const [saved, note] = mocks.create.mock.calls[0]
  expect(saved.slope).toBe(5)
  expect(note).toBe('steeper')
})

it('offers to use a version that is not in use', async () => {
  mocks.activate.mockResolvedValue(undefined)
  show()
  await screen.findByRole('button', { name: 'Save as new version' })
  expect(screen.queryByRole('button', { name: 'Use this version' })).toBeNull()

  fireEvent.click(screen.getByRole('button', { name: /Version 1/ }))
  fireEvent.click(
    await screen.findByRole('button', { name: 'Use this version' })
  )
  await waitFor(() => expect(mocks.activate).toHaveBeenCalledWith(1))
})

it('will not save with every factor off', async () => {
  show()
  fireEvent.click(await screen.findByRole('switch', { name: 'Trend' }))
  expect(screen.getByText('Turn on at least one factor.')).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Save as new version' })
  ).toBeDisabled()
})
