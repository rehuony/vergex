import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { FuturesState, LiveRun } from '@/api/futures'
import { FuturesHistory } from '@/components/futures/futures-history'

const mocks = vi.hoisted(() => ({ api: vi.fn(), confirm: vi.fn() }))
vi.mock('@/api/futures', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  futuresAPI: mocks.api,
}))
vi.mock('@/components/common/notify', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  confirmToast: mocks.confirm,
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))

beforeEach(() => vi.clearAllMocks())
afterEach(cleanup)

const run = (id: string, name: string) =>
  ({
    id,
    status: 'completed',
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

const draft = {
  status: 'cancelled',
  updated: '2026-11-01T10:00:00Z',
  plan: {
    id: 'draft',
    idea: {
      name: 'Old idea',
      entry: '1',
      stop: '1',
      target: '2',
      rationale: '',
      execution: { kind: 'limit' },
    },
    snapshot: { quote_asset: 'USDT' },
    risk: '1',
  },
} as unknown as FuturesState['records'][string]

const show = (
  active: boolean,
  runs: LiveRun[],
  conversations = [] as (typeof conversation)[],
  onOpenConversation = vi.fn(),
  more: {
    archived?: FuturesState['records'][string][]
    onRename?: (id: string, title: string) => Promise<void>
    onDelete?: () => Promise<void>
    onDeletePlan?: () => Promise<void>
  } = {}
) =>
  render(
    <QueryWrapper>
      <FuturesHistory
        active={active}
        accountID="account"
        runs={runs}
        archived={more.archived ?? []}
        clock={0}
        conversations={conversations}
        busy={false}
        onChoose={vi.fn()}
        onOpenConversation={onOpenConversation}
        onRenameConversation={more.onRename ?? vi.fn()}
        onDeleteConversation={more.onDelete ?? vi.fn()}
        onDeletePlan={more.onDeletePlan ?? vi.fn()}
      />
    </QueryWrapper>
  )

const openMenu = (button: HTMLElement) =>
  fireEvent.keyDown(button, { key: 'Enter' })

it('shows what each finished plan realized, and 0 when the exchange has no record', async () => {
  mocks.api.mockResolvedValue({ win: { realized_pnl: 12.5, closes: 1 } })
  show(true, [run('win', 'Pullback'), run('old', 'Breakout')])
  await waitFor(() =>
    expect(screen.getAllByText('+$12.5').length).toBeGreaterThan(0)
  )
  fireEvent.click(screen.getByRole('button', { name: /Breakout/ }))
  expect(screen.getAllByText('$0').length).toBeGreaterThan(0)
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

it('drops the directory and its search when there is nothing in it', () => {
  show(true, [])
  expect(screen.queryByRole('navigation')).toBeNull()
  expect(screen.queryByRole('textbox', { name: 'Search history' })).toBeNull()
  expect(screen.getByRole('status')).toBeVisible()
})

it('lists history as folders that open and close, and hides an empty one', () => {
  mocks.api.mockResolvedValue({ messages: [] })
  show(true, [run('win', 'Pullback')], [conversation])
  const folder = screen.getByRole('button', { name: /Finished plans/ })
  expect(folder).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByRole('button', { name: /Pullback/ })).toBeVisible()
  expect(screen.queryByRole('button', { name: /Expired drafts/ })).toBeNull()
  fireEvent.click(folder)
  expect(folder).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByRole('button', { name: /Pullback/ })).toBeNull()
  // Closing one leaves the others open.
  expect(screen.getByRole('button', { name: /Conversations/ })).toHaveAttribute(
    'aria-expanded',
    'true'
  )
})

it('renames a conversation in its row, and leaves it alone when nothing changed', async () => {
  mocks.api.mockResolvedValue({ messages: [] })
  const rename = vi.fn().mockResolvedValue(undefined)
  show(true, [], [conversation], vi.fn(), { onRename: rename })
  fireEvent.doubleClick(screen.getByRole('button', { name: /Plan a pullback/ }))
  const field = screen.getByRole('textbox', { name: 'Conversation name' })
  fireEvent.keyDown(field, { key: 'Enter' })
  expect(rename).not.toHaveBeenCalled()

  fireEvent.doubleClick(screen.getByRole('button', { name: /Plan a pullback/ }))
  const again = screen.getByRole('textbox', { name: 'Conversation name' })
  fireEvent.change(again, { target: { value: 'BTC pullback' } })
  fireEvent.keyDown(again, { key: 'Enter' })
  await waitFor(() =>
    expect(rename).toHaveBeenCalledWith('chat', 'BTC pullback')
  )
})

it('deletes a conversation only once that is confirmed', async () => {
  mocks.api.mockResolvedValue({ messages: [] })
  const remove = vi.fn().mockResolvedValue(undefined)
  show(true, [], [conversation], vi.fn(), { onDelete: remove })
  const menu = () => screen.getAllByRole('button', { name: 'More actions' })[0]

  mocks.confirm.mockResolvedValueOnce(false)
  openMenu(menu())
  fireEvent.click(
    await screen.findByRole('menuitem', { name: 'Delete conversation' })
  )
  await waitFor(() => expect(mocks.confirm).toHaveBeenCalledTimes(1))
  expect(remove).not.toHaveBeenCalled()

  mocks.confirm.mockResolvedValueOnce(true)
  openMenu(menu())
  fireEvent.click(
    await screen.findByRole('menuitem', { name: 'Delete conversation' })
  )
  await waitFor(() => expect(remove).toHaveBeenCalledWith(conversation))
})

it('deletes a draft, and a finished plan with a word on what stays', async () => {
  mocks.api.mockResolvedValue({})
  const remove = vi.fn().mockResolvedValue(undefined)
  mocks.confirm.mockResolvedValue(true)
  const finished = run('win', 'Pullback')
  show(true, [finished], [], vi.fn(), { onDeletePlan: remove })
  openMenu(screen.getAllByRole('button', { name: 'More actions' })[0])
  expect(
    await screen.findByRole('menuitem', { name: 'View trade on chart' })
  ).toBeVisible()
  fireEvent.click(screen.getByRole('menuitem', { name: 'Delete plan' }))
  await waitFor(() => expect(remove).toHaveBeenCalledWith(finished.plan))
  expect(mocks.confirm.mock.calls[0][0]).toContain('stay in the account')
  cleanup()

  show(true, [], [], vi.fn(), { archived: [draft], onDeletePlan: remove })
  openMenu(screen.getAllByRole('button', { name: 'More actions' })[0])
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete draft' }))
  await waitFor(() => expect(remove).toHaveBeenCalledWith(draft.plan))
})

it('says so when there is nothing yet', () => {
  show(true, [])
  expect(screen.getByRole('status')).toHaveTextContent('Nothing yet')
})
