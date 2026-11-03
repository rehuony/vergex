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
import { FuturesAccount as AccountSection } from '@/components/futures/futures-account'
import { useAccountWatch } from '@/hooks/futures/use-account-watch'
import { QueryWrapper } from '@/tests/query-wrapper'
import { CloseNotSubmittedError, type LiveRun } from '@/api/futures'
import { positionDecimal } from '@/utils/futures/futures-account'
import type { Account } from '@/types'

vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
const mocks = vi.hoisted(() => ({
  traders: vi.fn(),
  positions: vi.fn(),
  marks: vi.fn(),
  close: vi.fn(),
  planning: vi.fn(),
}))
vi.mock('@/api', () => ({
  api: {
    getTraders: mocks.traders,
    getAccountPositions: mocks.positions,
    getAccountMarks: mocks.marks,
    closePosition: mocks.close,
  },
}))
vi.mock('@/api/futures', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api/futures')>()),
  futuresAPI: mocks.planning,
}))
vi.mock('@/api/helpers', () => ({
  getAuthHeaders: () => ({}),
  handleJSONResponse: async (r: Response) => {
    if (!r.ok) throw new Error('Exchange read failed')
    return r.json()
  },
}))
// The section as the page uses it, with the watch above it.
function FuturesAccount(
  props: Omit<ComponentProps<typeof AccountSection>, 'watch'>
) {
  const watch = useAccountWatch(props.account?.id ?? '', props.symbol)
  return (
    <QueryWrapper>
      <MemoryRouter>
        <AccountSection {...props} watch={watch} />
      </MemoryRouter>
    </QueryWrapper>
  )
}
const account = {
  id: 'account',
  name: 'My Binance',
  venue: 'binance',
} as Account
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
  mocks.marks.mockReset().mockResolvedValue({})
  mocks.traders
    .mockReset()
    .mockResolvedValue([
      { trader_id: 'trader', account_id: 'account', running: false },
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
    <FuturesAccount
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
  await screen.findByRole('columnheader', { name: 'Exchange orders · BTCUSDT' })
  await screen.findByText('Take profit')
  expect(screen.getByText('Stop loss')).toBeVisible()
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
  // On the panel's title row, with the rest of what there is to say a click away.
  fireEvent.click(
    await screen.findByRole('button', {
      name: /position is open, but no working orders were reported/,
    })
  )
  expect(
    await screen.findByRole('link', {
      name: /Check protection at the exchange/,
    })
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
        account_id: 'account',
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

it('leaves nothing on the page once a close is accepted', async () => {
  mocks.planning.mockResolvedValue({ warning: '', order: {} })
  open()
  await screen.findByText('ETHUSDT', { selector: 'strong' })
  fireEvent.click(screen.getAllByRole('button', { name: 'Close position' })[0])
  fireEvent.click(screen.getByRole('button', { name: 'Confirm market close' }))
  await waitFor(() => expect(mocks.planning).toHaveBeenCalled())
  await waitFor(() =>
    expect(screen.queryByText(/Stopping remaining entries/)).toBeNull()
  )
  expect(
    screen.queryByText(/Close request accepted/, { selector: 'p,div' })
  ).toBeNull()
})

it('keeps what still needs checking on the page after a close', async () => {
  mocks.planning.mockResolvedValue({
    warning: 'Execution records could not be refreshed.',
    order: {},
  })
  open()
  await screen.findByText('ETHUSDT', { selector: 'strong' })
  fireEvent.click(screen.getAllByRole('button', { name: 'Close position' })[0])
  fireEvent.click(screen.getByRole('button', { name: 'Confirm market close' }))
  expect(await screen.findByText(/could not be refreshed/)).toBeInTheDocument()
})

it('does not present a failed exchange read as an empty account', async () => {
  mocks.positions.mockRejectedValue(new Error('network unavailable'))
  open()
  await screen.findByRole('alert')
  expect(
    screen.queryByText('No open positions reported by this account')
  ).toBeNull()
  expect(screen.queryByRole('button', { name: 'Close position' })).toBeNull()
})

it("offers a cancel only on orders no running plan holds, and moves a plan's exits", async () => {
  const run = {
    id: 'run',
    account_id: 'account',
    plan_id: 'plan',
    status: 'position_open',
    plan: {
      idea: { name: 'Short', side: 'short' },
      snapshot: { symbol: 'BTCUSDT', venue: 'binance' },
    },
    orders: [{ role: 'sl', order_id: 'sl', status: 'NEW', price: '84000' }],
  }
  render(
    <FuturesAccount
      account={account}
      accounts={[account]}
      venue="binance"
      symbol="BTCUSDT"
      runs={[run] as unknown as LiveRun[]}
      busy={false}
      onView={vi.fn()}
      onManual={vi.fn()}
      onBusy={vi.fn()}
      onRefresh={async () => {}}
    />
  )
  await screen.findByText('tp')
  expect(screen.getAllByRole('button', { name: 'Cancel order' })).toHaveLength(
    1
  )
  expect(
    screen.getAllByRole('button', { name: 'Stop and target' })
  ).toHaveLength(1)
  fireEvent.click(screen.getByRole('button', { name: 'Stop and target' }))
  expect(await screen.findByRole('textbox', { name: 'Stop loss' })).toHaveValue(
    '84000'
  )
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
    <FuturesAccount
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
  expect(screen.getByText('3x').closest('td')).toHaveTextContent('Short 3x')
  expect(
    screen.getByRole('columnheader', { name: 'Est. initial margin' })
  ).toBeVisible()
  expect(mocks.close).not.toHaveBeenCalled()
  expect(mocks.planning).not.toHaveBeenCalled()
})

it('reads a saved account without any trader and never creates one', async () => {
  mocks.traders.mockResolvedValue([])
  open()
  expect(
    await screen.findByText('ETHUSDT', { selector: 'strong' })
  ).toBeVisible()
  expect(mocks.positions).toHaveBeenCalledWith('account', true)
  expect(fetch).toHaveBeenCalledWith(
    '/api/accounts/account/open-orders?symbol=BTCUSDT',
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
      name: 'Trading two',
      venue: venue,
    }
    render(
      <FuturesAccount
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
          account_id: `${venue}-two`,
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
    venue: 'binance',
    name: 'First',
  }
  const second = { ...first, id: 'two', name: 'Second' }
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
  const view = render(<FuturesAccount {...props} account={first} />)
  await screen.findByText('ETHUSDT', { selector: 'strong' })
  fireEvent.click(screen.getAllByRole('button', { name: 'Close position' })[0])
  view.rerender(<FuturesAccount {...props} account={second} />)
  await waitFor(() => expect(mocks.positions).toHaveBeenCalledWith('two', true))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(mocks.planning).not.toHaveBeenCalled()
  expect(mocks.close).not.toHaveBeenCalled()
})

it.each([true, false])(
  'only unlocks a close when the server confirms no submission: %s',
  async (notSubmitted) => {
    const selected = { ...account, venue: 'binance' }
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
      <FuturesAccount
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
    <FuturesAccount
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
  expect(screen.getByText('No OKX account yet')).toBeVisible()
  expect(screen.getByRole('link', { name: 'Add an account' })).toHaveAttribute(
    'href',
    '?settings=accounts'
  )
  for (const name of ['Buy / Long', 'Sell / Short']) {
    expect(screen.queryByRole('button', { name })).toBeNull()
  }
  expect(mocks.positions).not.toHaveBeenCalled()
})

it('keeps its P/L on the live mark price and asks for nothing by hand', async () => {
  mocks.marks.mockResolvedValue({ BTCUSDT: 90 })
  const props = {
    account,
    accounts: [account],
    venue: 'binance',
    symbol: 'BTCUSDT',
    runs: [],
    busy: false,
    onView: vi.fn(),
    onManual: vi.fn(),
    onBusy: vi.fn(),
    onRefresh: async () => {},
  }
  mocks.positions.mockResolvedValue([
    {
      symbol: 'BTCUSDT',
      side: 'short',
      quantity: 2,
      entry_price: 100,
      mark_price: 95,
      unrealized_pnl: 10,
      leverage: 5,
    },
  ])
  render(<FuturesAccount {...props} />)
  // Short 2 from 100, now at a mark of 90.
  expect(await screen.findAllByText('+20.00')).not.toHaveLength(0)
  expect(mocks.marks).toHaveBeenCalledWith('account', 'BTCUSDT')
  expect(screen.queryByRole('button', { name: /Refresh/ })).toBeNull()
  expect(screen.queryByRole('link', { name: /Open exchange/ })).toBeNull()
})
