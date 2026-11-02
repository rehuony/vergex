import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { DryRunDialog } from '@/components/trader/dry-run-dialog'
import type { AgentConfig, TokenEstimate } from '@/types'

const mocks = vi.hoisted(() => ({
  dryRun: vi.fn(),
  models: [] as unknown[] | undefined,
}))
vi.mock('@/api', () => ({ api: { dryRun: mocks.dryRun } }))
vi.mock('@/hooks/use-connections', () => ({
  useModelConfigs: () => ({ data: mocks.models }),
}))

const config = { coin_source: { source_type: 'static' } } as AgentConfig
const estimate = { total: 6434 } as TokenEstimate

const show = () =>
  render(
    <MemoryRouter>
      <DryRunDialog
        kind="agent"
        config={config}
        estimate={estimate}
        language="en"
        onClose={vi.fn()}
      />
    </MemoryRouter>
  )

const built = {
  system_prompt: 'system text',
  user_prompt: 'user text',
  candidate_count: 1,
  candidates: [{ symbol: 'BTCUSDT', sources: ['static'] }],
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.models = [
    { id: 'off', name: 'Disabled', provider: 'openai', enabled: false },
    { id: 'm1', name: 'DeepSeek AI', provider: 'deepseek', enabled: true },
  ]
})
afterEach(cleanup)

it('states the cost before running and only then calls the model', async () => {
  mocks.dryRun.mockResolvedValue({ ...built, agent_response: 'hold' })
  show()
  expect(
    screen.getByText(/about 6,434 tokens to DeepSeek AI on your API key/)
  ).toBeVisible()
  expect(mocks.dryRun).not.toHaveBeenCalled()

  fireEvent.click(screen.getByRole('button', { name: 'Run test' }))
  await waitFor(() =>
    expect(mocks.dryRun).toHaveBeenCalledWith({
      kind: 'agent',
      config,
      agent_model_id: 'm1',
    })
  )
  expect(await screen.findByText('hold')).toBeInTheDocument()
})

it('previews prompts without a model', async () => {
  mocks.dryRun.mockResolvedValue(built)
  show()
  fireEvent.click(screen.getByRole('button', { name: 'Preview prompts' }))
  await waitFor(() =>
    expect(mocks.dryRun).toHaveBeenCalledWith({
      kind: 'agent',
      config,
      agent_model_id: undefined,
    })
  )
  expect(await screen.findByText('system text')).toBeInTheDocument()
  expect(screen.getByText('BTCUSDT')).toBeVisible()
})

it('shows why the model call failed, keeping the prompts', async () => {
  mocks.dryRun.mockResolvedValue({
    ...built,
    agent_response: 'failed',
    agent_error: 'invalid key',
  })
  show()
  fireEvent.click(screen.getByRole('button', { name: 'Run test' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('invalid key')
  expect(screen.getByText('user text')).toBeInTheDocument()
})

it('cannot run without an enabled model, but can still preview', () => {
  mocks.models = [
    { id: 'off', name: 'Disabled', provider: 'openai', enabled: false },
  ]
  show()
  expect(screen.getByText('No model is enabled')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Run test' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Preview prompts' })).toBeEnabled()
})
