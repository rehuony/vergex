import {
  render,
  screen,
  fireEvent,
  cleanup,
  waitFor,
} from '@testing-library/react'
import type { ComponentProps } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { FrontierAccount as AccountSection } from '@/components/planning/frontier-account'
import { useAccountWatch } from '@/hooks/planning/use-account-watch'
import { CloseNotSubmittedError } from '@/api/planning'
import { positionDecimal } from '@/utils/planning/frontier-account'
import type { Exchange } from '@/types'

vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
const mocks = vi.hoisted(() => ({
  traders: vi.fn(),
  positions: vi.fn(),
  close: vi.fn(),
  planning: vi.fn(),
}))
vi.mock('@/api', () => ({
  api: {
    getTraders: mocks.traders,
    getPositions: mocks.positions,
    closePosition: mocks.close,
  },
}))
vi.mock('@/api/planning', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api/planning')>()),
  planningAPI: mocks.planning,
}))
vi.mock('@/api/helpers', () => ({
  getAuthHeaders: () => ({}),
  handleJSONResponse: async (r: Response) => {
    if (!r.ok) throw new Error('Exchange read failed')
    return r.json()
  },
}))
// The section as the page uses it, with the watch above it.
function FrontierAccount(
  props: Omit<ComponentProps<typeof AccountSection>, 'watch'>
) {
  const watch = useAccountWatch(props.account?.id ?? '', props.symbol)
  return (
    <MemoryRouter>
      <AccountSection {...props} watch={watch} />
    </MemoryRouter>
  )
}
const account = {
  id: 'account',
  name: 'My Binance',
  exchange_type: 'binance',
  enabled: true,
} as Exchange
const positions = [
  {
    symbol: 'BTCUSDT',
    side: 'short',
    positionAmt: 0.01,
    entryPrice: 83000,
    markPrice: 82000,
    unRealizedProfit: 10,
    leverage: 3,
  },
  {
    symbol: 'ETHUSDT',
    side: 'long',
    positionAmt: 0.1,
    entryPrice: 3000,
    markPrice: 3100,
    unRealizedProfit: 10,
    leverage: 2,
  },
]
beforeEach(() => {
  mocks.traders
    .mockReset()
    .mockResolvedValue([
      { trader_id: 'trader', exchange_id: 'account', is_running: false },
    ])
  mocks.positions.mockReset().mockResolvedValue(positions)
  mocks.close.mockReset().mockResolvedValue({ message: 'accepted' })
  mocks.planning.mockReset()
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation(
      async () =>
        new Response(
          JSON.stringify([
            {
              order_id: 'sl',
              symbol: 'BTCUSDT',
              side: 'BUY',
              type: 'STOP_MARKET',
              stop_price: 84000,
              quantity: 0.01,
              status: 'NEW',
            },
            {
              order_id: 'tp',
              symbol: 'BTCUSDT',
              side: 'BUY',
              type: 'TAKE_PROFIT_MARKET',
              stop_price: 79000,
              quantity: 0.01,
              status: 'NEW',
            },
          ])
        )
    )
  )
})
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})
function open(symbol = 'BTCUSDT', onManual = vi.fn()) {
  return render(
    <FrontierAccount
      account={account}
      accounts={[account]}
      venue="binance"
      symbol={symbol}
      runs={[]}
      busy={false}
      onView={vi.fn()}
      onManual={onManual}
      onBusy={vi.fn()}
      onRefresh={vi.fn().mockResolvedValue(undefined)}
    />
  )
}

it('shows all account positions and native stop/target orders without submitting anything', async () => {
  open()
  await screen.findByText('ETHUSDT', { selector: 'strong' })
  expect(
    screen.getByText('BTCUSDT', { selector: 'strong' })
  ).toBeInTheDocument()
  await screen.findByText('Exchange exit orders · BTCUSDT')
  expect(screen.getAllByText('84,000')[0]).toBeInTheDocument()
  expect(screen.getAllByText('79,000')[0]).toBeInTheDocument()
  expect(
    screen.getAllByRole('button', { name: 'Close position' })
  ).toHaveLength(2)
  expect(mocks.close).not.toHaveBeenCalled()
  expect(mocks.planning).not.toHaveBeenCalled()
})

it('flags a live position with no reported exit orders without taking action', async () => {
  vi.mocked(fetch).mockResolvedValue(new Response('[]'))
  open()
  await screen.findByText(
    /position is open, but no working orders were reported/
  )
  expect(
    screen.getByRole('link', { name: /Check protection at the exchange/ })
  ).toHaveAttribute('href', 'https://www.binance.com/en/futures')
  expect(mocks.close).not.toHaveBeenCalled()
  expect(mocks.planning).not.toHaveBeenCalled()
})

it('requires an explicit close click, uses the account close API and never repeats a timeout', async () => {
  mocks.planning.mockRejectedValue(new Error('timeout'))
  open()
  await screen.findByText('ETHUSDT', { selector: 'strong' })
  fireEvent.click(screen.getAllByRole('button', { name: 'Close position' })[0])
  expect(mocks.planning).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Keep position' }))
  expect(mocks.planning).not.toHaveBeenCalled()
  fireEvent.click(screen.getAllByRole('button', { name: 'Close position' })[0])
  fireEvent.click(screen.getByRole('button', { name: 'Confirm market close' }))
  await waitFor(() =>
    expect(mocks.planning).toHaveBeenCalledWith(
      'close-position',
      expect.objectContaining({
        exchange_id: 'account',
        symbol: 'BTCUSDT',
        side: 'short',
      })
    )
  )
  await screen.findByText(/timeout/)
  expect(
    screen.getByRole('button', { name: 'Verify close at exchange' })
  ).toBeDisabled()
  expect(mocks.planning).toHaveBeenCalledTimes(1)
  expect(mocks.close).not.toHaveBeenCalled()
})

it('does not present a failed exchange read as an empty account', async () => {
  mocks.positions.mockRejectedValue(new Error('network unavailable'))
  open()
  await screen.findByRole('alert')
  expect(
    screen.queryByText('No open positions reported by this account.')
  ).toBeNull()
  expect(screen.queryByRole('button', { name: 'Close position' })).toBeNull()
})

it('manual entry opens a review form through its callback, never a model or order request', async () => {
  const manual = vi.fn()
  open('SOLUSDT', manual)
  await screen.findByText('ETHUSDT', { selector: 'strong' })
  fireEvent.click(screen.getByRole('button', { name: 'Sell / Short' }))
  expect(manual).toHaveBeenCalledWith('short')
  expect(mocks.close).not.toHaveBeenCalled()
  expect(mocks.planning).not.toHaveBeenCalled()
})

it('refuses to close a position that grew after the user reviewed it', async () => {
  open()
  await screen.findByText('ETHUSDT', { selector: 'strong' })
  fireEvent.click(screen.getAllByRole('button', { name: 'Close position' })[0])
  mocks.positions.mockResolvedValue([
    { ...positions[0], positionAmt: 1 },
    positions[1],
  ])
  fireEvent.click(screen.getByRole('button', { name: 'Confirm market close' }))
  await screen.findByText(/Position changed/)
  expect(mocks.close).not.toHaveBeenCalled()
  expect(mocks.planning).not.toHaveBeenCalled()
})

it('shows position value, visible leverage and explicitly estimated margin without placing orders', async () => {
  render(
    <FrontierAccount
      account={account}
      accounts={[account]}
      venue="binance"
      symbol="BTCUSDT"
      runs={[]}
      busy={false}
      onView={() => {}}
      onManual={() => {}}
      onBusy={() => {}}
      onRefresh={async () => {}}
    />
  )
  expect(await screen.findByText('820.00')).toBeVisible()
  expect(screen.getByText('≈ 273.33')).toBeVisible()
  expect(screen.getByText('3x')).toBeVisible()
  expect(
    screen.getByText('3x').closest('.frontier-position-badge')
  ).toHaveTextContent('Short')
  expect(
    screen.getByRole('columnheader', { name: 'Est. initial margin' })
  ).toBeVisible()
  expect(screen.getByText(/actual exchange margin may differ/)).toBeVisible()
  expect(mocks.close).not.toHaveBeenCalled()
  expect(mocks.planning).not.toHaveBeenCalled()
})

it('uses USDT for Binance position amounts', async () => {
  const binance = { ...account, exchange_type: 'binance' }
  render(
    <FrontierAccount
      account={binance}
      accounts={[binance]}
      venue="binance"
      symbol="BTCUSDT"
      runs={[]}
      busy={false}
      onView={() => {}}
      onManual={() => {}}
      onBusy={() => {}}
      onRefresh={async () => {}}
    />
  )
  expect(await screen.findAllByText('USDT notional')).toHaveLength(2)
  expect(screen.queryByText('USDC notional')).toBeNull()
})

it('reads a saved account without any trader and never creates one', async () => {
  mocks.traders.mockResolvedValue([])
  open()
  expect(
    await screen.findByText('ETHUSDT', { selector: 'strong' })
  ).toBeVisible()
  expect(mocks.positions).toHaveBeenCalledWith(undefined, true, 'account')
  expect(fetch).toHaveBeenCalledWith(
    '/api/open-orders?exchange_id=account&symbol=BTCUSDT',
    expect.anything()
  )
  expect(screen.queryByText(/Connect a Vergex trader/)).toBeNull()
  expect(
    screen.getAllByRole('button', { name: 'Close position' })
  ).toHaveLength(2)
  expect(mocks.close).not.toHaveBeenCalled()
  expect(mocks.planning).not.toHaveBeenCalled()
})

it.each(['binance', 'okx'])(
  'closes a standalone %s account through its account ID after explicit review',
  async (venue) => {
    mocks.traders.mockResolvedValue([])
    mocks.planning.mockResolvedValue({ message: 'accepted' })
    const market = venue === 'okx' ? 'BTC-USDT-SWAP' : 'BTCUSDT'
    mocks.positions.mockResolvedValue(
      positions.map((position) => ({
        ...position,
        symbol:
          venue === 'okx'
            ? position.symbol.replace('USDT', '-USDT-SWAP')
            : position.symbol,
      }))
    )
    const selectedAccount = {
      ...account,
      id: `${venue}-two`,
      account_name: 'Trading two',
      exchange_type: venue,
    }
    render(
      <FrontierAccount
        account={selectedAccount}
        accounts={[selectedAccount]}
        venue={venue}
        symbol={market}
        runs={[]}
        busy={false}
        onView={vi.fn()}
        onManual={vi.fn()}
        onBusy={vi.fn()}
        onRefresh={async () => {}}
      />
    )
    await screen.findByText(venue === 'okx' ? 'ETH-USDT-SWAP' : 'ETHUSDT', {
      selector: 'strong',
    })
    expect(
      screen.queryByRole('link', { name: 'Close on exchange ↗' })
    ).toBeNull()
    fireEvent.click(
      screen.getAllByRole('button', { name: 'Close position' })[0]
    )
    expect(screen.getByRole('dialog')).toHaveTextContent('Trading two')
    expect(mocks.planning).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Confirm market close' })
    )
    await waitFor(() =>
      expect(mocks.planning).toHaveBeenCalledWith(
        'close-position',
        expect.objectContaining({
          exchange_id: `${venue}-two`,
          symbol: market,
          side: 'short',
          quantity: '0.01',
          entry_price: '83000',
          request_id: expect.any(String),
        })
      )
    )
    expect(mocks.close).not.toHaveBeenCalled()
  }
)

it('discards a close confirmation when the selected Binance account changes', async () => {
  mocks.traders.mockResolvedValue([])
  const first = {
    ...account,
    id: 'one',
    exchange_type: 'binance',
    account_name: 'First',
  }
  const second = { ...first, id: 'two', account_name: 'Second' }
  const props = {
    accounts: [first, second],
    venue: 'binance',
    symbol: 'BTCUSDT',
    runs: [],
    busy: false,
    onView: vi.fn(),
    onManual: vi.fn(),
    onBusy: vi.fn(),
    onRefresh: async () => {},
  }
  const view = render(<FrontierAccount {...props} account={first} />)
  await screen.findByText('ETHUSDT', { selector: 'strong' })
  fireEvent.click(screen.getAllByRole('button', { name: 'Close position' })[0])
  view.rerender(<FrontierAccount {...props} account={second} />)
  await waitFor(() =>
    expect(mocks.positions).toHaveBeenCalledWith(undefined, true, 'two')
  )
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(mocks.planning).not.toHaveBeenCalled()
  expect(mocks.close).not.toHaveBeenCalled()
})

it.each([true, false])(
  'only unlocks a close when the server confirms no submission: %s',
  async (notSubmitted) => {
    const selected = { ...account, exchange_type: 'binance' }
    mocks.positions.mockResolvedValue([
      {
        ...positions[0],
        symbol: 'QUSDT',
        positionAmt: -12257,
        entryPrice: 0.1287878419678551,
      },
    ])
    mocks.planning.mockRejectedValue(
      notSubmitted
        ? new CloseNotSubmittedError('Position changed')
        : new Error('The exchange close is unconfirmed')
    )
    render(
      <FrontierAccount
        account={selected}
        accounts={[selected]}
        venue="binance"
        symbol="QUSDT"
        runs={[]}
        busy={false}
        onView={vi.fn()}
        onManual={vi.fn()}
        onBusy={vi.fn()}
        onRefresh={async () => {}}
      />
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Close position' })
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Confirm market close' })
    )
    await waitFor(() =>
      expect(mocks.planning).toHaveBeenCalledWith(
        'close-position',
        expect.objectContaining({
          quantity: '12257',
          entry_price: '0.1287878419678551',
        })
      )
    )
    if (notSubmitted) {
      await screen.findByText(/No close order was submitted/)
      expect(
        screen.getByRole('button', { name: 'Close position' })
      ).toBeEnabled()
      expect(
        screen.queryByText(/Check the exchange before sending another close/)
      ).toBeNull()
    } else {
      await screen.findByText(/Check the exchange before sending another close/)
      expect(
        screen.getByRole('button', { name: 'Verify close at exchange' })
      ).toBeDisabled()
    }
    expect(mocks.planning).toHaveBeenCalledTimes(1)
  }
)

it('serializes tiny position observations without losing precision or using exponents', () => {
  expect(positionDecimal(0.1287878419678551)).toBe('0.1287878419678551')
  expect(positionDecimal(1.23456789e-8)).toBe('0.0000000123456789')
  expect(positionDecimal(1e21)).toBe('1000000000000000000000')
  expect(() => positionDecimal(NaN)).toThrow('Invalid position value')
})

it('offers only a way to add an account when there is none', () => {
  render(
    <FrontierAccount
      accounts={[]}
      venue="okx"
      symbol="BTC-USDT-SWAP"
      runs={[]}
      busy={false}
      onView={vi.fn()}
      onManual={vi.fn()}
      onBusy={vi.fn()}
      onRefresh={vi.fn().mockResolvedValue(undefined)}
    />
  )
  expect(screen.getByText('No OKX account is selected.')).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'Add an exchange account' })
  ).toHaveAttribute('href', '?settings=exchanges')
  for (const name of ['Refresh', 'Buy / Long', 'Sell / Short']) {
    expect(screen.queryByRole('button', { name })).toBeNull()
  }
  expect(mocks.positions).not.toHaveBeenCalled()
})
