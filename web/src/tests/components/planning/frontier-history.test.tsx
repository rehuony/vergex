import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { SWRConfig } from 'swr'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { LiveRun } from '@/api/planning'
import { FrontierHistory } from '@/components/planning/frontier-history'

const mocks = vi.hoisted(() => ({ api: vi.fn() }))
vi.mock('@/api/planning', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  planningAPI: mocks.api,
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
    plan: { idea: { name } },
  }) as unknown as LiveRun

const show = (active: boolean, runs: LiveRun[]) =>
  render(
    <SWRConfig value={{ provider: () => new Map() }}>
      <FrontierHistory
        active={active}
        accountID="account"
        runs={runs}
        archived={[]}
        selected=""
        compare={[]}
        clock={0}
        conversations={[]}
        busy={false}
        onChoose={vi.fn()}
        onOpenConversation={vi.fn()}
      />
    </SWRConfig>
  )

it('shows what each finished plan realized, and a dash when the exchange has no record', async () => {
  mocks.api.mockResolvedValue({ win: { realized_pnl: 12.5, closes: 1 } })
  show(true, [run('win', 'Pullback'), run('old', 'Breakout')])
  await waitFor(() => expect(screen.getByText('+$12.5')).toBeInTheDocument())
  expect(screen.getByText('—')).toBeInTheDocument()
  expect(mocks.api).toHaveBeenCalledWith('results?exchange_id=account')
})

it('does not ask the exchange until the section is shown', () => {
  show(false, [run('win', 'Pullback')])
  expect(mocks.api).not.toHaveBeenCalled()
})
