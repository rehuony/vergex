import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { SWRConfig } from 'swr'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ForecastSettings } from '@/components/settings/forecast-settings'

const mocks = vi.hoisted(() => ({
  settings: vi.fn(),
  status: vi.fn(),
  forecast: vi.fn(),
  save: vi.fn(),
  saveForecast: vi.fn(),
}))
vi.mock('@/api', () => ({
  api: {
    getMarketDataSettings: mocks.settings,
    getMarketDataStatus: mocks.status,
    getForecastSettings: mocks.forecast,
    saveMarketDataSettings: mocks.save,
    saveForecastSettings: mocks.saveForecast,
  },
}))

const show = () =>
  render(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <ForecastSettings language="en" />
    </SWRConfig>
  )

beforeEach(() => {
  vi.clearAllMocks()
  mocks.settings.mockResolvedValue({
    enabled: true,
    watchlist: ['BTCUSDT'],
    top_n: 200,
  })
  mocks.forecast.mockResolvedValue({ alert_threshold: 70 })
  mocks.status.mockResolvedValue({
    ready: 5,
    series: 8,
    behind: 2,
    limiter: { banned: false, used_weight: 3 },
  })
  mocks.save.mockImplementation(async (value) => value)
  mocks.saveForecast.mockResolvedValue(undefined)
})
afterEach(cleanup)

it('shows progress and saves what changed', async () => {
  show()
  expect(await screen.findByText('5 of 8 ready · 2 behind')).toBeVisible()
  const save = screen.getByRole('button', { name: 'Save' })
  await waitFor(() =>
    expect(screen.getByLabelText('Alert at score')).toHaveValue(70)
  )
  expect(save).toBeDisabled()

  fireEvent.change(screen.getByLabelText('Top coins'), {
    target: { value: '50' },
  })
  fireEvent.change(screen.getByLabelText('Alert at score'), {
    target: { value: '80' },
  })
  expect(screen.getByText('up, or 20 down')).toBeVisible()
  fireEvent.click(save)

  await waitFor(() =>
    expect(mocks.save).toHaveBeenCalledWith({
      enabled: true,
      watchlist: ['BTCUSDT'],
      top_n: 50,
    })
  )
  await waitFor(() => expect(mocks.saveForecast).toHaveBeenCalledWith(80))
})

it('reports a rate limit pause', async () => {
  mocks.status.mockResolvedValue({
    ready: 5,
    series: 8,
    behind: 8,
    limiter: {
      banned: false,
      used_weight: 1300,
      reason: 'used weight 1300',
    },
  })
  show()
  expect(await screen.findByText('used weight 1300')).toBeVisible()
})
