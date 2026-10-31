import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { AITradersPage } from '@/pages/ai-traders-page'
const mocks = vi.hoisted(() => ({
  user: { id: 'user' },
  models: { data: [] as unknown[], error: undefined as unknown },
  accounts: { data: [] as unknown[], error: undefined as unknown },
  update: vi.fn(),
}))
vi.mock('@/contexts/auth-context', () => ({
  useAuth: () => ({ user: mocks.user, token: 'fixture' }),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('swr', () => ({
  default: () => ({ data: [], mutate: vi.fn(), isLoading: false }),
}))
vi.mock('@/hooks/use-connections', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/hooks/use-connections')>()),
  useModelConfigs: () => ({ ...mocks.models, mutate: vi.fn() }),
  useExchangeConfigs: () => ({ ...mocks.accounts, mutate: vi.fn() }),
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
        id: 'ai',
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
        exchange_type: 'binance',
        enabled: true,
        type: 'cex',
      },
    ],
    error: undefined,
  }
})
afterEach(cleanup)
const open = () =>
  render(
    <MemoryRouter>
      <AITradersPage />
    </MemoryRouter>
  )
it('offers creation once connections are saved', () => {
  open()
  expect(screen.getByRole('button', { name: 'Create trader' })).toBeEnabled()
  expect(screen.getByRole('link', { name: 'Strategies' })).toHaveAttribute(
    'href',
    '/autopilot/strategies'
  )
  expect(mocks.update).not.toHaveBeenCalled()
})
it('holds creation until an AI model is connected', () => {
  mocks.models.data = []
  open()
  expect(screen.getByRole('button', { name: 'Create trader' })).toBeDisabled()
})
it('shows a read failure instead of inventing an empty configuration', () => {
  mocks.models = { data: undefined as never, error: new Error('fixture') }
  open()
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Connections could not be loaded'
  )
  expect(screen.getByRole('button', { name: 'Create trader' })).toBeDisabled()
})
