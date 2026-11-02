import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { AccountLedger } from '@/components/settings/account-ledger'
import { QueryWrapper } from '@/tests/query-wrapper'
import type { Account } from '@/types'

const mocks = vi.hoisted(() => ({
  orders: vi.fn(),
  fills: vi.fn(),
  download: vi.fn(),
}))
vi.mock('@/api', () => ({
  api: { getAccountOrders: mocks.orders, getAccountFills: mocks.fills },
}))
vi.mock('@/utils/orders-csv', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  downloadCsv: mocks.download,
}))

beforeEach(() => vi.clearAllMocks())
afterEach(cleanup)

const account = { id: 'a1', name: 'Main', venue_name: 'Binance' } as Account

const order = (id: number, source: string, symbol: string) => ({
  id,
  source,
  symbol,
  side: 'BUY',
  position_side: 'LONG',
  type: 'MARKET',
  quantity: 1,
  price: 0,
  status: 'FILLED',
  filled_quantity: 1,
  avg_fill_price: 100,
  commission: 0.1,
  commission_asset: 'USDT',
  order_action: 'open_long',
  created_at: Date.UTC(2026, 0, 2),
})

const fill = (id: number, source: string, realized: number) => ({
  id,
  source,
  symbol: 'ETHUSDT',
  side: 'SELL',
  quantity: 1,
  price: 100,
  commission: 0.1,
  commission_asset: 'USDT',
  realized_pnl: realized,
  created_at: Date.UTC(2026, 0, 2),
})

const show = () =>
  render(
    <QueryWrapper>
      <AccountLedger account={account} language="en" onBack={vi.fn()} />
    </QueryWrapper>
  )

it('lists what nothing else does, and sums it by who placed it', async () => {
  mocks.orders.mockResolvedValue([
    order(1, 'manual', 'BTCUSDT'),
    order(2, 'external', 'SOLUSDT'),
    order(3, 'external', 'XRPUSDT'),
  ])
  mocks.fills.mockResolvedValue([
    fill(1, 'external', 20),
    fill(2, 'external', -5),
  ])
  show()
  expect(await screen.findByText('SOL')).toBeInTheDocument()
  const external = screen.getAllByText('Exchange')[0].closest('tr')
  expect(external).toHaveTextContent('Exchange22+$15')
  expect(screen.getAllByText('Manual').length).toBeGreaterThan(0)
  expect(mocks.orders).toHaveBeenCalledWith('a1', 500, true)
})

it('exports what the open view shows', async () => {
  mocks.orders.mockResolvedValue([order(1, 'manual', 'BTCUSDT')])
  mocks.fills.mockResolvedValue([fill(7, 'external', 20)])
  show()
  await screen.findByText('BTC')
  fireEvent.click(screen.getByRole('button', { name: 'Export CSV' }))
  expect(mocks.download.mock.calls[0][0]).toBe('Main-orders.csv')
  expect(mocks.download.mock.calls[0][1]).toContain('manual,')

  fireEvent.click(screen.getByRole('button', { name: 'Fills' }))
  await screen.findByText('ETH')
  fireEvent.click(screen.getByRole('button', { name: 'Export CSV' }))
  expect(mocks.download.mock.calls[1][0]).toBe('Main-fills.csv')
  expect(mocks.download.mock.calls[1][1]).toContain('external,ETHUSDT')
})

it('says so when the ledger is empty or cannot be read', async () => {
  mocks.orders.mockResolvedValue([])
  mocks.fills.mockResolvedValue([])
  show()
  expect(await screen.findByText(/Nothing in the ledger/)).toBeInTheDocument()
  cleanup()
  mocks.orders.mockRejectedValue(new Error('no'))
  show()
  expect(await screen.findByRole('alert')).toHaveTextContent(/could not be/)
})
