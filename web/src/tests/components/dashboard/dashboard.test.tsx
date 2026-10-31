import { cleanup, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, expect, it, vi } from 'vitest'
import type { ActivityEvent } from '@/api/activity'
import type { Overview } from '@/api/overview'
import { AttentionList } from '@/components/dashboard/attention-list'
import { FirstRun } from '@/components/dashboard/first-run'
import { FlowEvents } from '@/components/dashboard/flow-events'
import { layoutFlow, lineOf } from '@/components/dashboard/flow-layout'
import { blockViews } from '@/components/dashboard/flow-members'
import { eventWhat, lineSummary } from '@/components/dashboard/flow-text'
import { PositionsOverview } from '@/components/dashboard/positions-overview'
import { t } from '@/i18n/translations'

afterEach(cleanup)

const overview = (fields: Partial<Overview> = {}): Overview => ({
  generated_at: '2026-10-30T12:00:00Z',
  accounts: [],
  models: [],
  traders: [],
  runs: [],
  telegram_bound: false,
  positions: [],
  positions_partial: false,
  realized_today: null,
  attention: [],
  ...fields,
})

const label = (key: string, params?: Record<string, string | number>) =>
  t(`dashboard.${key}`, 'en', params)

const event = (fields: Partial<ActivityEvent>): ActivityEvent => ({
  seq: 1,
  id: 'event',
  at: Date.now(),
  kind: 'ai',
  type: 'ai_call',
  from: { block: 'traders', id: 't', name: 'Trend' },
  to: { block: 'models', id: 'api.deepseek.com', name: 'api.deepseek.com' },
  ...fields,
})

const position = {
  exchange_id: 'e',
  symbol: 'BTCUSDT',
  side: 'long' as const,
  quantity: 1,
  entry_price: 100,
  mark_price: 110,
  unrealized_pnl: 10,
  unrealized_pnl_pct: 50,
  leverage: 5,
}

it('offers no way to act on a position from the overview', () => {
  render(
    <MemoryRouter>
      <PositionsOverview
        overview={overview({ positions: [position] })}
        language="en"
      />
    </MemoryRouter>
  )
  expect(screen.getByText('BTC')).toBeInTheDocument()
  expect(screen.getByText('Frontier')).toBeInTheDocument()
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
})

it('takes no space when nothing needs attention, and links to the fix when it does', () => {
  const { container, rerender } = render(
    <MemoryRouter>
      <AttentionList overview={overview()} language="en" />
    </MemoryRouter>
  )
  expect(container).toBeEmptyDOMElement()
  rerender(
    <MemoryRouter>
      <AttentionList
        overview={overview({
          traders: [
            {
              trader_id: 'abcd1234',
              trader_name: 'Alpha',
              is_running: true,
              ai_model_id: 'm',
              exchange_id: 'e',
            },
          ],
          attention: [
            {
              kind: 'trader_safe_mode',
              name: 'Alpha',
              detail: 'AI failed',
              trader_id: 'abcd1234',
            },
          ],
        })}
        language="en"
      />
    </MemoryRouter>
  )
  expect(screen.getByRole('link')).toHaveAttribute(
    'href',
    '/traders/Alpha-abcd'
  )
  expect(screen.getByText('Alpha is in safe mode')).toBeInTheDocument()
})

it('names a kind of problem once, however many have it', () => {
  render(
    <MemoryRouter>
      <AttentionList
        overview={overview({
          attention: [
            { kind: 'account_unavailable', name: 'Main', detail: 'timeout' },
            { kind: 'account_unavailable', name: 'Sub', detail: 'timeout' },
            { kind: 'account_unavailable', name: 'Spare', detail: 'timeout' },
          ],
        })}
        language="en"
      />
    </MemoryRouter>
  )
  expect(screen.getAllByRole('link')).toHaveLength(1)
  expect(screen.getByText('Main +2 is unavailable')).toBeInTheDocument()
})

it('adds up only the accounts it could read, and shows a dash for none', () => {
  const account = {
    id: 'b',
    name: 'B',
    exchange_type: 'okx',
    status: 'unavailable',
  }
  expect(
    blockViews(overview({ accounts: [account] }), label).accounts.sub
  ).toBe('—')
  const views = blockViews(
    overview({
      accounts: [
        account,
        {
          id: 'a',
          name: 'A',
          exchange_type: 'binance',
          status: 'ok',
          total_equity: 100,
        },
      ],
    }),
    label
  )
  expect(views.accounts.sub).toBe('$100')
  expect(views.accounts.members.map((member) => member.tone)).toEqual([
    'off',
    'on',
  ])
})

it('marks a position count as a lower bound when an account was unread', () => {
  const views = blockViews(
    overview({ positions: [position], positions_partial: true }),
    label
  )
  expect(views.positions.sub).toBe('1 long, 0+ short')
  expect(views.positions.warn).toBe(true)
})

it('shows one dot per model host, with the models that use it', () => {
  const views = blockViews(
    overview({
      models: [
        { id: 'a', name: 'Fast', provider: 'deepseek', host: 'one.example' },
        { id: 'b', name: 'Deep', provider: 'deepseek', host: 'one.example' },
        { id: 'c', name: 'Other', provider: 'openai', host: 'two.example' },
      ],
    }),
    label
  )
  expect(views.models.sub).toBe('2 hosts')
  expect(views.models.members[0].title).toBe('one.example · Fast, Deep')
})

it('keeps every route on the grid at any size', () => {
  for (const [width, height] of [
    [640, 352],
    [860, 700],
    [1500, 1100],
  ]) {
    const layout = layoutFlow(width, height)
    for (const line of layout.lines) {
      for (const dot of line.dots) {
        expect(dot.x % layout.pitch).toBe(0)
        expect(dot.y % layout.pitch).toBe(0)
      }
    }
  }
})

it('runs a reply back along the line its request took', () => {
  expect(
    lineOf(event({ from: { block: 'traders' }, to: { block: 'models' } }))
  ).toEqual({ key: 'traders>models', reverse: false })
  expect(
    lineOf(event({ from: { block: 'models' }, to: { block: 'traders' } }))
  ).toEqual({ key: 'traders>models', reverse: true })
  expect(
    lineOf(event({ from: { block: 'other' }, to: { block: 'models' } }))
  ).toBeNull()
})

it('labels AI costs as estimates, and shows money for both directions of a line', () => {
  expect(
    eventWhat(event({ type: 'ai_cost', kind: 'funds', amount: 0.003 }), label)
  ).toBe('≈ $0.0030')
  expect(
    lineSummary(
      'accounts>positions',
      {
        'accounts>positions': {
          h1: { count: 1, amount: 10 },
          h24: { count: 2, amount: 20 },
        },
        'positions>accounts': {
          h1: { count: 0 },
          h24: { count: 1, amount: -3 },
        },
      },
      label
    )
  ).toBe(
    'Accounts → Positions · 1h margin $10, realized $0 · 24h margin $20, realized -$3'
  )
})

const eventsProps = {
  overview: overview({
    traders: [
      {
        trader_id: 'abcd1234',
        trader_name: 'Alpha',
        is_running: true,
        ai_model_id: 'm',
        exchange_id: 'e',
      },
    ],
  }),
  loaded: true,
  stale: false,
  kind: 'all' as const,
  onKind: vi.fn(),
  selection: null,
  onPoint: vi.fn(),
  label,
}

it('opens the real thing behind an event, and filters by kind and by line', () => {
  const order = event({
    id: 'order',
    kind: 'orders',
    type: 'order',
    from: { block: 'traders', id: 'abcd1234', name: 'Alpha' },
    to: { block: 'accounts', id: 'e', name: 'Main' },
    symbol: 'BTCUSDT',
    action: 'open_long',
    quantity: 0.5,
    status: 'FILLED',
    trader_id: 'abcd1234',
  })
  const plan = event({
    id: 'plan',
    kind: 'orders',
    type: 'plan_order',
    from: { block: 'frontier', id: 'run', name: 'BTCUSDT' },
    to: { block: 'accounts', id: 'e', name: 'Main' },
    ref: 'p1',
    action: 'entry',
    symbol: 'BTCUSDT',
    quantity: 1,
  })
  const call = event({ id: 'call', tokens: 4812, detail: 'deepseek-chat' })
  const { rerender } = render(
    <MemoryRouter>
      <FlowEvents {...eventsProps} events={[order, plan, call]} />
    </MemoryRouter>
  )
  expect(screen.getByText('Open long BTC 0.5').closest('a')).toHaveAttribute(
    'href',
    '/traders/Alpha-abcd/orders'
  )
  expect(screen.getByText('Filled')).toBeInTheDocument()
  expect(screen.getByText('Entry BTC 1').closest('a')).toHaveAttribute(
    'href',
    '/frontier?plan=p1&tab=positions'
  )
  expect(screen.getByText('deepseek-chat · 4,812 tokens')).toBeInTheDocument()

  rerender(
    <MemoryRouter>
      <FlowEvents {...eventsProps} kind="ai" events={[order, plan, call]} />
    </MemoryRouter>
  )
  expect(screen.queryByText('Filled')).not.toBeInTheDocument()

  rerender(
    <MemoryRouter>
      <FlowEvents
        {...eventsProps}
        selection={{ type: 'line', key: 'frontier>accounts' }}
        events={[order, plan, call]}
      />
    </MemoryRouter>
  )
  expect(screen.getAllByRole('listitem')).toHaveLength(1)
})

it('says so when the day has been quiet, and offers the next step only if nothing is set up', () => {
  const { rerender } = render(
    <MemoryRouter>
      <FlowEvents {...eventsProps} events={[]} />
    </MemoryRouter>
  )
  expect(
    screen.getByText('Nothing has happened in the last 24 hours.')
  ).toBeInTheDocument()
  expect(screen.queryByText('Create trader')).not.toBeInTheDocument()
  rerender(
    <MemoryRouter>
      <FlowEvents {...eventsProps} overview={overview()} events={[]} />
    </MemoryRouter>
  )
  expect(screen.getByText('Create trader')).toBeInTheDocument()
})

it('leads a new account through setup, picking out the next step', () => {
  render(
    <MemoryRouter>
      <FirstRun
        overview={overview({
          models: [{ id: 'a', name: 'Fast', provider: 'deepseek' }],
        })}
        label={label}
      />
    </MemoryRouter>
  )
  expect(screen.getByLabelText('Done')).toBeInTheDocument()
  expect(screen.queryByText('Add AI model')).not.toBeInTheDocument()
  expect(screen.getByText('Add account')).toHaveAttribute(
    'href',
    '?settings=exchanges'
  )
})
