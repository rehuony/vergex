import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { StrategyTestDialog } from '@/components/strategy/strategy-test-dialog'
import type { StrategyConfig, StrategyTokenEstimate } from '@/types'

const mocks = vi.hoisted(() => ({
  testRun: vi.fn(),
  models: [] as unknown[] | undefined,
}))
vi.mock('@/api', () => ({ api: { testRunStrategy: mocks.testRun } }))
vi.mock('@/hooks/use-connections', () => ({
  useModelConfigs: () => ({ data: mocks.models }),
}))

const config: StrategyConfig = { strategy_type: 'ai_trading' }
const estimate = { total: 6434 } as StrategyTokenEstimate

const show = () =>
  render(
    <MemoryRouter>
      <StrategyTestDialog
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
  mocks.testRun.mockResolvedValue({ ...built, ai_response: 'hold' })
  show()
  expect(
    screen.getByText(/about 6,434 tokens to DeepSeek AI on your API key/)
  ).toBeVisible()
  expect(mocks.testRun).not.toHaveBeenCalled()

  fireEvent.click(screen.getByRole('button', { name: 'Run test' }))
  await waitFor(() =>
    expect(mocks.testRun).toHaveBeenCalledWith({
      config,
      ai_model_id: 'm1',
      run_real_ai: true,
    })
  )
  expect(await screen.findByText('hold')).toBeInTheDocument()
})

it('previews prompts without a model and hides the server reminder', async () => {
  mocks.testRun.mockResolvedValue({
    ...built,
    ai_response: 'Please select an AI model',
  })
  show()
  fireEvent.click(screen.getByRole('button', { name: 'Preview prompts' }))
  await waitFor(() =>
    expect(mocks.testRun).toHaveBeenCalledWith({
      config,
      ai_model_id: '',
      run_real_ai: false,
    })
  )
  expect(await screen.findByText('system text')).toBeInTheDocument()
  expect(screen.getByText('BTCUSDT')).toBeVisible()
  expect(screen.queryByText('Please select an AI model')).toBeNull()
})

it('shows why the model call failed, keeping the prompts', async () => {
  mocks.testRun.mockResolvedValue({
    ...built,
    ai_response: 'failed',
    ai_error: 'invalid key',
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
  expect(screen.getByText('No AI model is enabled.')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Run test' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Preview prompts' })).toBeEnabled()
})
