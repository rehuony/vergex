import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { LiveRun } from '@/api/futures'
import { FuturesHistory } from '@/components/futures/futures-history'

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

const run = (id: string, name: string) =>
  ({
    id,
    state: 'completed',
    orders: [],
    plan: {
      idea: { name, entry: '1', stop: '1', target: '2', rationale: '' },
      snapshot: { quote_asset: 'USDT' },
    },
  }) as unknown as LiveRun

const conversation = {
  id: 'chat',
  symbol: 'BTCUSDT',
  account_id: 'account',
  agent_model_id: 'model',
  title: 'Plan a pullback',
  updated_at: '2026-11-01T10:00:00Z',
}

const show = (
  active: boolean,
  runs: LiveRun[],
  conversations = [] as (typeof conversation)[],
  onOpenConversation = vi.fn()
) =>
  render(
    <QueryWrapper>
      <FuturesHistory
        active={active}
        accountID="account"
        runs={runs}
        archived={[]}
        clock={0}
        conversations={conversations}
        busy={false}
        onChoose={vi.fn()}
        onOpenConversation={onOpenConversation}
      />
    </QueryWrapper>
  )

it('shows what each finished plan realized, and a dash when the exchange has no record', async () => {
  mocks.api.mockResolvedValue({ win: { realized_pnl: 12.5, closes: 1 } })
  show(true, [run('win', 'Pullback'), run('old', 'Breakout')])
  await waitFor(() =>
    expect(screen.getAllByText('+$12.5').length).toBeGreaterThan(0)
  )
  fireEvent.click(screen.getByRole('button', { name: /Breakout/ }))
  expect(screen.getAllByText('—').length).toBeGreaterThan(0)
  expect(mocks.api).toHaveBeenCalledWith('results?account_id=account')
})

it('does not ask the exchange until the section is shown', () => {
  show(false, [run('win', 'Pullback')])
  expect(mocks.api).not.toHaveBeenCalled()
})

it('reads a past conversation by its title, and carries on with it', async () => {
  mocks.api.mockResolvedValue({
    messages: [
      { role: 'user', content: 'Where is support?' },
      { role: 'assistant', content: 'Near **77,500**.' },
    ],
  })
  const open = vi.fn()
  show(true, [], [conversation], open)
  expect(await screen.findByText('Where is support?')).toBeVisible()
  expect(screen.getByText('77,500').tagName).toBe('STRONG')
  expect(mocks.api).toHaveBeenCalledWith('conversations/chat')
  fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
  expect(open).toHaveBeenCalledWith(conversation)
})

it('says what will show when there is nothing yet', () => {
  show(true, [])
  expect(screen.getByRole('status')).toHaveTextContent(/will show here/)
})
