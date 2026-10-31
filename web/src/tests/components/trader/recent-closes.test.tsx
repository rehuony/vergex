import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import {
  CLOSES_PAGE_SIZE,
  RecentCloses,
} from '@/components/trader/recent-closes'
import type { HistoricalPosition } from '@/types'

const mocks = vi.hoisted(() => ({ older: vi.fn() }))
vi.mock('@/api', () => ({ api: { getOlderPositions: mocks.older } }))

beforeEach(() => vi.clearAllMocks())
afterEach(cleanup)

const trades = (from: number, count: number) =>
  Array.from(
    { length: count },
    (_, index) =>
      ({
        id: from - index,
        symbol: 'BTCUSDT',
        side: 'long',
        entry_time: 1,
        exit_time: 2,
        realized_pnl: 1,
      }) as unknown as HistoricalPosition
  )

const props = { traderId: 't1', failed: false, language: 'en' as const }
const rows = () => screen.getAllByRole('row').length - 1

it('offers no older page when the first one is not full', () => {
  render(<RecentCloses {...props} trades={trades(10, 3)} />)
  expect(rows()).toBe(3)
  expect(screen.queryByText('Load older')).not.toBeInTheDocument()
})

it('continues from the last row until the history ends, showing each trade once', async () => {
  const first = trades(100, CLOSES_PAGE_SIZE)
  // A trade that closed meanwhile moved the last row into this page as well.
  mocks.older.mockResolvedValueOnce(trades(51, 2))
  render(<RecentCloses {...props} trades={first} />)

  fireEvent.click(screen.getByText('Load older'))
  await waitFor(() => expect(rows()).toBe(CLOSES_PAGE_SIZE + 1))
  expect(mocks.older).toHaveBeenCalledWith('t1', 51, CLOSES_PAGE_SIZE)
  expect(screen.queryByText('Load older')).not.toBeInTheDocument()
})

it('says when an older page failed, and keeps the rows and the way to retry', async () => {
  mocks.older.mockRejectedValueOnce(new Error('down'))
  render(<RecentCloses {...props} trades={trades(100, CLOSES_PAGE_SIZE)} />)

  fireEvent.click(screen.getByText('Load older'))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Closed trades could not be loaded.'
  )
  expect(rows()).toBe(CLOSES_PAGE_SIZE)
  expect(screen.getByText('Load older')).toBeEnabled()
})

it('says when closed trades could not be read at all', () => {
  render(<RecentCloses {...props} failed />)
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Closed trades could not be loaded.'
  )
})
