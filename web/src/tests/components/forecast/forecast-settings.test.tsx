import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ForecastSettings } from '@/components/forecast/forecast-settings'

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
    <QueryWrapper>
      <ForecastSettings language="en" />
    </QueryWrapper>
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
    stream: { connected: true, topics: 41 },
    limiter: { banned: false, used_weight: 3 },
  })
  mocks.save.mockImplementation(async (value) => value)
  mocks.saveForecast.mockResolvedValue(undefined)
})
afterEach(cleanup)

const card = (name: string) =>
  within(screen.getByRole('heading', { name }).closest('section')!)

it('shows how collection is doing', async () => {
  show()
  expect(await screen.findByText('5 / 8')).toBeVisible()
  expect(screen.getByText('Connected · 41 topics')).toBeVisible()
})

it('saves collection with its button, and only what changed', async () => {
  show()
  await screen.findByLabelText('Top coins')
  fireEvent.click(card('Collection').getByRole('button', { name: 'Save' }))
  expect(mocks.save).not.toHaveBeenCalled()

  fireEvent.change(screen.getByLabelText('Top coins'), {
    target: { value: '50' },
  })
  fireEvent.click(card('Collection').getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(mocks.save).toHaveBeenCalledWith({
      enabled: true,
      watchlist: ['BTCUSDT'],
      top_n: 50,
    })
  )
  expect(mocks.saveForecast).not.toHaveBeenCalled()
})

it('saves the alert on leaving its field, and only when it changed', async () => {
  show()
  const field = await screen.findByLabelText('Alert at score')
  expect(card('Alerts').queryByRole('button')).toBeNull()
  fireEvent.blur(field)
  expect(mocks.saveForecast).not.toHaveBeenCalled()

  fireEvent.change(field, { target: { value: '80' } })
  expect(screen.getByText('up, or 20 down')).toBeVisible()
  expect(mocks.saveForecast).not.toHaveBeenCalled()
  fireEvent.blur(field)
  await waitFor(() => expect(mocks.saveForecast).toHaveBeenCalledWith(80))
})

it('saves an alert out of range at its limit', async () => {
  show()
  const field = await screen.findByLabelText('Alert at score')
  fireEvent.change(field, { target: { value: '120' } })
  fireEvent.blur(field)
  await waitFor(() => expect(mocks.saveForecast).toHaveBeenCalledWith(99))
  expect(field).toHaveValue(99)
})

it('goes back to the saved alert when saving fails', async () => {
  mocks.saveForecast.mockRejectedValue(new Error('Refused'))
  show()
  const field = await screen.findByLabelText('Alert at score')
  fireEvent.change(field, { target: { value: '80' } })
  fireEvent.blur(field)
  await waitFor(() => expect(mocks.saveForecast).toHaveBeenCalledWith(80))
  await waitFor(() => expect(field).toHaveValue(70))
})

it('reports a rate limit pause', async () => {
  mocks.status.mockResolvedValue({
    ready: 5,
    series: 8,
    behind: 8,
    stream: { connected: true, topics: 41 },
    limiter: {
      banned: false,
      used_weight: 1300,
      reason: 'used weight 1300',
    },
  })
  show()
  expect(await screen.findByText('used weight 1300')).toBeVisible()
})
