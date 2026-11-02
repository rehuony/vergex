import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { LiveRun } from '@/api/futures'
import { ExitDialog, workingExit } from '@/components/futures/exit-dialog'
import type { AccountPosition } from '@/utils/futures/futures-account'

const mocks = vi.hoisted(() => ({ api: vi.fn() }))
vi.mock('@/api/futures', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  futuresAPI: mocks.api,
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))

beforeEach(() => vi.clearAllMocks())
afterEach(cleanup)

const order = (role: string, price: string, status: string) => ({
  client_id: role + price,
  order_id: role + price,
  role,
  price,
  status,
  quantity: '1',
  filled: '0',
})

const run = {
  id: 'run',
  plan: { snapshot: { tick: '0.1' } },
  orders: [
    order('entry', '100', 'FILLED'),
    order('sl', '90', 'CANCELED'),
    order('tp', '120', 'NEW'),
    order('sl', '95', 'NEW'),
  ],
} as unknown as LiveRun

const position = {
  symbol: 'BTCUSDT',
  side: 'long',
  quantity: 1,
  entry: 100.04,
  mark: 110,
  pnl: 10,
  leverage: 5,
} as AccountPosition

const show = (
  onClose = vi.fn(),
  onDone = vi.fn().mockResolvedValue(undefined)
) =>
  render(
    <ExitDialog
      run={run}
      position={position}
      onClose={onClose}
      onDone={onDone}
    />
  )

it('starts from what is working now, not what the plan said', () => {
  expect(workingExit(run, 'sl')).toBe('95')
  show()
  expect(screen.getByRole('textbox', { name: 'Stop loss' })).toHaveValue('95')
  expect(screen.getByRole('textbox', { name: 'Take profit' })).toHaveValue(
    '120'
  )
})

it('sends only what changed, and takes the stop to the entry on asking', async () => {
  mocks.api.mockResolvedValue({})
  const onClose = vi.fn()
  show(onClose)
  expect(screen.getByRole('button', { name: 'Move' })).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: 'To breakeven' }))
  fireEvent.click(screen.getByRole('button', { name: 'Move' }))
  await waitFor(() => expect(onClose).toHaveBeenCalled())
  expect(mocks.api).toHaveBeenCalledTimes(1)
  expect(mocks.api).toHaveBeenCalledWith('amend-exit', {
    id: 'run',
    role: 'sl',
    price: '100.1',
  })
})

it('stays open and says why when the server refuses', async () => {
  mocks.api.mockRejectedValue(new Error('That price would fire at once.'))
  const onClose = vi.fn()
  show(onClose)
  fireEvent.change(screen.getByRole('textbox', { name: 'Take profit' }), {
    target: { value: '90' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Move' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(/fire at once/)
  expect(onClose).not.toHaveBeenCalled()
})
