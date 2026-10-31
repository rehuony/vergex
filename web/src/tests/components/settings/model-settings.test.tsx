import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { SWRConfig } from 'swr'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ModelSettings } from '@/components/settings/model-settings'

const mocks = vi.hoisted(() => ({
  models: vi.fn(),
  update: vi.fn(),
  test: vi.fn(),
  traders: vi.fn(),
  confirm: vi.fn(),
}))
vi.mock('@/api', () => ({
  api: {
    getModelConfigs: mocks.models,
    getSupportedModels: async () => [
      { id: 'deepseek', name: 'DeepSeek AI', provider: 'deepseek' },
      { id: 'openai', name: 'OpenAI', provider: 'openai' },
    ],
    getTraders: mocks.traders,
    updateModelConfigs: mocks.update,
    testModelConfig: mocks.test,
  },
}))
vi.mock('@/components/common/notify', () => ({ confirmToast: mocks.confirm }))

beforeEach(() => {
  mocks.models.mockResolvedValue([
    {
      id: 'ai',
      name: 'My AI',
      provider: 'deepseek',
      enabled: true,
      has_api_key: true,
      customModelName: 'configured-model',
    },
  ])
  mocks.update.mockResolvedValue(undefined)
  mocks.traders.mockResolvedValue([])
  mocks.confirm.mockResolvedValue(true)
})
afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

const show = () =>
  render(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <ModelSettings language="en" />
    </SWRConfig>
  )

it('adds a provider that is not connected yet and sends only that one', async () => {
  show()
  expect(await screen.findByText('configured-model')).toBeVisible()
  expect(screen.getByText('Enabled')).toBeVisible()

  fireEvent.click(screen.getByRole('button', { name: 'Add AI model' }))
  // DeepSeek is connected already, so OpenAI is the only choice.
  expect(screen.getByRole('combobox', { name: 'Provider' })).toHaveTextContent(
    'OpenAI'
  )
  const save = screen.getByRole('button', { name: 'Save Configuration' })
  expect(save).toBeDisabled()
  fireEvent.change(screen.getByLabelText('API Key'), {
    target: { value: 'fixture-key' },
  })
  fireEvent.click(save)

  await waitFor(() =>
    expect(mocks.update).toHaveBeenCalledWith({
      models: {
        openai: {
          enabled: true,
          api_key: 'fixture-key',
          custom_api_url: '',
          custom_model_name: '',
        },
      },
    })
  )
  expect(
    await screen.findByRole('button', { name: 'Add AI model' })
  ).toBeVisible()
})

it('keeps the saved key when an edit leaves it blank', async () => {
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Edit My AI' }))
  expect(screen.getByLabelText('Model name')).toHaveValue('configured-model')
  fireEvent.change(screen.getByLabelText('Model name'), {
    target: { value: 'deepseek-reasoner' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save Configuration' }))
  await waitFor(() =>
    expect(mocks.update).toHaveBeenCalledWith({
      models: {
        ai: {
          enabled: true,
          api_key: '',
          custom_api_url: '',
          custom_model_name: 'deepseek-reasoner',
        },
      },
    })
  )
})

it('will not remove a model an Autopilot uses', async () => {
  mocks.traders.mockResolvedValue([
    { trader_id: 't1', trader_name: 'Alpha', ai_model: 'ai' },
  ])
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Edit My AI' }))
  await waitFor(() => expect(mocks.traders).toHaveBeenCalled())
  await new Promise((resolve) => setTimeout(resolve, 0))
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
  expect(
    await screen.findByText('Used by Alpha. Delete those Autopilots first.')
  ).toBeVisible()
  expect(mocks.confirm).not.toHaveBeenCalled()
  expect(mocks.update).not.toHaveBeenCalled()
})

it('centres a note while no model is connected', async () => {
  mocks.models.mockResolvedValue([])
  show()
  expect(await screen.findByText('No AI model connected yet')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Add AI model' })).toBeEnabled()
})

it('tests the form values without saving and clears the result on edit', async () => {
  mocks.test.mockResolvedValue({
    ok: true,
    model: 'configured-model',
    latency_ms: 840,
  })
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Edit My AI' }))
  fireEvent.click(screen.getByRole('button', { name: 'Test' }))

  // A blank key field asks the server to use the saved key.
  expect(mocks.test).toHaveBeenCalledWith({
    model_id: 'ai',
    api_key: '',
    custom_api_url: '',
    custom_model_name: 'configured-model',
  })
  expect(
    await screen.findByText('Connected · configured-model · 0.8 s')
  ).toBeVisible()
  expect(mocks.update).not.toHaveBeenCalled()

  fireEvent.change(screen.getByLabelText('Model name'), {
    target: { value: 'other-model' },
  })
  expect(
    screen.queryByText('Connected · configured-model · 0.8 s')
  ).not.toBeInTheDocument()

  mocks.test.mockResolvedValue({
    ok: false,
    model: 'other-model',
    error: 'HTTP 404: model not found',
  })
  fireEvent.click(screen.getByRole('button', { name: 'Test' }))
  expect(await screen.findByText('HTTP 404: model not found')).toBeVisible()
})
