import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ModelSettings } from '@/components/settings/model-settings'

const mocks = vi.hoisted(() => ({
  models: vi.fn(),
  update: vi.fn(),
  test: vi.fn(),
  list: vi.fn(),
  traders: vi.fn(),
  confirm: vi.fn(),
}))
vi.mock('@/api', () => ({
  api: {
    getModelConfigs: mocks.models,
    getSupportedModels: async () => [
      {
        id: 'deepseek',
        name: 'DeepSeek AI',
        provider: 'deepseek',
        defaultBaseUrl: 'https://api.deepseek.com',
      },
      {
        id: 'openai',
        name: 'OpenAI',
        provider: 'openai',
        defaultBaseUrl: 'https://api.openai.com/v1',
      },
    ],
    getTraders: mocks.traders,
    updateModelConfigs: mocks.update,
    testModelConfig: mocks.test,
    listAvailableModels: mocks.list,
  },
}))
vi.mock('@/components/common/notify', () => ({ confirmToast: mocks.confirm }))

beforeEach(() => {
  Element.prototype.scrollIntoView = vi.fn()
  mocks.models.mockResolvedValue([
    {
      id: 'agent',
      name: 'My AI',
      provider: 'deepseek',
      enabled: true,
      has_api_key: true,
      customModelName: 'configured-model',
    },
  ])
  mocks.update.mockResolvedValue(undefined)
  mocks.list.mockResolvedValue([])
  mocks.traders.mockResolvedValue([])
  mocks.confirm.mockResolvedValue(true)
})
afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

const show = () =>
  render(
    <QueryWrapper>
      <ModelSettings language="en" />
    </QueryWrapper>
  )

const nameField = () => screen.getByLabelText('Model name')
const typeModel = (name: string) =>
  fireEvent.change(nameField(), { target: { value: name } })
const openList = () =>
  fireEvent.click(screen.getByRole('button', { name: 'Model list' }))

it('adds a provider that is not connected yet and sends only that one', async () => {
  show()
  expect(await screen.findByText('configured-model')).toBeVisible()
  expect(screen.getByText('Enabled')).toBeVisible()

  fireEvent.click(screen.getByRole('button', { name: 'Add model' }))
  // DeepSeek is connected already, so OpenAI is the only choice.
  expect(screen.getByRole('combobox', { name: 'Provider' })).toHaveTextContent(
    'OpenAI'
  )
  const save = screen.getByRole('button', { name: 'Save' })
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
  expect(await screen.findByRole('button', { name: 'Add model' })).toBeVisible()
})

it('keeps the saved key when an edit leaves it blank', async () => {
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Edit My AI' }))
  expect(nameField()).toHaveValue('configured-model')
  typeModel('deepseek-reasoner')
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(mocks.update).toHaveBeenCalledWith({
      models: {
        agent: {
          enabled: true,
          api_key: '',
          custom_api_url: '',
          custom_model_name: 'deepseek-reasoner',
        },
      },
    })
  )
})

it('will not remove a model a trader uses', async () => {
  mocks.traders.mockResolvedValue([
    { trader_id: 't1', trader_name: 'Alpha', agent_model: 'agent' },
  ])
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Edit My AI' }))
  await waitFor(() => expect(mocks.traders).toHaveBeenCalled())
  await new Promise((resolve) => setTimeout(resolve, 0))
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
  expect(
    await screen.findByText('Used by Alpha. Delete those traders first.')
  ).toBeVisible()
  expect(mocks.confirm).not.toHaveBeenCalled()
  expect(mocks.update).not.toHaveBeenCalled()
})

it('centres a note while no model is connected', async () => {
  mocks.models.mockResolvedValue([])
  show()
  expect(await screen.findByText('No model connected yet')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Add model' })).toBeEnabled()
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
    agent_model_id: 'agent',
    api_key: '',
    custom_api_url: '',
    custom_model_name: 'configured-model',
  })
  expect(
    await screen.findByText('Connected · configured-model · 0.8 s')
  ).toBeVisible()
  expect(mocks.update).not.toHaveBeenCalled()

  typeModel('other-model')
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

it("fills in the provider's endpoint, and saves it as none", async () => {
  show()
  const add = await screen.findByRole('button', { name: 'Add model' })
  await waitFor(() => expect(add).toBeEnabled())
  fireEvent.click(add)
  const url = screen.getByLabelText('Base URL')
  expect(url).toHaveValue('https://api.openai.com/v1')
  fireEvent.change(screen.getByLabelText('API Key'), {
    target: { value: 'sk-new' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(mocks.update).toHaveBeenCalledWith({
      models: {
        openai: {
          enabled: true,
          api_key: 'sk-new',
          custom_api_url: '',
          custom_model_name: '',
        },
      },
    })
  )
})

it('keeps a different endpoint, and shows it again', async () => {
  mocks.models.mockResolvedValue([
    {
      id: 'agent',
      name: 'My AI',
      provider: 'deepseek',
      enabled: true,
      has_api_key: true,
      customApiUrl: 'https://proxy.example.com/v1',
    },
  ])
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Edit My AI' }))
  expect(screen.getByLabelText('Base URL')).toHaveValue(
    'https://proxy.example.com/v1'
  )
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(mocks.update).toHaveBeenCalledWith({
      models: {
        agent: expect.objectContaining({
          custom_api_url: 'https://proxy.example.com/v1',
        }),
      },
    })
  )
})

it('takes a name from the list or as typed, and holds only the one in the field', async () => {
  mocks.list.mockResolvedValue(['listed-a', 'listed-b'])
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Edit My AI' }))
  // It asks with the saved key, which it never sees.
  await waitFor(() =>
    expect(mocks.list).toHaveBeenCalledWith({
      agent_model_id: 'agent',
      api_key: '',
      custom_api_url: '',
    })
  )
  openList()
  expect(await screen.findByRole('option', { name: 'listed-b' })).toBeVisible()
  // The list holds what the provider has and the provider's default, and nothing else:
  // not a search box, and not a name of the user's own.
  expect(
    screen.getAllByRole('option').map((option) => option.textContent)
  ).toEqual(['deepseek-v4-pro', 'listed-a', 'listed-b'])
  expect(screen.getAllByRole('combobox')).toEqual([nameField()])

  fireEvent.click(screen.getByRole('option', { name: 'listed-b' }))
  expect(nameField()).toHaveValue('listed-b')

  typeModel('my-proxy')
  openList()
  expect(screen.queryByRole('option', { name: 'my-proxy' })).toBeNull()
  fireEvent.click(screen.getByRole('option', { name: 'listed-a' }))
  expect(nameField()).toHaveValue('listed-a')

  // Picked and typed names are gone once replaced.
  openList()
  expect(screen.queryByRole('option', { name: 'my-proxy' })).toBeNull()
  expect(screen.getByRole('option', { name: 'listed-a' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  fireEvent.keyDown(nameField(), { key: 'ArrowDown' })
  fireEvent.keyDown(nameField(), { key: 'Enter' })
  expect(nameField()).toHaveValue('listed-b')

  typeModel('my-proxy')
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(mocks.update).toHaveBeenCalledWith({
      models: {
        agent: expect.objectContaining({ custom_model_name: 'my-proxy' }),
      },
    })
  )
})

it('fills in the provider default, and saves it as no name of its own', async () => {
  mocks.models.mockResolvedValue([
    {
      id: 'agent',
      name: 'My AI',
      provider: 'deepseek',
      enabled: true,
      has_api_key: true,
    },
  ])
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Edit My AI' }))
  expect(nameField()).toHaveValue('deepseek-v4-pro')
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(mocks.update).toHaveBeenCalledWith({
      models: { agent: expect.objectContaining({ custom_model_name: '' }) },
    })
  )
})

it('still takes a typed name when the provider will not list', async () => {
  mocks.list.mockRejectedValue(new Error('no list'))
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Edit My AI' }))
  await waitFor(() => expect(mocks.list).toHaveBeenCalled())
  typeModel('typed-model')
  expect(nameField()).toHaveValue('typed-model')
  expect(screen.queryByRole('alert')).toBeNull()
})
