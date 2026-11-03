import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { TelegramSettings } from '@/components/settings/telegram-settings'
import type { TelegramVisitor } from '@/types'
import { QueryWrapper } from '@/tests/query-wrapper'

const inQuery = (language: 'en' = 'en') =>
  render(
    <QueryWrapper>
      <TelegramSettings language={language} />
    </QueryWrapper>
  )

const mocks = vi.hoisted(() => ({
  config: vi.fn(),
  models: vi.fn(),
  alerts: vi.fn(),
  test: vi.fn(),
  bindLink: vi.fn(),
  save: vi.fn(),
  model: vi.fn(),
  unbind: vi.fn(),
  clear: vi.fn(),
  confirm: vi.fn(),
  visitors: vi.fn(),
}))
vi.mock('@/components/common/notify', () => ({
  confirmToast: mocks.confirm,
}))
vi.mock('@/api', () => ({
  api: {
    getTelegramConfig: mocks.config,
    getModelConfigs: mocks.models,
    updateTelegramAlerts: mocks.alerts,
    testTelegramAlert: mocks.test,
    createTelegramBindLink: mocks.bindLink,
    updateTelegramConfig: mocks.save,
    updateTelegramModel: mocks.model,
    unbindTelegram: mocks.unbind,
    clearTelegramToken: mocks.clear,
    getTelegramVisitors: mocks.visitors,
  },
}))

beforeEach(() => {
  vi.clearAllMocks()
  Element.prototype.scrollIntoView = vi.fn()
  mocks.models.mockResolvedValue([])
  mocks.visitors.mockResolvedValue([])
  mocks.config.mockResolvedValue({
    token_masked: '***abcdef',
    is_bound: true,
    alerts: { order_rejected: true, safe_mode: true, forecast: false },
  })
})
afterEach(cleanup)

it('turns alert kinds on and off and shows only those the server knows', async () => {
  mocks.alerts.mockResolvedValue({})
  inQuery()

  const rejected = await screen.findByRole('button', {
    name: 'Rejected orders',
  })
  expect(rejected).toHaveAttribute('aria-pressed', 'true')
  expect(
    screen.getByRole('button', { name: 'Forecast signals' })
  ).toHaveAttribute('aria-pressed', 'false')
  expect(screen.queryByRole('button', { name: 'Closed positions' })).toBeNull()

  fireEvent.click(rejected)
  await waitFor(() => expect(rejected).toHaveAttribute('aria-pressed', 'false'))
  await waitFor(() =>
    expect(mocks.alerts).toHaveBeenCalledWith({ order_rejected: false })
  )
})

it('lands every press of a quick run, one after another', async () => {
  let finishFirst = () => {}
  mocks.alerts.mockImplementationOnce(
    () => new Promise<void>((resolve) => (finishFirst = resolve))
  )
  mocks.alerts.mockResolvedValue({})
  inQuery()

  fireEvent.click(await screen.findByRole('button', { name: 'Safe mode' }))
  fireEvent.click(screen.getByRole('button', { name: 'Forecast signals' }))
  await waitFor(() => expect(mocks.alerts).toHaveBeenCalledTimes(1))
  expect(
    screen.getByRole('button', { name: 'Forecast signals' })
  ).toHaveAttribute('aria-pressed', 'true')

  finishFirst()
  await waitFor(() =>
    expect(mocks.alerts).toHaveBeenLastCalledWith({ forecast: true })
  )
})

it('takes back a press that could not be saved', async () => {
  mocks.alerts.mockRejectedValue(new Error('down'))
  inQuery()
  const safeMode = await screen.findByRole('button', { name: 'Safe mode' })
  fireEvent.click(safeMode)
  await waitFor(() => expect(mocks.config).toHaveBeenCalledTimes(2))
  await waitFor(() => expect(safeMode).toHaveAttribute('aria-pressed', 'true'))
})

it('names the bound user by name and id, or by id alone', async () => {
  mocks.config.mockResolvedValue({
    token_masked: '***abcdef',
    is_bound: true,
    username: '@alice',
    bound_chat_id: 12345,
    bound_at: new Date('2026-11-02T09:05:00').toISOString(),
    alerts: {},
  })
  inQuery()
  expect(await screen.findByText('@alice · ID 12345')).toBeVisible()
  expect(screen.getByText('Bound 11-02 09:05')).toBeVisible()
  cleanup()

  mocks.config.mockResolvedValue({
    token_masked: '***abcdef',
    is_bound: true,
    username: '',
    bound_chat_id: 12345,
    alerts: {},
  })
  inQuery()
  expect(await screen.findByText('ID 12345')).toBeVisible()
})

it('asks before unbinding, naming who', async () => {
  mocks.config.mockResolvedValue({
    token_masked: '***abcdef',
    is_bound: true,
    username: '@alice',
    bound_chat_id: 12345,
    alerts: {},
  })
  mocks.confirm.mockResolvedValue(false)
  inQuery()
  fireEvent.click(await screen.findByRole('button', { name: 'Unbind account' }))
  await waitFor(() => expect(mocks.confirm).toHaveBeenCalled())
  expect(mocks.confirm.mock.calls[0][0]).toContain('@alice · ID 12345')
  expect(mocks.unbind).not.toHaveBeenCalled()

  mocks.confirm.mockResolvedValue(true)
  mocks.unbind.mockResolvedValue(undefined)
  fireEvent.click(screen.getByRole('button', { name: 'Unbind account' }))
  await waitFor(() => expect(mocks.unbind).toHaveBeenCalled())
})

it('saves the model as it is chosen, with nothing left to save by hand', async () => {
  mocks.models.mockResolvedValue([
    { id: 'm1', name: 'DeepSeek', enabled: true },
  ])
  mocks.model.mockResolvedValue(undefined)
  inQuery()
  await screen.findByRole('button', { name: 'Send test' })
  expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
  fireEvent.click(await screen.findByRole('combobox'))
  fireEvent.click(await screen.findByRole('option', { name: 'DeepSeek' }))
  await waitFor(() => expect(mocks.model).toHaveBeenCalledWith('m1'))
})

it('sends a test alert', async () => {
  mocks.test.mockResolvedValue(undefined)
  inQuery()
  fireEvent.click(await screen.findByRole('button', { name: 'Send test' }))
  await waitFor(() => expect(mocks.test).toHaveBeenCalled())
})

it('binds through a link of its own and says when it cannot make one', async () => {
  mocks.config.mockResolvedValue({ token_masked: '***abcdef', is_bound: false })
  mocks.bindLink.mockResolvedValueOnce({
    code: 'abc123',
    link: 'https://t.me/some_bot?start=abc123',
    expires_at: '2026-11-02T00:10:00Z',
  })
  const { unmount } = inQuery()
  const link = await screen.findByRole('link', { name: 'Open binding link' })
  expect(link).toHaveAttribute('href', 'https://t.me/some_bot?start=abc123')
  expect(screen.getByText('abc123')).toBeInTheDocument()
  expect(screen.getByTitle('Scan')).toBeInTheDocument()
  unmount()

  mocks.bindLink.mockRejectedValueOnce(
    new Error('Telegram does not recognise this bot token')
  )
  inQuery()
  expect(
    await screen.findByText('Telegram does not recognise this bot token')
  ).toBeInTheDocument()
  expect(screen.queryByRole('link', { name: 'Open binding link' })).toBeNull()
})

it('replaces a code that runs out while it is shown', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    mocks.config.mockResolvedValue({
      token_masked: '***abcdef',
      is_bound: false,
      bot_state: 'running',
    })
    mocks.bindLink.mockResolvedValueOnce({
      code: 'first',
      link: 'https://t.me/some_bot?start=first',
      expires_at: new Date(Date.now() + 10 * 60000).toISOString(),
    })
    mocks.bindLink.mockResolvedValueOnce({
      code: 'second',
      link: 'https://t.me/some_bot?start=second',
      expires_at: new Date(Date.now() + 20 * 60000).toISOString(),
    })
    inQuery()
    expect(await screen.findByText('first')).toBeVisible()
    expect(screen.getByText('Waiting to be bound')).toBeVisible()
    expect(screen.getByText(/New code in (10:00|09:5\d)/)).toBeVisible()
    await vi.advanceTimersByTimeAsync(10 * 60000)
    expect(await screen.findByText('second')).toBeVisible()
  } finally {
    vi.useRealTimers()
  }
})

it('says when the commands are not in place yet', async () => {
  mocks.config.mockResolvedValue({
    token_masked: '***abcdef',
    is_bound: true,
    bot_state: 'degraded',
    bot_error: 'commands not published: Bad Gateway',
  })
  inQuery()
  expect(await screen.findByText('Commands not ready')).toBeInTheDocument()
  expect(
    screen.getByText('commands not published: Bad Gateway')
  ).toBeInTheDocument()
})

it('says how the bot is really doing, whatever is bound', async () => {
  mocks.config.mockResolvedValue({
    token_masked: '***abcdef',
    is_bound: true,
    bot_state: 'failed',
    bot_error: 'start: Unauthorized',
    alerts_error: 'could not reach Telegram',
  })
  inQuery()
  expect(await screen.findByText('Not running')).toBeInTheDocument()
  expect(screen.getByText('start: Unauthorized')).toBeInTheDocument()
  expect(
    screen.getByText(
      'The last alert could not be sent: could not reach Telegram'
    )
  ).toBeInTheDocument()
})

it('names the running bot', async () => {
  mocks.config.mockResolvedValue({
    token_masked: '***abcdef',
    is_bound: true,
    bot_state: 'running',
    bot_username: 'my_bot',
    username: '@alice',
  })
  inQuery()
  expect(await screen.findByText('Running')).toBeInTheDocument()
  expect(screen.getByText('@my_bot')).toBeInTheDocument()
  expect(screen.getByText('@alice')).toBeInTheDocument()
})

it('moves on by itself once the chat is bound', async () => {
  mocks.bindLink.mockResolvedValue({
    code: 'abc123',
    link: 'https://t.me/some_bot?start=abc123',
    expires_at: '2026-11-02T00:10:00Z',
  })
  mocks.config.mockResolvedValueOnce({
    token_masked: '***abcdef',
    is_bound: false,
  })
  mocks.config.mockResolvedValue({
    token_masked: '***abcdef',
    is_bound: true,
    bot_state: 'running',
    alerts: { safe_mode: true },
  })
  inQuery()
  await screen.findByRole('link', { name: 'Open binding link' })
  expect(
    await screen.findByRole('button', { name: 'Safe mode' }, { timeout: 5000 })
  ).toBeInTheDocument()
})

it('says when its settings cannot be read, rather than asking to start over', async () => {
  mocks.config.mockRejectedValue(new Error('down'))
  inQuery()
  expect(
    await screen.findByText('Telegram settings could not be loaded')
  ).toBeInTheDocument()
  expect(screen.queryByLabelText('Bot Token')).toBeNull()
})

it('stays on the token when it is refused', async () => {
  mocks.config.mockResolvedValue({ token_masked: '', is_bound: false })
  mocks.save.mockRejectedValue(
    new Error('Telegram does not recognise this bot token')
  )
  inQuery()
  const input = await screen.findByLabelText('Bot Token')
  fireEvent.change(input, {
    target: { value: '123456789:ABCdefGHIjklmNOPQRstuvwxYZ_abcdefghij' },
  })
  fireEvent.click(screen.getByRole('button', { name: /Save and continue/ }))
  await waitFor(() => expect(mocks.save).toHaveBeenCalled())
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: /Save and continue/ })
    ).toBeEnabled()
  )
  expect(screen.getByLabelText('Bot Token')).toBeInTheDocument()
  expect(
    screen.getByText('Telegram does not recognise this bot token')
  ).toBeVisible()
})

it('says a malformed token in its bar, until it is edited', async () => {
  mocks.config.mockResolvedValue({ token_masked: '', is_bound: false })
  inQuery()
  const input = await screen.findByLabelText('Bot Token')
  fireEvent.change(input, { target: { value: 'not a token' } })
  fireEvent.click(screen.getByRole('button', { name: /Save and continue/ }))
  const refusal = /Invalid Bot Token format/
  expect(await screen.findByText(refusal)).toBeVisible()
  expect(mocks.save).not.toHaveBeenCalled()
  fireEvent.change(input, { target: { value: 'not a token either' } })
  expect(screen.queryByText(refusal)).toBeNull()
})

it('goes back to the first step only by clearing the token, and has no way back', async () => {
  mocks.config.mockResolvedValue({ token_masked: '***abcdef', is_bound: false })
  mocks.bindLink.mockResolvedValue({
    code: 'abc123',
    link: 'https://t.me/some_bot?start=abc123',
    expires_at: '2026-11-02T00:10:00Z',
  })
  mocks.confirm.mockResolvedValue(false)
  inQuery()
  fireEvent.click(await screen.findByRole('button', { name: 'Reconfigure' }))
  await waitFor(() => expect(mocks.confirm).toHaveBeenCalled())
  expect(mocks.clear).not.toHaveBeenCalled()
  expect(screen.queryByLabelText('Bot Token')).toBeNull()

  mocks.confirm.mockResolvedValue(true)
  mocks.clear.mockImplementation(async () => {
    mocks.config.mockResolvedValue({ token_masked: '', is_bound: false })
  })
  fireEvent.click(screen.getByRole('button', { name: 'Reconfigure' }))
  expect(await screen.findByLabelText('Bot Token')).toBeVisible()
  expect(mocks.clear).toHaveBeenCalled()
  expect(screen.queryByRole('button', { name: 'Cancel' })).toBeNull()
  expect(screen.queryByRole('link', { name: 'Open binding link' })).toBeNull()
})

const visitor = (patch: Partial<TelegramVisitor> = {}): TelegramVisitor => ({
  chat_id: 2002,
  sender_id: 2002,
  chat_type: 'private',
  username: 'mallory',
  name: 'Mal Lory',
  attempts: 3,
  last_text: '/start',
  first_at: new Date('2026-11-01T08:00:00').getTime(),
  last_at: new Date('2026-11-02T09:05:00').getTime(),
  ...patch,
})

it('keeps its buttons out of what scrolls', async () => {
  inQuery()
  const alert = await screen.findByRole('button', { name: 'Safe mode' })
  expect(alert.closest('.overflow-y-auto')).not.toBeNull()
  expect(
    screen
      .getByRole('button', { name: 'Send test' })
      .closest('.overflow-y-auto')
  ).toBeNull()
})

it('counts the visitors and opens them from a button', async () => {
  mocks.visitors.mockResolvedValue([
    visitor(),
    visitor({
      chat_id: -3003,
      chat_type: 'supergroup',
      chat_title: 'Some group',
      attempts: 1,
      username: '',
      name: '',
      sender_id: 4004,
      last_text: '',
      last_at: new Date('2026-11-02T08:00:00').getTime(),
    }),
  ])
  inQuery()
  const open = await screen.findByRole('button', { name: 'View details' })
  expect(await screen.findByText('People: 2 · Attempts: 4')).toBeVisible()
  expect(screen.queryByText('Mal Lory')).toBeNull()

  fireEvent.click(open)
  expect(await screen.findByText('Mal Lory')).toBeVisible()
  expect(screen.getByText('@mallory · ID 2002 · /start')).toBeVisible()
  expect(screen.getByText('3×')).toBeVisible()
  // With neither a name nor a @username, the id has to do, and a group says so.
  expect(screen.getByText('ID 4004')).toBeVisible()
  expect(screen.getByText('Some group')).toBeVisible()
  expect(screen.getByText('Group')).toBeVisible()

  fireEvent.click(screen.getByRole('button', { name: 'Back' }))
  expect(screen.getByRole('button', { name: 'Send test' })).toBeVisible()
})

it('says when there are no visitors, before and after opening them', async () => {
  inQuery()
  const open = await screen.findByRole('button', { name: 'View details' })
  expect(await screen.findByText('No visitors yet')).toBeVisible()
  fireEvent.click(open)
  expect(await screen.findByText('No visitors yet')).toBeVisible()
  expect(screen.queryByText(/People:/)).toBeNull()
})

it('offers a search once the list is longer than a screen', async () => {
  mocks.visitors.mockResolvedValue(
    Array.from({ length: 9 }, (_, index) =>
      visitor({
        chat_id: index + 1,
        sender_id: index + 1,
        name: `Person ${index + 1}`,
      })
    )
  )
  inQuery()
  fireEvent.click(await screen.findByRole('button', { name: 'View details' }))
  fireEvent.change(
    await screen.findByLabelText('Search by name, ID or message'),
    {
      target: { value: 'person 7' },
    }
  )
  expect(screen.getByText('Person 7')).toBeVisible()
  expect(screen.queryByText('Person 8')).toBeNull()
  fireEvent.change(screen.getByLabelText('Search by name, ID or message'), {
    target: { value: 'nobody like this' },
  })
  expect(screen.getByText('No visitor matches that')).toBeVisible()
})
