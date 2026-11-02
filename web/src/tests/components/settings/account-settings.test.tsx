import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { AccountSettings } from '@/components/settings/account-settings'

const mocks = vi.hoisted(() => ({
  accounts: vi.fn(),
  states: vi.fn(),
  verify: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  traders: vi.fn(),
  confirm: vi.fn(),
}))
vi.mock('@/api', () => ({
  api: {
    getAccounts: mocks.accounts,
    getAccountStates: mocks.states,
    verifyAccount: mocks.verify,
    createAccountEncrypted: mocks.create,
    updateAccountsEncrypted: mocks.update,
    deleteAccount: mocks.remove,
    getTraders: mocks.traders,
    getServerIP: async () => ({ public_ip: '203.0.113.7', message: '' }),
  },
}))
vi.mock('@/utils/crypto', () => ({
  CryptoService: {
    fetchCryptoConfig: async () => ({ transport_encryption: false }),
  },
  diagnoseWebCryptoEnvironment: () => ({
    isBrowser: true,
    isSecureContext: true,
    hasSubtleCrypto: true,
  }),
}))
vi.mock('@/components/common/notify', () => ({ confirmToast: mocks.confirm }))

const account = {
  id: 'exchange',
  venue_name: 'Binance',
  name: 'Main',
  venue: 'binance',
  has_api_key: true,
  has_secret_key: true,
}

beforeEach(() => {
  mocks.accounts.mockResolvedValue([account])
  mocks.states.mockResolvedValue({ states: {} })
  mocks.create.mockResolvedValue({ id: 'new' })
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
    <QueryWrapper>
      <AccountSettings language="en" />
    </QueryWrapper>
  )

it('checks a newly added account and shows its balance once, with the currency', async () => {
  show()
  expect(await screen.findByText('Not verified')).toBeVisible()
  mocks.states.mockResolvedValue({
    states: {
      exchange: {
        account_id: 'exchange',
        status: 'ok',
        display_balance: '321.9725 USDT',
        asset: 'USDT',
        available_balance: 300.5,
      },
    },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Add account' }))
  const save = screen.getByRole('button', { name: 'Save Configuration' })
  fireEvent.change(screen.getByLabelText('Account name'), {
    target: { value: 'Second' },
  })
  fireEvent.change(screen.getByLabelText('API Key'), {
    target: { value: 'fixture-key' },
  })
  expect(save).toBeDisabled()
  fireEvent.change(screen.getByLabelText('Secret Key'), {
    target: { value: 'fixture-secret' },
  })
  await waitFor(() => expect(save).toBeEnabled())
  fireEvent.click(save)

  expect(await screen.findByText('Connected')).toBeVisible()
  expect(screen.getByText('Binance · 321.97 USDT')).toBeVisible()
  expect(mocks.create).toHaveBeenCalledWith({
    venue: 'binance',
    name: 'Second',
    api_key: 'fixture-key',
    secret_key: 'fixture-secret',
    passphrase: '',
  })
  expect(mocks.states).toHaveBeenCalledTimes(2)

  fireEvent.click(screen.getByRole('button', { name: 'Edit Main' }))
  expect(screen.getByText('321.97 USDT')).toBeVisible()
  expect(screen.getByText('available 300.50 USDT')).toBeVisible()
})

it('checks one account again after the list check fails', async () => {
  mocks.states.mockRejectedValueOnce(new Error('timeout'))
  show()
  expect(await screen.findByText('Unavailable')).toBeVisible()
  mocks.verify.mockResolvedValue({
    account_id: 'exchange',
    status: 'permission_denied',
    error_message: 'Check Futures permissions and IP allowlist',
  })
  fireEvent.click(screen.getByRole('button', { name: 'Verify Main' }))
  expect(await screen.findByText('Check permissions')).toBeVisible()
  expect(mocks.verify).toHaveBeenCalledWith('exchange')
  expect(mocks.states).toHaveBeenCalledTimes(1)
  fireEvent.click(screen.getByRole('button', { name: 'Edit Main' }))
  expect(
    screen.getByText('Check Futures permissions and IP allowlist')
  ).toBeVisible()
  expect(mocks.update).not.toHaveBeenCalled()
  expect(mocks.create).not.toHaveBeenCalled()
})

it('replaces only the keys typed into an edit', async () => {
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Edit Main' }))
  const save = screen.getByRole('button', { name: 'Save Configuration' })
  expect(save).toBeDisabled()
  fireEvent.change(screen.getByLabelText('Secret Key'), {
    target: { value: 'new-secret' },
  })
  await waitFor(() => expect(save).toBeEnabled())
  fireEvent.click(save)
  await waitFor(() =>
    expect(mocks.update).toHaveBeenCalledWith({
      accounts: {
        exchange: {
          api_key: '',
          secret_key: 'new-secret',
          passphrase: '',
        },
      },
    })
  )
})

it('will not delete an account a trader trades on', async () => {
  mocks.traders.mockResolvedValue([
    { trader_id: 't1', trader_name: 'Alpha', account_id: 'exchange' },
  ])
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Edit Main' }))
  await waitFor(() => expect(mocks.traders).toHaveBeenCalled())
  await new Promise((resolve) => setTimeout(resolve, 0))
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
  expect(
    await screen.findByText('Used by Alpha. Delete those traders first.')
  ).toBeVisible()
  expect(mocks.confirm).not.toHaveBeenCalled()
  expect(mocks.remove).not.toHaveBeenCalled()
})

it('waits for the saved accounts before offering to add one', () => {
  mocks.accounts.mockReturnValue(new Promise(() => {}))
  show()
  expect(screen.getByRole('button', { name: 'Add account' })).toBeDisabled()
})
