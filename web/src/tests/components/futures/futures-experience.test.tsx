import { afterEach, it, expect, vi } from 'vitest'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'
import type { Plan, FuturesEvent } from '@/api/futures'
import { defaultLimits } from '@/api/futures'
import { ExecutionSchedule } from '@/components/futures/execution-schedule'
import {
  FuturesActivity,
  recordActivity,
} from '@/components/futures/futures-activity'
import { FuturesPlanDeck } from '@/components/futures/futures-plan-deck'
import { PlanStructures } from '@/components/futures/plan-structures'

vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
afterEach(cleanup)

it('shows the unfunded planning explanation as activity, not an interruption', () => {
  const rows = recordActivity([], {
    type: 'planning_sizing',
    data: { planning_only: true },
  })
  render(<FuturesActivity items={rows} busy />)
  expect(screen.getByText(/No free margin is available/)).toBeInTheDocument()
  expect(rows[0].state).toBe('done')
  expect(screen.queryByText('Planning interrupted')).toBeNull()
})
const plan = (kind: 'market' | 'limit' | 'ladder' | 'twap'): Plan => ({
  id: kind,
  hash: kind,
  created: new Date().toISOString(),
  expires: new Date(Date.now() + 60000).toISOString(),
  quantity: '0.002',
  risk: '2',
  reward: '5',
  reward_risk: '2.5',
  constraints: defaultLimits,
  snapshot: {
    venue: 'binance',
    symbol: 'BTCUSDT',
    kind: 'perpetual',
    tick: '1',
    step: '0.00001',
    quote_asset: 'USDT',
    base_asset: 'BTC',
  },
  idea: {
    name: kind + ' candidate',
    side: 'long',
    entry: '83000',
    stop: '82000',
    target: '86000',
    rationale: 'Conditional fixture',
    execution: { kind, slices: 2, interval_seconds: 30 },
  },
  orders: [
    { index: 1, price: '83000', quantity: '0.001', offset_seconds: 0 },
    {
      index: 2,
      price: kind === 'ladder' ? '82900' : '83000',
      quantity: '0.001',
      offset_seconds: kind === 'twap' ? 30 : 0,
    },
  ],
})
it('shows each TWAP release on a seconds axis before execution', () => {
  render(<ExecutionSchedule plan={plan('twap')} />)
  expect(
    screen.getByRole('region', { name: 'TWAP release schedule' })
  ).toBeInTheDocument()
  expect(screen.getByText('T+0s')).toBeInTheDocument()
  expect(screen.getByText('T+30s')).toBeInTheDocument()
  expect(screen.getAllByText(/0.001 BTC/)).toHaveLength(2)
  expect(screen.getByText(/starts only after approval/)).toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: /execute/i })
  ).not.toBeInTheDocument()
})
it('shows ladder prices without fake time staggering', () => {
  render(<ExecutionSchedule plan={plan('ladder')} />)
  expect(screen.getByText('Limit 83,000')).toBeInTheDocument()
  expect(screen.getByText('Limit 82,900')).toBeInTheDocument()
  expect(screen.getAllByText('Placed together at start')).toHaveLength(2)
  expect(screen.queryByText('T+30s')).not.toBeInTheDocument()
})
it('shows four structure outcomes without inventing missing valid drafts', () => {
  const select = vi.fn()
  render(
    <PlanStructures
      checks={[
        {
          id: 'rejected',
          kind: 'ladder',
          name: 'Ladder',
          status: 'rejected',
          reason: 'Child order below venue minimum',
        },
      ]}
      plans={[plan('limit'), plan('twap')]}
      busy={false}
      selected="twap"
      onSelect={select}
    />
  )
  expect(screen.getByText('Ladder')).toBeInTheDocument()
  expect(
    screen.getByText('Child order below venue minimum')
  ).toBeInTheDocument()
  expect(
    screen.getAllByRole('button', { name: /^View on chart/ })
  ).toHaveLength(2)
  fireEvent.click(screen.getByRole('button', { name: /^View on chart: limit/ }))
  expect(select).toHaveBeenCalledWith(expect.objectContaining({ id: 'limit' }))
})
const event = (type: string, data: Record<string, unknown>): FuturesEvent => ({
  type,
  data,
})

it('shows deferred entry types alongside valid pullback drafts without offering orders for them', () => {
  const select = vi.fn()
  render(
    <PlanStructures
      checks={[
        {
          id: 'wait-market',
          kind: 'market',
          name: 'Market',
          status: 'deferred',
          reason: 'Immediate entry is extended; reassess after a pullback.',
        },
        {
          id: 'wait-twap',
          kind: 'twap',
          name: 'TWAP',
          status: 'deferred',
          reason: 'Timed releases cannot confirm a retest.',
        },
      ]}
      plans={[plan('limit'), plan('ladder')]}
      busy={false}
      selected=""
      onSelect={select}
    />
  )
  expect(screen.getAllByRole('article')).toHaveLength(4)
  expect(screen.getAllByText('Wait for entry')).toHaveLength(2)
  expect(
    screen.getByText('Timed releases cannot confirm a retest.')
  ).toBeInTheDocument()
  expect(
    screen.getAllByRole('button', { name: /^View on chart/ })
  ).toHaveLength(2)
  fireEvent.click(screen.getByRole('button', { name: /^View on chart: limit/ }))
  expect(select).toHaveBeenCalledWith(expect.objectContaining({ id: 'limit' }))
  // The reason takes one line until it is opened.
  const reason = screen.getByRole('button', { name: /Immediate entry/ })
  expect(reason).toHaveAttribute('aria-expanded', 'false')
  fireEvent.click(reason)
  expect(reason).toHaveAttribute('aria-expanded', 'true')
})

it('offers a comparison only when there are plans to compare', () => {
  const deck = (plans: Plan[]) => (
    <FuturesPlanDeck
      batch={{ id: 'turn', ids: [], created: new Date().toISOString() }}
      plans={plans}
      checks={[]}
      futuresBusy={false}
      waitingForEntry={false}
      selected=""
      clock={0}
      connected
      comparing={false}
      onChoose={vi.fn()}
      onCompare={vi.fn()}
      onExitCompare={vi.fn()}
    />
  )
  const { rerender } = render(deck([plan('limit')]))
  expect(screen.queryByRole('button', { name: 'Compare plans' })).toBeNull()
  rerender(deck([plan('limit'), plan('ladder')]))
  expect(screen.getByRole('button', { name: 'Compare plans' })).toBeEnabled()
})

it('lists every plan of a kind, each on its own row', () => {
  render(
    <PlanStructures
      checks={[]}
      plans={[plan('limit'), { ...plan('limit'), id: 'limit-low' }]}
      busy={false}
      selected="limit-low"
      onSelect={vi.fn()}
    />
  )
  const rows = screen.getAllByRole('button', { name: /^View on chart/ })
  expect(rows).toHaveLength(2)
  expect(rows[1]).toHaveAttribute('aria-current', 'true')
})

it('reports a completed wait decision without claiming drafts were saved or awaiting approval', () => {
  const checks = ['market', 'limit', 'ladder', 'twap'].map((kind) => ({
    id: kind,
    kind,
    name: kind,
    status: 'deferred',
    reason: 'Reassess after the broader trend recovers.',
  }))
  let rows = checks.reduce(
    (items, check) => recordActivity(items, event('candidate_result', check)),
    [] as ReturnType<typeof recordActivity>
  )
  rows = recordActivity(
    rows,
    event('plans', { plans: [], checks, rejected: [], waiting: true })
  )
  rows = recordActivity(
    rows,
    event('outcome', { drafts: 0, waiting: true, stop_reason: 'completed' })
  )
  expect(rows.every((row) => row.state === 'done')).toBe(true)
  expect(rows[rows.length - 1]).toMatchObject({
    label: 'Waiting for an entry',
    detail: 'No order to approve. See the entry conditions under Plans',
  })
  expect(
    rows.some((row) =>
      /Save validated|awaiting your review|Passed mathematical/.test(
        row.label + row.detail
      )
    )
  ).toBe(false)
  expect(
    recordActivity(
      [],
      event('outcome', { drafts: 0, stop_reason: 'completed' })
    )[0].detail
  ).not.toContain('awaiting your review')
})

it('updates a live task row with actual timeframe, candle count and measured duration', () => {
  let rows = recordActivity(
    [],
    event('tool_start', { id: 'context', name: 'get_candles' }),
    1000
  )
  rows = recordActivity(
    rows,
    event('tool_result', {
      id: 'context',
      name: 'get_candles',
      ok: true,
      interval: '1h',
      count: 1200,
      target_count: 1200,
      scope: 'Latest history · independent of chart view',
    }),
    4000
  )
  expect(rows).toHaveLength(1)
  expect(rows[0]).toMatchObject({ state: 'done', started: 1000, ended: 4000 })
  render(<FuturesActivity items={rows} busy={false} />)
  // A finished turn folds its steps away, into one line that opens again.
  expect(screen.queryByText('3s')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: /Thought for 3s/ }))
  expect(
    screen.getByText(
      '1h · 1200 / 1200 candles · Latest history · independent of chart view'
    )
  ).toBeInTheDocument()
  expect(screen.getByText('3s')).toBeInTheDocument()
})
it('streams heartbeats into one waiting row and preserves compiler rejection details', () => {
  let rows = recordActivity([], event('iteration', { round: 1 }), 1000)
  rows = recordActivity(rows, event('status', { elapsed_seconds: 12 }), 13000)
  expect(rows).toHaveLength(1)
  expect(rows[0].detail).toContain('12s')
  rows = recordActivity(
    rows,
    event('candidate_start', {
      id: 'candidate_1_0',
      kind: 'ladder',
      name: 'Weighted entry',
    }),
    14000
  )
  rows = recordActivity(
    rows,
    event('candidate_result', {
      id: 'candidate_1_0',
      kind: 'ladder',
      name: 'Weighted entry',
      status: 'rejected',
      reason: 'Net reward/risk below minimum',
    }),
    14100
  )
  expect(rows[0].state).toBe('done')
  expect(rows[1]).toMatchObject({
    state: 'failed',
    detail: 'Net reward/risk below minimum',
  })
})
it('updates plugin observations without duplicate rows and identifies cancellation', () => {
  let rows = recordActivity(
    [],
    event('tool_start', { id: 'plugin_funding', name: 'get_data_plugin' }),
    1000
  )
  rows = recordActivity(
    rows,
    event('plugin_data', {
      plugin: 'funding',
      metrics: [
        { label: 'Hourly funding', value: '0.00001', unit: 'fraction' },
      ],
    }),
    3000
  )
  expect(rows).toHaveLength(1)
  expect(rows[0].detail).toContain('0.00100%')
  rows = recordActivity(rows, event('iteration', { round: 1 }), 4000)
  rows = recordActivity(rows, event('cancelled', {}), 6000)
  expect(rows.find((row) => row.id === 'model_1')?.state).toBe('failed')
  expect(rows.find((row) => row.id === 'outcome')?.label).toBe(
    'Planning stopped'
  )
})

it('puts market first regardless of model order and labels the estimate', () => {
  const select = vi.fn()
  render(
    <PlanStructures
      checks={[
        { id: 'market', kind: 'market', name: 'Market', status: 'saved' },
      ]}
      plans={[plan('twap'), plan('limit'), plan('market'), plan('ladder')]}
      selected=""
      busy={false}
      onSelect={select}
    />
  )
  const cards = screen.getAllByRole('article')
  expect(cards).toHaveLength(4)
  expect(cards[0]).toHaveTextContent('Market')
  expect(cards[1]).toHaveTextContent('Limit')
  expect(cards[2]).toHaveTextContent('Ladder')
  expect(cards[3]).toHaveTextContent('TWAP')
  expect(cards[0]).toHaveTextContent('83,000')
  fireEvent.click(
    screen.getByRole('button', { name: /^View on chart: market/ })
  )
  expect(select).toHaveBeenCalledWith(expect.objectContaining({ id: 'market' }))
})
it('does not mislabel immediate market entry as a ladder or timed schedule', () => {
  const { container } = render(<ExecutionSchedule plan={plan('market')} />)
  expect(container).toBeEmptyDOMElement()
})

it('updates paginated history progress and discloses a newly listed market with limited history', () => {
  let rows = recordActivity(
    [],
    event('tool_start', { id: 'context', name: 'get_candles' }),
    1000
  )
  rows = recordActivity(
    rows,
    event('tool_progress', {
      id: 'context',
      name: 'get_candles',
      interval: '1h',
      count: 300,
      target_count: 1200,
      scope: 'Latest history',
    }),
    2000
  )
  expect(rows).toHaveLength(1)
  expect(rows[0]).toMatchObject({
    state: 'running',
    started: 1000,
    detail:
      '1h · 300 / 1200 candles · Latest history · independent of chart view',
  })
  rows = recordActivity(
    rows,
    event('tool_result', {
      id: 'context',
      name: 'get_candles',
      interval: '1h',
      count: 345,
      target_count: 1200,
      scope: 'Latest history',
      truncated: true,
      ok: true,
    }),
    3000
  )
  expect(rows).toHaveLength(1)
  expect(rows[0]).toMatchObject({
    state: 'done',
    started: 1000,
    ended: 3000,
    detail:
      '1h · 345 / 1200 candles · Limited exchange history · Latest history · independent of chart view',
  })
})

it('says what a turn cost beside its steps, and does not count that as one', () => {
  const steps = recordActivity(
    [],
    { type: 'iteration', data: { round: 1 } },
    1000,
    'en'
  )
  const done = recordActivity(steps, { type: 'outcome', data: {} }, 3000, 'en')
  const items = recordActivity(
    done,
    {
      type: 'usage',
      data: {
        prompt_tokens: 40000,
        completion_tokens: 800,
        cost_usd: 0.0123,
        unpriced: false,
      },
    },
    3000,
    'en'
  )
  render(<FuturesActivity items={items} busy={false} />)
  expect(
    screen.getByText(/2 steps · 40,000 in, 800 out · \$0\.0123/)
  ).toBeInTheDocument()

  const unpriced = recordActivity(
    done,
    {
      type: 'usage',
      data: {
        prompt_tokens: 1,
        completion_tokens: 1,
        cost_usd: 0,
        unpriced: true,
      },
    },
    3000,
    'zh-CN'
  )
  expect(unpriced.find((row) => row.id === 'usage')?.detail).toContain('未定价')
})
