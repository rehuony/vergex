import { cleanup, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, expect, it } from 'vitest'
import type { Overview } from '@/api/overview'
import { AttentionList } from '@/components/dashboard/attention-list'
import { OverviewMetrics } from '@/components/dashboard/overview-metrics'
import { PositionsOverview } from '@/components/dashboard/positions-overview'

afterEach(cleanup)

const overview = (fields: Partial<Overview> = {}): Overview => ({
  generated_at: '2026-10-30T12:00:00Z',
  accounts: [],
  models: [],
  traders: [],
  runs: [],
  services: [],
  positions: [],
  positions_partial: false,
  open_orders: 0,
  realized_today: null,
  orders: [],
  attention: [],
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

const metric = (name: string) =>
  screen.getByText(name).closest('div')?.querySelector('dd')?.textContent

it('adds up only the accounts it could read, and shows a dash for none', () => {
  const { rerender } = render(
    <OverviewMetrics overview={overview()} language="en" />
  )
  expect(metric('Total equity')).toBe('—')
  rerender(
    <OverviewMetrics
      overview={overview({
        accounts: [
          {
            id: 'a',
            name: 'A',
            exchange_type: 'binance',
            status: 'ok',
            total_equity: 100,
          },
          { id: 'b', name: 'B', exchange_type: 'okx', status: 'unavailable' },
        ],
      })}
      language="en"
    />
  )
  expect(metric('Total equity')).toBe('$100')
})

it('marks a position count as a lower bound when an account was unread', () => {
  render(
    <OverviewMetrics
      overview={overview({ positions: [position], positions_partial: true })}
      language="en"
    />
  )
  expect(metric('Positions')).toBe('1+')
})

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
    '/autopilot/traders/Alpha-abcd'
  )
  expect(screen.getByText('Alpha is in safe mode')).toBeInTheDocument()
})
