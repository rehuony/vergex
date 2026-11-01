import {
  render,
  screen,
  fireEvent,
  waitFor,
  cleanup,
  within,
} from '@testing-library/react'
import { MemoryRouter, useSearchParams } from 'react-router-dom'
import { SWRConfig } from 'swr'
import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest'
import { TradingPlanningPage } from '@/pages/trading-planning-page'
const mocks = vi.hoisted(() => ({
  api: vi.fn(),
  stream: vi.fn(),
  exchangesGate: undefined as Promise<void> | undefined,
  models: [] as {
    id: string
    name: string
    provider: string
    enabled: boolean
    has_api_key: boolean
  }[],
  exchanges: [] as {
    id: string
    name: string
    account_name?: string
    exchange_type: string
  }[],
  chartOptions: undefined as
    | undefined
    | {
        onError: (message: string) => void
        onAvailability: (available: boolean) => void
      },
  plot: vi.fn().mockResolvedValue(undefined),
  focus: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
// A native select keeps these tests on the page's logic; Radix has its own.
vi.mock('@/components/ui/select', () => ({
  AppSelect: ({
    options,
    onChange,
    value,
    disabled,
    'aria-label': ariaLabel,
  }: {
    options: { value: string | number; label: string }[]
    onChange: (value: string) => void
    value: string | number
    disabled?: boolean
    'aria-label'?: string
  }) => (
    <select
      aria-label={ariaLabel}
      value={value}
      disabled={disabled}
      onChange={(event) => onChange(event.target.value)}
    >
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  ),
}))
vi.mock('@/api', () => ({
  api: {
    getExchangeConfigs: async () => {
      await mocks.exchangesGate
      return mocks.exchanges
    },
    getTraders: async () => [],
    getModelConfigs: async () => mocks.models,
  },
}))
vi.mock('@/api/planning', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  planningAPI: mocks.api,
  streamPlanning: mocks.stream,
}))
vi.mock('@/components/planning/planning-chart/load-planning-chart', () => ({
  loadPlanningChart: async () => {
    window.WorkspaceChart = class {
      constructor(options: NonNullable<typeof mocks.chartOptions>) {
        mocks.chartOptions = options
      }
      context() {
        return {
          venue: 'binance',
          symbol: 'BTCUSDT',
          interval: '1h',
          resolution: '60',
        }
      }
      async whenReady() {}
      async setContext() {}
      setForecast() {}
      plot = mocks.plot
      focusPlan = mocks.focus
      async draw() {}
      dispose() {}
    } as unknown as Window['WorkspaceChart']
  },
}))
const plan = (id: string) => ({
  id,
  hash: id.repeat(4),
  created: new Date().toISOString(),
  expires: new Date(Date.now() + 300000).toISOString(),
  quantity: '0.01',
  risk: '10',
  reward: '25',
  reward_risk: '2.5',
  constraints: { min_reward_risk: '2' },
  snapshot: {
    venue: 'binance',
    symbol: 'BTCUSDT',
    kind: 'perpetual',
    base_asset: 'BTC',
    quote_asset: 'USDT',
  },
  idea: {
    name: 'Test plan ' + id,
    side: 'long',
    entry: '80000',
    stop: '79000',
    target: '83000',
    rationale: 'Conditional hypothesis',
    execution: { kind: id === 'two' ? 'limit' : 'market' },
  },
  orders: [{ index: 1, price: '80000', quantity: '0.01', offset_seconds: 0 }],
})
beforeEach(() => {
  localStorage.clear()
  localStorage.setItem(
    'frontier-batch-v1//ex/binance/BTCUSDT',
    JSON.stringify({
      id: 'test-turn',
      ids: ['one', 'two'],
      created: new Date().toISOString(),
    })
  )
  Element.prototype.scrollIntoView = vi.fn()
  mocks.models = [
    {
      id: 'model',
      name: 'Configured DeepSeek',
      provider: 'deepseek',
      enabled: true,
      has_api_key: true,
    },
  ]
  mocks.exchangesGate = undefined
  mocks.exchanges = [
    {
      id: 'ex',
      name: 'Connected account',
      exchange_type: 'binance',
    },
  ]
  mocks.api.mockReset()
  mocks.plot.mockClear()
  mocks.stream
    .mockReset()
    .mockRejectedValue(new Error('Account sizing unavailable'))
  mocks.api.mockImplementation(async (path: string) => {
    if (path === 'state')
      return {
        records: {
          one: { plan: plan('one'), state: 'draft' },
          two: { plan: plan('two'), state: 'draft' },
        },
        accounts: { one: 'ex', two: 'ex' },
        runs: [],
        mode: 'live',
        now: new Date().toISOString(),
      }
    if (path.startsWith('account?'))
      return { equity: '1000', available: '1000', fee_bps: '5' }
    if (path === 'preferences') return { text: '' }
    if (path === 'conversations') return []
    if (path === 'compile')
      return { ...plan('resized'), constraints: { leverage: 5 } }
    if (path === 'approve') return { id: 'run', approval_hash: 'locked' }
    return {}
  })
})
afterEach(cleanup)
// A fresh connection cache per render, as a page load would have.
function Page() {
  const [params] = useSearchParams()
  return (
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <TradingPlanningPage />
      <span data-testid="settings">{params.get('settings')}</span>
    </SWRConfig>
  )
}
function open() {
  return render(
    <MemoryRouter>
      <Page />
    </MemoryRouter>
  )
}
describe('Vergex live planning', () => {
  it('refreshes account sizing and executes only after the explicit review click', async () => {
    open()
    const card = await screen.findByRole('button', { name: /Test plan one/ })
    fireEvent.click(card)
    await waitFor(() =>
      expect(screen.getByLabelText('Model')).toHaveValue('model')
    )
    fireEvent.click(screen.getByRole('button', { name: 'Review order' }))
    const execute = await screen.findByRole('button', {
      name: 'Confirm and place live order',
    })
    expect(execute).toBeEnabled()
    expect(screen.queryByLabelText('Live trade confirmation text')).toBeNull()
    expect(mocks.api).toHaveBeenCalledWith(
      'compile',
      expect.objectContaining({
        auto_size: true,
        exchange_id: 'ex',
        constraints: expect.objectContaining({ min_reward_risk: '0' }),
      })
    )
    expect(
      mocks.api.mock.calls.some(([p]) => p === 'approve' || p === 'execute')
    ).toBe(false)
    fireEvent.click(execute)
    await waitFor(() =>
      expect(mocks.api).toHaveBeenCalledWith('execute', {
        id: 'run',
        hash: 'locked',
      })
    )
    expect(mocks.api).toHaveBeenCalledWith('approve', {
      plan_id: 'resized',
      hash: 'resized'.repeat(4),
      confirmed: true,
    })
  })
  it('selecting a plan exits comparison and displays only that plan on the chart', async () => {
    open()
    await screen.findByRole('button', { name: /Test plan one/ })
    fireEvent.click(screen.getByRole('button', { name: 'Compare plans' }))
    await waitFor(() =>
      expect(
        mocks.plot.mock.calls.some(([, , list]) => list?.length === 2)
      ).toBe(true)
    )
    fireEvent.click(screen.getByRole('button', { name: /Test plan two/ }))
    await waitFor(() => {
      const args = mocks.plot.mock.lastCall!
      expect(args[0].id).toBe('two')
      expect(args[2]).toEqual([])
    })
  })
  it('custom entry mode does not attach TWAP parameters to a ladder', async () => {
    open()
    await screen.findByRole('button', { name: /Test plan one/ })
    fireEvent.click(screen.getByText('Advanced tools'))
    fireEvent.click(
      screen.getByText('Risk budget, entry orders and advanced tools')
    )
    fireEvent.click(
      screen.getByRole('button', {
        name: 'Custom market / limit / ladder / TWAP',
      })
    )
    fireEvent.change(screen.getByLabelText('Custom entry method'), {
      target: { value: 'ladder' },
    })
    expect(screen.queryByLabelText('Slices')).toBeNull()
    expect(
      screen.getByText(/Enter one price and quantity weight per line/)
    ).toBeInTheDocument()
  })
})

it('opens the only connected account by default and switches venues', async () => {
  mocks.exchanges = [
    {
      id: 'okx-main',
      name: 'OKX account',
      exchange_type: 'okx',
    },
  ]
  open()
  await waitFor(() =>
    expect(screen.getByLabelText('Trading account')).toHaveValue('okx-main')
  )
  expect(screen.getByLabelText('Market venue')).toHaveValue('okx')
  fireEvent.change(screen.getByLabelText('Market venue'), {
    target: { value: 'binance' },
  })
  expect(screen.getByLabelText('Market venue')).toHaveValue('binance')
  expect(screen.getByLabelText('Trading account')).toHaveValue('')
  fireEvent.change(screen.getByLabelText('Market venue'), {
    target: { value: 'okx' },
  })
  expect(screen.getByLabelText('Trading account')).toHaveValue('okx-main')
  expect(
    screen.getByRole('button', { name: 'Market BTC-USDT-SWAP' })
  ).toBeInTheDocument()
})

it('clears a chart failure after recovery without clearing unrelated errors', async () => {
  open()
  await waitFor(() => expect(mocks.chartOptions).toBeDefined())
  const { act } = await import('@testing-library/react')
  act(() => mocks.chartOptions!.onError('Market chart timed out.'))
  expect(screen.getByText('Market chart timed out.')).toBeInTheDocument()
  act(() => mocks.chartOptions!.onAvailability(true))
  expect(screen.queryByText('Market chart timed out.')).not.toBeInTheDocument()
})

it('keeps a venue chosen while account configuration is still loading', async () => {
  let release!: () => void
  mocks.exchanges = [
    {
      id: 'okx-main',
      name: 'OKX account',
      exchange_type: 'okx',
    },
  ]
  mocks.exchangesGate = new Promise<void>((resolve) => {
    release = resolve
  })
  open()
  fireEvent.change(screen.getByLabelText('Market venue'), {
    target: { value: 'okx' },
  })
  release()
  await waitFor(() =>
    expect(screen.getByLabelText('Trading account')).toHaveValue('okx-main')
  )
  expect(
    screen.getByRole('button', { name: 'Market BTC-USDT-SWAP' })
  ).toBeInTheDocument()
  expect(screen.getByLabelText('Market venue')).toHaveValue('okx')
})

it('preflight failure clears busy state without accumulating phantom user messages', async () => {
  open()
  await screen.findByRole('button', { name: /Test plan one/ })
  const generate = screen.getByRole('button', { name: 'Generate plans' })
  for (let attempt = 0; attempt < 2; attempt++) {
    fireEvent.click(generate)
    await waitFor(() => expect(mocks.stream).toHaveBeenCalledTimes(attempt + 1))
    await waitFor(() => expect(generate).toBeEnabled())
  }
  expect(
    screen.queryByText(
      'Compare single-limit, weighted ladder and TWAP plans for this market.'
    )
  ).toBeNull()
  expect(screen.getAllByText('Account sizing unavailable')).toHaveLength(1)
  expect(screen.getByLabelText('Trade idea')).toBeEnabled()
  expect(
    JSON.parse(localStorage.getItem('frontier-batch-v1//ex/binance/BTCUSDT')!)
      .ids
  ).toEqual(['one', 'two'])
})

it('account review does not restart model activity or show a planning stop button', async () => {
  let finish!: (value: unknown) => void
  const pending = new Promise((resolve) => {
    finish = resolve
  })
  const base = mocks.api.getMockImplementation()!
  mocks.api.mockImplementation((path: string, body?: unknown) =>
    path === 'compile' ? pending : base(path, body)
  )
  open()
  fireEvent.click(await screen.findByRole('button', { name: /Test plan one/ }))
  fireEvent.click(screen.getByRole('button', { name: 'Review order' }))
  await screen.findByText('Validating account and order details…')
  expect(screen.queryByText('Live activity')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Stop planning' })).toBeNull()
  finish({ ...plan('resized'), constraints: { leverage: 5 } })
  await screen.findByRole('dialog')
})

it.each([true, false])(
  'archives the previous batch only for an explicit wait decision (%s)',
  async (waiting) => {
    mocks.stream.mockImplementation(async (_input, _signal, emit) => {
      if (waiting) emit({ type: 'plans', data: { plans: [], waiting: true } })
      emit({
        type: 'assistant',
        data: {
          message: waiting
            ? 'Wait for a confirmed uptrend.'
            : 'Here is the market context.',
        },
      })
      emit({ type: 'outcome', data: { drafts: 0, stop_reason: 'completed' } })
    })
    open()
    await screen.findByRole('button', { name: /Test plan one/ })
    const generate = screen.getByRole('button', { name: 'Generate plans' })
    fireEvent.click(generate)
    await waitFor(() => expect(mocks.stream).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(generate).toBeEnabled())
    expect(
      JSON.parse(localStorage.getItem('frontier-batch-v1//ex/binance/BTCUSDT')!)
        .ids
    ).toEqual(waiting ? [] : ['one', 'two'])
    if (waiting) {
      expect(screen.getByText('Earlier drafts')).toBeInTheDocument()
      expect(
        screen.getByRole('heading', { name: 'Waiting for an entry' })
      ).toBeInTheDocument()
    }
  }
)

it('restores the reasons for waiting after the page is reloaded', async () => {
  const deferred = ['market', 'limit', 'ladder', 'twap'].map((kind) => ({
    kind,
    reason: `${kind}: waiting for a supported entry.`,
  }))
  localStorage.setItem(
    'frontier-batch-v1//ex/binance/BTCUSDT',
    JSON.stringify({
      id: 'wait-turn',
      ids: [],
      created: new Date().toISOString(),
      waiting: true,
      deferred,
    })
  )
  open()
  await screen.findByRole('heading', { name: 'Waiting for an entry' })
  expect(screen.getAllByText('Wait for entry')).toHaveLength(4)
  for (const entry of deferred)
    expect(screen.getByText(entry.reason)).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Review Market' })).toBeNull()
})

it('opens settings before a paid request and retains the original idea when AI is missing', async () => {
  mocks.models = []
  open()
  await waitFor(() =>
    expect(screen.getByRole('link', { name: 'Add a model' })).toBeVisible()
  )
  const idea = screen.getByRole('textbox', { name: 'Trade idea' })
  fireEvent.change(idea, {
    target: { value: 'Compare a ladder and TWAP for BTC' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Send' }))
  await waitFor(() =>
    expect(screen.getByTestId('settings')).toHaveTextContent('models')
  )
  expect(idea).toHaveValue('Compare a ladder and TWAP for BTC')
  expect(mocks.stream).not.toHaveBeenCalled()
})

it('offers a safe starting point, keeps real account activity separate and keeps plans visible outside the assistant', async () => {
  open()
  await waitFor(() =>
    expect(screen.getByLabelText('Model')).toHaveValue('model')
  )
  fireEvent.click(screen.getByRole('tab', { name: 'Positions and orders' }))
  expect(
    screen.getByRole('region', { name: 'Account positions and orders' })
  ).toBeVisible()
  fireEvent.click(screen.getByRole('tab', { name: 'Plans' }))
  expect(
    screen.getByText('Advanced tools').closest('details')
  ).not.toHaveAttribute('open')
  fireEvent.click(screen.getByRole('button', { name: /Find a pullback entry/ }))
  expect(screen.getByLabelText('Trade idea')).toHaveValue(
    'Find a pullback entry with clear risk and exits.'
  )
  expect(mocks.stream).not.toHaveBeenCalled()
  expect(
    mocks.api.mock.calls.some(
      ([path]) => path === 'approve' || path === 'execute'
    )
  ).toBe(false)
  const deck = screen.getByRole('region', { name: 'Entry plans' })
  expect(deck.closest('#frontier-assistant')).toBeNull()
  expect(screen.getByLabelText('Trade idea')).toHaveValue(
    'Find a pullback entry with clear risk and exits.'
  )
})

it('shows generated plans automatically and uses the user idea instead of a generic prompt', async () => {
  mocks.stream.mockImplementation(async (_body, _signal, onEvent) => {
    onEvent({
      type: 'plans',
      data: { plans: [plan('one'), plan('two')], checks: [] },
    })
  })
  open()
  await waitFor(() =>
    expect(screen.getByLabelText('Model')).toHaveValue('model')
  )
  fireEvent.change(screen.getByLabelText('Trade idea'), {
    target: { value: 'Compare timed entries for a pullback.' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Generate plans' }))
  await waitFor(() =>
    expect(
      screen.getByRole('region', { name: 'Entry plans' })
    ).toHaveTextContent('Test plan one')
  )
  expect(mocks.stream).toHaveBeenCalledWith(
    expect.objectContaining({
      message: 'Compare timed entries for a pullback.',
    }),
    expect.anything(),
    expect.any(Function)
  )
  expect(
    mocks.api.mock.calls.some(
      ([path]) => path === 'approve' || path === 'execute'
    )
  ).toBe(false)
})

it('starts a separate draft from a locked trade without revising or executing that trade', async () => {
  const base = mocks.api.getMockImplementation()!
  mocks.api.mockImplementation((path: string, body?: unknown) =>
    path === 'state'
      ? {
          records: { one: { plan: plan('one'), state: 'approved' } },
          accounts: { one: 'ex' },
          runs: [],
          mode: 'live',
          now: new Date().toISOString(),
        }
      : base(path, body)
  )
  mocks.stream.mockResolvedValue(undefined)
  open()
  await waitFor(() =>
    expect(screen.getByLabelText('Model')).toHaveValue('model')
  )
  fireEvent.click(screen.getByRole('tab', { name: 'History' }))
  fireEvent.click(await screen.findByRole('button', { name: /Test plan one/ }))
  fireEvent.change(screen.getByLabelText('Trade idea'), {
    target: { value: 'Generate a new independent entry plan.' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Generate plans' }))
  await waitFor(() => expect(mocks.stream).toHaveBeenCalledTimes(1))
  const input = mocks.stream.mock.calls[0][0]
  expect(input.plan_id).toBeUndefined()
  expect(input.history).toEqual([])
  expect(input.message).toBe('Generate a new independent entry plan.')
  expect(
    mocks.api.mock.calls.some(([path]) =>
      ['approve', 'execute', 'stop'].includes(path)
    )
  ).toBe(false)
  expect(screen.queryByText(/AI cannot modify a locked plan/)).toBeNull()
})

describe('previous trade recovery', () => {
  it('reviews a new plan without cancelling an unused approval, then replaces only that approval on confirmation', async () => {
    const base = mocks.api.getMockImplementation()!
    const previous = {
      id: 'unused-run',
      exchange_id: 'ex',
      plan_id: 'old',
      approval_hash: 'previous-hash',
      state: 'approved',
      plan: plan('old'),
      orders: [],
    }
    mocks.api.mockImplementation(async (path: string, body?: unknown) =>
      path === 'state'
        ? { ...(await base(path, body)), runs: [previous] }
        : base(path, body)
    )
    open()
    await screen.findByText('BTCUSDT order not submitted')
    fireEvent.click(screen.getByRole('button', { name: 'Review Market' }))
    let dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('note')).toHaveTextContent(
      'replaces your earlier unsubmitted BTCUSDT approval'
    )
    expect(
      mocks.api.mock.calls.some(([path]) =>
        ['approve', 'execute', 'stop'].includes(path)
      )
    ).toBe(false)
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Back to review' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Review Single limit' }))
    dialog = await screen.findByRole('dialog')
    fireEvent.click(
      within(dialog).getByRole('button', {
        name: 'Confirm and place live order',
      })
    )
    await waitFor(() =>
      expect(mocks.api).toHaveBeenCalledWith(
        'approve',
        expect.objectContaining({
          confirmed: true,
          replace_approval_id: 'unused-run',
          replace_approval_hash: 'previous-hash',
        })
      )
    )
    await waitFor(() =>
      expect(mocks.api).toHaveBeenCalledWith('execute', {
        id: 'run',
        hash: 'locked',
      })
    )
    expect(mocks.api.mock.calls.some(([path]) => path === 'stop')).toBe(false)
    expect(
      screen.queryByText(/An earlier order is approved but not submitted/)
    ).toBeNull()
  })

  it('does not leave a success notice when previous trade recovery is followed by a review failure', async () => {
    const base = mocks.api.getMockImplementation()!
    let closed = false
    const previous = {
      id: 'old-run',
      exchange_id: 'ex',
      plan_id: 'old',
      state: 'needs_attention',
      plan: plan('old'),
      orders: [],
    }
    mocks.api.mockImplementation(async (path: string, body?: unknown) => {
      if (path === 'state')
        return {
          ...(await base(path, body)),
          runs: [{ ...previous, state: closed ? 'completed' : previous.state }],
        }
      if (path === 'check-status') {
        closed = true
        return { ...previous, state: 'completed' }
      }
      if (path === 'compile')
        throw new Error('Account balance is temporarily unavailable.')
      return base(path, body)
    })
    open()
    await screen.findByRole('button', { name: 'Verify status' })
    fireEvent.click(screen.getByRole('button', { name: 'Review Market' }))
    expect(
      await screen.findByText('Account balance is temporarily unavailable.')
    ).toBeVisible()
    expect(screen.queryByText(/Previous trade closed/)).toBeNull()
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(screen.getByRole('button', { name: 'Review Market' })).toBeEnabled()
  })

  it('identifies the blocking trade and checks it without submitting or cancelling orders', async () => {
    const base = mocks.api.getMockImplementation()!
    let closed = false
    const old = {
      id: 'previous-run',
      exchange_id: 'ex',
      plan_id: 'old',
      approval_hash: 'hash',
      state: 'needs_attention',
      plan: plan('old'),
      orders: [],
    }
    mocks.api.mockImplementation(async (path: string, body?: unknown) => {
      if (path === 'state') {
        const data = await base(path, body)
        return {
          ...data,
          runs: [{ ...old, state: closed ? 'completed' : old.state }],
        }
      }
      if (path === 'check-status') {
        closed = true
        return { ...old, state: 'completed' }
      }
      return base(path, body)
    })
    open()
    await screen.findByRole('button', { name: 'Verify status' })
    expect(
      screen.getByRole('region', { name: 'Current trade status' })
    ).toHaveTextContent('BTCUSDT trade status needs verification')
    fireEvent.click(
      await screen.findByRole('button', { name: /Test plan one/ })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Review order' }))
    await screen.findByRole('dialog')
    expect(screen.queryByText(/Previous trade closed/)).toBeNull()
    expect(mocks.api).toHaveBeenCalledWith(
      'compile',
      expect.objectContaining({ auto_size: true })
    )
    expect(mocks.api).toHaveBeenCalledWith('check-status', {
      id: 'previous-run',
    })
    expect(
      mocks.api.mock.calls.some(([path]) =>
        ['execute', 'approve', 'stop', 'reconcile'].includes(path)
      )
    ).toBe(false)
  })
  it('keeps new orders blocked when the exchange check fails', async () => {
    const base = mocks.api.getMockImplementation()!
    mocks.api.mockImplementation(async (path: string, body?: unknown) => {
      if (path === 'state')
        return {
          ...(await base(path, body)),
          runs: [
            {
              id: 'previous-run',
              exchange_id: 'ex',
              plan_id: 'old',
              state: 'needs_attention',
              plan: plan('old'),
              orders: [],
            },
          ],
        }
      if (path === 'check-status')
        throw new Error('Could not confirm stop order. Check the exchange.')
      return base(path, body)
    })
    open()
    fireEvent.click(
      await screen.findByRole('button', { name: /Review Market/ })
    )
    expect(
      await screen.findByText(
        'Could not confirm stop order. Check the exchange.'
      )
    ).toBeVisible()
    expect(
      screen.getByRole('region', { name: 'Current trade status' })
    ).toBeVisible()
    expect(
      mocks.api.mock.calls.some(([path]) =>
        ['execute', 'approve', 'stop', 'compile'].includes(path)
      )
    ).toBe(false)
  })
  it('clears a failed verification after the exchange confirms closure, without submitting an order', async () => {
    const base = mocks.api.getMockImplementation()!
    let closed = false
    let attempts = 0
    const old = {
      id: 'previous-run',
      exchange_id: 'ex',
      plan_id: 'old',
      state: 'needs_attention',
      plan: plan('old'),
      orders: [],
    }
    mocks.api.mockImplementation(async (path: string, body?: unknown) => {
      if (path === 'state')
        return {
          ...(await base(path, body)),
          runs: [{ ...old, state: closed ? 'completed' : old.state }],
        }
      if (path === 'check-status') {
        if (++attempts === 1)
          throw new Error('Could not confirm stop order. Check the exchange.')
        closed = true
        return { ...old, state: 'completed' }
      }
      return base(path, body)
    })
    open()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Verify status' })
    )
    const blocker = screen.getByRole('region', { name: 'Current trade status' })
    expect(await within(blocker).findByRole('alert')).toHaveTextContent(
      'Could not confirm stop order'
    )
    const retry = screen.getByRole('button', { name: 'Verify status' })
    await waitFor(() => expect(retry).toBeEnabled())
    fireEvent.click(retry)
    await waitFor(() =>
      expect(
        screen.queryByRole('region', { name: 'Current trade status' })
      ).toBeNull()
    )
    expect(
      screen.queryByText('Could not confirm stop order. Check the exchange.')
    ).toBeNull()
    expect(screen.getByRole('status')).toHaveTextContent(
      'Previous trade is closed'
    )
    expect(
      mocks.api.mock.calls.some(([path]) =>
        ['execute', 'approve', 'stop', 'reconcile'].includes(path)
      )
    ).toBe(false)
  })
})

it('keeps expired drafts visible and refreshes them before review, without executing', async () => {
  const base = mocks.api.getMockImplementation()!
  const stale = {
    ...plan('one'),
    expires: new Date(Date.now() - 60000).toISOString(),
  }
  mocks.api.mockImplementation(async (path: string, body?: unknown) =>
    path === 'state'
      ? {
          ...(await base(path, body)),
          records: { one: { plan: stale, state: 'draft' } },
        }
      : base(path, body)
  )
  open()
  fireEvent.click(await screen.findByRole('button', { name: /Review Market/ }))
  await screen.findByRole('dialog')
  expect(mocks.api).toHaveBeenCalledWith(
    'compile',
    expect.objectContaining({ auto_size: true })
  )
  expect(
    mocks.api.mock.calls.some(([p]) => p === 'approve' || p === 'execute')
  ).toBe(false)
})

it('distinguishes Binance accounts by their names and remembers each venue selection', async () => {
  mocks.exchanges = [
    {
      id: 'binance-a',
      name: 'Binance Futures',
      account_name: 'Main portfolio',
      exchange_type: 'binance',
    },
    {
      id: 'binance-b',
      name: 'Binance Futures',
      account_name: 'Second portfolio',
      exchange_type: 'binance',
    },
    {
      id: 'okx-main',
      name: 'OKX',
      exchange_type: 'okx',
    },
  ]
  const view = open()
  await waitFor(() =>
    expect(screen.getByLabelText('Trading account')).toHaveValue('binance-a')
  )
  expect(
    screen.getByRole('option', { name: 'Second portfolio' })
  ).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Market BTCUSDT' }))
  const search = await screen.findByLabelText('Search exchange markets')
  fireEvent.change(search, { target: { value: 'ethusdt' } })
  fireEvent.submit(search)
  await screen.findByRole('button', { name: 'Market ETHUSDT' })
  fireEvent.change(screen.getByLabelText('Trading account'), {
    target: { value: 'binance-b' },
  })
  expect(screen.getByRole('button', { name: 'Market ETHUSDT' })).toBeVisible()
  fireEvent.change(screen.getByLabelText('Market venue'), {
    target: { value: 'okx' },
  })
  fireEvent.change(screen.getByLabelText('Market venue'), {
    target: { value: 'binance' },
  })
  expect(screen.getByLabelText('Trading account')).toHaveValue('binance-b')
  view.unmount()
  open()
  await waitFor(() =>
    expect(screen.getByLabelText('Trading account')).toHaveValue('binance-b')
  )
  expect(mocks.stream).not.toHaveBeenCalled()
})

describe('the address', () => {
  function Address() {
    const [params] = useSearchParams()
    return <output aria-label="address">{params.toString()}</output>
  }
  const open = (entry: string) =>
    render(
      <MemoryRouter initialEntries={[entry]}>
        <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
          <Address />
          <TradingPlanningPage />
        </SWRConfig>
      </MemoryRouter>
    )
  const address = () =>
    new URLSearchParams(screen.getByLabelText('address').textContent ?? '')

  it('follows what is open and keeps other parameters', async () => {
    open('/frontier?settings=models')
    await waitFor(() => expect(address().get('account')).toBe('ex'))
    expect(address().get('venue')).toBe('binance')
    expect(address().get('symbol')).toBe('BTCUSDT')
    expect(address().get('settings')).toBe('models')
    expect(address().has('tab')).toBe(false)

    fireEvent.click(screen.getByRole('tab', { name: 'Positions and orders' }))
    await waitFor(() => expect(address().get('tab')).toBe('positions'))
  })

  it('opens the market, account and section it names', async () => {
    mocks.exchanges = [
      {
        id: 'ex',
        name: 'Connected account',
        exchange_type: 'binance',
      },
      {
        id: 'okx-one',
        name: 'OKX account',
        exchange_type: 'okx',
      },
    ]
    open('/frontier?account=okx-one&symbol=ETHUSDT&tab=history')
    await waitFor(() => expect(address().get('account')).toBe('okx-one'))
    await waitFor(() => expect(address().get('venue')).toBe('okx'))
    expect(address().get('symbol')).toBe('ETH-USDT-SWAP')
    expect(screen.getByRole('tab', { name: 'History' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })

  it('ignores a market name it cannot use', async () => {
    open('/frontier?symbol=BTC%2F..%2Fx&venue=nowhere')
    await waitFor(() => expect(address().get('account')).toBe('ex'))
    expect(address().get('venue')).toBe('binance')
    expect(address().get('symbol')).toBe('BTCUSDT')
  })

  it('opens and fits a plan it names, and drops one that no longer exists', async () => {
    const { unmount } = open('/frontier?plan=two')
    await waitFor(() => expect(address().get('plan')).toBe('two'))
    await waitFor(() => expect(mocks.focus).toHaveBeenCalled())
    await waitFor(() => expect(mocks.plot.mock.lastCall?.[0]?.id).toBe('two'))
    unmount()

    open('/frontier?plan=gone')
    await waitFor(() => expect(address().get('account')).toBe('ex'))
    await waitFor(() => expect(address().has('plan')).toBe(false))
  })
})
