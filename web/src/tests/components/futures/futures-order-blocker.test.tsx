import {
  render,
  screen,
  cleanup,
  fireEvent,
  within,
} from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import type { LiveRun } from '@/api/futures'
import type { AccountObservation } from '@/utils/futures/futures-workspace'
import { FuturesOrderBlocker } from '@/components/futures/futures-order-blocker'

vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))

afterEach(cleanup)
const run = {
  account_id: 'account',
  status: 'needs_attention',
  plan: {
    snapshot: { venue: 'binance', symbol: 'BTC', quote_asset: 'USDC' },
  },
} as LiveRun
const observation: AccountObservation = {
  accountID: 'account',
  symbol: 'BTC',
  positions: [
    {
      symbol: 'BTC',
      side: 'short',
      quantity: 0.01,
      mark: 80000,
      entry: 81000,
      pnl: 10,
      leverage: 5,
    },
  ],
  orders: [],
  checkedAt: Date.now(),
}
const actions = {
  now: Date.now(),
  busy: false,
  onCheck: vi.fn(),
  onView: vi.fn(),
  onRevoke: vi.fn(),
  onAccount: vi.fn(),
}
// The line names the trade; what can be done about it opens from there.
const details = (line: string) => {
  fireEvent.click(screen.getByRole('button', { name: line }))
  return within(screen.getByRole('dialog'))
}

it('shows a verified live position separately from its monitoring issue', () => {
  render(
    <FuturesOrderBlocker {...actions} run={run} observation={observation} />
  )
  const shown = details('BTC Short open · 5x')
  expect(shown.getByText(/800 USDC position/)).toBeVisible()
  expect(shown.getByText(/Exit order status needs verification/)).toBeVisible()
  expect(shown.getByRole('button', { name: 'Verify status' })).toBeVisible()
  fireEvent.click(shown.getByRole('button', { name: 'View position & orders' }))
  expect(actions.onAccount).toHaveBeenCalledOnce()
  expect(actions.onRevoke).not.toHaveBeenCalled()
  expect(screen.queryByRole('dialog')).toBeNull()
})
it('removes the open-position claim as soon as a fresh flat snapshot arrives', () => {
  const { rerender } = render(
    <FuturesOrderBlocker {...actions} run={run} observation={observation} />
  )
  expect(screen.getByText('BTC Short open · 5x')).toBeVisible()
  rerender(
    <FuturesOrderBlocker
      {...actions}
      run={run}
      observation={{ ...observation, positions: [], orders: [] }}
    />
  )
  expect(screen.queryByText('BTC Short open · 5x')).toBeNull()
  const shown = details(
    'No open BTC position · previous orders need verification'
  )
  expect(shown.getByText(/You can still generate plans/)).toBeVisible()
  expect(shown.getByRole('button', { name: 'Verify status' })).toBeVisible()
  expect(shown.queryByRole('button', { name: 'Cancel approval' })).toBeNull()
})
it('keeps verification errors next to the unresolved trade and allows another read-only check', () => {
  render(
    <FuturesOrderBlocker
      {...actions}
      run={run}
      observation={observation}
      checkError="Stop-loss status could not be confirmed."
    />
  )
  const shown = details('Stop-loss status could not be confirmed.')
  expect(shown.getByRole('alert')).toHaveTextContent(
    'Stop-loss status could not be confirmed.'
  )
  fireEvent.click(shown.getByRole('button', { name: 'Verify status' }))
  expect(actions.onCheck).toHaveBeenCalled()
})
it('does not claim a position is open using stale or other-account data', () => {
  const { rerender } = render(
    <FuturesOrderBlocker
      {...actions}
      run={run}
      observation={{ ...observation, checkedAt: 0 }}
    />
  )
  expect(screen.getByText('BTC trade status needs verification')).toBeVisible()
  rerender(
    <FuturesOrderBlocker
      {...actions}
      run={run}
      observation={{ ...observation, accountID: 'other' }}
    />
  )
  expect(screen.queryByText('BTC Short open · 5x')).toBeNull()
  expect(
    details('BTC trade status needs verification').getByRole('button', {
      name: 'Verify status',
    })
  ).toBeVisible()
})
it('distinguishes an approved unsent order from a live position', () => {
  render(
    <FuturesOrderBlocker {...actions} run={{ ...run, status: 'approved' }} />
  )
  const shown = details('BTC order not submitted')
  expect(
    shown.getByText(/confirming it replaces this unused approval/)
  ).toBeVisible()
  expect(shown.getByRole('button', { name: 'Cancel approval' })).toBeVisible()
  expect(shown.queryByRole('button', { name: 'Verify status' })).toBeNull()
})
