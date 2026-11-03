import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { AssistantDock } from '@/components/assistant/assistant-dock'
import { confirmToast } from '@/components/common/notify'
import { api } from '@/api'
import { streamAssistant, type AssistantEvent } from '@/api/assistant'
import { QueryWrapper } from '@/tests/query-wrapper'

vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@/api/assistant', async (original) => ({
  ...(await original<typeof import('@/api/assistant')>()),
  streamAssistant: vi.fn(),
}))
vi.mock('@/components/common/notify', async (original) => ({
  ...(await original<typeof import('@/components/common/notify')>()),
  confirmToast: vi.fn(),
}))
vi.mock('@/api', () => ({
  api: {
    clearAssistantConversations: vi.fn(),
    getModelConfigs: vi.fn(),
    getAvailableModels: vi.fn(),
    getAssistantConversations: vi.fn(),
    getAssistantConversation: vi.fn(),
    answerAssistant: vi.fn(),
    deleteAssistantConversation: vi.fn(),
  },
}))

const mocked = vi.mocked(api)
const stream = vi.mocked(streamAssistant)
const model = { id: 'm1', name: 'DeepSeek', enabled: true, has_api_key: true }
const confirm = {
  id: 'c1',
  lines: ['stop trader "Grid" on account "Main" (real money)'],
}

// A turn that says these things, then ends.
const says = (...events: AssistantEvent[]) =>
  stream.mockImplementation(async (_body, _signal, onEvent) => {
    events.forEach(onEvent)
  })

function openDock() {
  render(
    <MemoryRouter>
      <QueryWrapper>
        <AssistantDock />
      </QueryWrapper>
    </MemoryRouter>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Open the assistant' }))
}

const send = async (text: string) => {
  const input = await screen.findByRole('textbox', {
    name: 'Message to the assistant',
  })
  fireEvent.change(input, { target: { value: text } })
  fireEvent.keyDown(input, { key: 'Enter' })
}

beforeEach(() => {
  localStorage.clear()
  vi.resetAllMocks()
  mocked.getModelConfigs.mockResolvedValue([model] as never)
  mocked.getAvailableModels.mockResolvedValue([])
  mocked.getAssistantConversations.mockResolvedValue([])
})
afterEach(cleanup)

describe('AssistantDock', () => {
  it('reads nothing until its window is opened', async () => {
    render(
      <MemoryRouter>
        <QueryWrapper>
          <AssistantDock />
        </QueryWrapper>
      </MemoryRouter>
    )
    await waitFor(() => expect(mocked.getModelConfigs).toHaveBeenCalled())
    expect(mocked.getAssistantConversations).not.toHaveBeenCalled()
    expect(screen.queryByRole('complementary')).toBeNull()
  })

  it('is no dialog, since Dialog finds the topmost one by that role', async () => {
    openDock()
    expect(await screen.findByRole('complementary')).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('says a turn and what it asks to confirm in the words it was given', async () => {
    says(
      { type: 'stage', data: { text: 'Reading traders…' } },
      { type: 'delta', data: { text: 'Half an' } },
      {
        type: 'reply',
        data: { text: 'It waits for you', confirm, changed: false },
      },
      { type: 'done', data: { ok: true } }
    )
    openDock()
    await send('stop Grid')
    expect(await screen.findByText('It waits for you')).toBeInTheDocument()
    expect(screen.getByText(confirm.lines[0])).toBeInTheDocument()
    expect(screen.queryByText('Half an')).toBeNull()
    expect(stream.mock.calls[0][0]).toMatchObject({
      message: 'stop Grid',
      agent_model_id: 'm1',
      language: 'en',
    })
  })

  it('answers what waits, and goes by what the server says came of it', async () => {
    says(
      {
        type: 'reply',
        data: { text: 'It waits for you', confirm, changed: false },
      },
      { type: 'done', data: { ok: true } }
    )
    mocked.answerAssistant.mockResolvedValue({
      live: true,
      text: 'Done: stop trader "Grid"',
      confirm: null,
      changed: true,
    })
    openDock()
    await send('stop Grid')
    fireEvent.click(await screen.findByRole('button', { name: 'Confirm' }))
    expect(
      await screen.findByText('Done: stop trader "Grid"')
    ).toBeInTheDocument()
    expect(mocked.answerAssistant.mock.calls[0].slice(1)).toEqual([
      'c1',
      true,
      'en',
    ])
    expect(screen.queryByRole('button', { name: 'Confirm' })).toBeNull()
  })

  it('marks what cannot be taken back, and leads where the server says', async () => {
    says(
      {
        type: 'reply',
        data: {
          text: 'It waits for you',
          confirm: { ...confirm, destructive: true },
          changed: false,
        },
      },
      { type: 'done', data: { ok: true } }
    )
    mocked.answerAssistant.mockResolvedValue({
      live: true,
      text: 'Done',
      confirm: null,
      changed: true,
      links: [
        { to: '/traders?id=t1&tab=settings', label: 'Open this trader' },
        { to: 'https://example.com/', label: 'Somewhere else' },
      ],
    })
    openDock()
    await send('delete Grid')
    const button = await screen.findByRole('button', { name: 'Confirm' })
    expect(button.className).toContain('bg-vergex-danger')
    fireEvent.click(button)
    const link = await screen.findByRole('link', { name: 'Open this trader' })
    expect(link).toHaveAttribute('href', '/traders?id=t1&tab=settings')
    expect(screen.queryByRole('link', { name: 'Somewhere else' })).toBeNull()
    // Refreshing the pages after a change leaves this conversation as it is.
    expect(mocked.getAssistantConversation).not.toHaveBeenCalled()
  })

  it('says an old answer did nothing', async () => {
    says(
      { type: 'reply', data: { text: 'Waiting', confirm, changed: false } },
      { type: 'done', data: { ok: true } }
    )
    mocked.answerAssistant.mockResolvedValue({
      live: false,
      text: '',
      confirm: null,
      changed: false,
    })
    openDock()
    await send('stop Grid')
    fireEvent.click(await screen.findByRole('button', { name: 'Confirm' }))
    expect(
      await screen.findByText(/no longer waits for an answer/)
    ).toBeInTheDocument()
  })

  it('offers to ask again after a failure, without saying it twice', async () => {
    says(
      {
        type: 'error',
        data: {
          message: 'The provider is down',
          confirm: null,
          changed: false,
        },
      },
      { type: 'done', data: { ok: false } }
    )
    openDock()
    await send('hello')
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'The provider is down'
    )
    says(
      { type: 'reply', data: { text: 'Hi', confirm: null, changed: false } },
      { type: 'done', data: { ok: true } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByText('Hi')).toBeInTheDocument()
    expect(screen.getAllByText('hello', { selector: 'div' })).toHaveLength(1)
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('opens on the latest conversation with what still waits', async () => {
    mocked.getAssistantConversations.mockResolvedValue([
      { id: 'a', title: 'Latest', updated_at: '2026-11-03T10:00:00Z' },
      { id: 'b', title: 'Older', updated_at: '2026-11-02T10:00:00Z' },
    ])
    mocked.getAssistantConversation.mockResolvedValue({
      messages: [{ role: 'user', content: 'stop it' }],
      confirm,
    })
    openDock()
    expect(await screen.findByText(confirm.lines[0])).toBeInTheDocument()
    expect(mocked.getAssistantConversation).toHaveBeenCalledWith('a')
  })

  it('leads to the models when none is set up', async () => {
    mocked.getModelConfigs.mockResolvedValue([])
    openDock()
    expect(
      await screen.findByText('No model is set up yet')
    ).toBeInTheDocument()
    expect(screen.getAllByText('Add a model').length).toBeGreaterThan(0)
  })

  it('always offers to search the conversations, and to open one', async () => {
    mocked.getAssistantConversations.mockResolvedValue([])
    openDock()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Conversations' })
    )
    expect(
      screen.getByRole('textbox', { name: 'Search conversations' })
    ).toBeInTheDocument()
    expect(await screen.findByText('No conversations yet')).toBeInTheDocument()
  })

  it('filters conversations as you type and opens the one pressed', async () => {
    mocked.getAssistantConversations.mockResolvedValue([
      { id: 'a', title: 'About my grid', updated_at: '2026-11-03T10:00:00Z' },
      { id: 'b', title: 'Model costs', updated_at: '2026-11-02T10:00:00Z' },
    ])
    mocked.getAssistantConversation.mockImplementation(async (id) => ({
      messages: [{ role: 'user', content: `said in ${id}` }],
      confirm: null,
    }))
    openDock()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Conversations' })
    )
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Search conversations' }),
      { target: { value: 'cost' } }
    )
    expect(screen.queryByText('About my grid')).toBeNull()
    fireEvent.click(await screen.findByText('Model costs'))
    expect(await screen.findByText('said in b')).toBeInTheDocument()
  })

  it('asks before clearing the conversations, and starts afresh after', async () => {
    const rows = [
      { id: 'a', title: 'About my grid', updated_at: '2026-11-03T10:00:00Z' },
    ]
    mocked.getAssistantConversations.mockResolvedValue(rows)
    mocked.getAssistantConversation.mockResolvedValue({
      messages: [{ role: 'user', content: 'said in a' }],
      confirm: null,
    })
    openDock()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Conversations' })
    )
    await screen.findByText('About my grid')

    vi.mocked(confirmToast).mockResolvedValue(false)
    fireEvent.click(screen.getByRole('button', { name: 'Clear conversations' }))
    await waitFor(() => expect(confirmToast).toHaveBeenCalledOnce())
    expect(vi.mocked(confirmToast).mock.calls[0][1]).toMatchObject({
      tone: 'danger',
      okText: 'Clear conversations',
    })
    expect(mocked.clearAssistantConversations).not.toHaveBeenCalled()

    vi.mocked(confirmToast).mockResolvedValue(true)
    mocked.clearAssistantConversations.mockResolvedValue()
    mocked.getAssistantConversations.mockResolvedValue([])
    fireEvent.click(screen.getByRole('button', { name: 'Clear conversations' }))
    expect(await screen.findByText('No conversations yet')).toBeInTheDocument()
    expect(mocked.clearAssistantConversations).toHaveBeenCalledOnce()
    // Nothing goes back to read what is gone, and the window is on a new conversation.
    expect(mocked.getAssistantConversation).toHaveBeenCalledTimes(1)
    fireEvent.click(
      screen.getByRole('button', { name: 'Back to the conversation' })
    )
    expect(
      await screen.findByText('Ask about what is on this page')
    ).toBeInTheDocument()
  })

  it('has nothing to ask about with no conversations', async () => {
    openDock()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Conversations' })
    )
    await screen.findByText('No conversations yet')
    fireEvent.click(screen.getByRole('button', { name: 'Clear conversations' }))
    expect(confirmToast).not.toHaveBeenCalled()
    expect(mocked.clearAssistantConversations).not.toHaveBeenCalled()
  })

  it('lays out what waits in parts, pinned by the input, and says what the button does', async () => {
    says(
      {
        type: 'reply',
        data: {
          text: 'Two things wait',
          changed: false,
          confirm: {
            id: 'c2',
            lines: ['start trader "Grid"', 'delete trader "Old"'],
            destructive: true,
            items: [
              {
                action: 'start',
                target: 'Grid',
                account: 'Test',
                money: 'testnet',
                details: ['grid'],
              },
              {
                action: 'delete',
                target: 'Old',
                account: 'Main',
                money: 'real',
                destructive: true,
              },
            ],
          },
        },
      },
      { type: 'done', data: { ok: true } }
    )
    openDock()
    await send('start Grid and delete Old')
    const card = await screen.findByRole('region', {
      name: 'Waiting for your confirmation',
    })
    expect(card).toHaveTextContent('Waiting for your confirmation · 2')
    expect(card).toHaveTextContent('Start trader Grid')
    // The account goes on the action's line, and what else it sets under it.
    expect(card).toHaveTextContent('Start trader Grid · Test')
    expect(card).toHaveTextContent('grid')
    expect(card).toHaveTextContent('Testnet')
    expect(card).toHaveTextContent('Real money')
    // It is not part of what scrolls.
    expect(screen.getByText('Two things wait').closest('section')).toBeNull()
    const run = screen.getByRole('button', { name: 'Run all 2' })
    expect(run.className).toContain('bg-vergex-danger')
    expect(
      screen.getByRole('textbox', { name: 'Message to the assistant' })
    ).toHaveAttribute('placeholder', 'Or write something else to cancel it')
  })

  it('names the one action on its button, and says how each went', async () => {
    says(
      {
        type: 'reply',
        data: {
          text: '',
          changed: false,
          confirm: {
            id: 'c3',
            lines: ['start trader "Grid"'],
            items: [{ action: 'start', target: 'Grid', money: 'real' }],
          },
        },
      },
      { type: 'done', data: { ok: true } }
    )
    mocked.answerAssistant.mockResolvedValue({
      live: true,
      text: '',
      confirm: null,
      changed: true,
      results: [
        { ok: true, text: 'start trader "Grid"' },
        { ok: false, text: 'stop trader "B"', note: 'it refused' },
      ],
    })
    openDock()
    await send('start Grid')
    fireEvent.click(await screen.findByRole('button', { name: 'Start trader' }))
    const results = await screen.findByRole('list', { name: 'What came of it' })
    expect(results).toHaveTextContent('start trader "Grid"')
    expect(results).toHaveTextContent('it refused')
    expect(screen.getByLabelText('Failed')).toBeInTheDocument()
    expect(
      screen.queryByRole('region', { name: 'Waiting for your confirmation' })
    ).toBeNull()
  })
})
