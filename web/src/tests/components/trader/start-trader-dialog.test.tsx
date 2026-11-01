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
import { ApiError } from '@/api/http-client'
import { StartTraderDialog } from '@/components/trader/start-trader-dialog'

const mocks = vi.hoisted(() => ({ preflight: vi.fn(), start: vi.fn() }))
vi.mock('@/api', () => ({
  api: { getTraderPreflight: mocks.preflight, startTrader: mocks.start },
}))

beforeEach(() => vi.clearAllMocks())
afterEach(cleanup)

const open = () => {
  const onStarted = vi.fn()
  render(
    <MemoryRouter>
      <SWRConfig value={{ provider: () => new Map() }}>
        <StartTraderDialog
          traderId="t1"
          traderName="Alpha"
          strategy="Trend"
          model="DeepSeek"
          account="Main"
          scanMinutes={15}
          language="en"
          onStarted={onStarted}
          onClose={vi.fn()}
        />
      </SWRConfig>
    </MemoryRouter>
  )
  return onStarted
}

const ready = {
  ready: true,
  min_trading_usdc: 12,
  checks: [{ id: 'agent_model', status: 'ok' }],
}

it('holds the start until the checks have passed', async () => {
  let finish: (value: unknown) => void = () => {}
  mocks.preflight.mockReturnValue(new Promise((resolve) => (finish = resolve)))
  const onStarted = open()
  const button = screen.getByRole('button', { name: 'Start trader' })
  expect(button).toBeDisabled()
  finish(ready)
  await waitFor(() => expect(button).toBeEnabled())
  mocks.start.mockResolvedValue(undefined)
  fireEvent.click(button)
  await waitFor(() => expect(onStarted).toHaveBeenCalled())
  expect(mocks.start).toHaveBeenCalledWith('t1')
})

it('names what failed and offers no way to start', async () => {
  mocks.preflight.mockResolvedValue({
    ready: false,
    min_trading_usdc: 12,
    checks: [
      {
        id: 'exchange_funds',
        status: 'failed',
        message: 'Deposit more USDT.',
        required: 12,
        actual: 8.4,
        asset: 'USDT',
      },
    ],
  })
  open()
  await waitFor(() =>
    expect(screen.getByText(/Deposit more USDT/)).toBeInTheDocument()
  )
  expect(screen.getByText('8.40 / 12 USDT')).toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Start trader' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Check again' })).toBeEnabled()
})

it('keeps the dialog open and shows why the backend refused', async () => {
  mocks.preflight.mockResolvedValue(ready)
  mocks.start.mockRejectedValue(
    new ApiError('The model has no key.', 'trader.start.preflight_failed')
  )
  const onStarted = open()
  const button = await screen.findByRole('button', { name: 'Start trader' })
  await waitFor(() => expect(button).toBeEnabled())
  fireEvent.click(button)
  await waitFor(() =>
    expect(screen.getByRole('alert')).toHaveTextContent('The model has no key.')
  )
  expect(onStarted).not.toHaveBeenCalled()
})
