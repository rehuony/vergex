import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { TradersPage } from '@/pages/traders-page'
const mocks = vi.hoisted(() => ({
  user: { id: 'user' },
  models: { data: [] as unknown[], error: undefined as unknown },
  accounts: { data: [] as unknown[], error: undefined as unknown },
  update: vi.fn(),
  notifyError: vi.fn(),
}))
vi.mock('@/components/common/notify', () => ({
  confirmToast: vi.fn(),
  notify: { error: mocks.notifyError },
}))
vi.mock('@/contexts/auth-context', () => ({
  useAuth: () => ({ user: mocks.user, token: 'fixture' }),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@tanstack/react-query', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-query')>()),
  useQuery: () => ({ data: [], refetch: vi.fn() }),
}))
vi.mock('@/hooks/use-connections', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/hooks/use-connections')>()),
  useModelConfigs: () => ({ ...mocks.models, refetch: vi.fn() }),
  useAccounts: () => ({ ...mocks.accounts, refetch: vi.fn() }),
}))
vi.mock('@/api', () => ({
  api: { createTrader: mocks.update, getTraders: async () => [] },
}))
vi.mock('@/components/trader/trader-config-modal', () => ({
  TraderConfigModal: () => <div role="dialog">Create trader</div>,
}))
beforeEach(() => {
  vi.clearAllMocks()
  mocks.models = {
    data: [
      {
        id: 'agent',
        name: 'My AI',
        provider: 'deepseek',
        enabled: true,
        has_api_key: true,
      },
    ],
    error: undefined,
  }
  mocks.accounts = {
    data: [
      {
        id: 'exchange',
        name: 'My account',
        venue: 'binance',
        has_api_key: true,
      },
    ],
    error: undefined,
  }
})
afterEach(cleanup)
const open = (state?: unknown) =>
  render(
    <MemoryRouter initialEntries={[{ pathname: '/traders', state }]}>
      <TradersPage />
    </MemoryRouter>
  )
it('drops the list with no traders, and offers creation in the panel', () => {
  open()
  expect(screen.queryByRole('navigation', { name: 'Traders' })).toBeNull()
  const empty = screen.getByRole('status')
  expect(within(empty).getByText('No traders yet')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Create trader' })).toBeEnabled()
  expect(mocks.update).not.toHaveBeenCalled()
})
it('leads to the missing model instead of offering creation', () => {
  mocks.models.data = []
  open()
  const empty = screen.getByRole('status')
  expect(
    within(empty).queryByRole('button', { name: 'Create trader' })
  ).toBeNull()
  expect(within(empty).getByRole('link', { name: 'Add model' })).toBeVisible()
})
it('opens creation when the overview asks for it', () => {
  open({ create: true })
  expect(screen.getByRole('dialog')).toHaveTextContent('Create trader')
})
it('shows a read failure instead of inventing an empty configuration', () => {
  mocks.models = { data: undefined as never, error: new Error('fixture') }
  open()
  expect(mocks.notifyError).toHaveBeenCalledWith(
    expect.stringContaining('Connections could not be loaded')
  )
})
