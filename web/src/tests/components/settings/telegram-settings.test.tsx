import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { TelegramSettings } from '@/components/settings/telegram-settings'

const mocks = vi.hoisted(() => ({
  config: vi.fn(),
  models: vi.fn(),
  alerts: vi.fn(),
  test: vi.fn(),
}))
vi.mock('@/api', () => ({
  api: {
    getTelegramConfig: mocks.config,
    getModelConfigs: mocks.models,
    updateTelegramAlerts: mocks.alerts,
    testTelegramAlert: mocks.test,
  },
}))

beforeEach(() => {
  vi.clearAllMocks()
  mocks.models.mockResolvedValue([])
  mocks.config.mockResolvedValue({
    token_masked: '***abcdef',
    is_bound: true,
    alerts: { order_rejected: true, safe_mode: true, forecast: false },
  })
})
afterEach(cleanup)

it('turns alert kinds on and off and shows only those the server knows', async () => {
  mocks.alerts.mockResolvedValue({
    order_rejected: false,
    safe_mode: true,
    forecast: false,
  })
  render(<TelegramSettings language="en" />)

  const rejected = await screen.findByRole('switch', {
    name: 'Rejected orders',
  })
  expect(rejected).toHaveAttribute('aria-checked', 'true')
  expect(
    screen.getByRole('switch', { name: 'Forecast signals' })
  ).toHaveAttribute('aria-checked', 'false')
  expect(screen.queryByRole('switch', { name: 'Closed positions' })).toBeNull()

  fireEvent.click(rejected)
  await waitFor(() =>
    expect(mocks.alerts).toHaveBeenCalledWith({ order_rejected: false })
  )
  await waitFor(() =>
    expect(
      screen.getByRole('switch', { name: 'Rejected orders' })
    ).toHaveAttribute('aria-checked', 'false')
  )
})

it('sends a test alert', async () => {
  mocks.test.mockResolvedValue(undefined)
  render(<TelegramSettings language="en" />)
  fireEvent.click(await screen.findByRole('button', { name: 'Send test' }))
  await waitFor(() => expect(mocks.test).toHaveBeenCalled())
})
