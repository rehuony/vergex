import type { ComponentProps } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'
import { TraderMetrics } from '@/components/trader/trader-metrics'
import type { AccountInfo } from '@/types'

afterEach(cleanup)

const props = { nextCycleMs: null, language: 'en' as const }

const account = {
  total_equity: 1200,
  total_pnl: 200,
  total_pnl_pct: 20,
} as AccountInfo

const stored = {
  equity: 1072.65,
  equity_at: 1793400000,
  return_pct: 7.26,
}

const cell = (name: string) =>
  screen.getByText(name).closest('div')?.textContent ?? ''

it('shows the account as it is, with no mark, while it can be read', () => {
  render(
    <TraderMetrics
      {...props}
      account={account}
      accountFailed={false}
      stored={stored}
    />
  )
  expect(cell('Account equity')).toBe('Account equity$1,200')
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
})

it('falls back to the last stored equity, marked with its time, when the account was never read', () => {
  render(<TraderMetrics {...props} accountFailed stored={stored} />)
  expect(cell('Account equity')).toContain('$1,072.65')
  expect(cell('Total P/L')).toContain('+7.26%')
  expect(screen.getByRole('status')).toHaveTextContent(
    /^Equity and P\/L as of \d/
  )
})

it('keeps what it last read and marks it when a later read fails', () => {
  render(
    <TraderMetrics {...props} account={account} accountFailed stored={stored} />
  )
  expect(cell('Account equity')).toContain('$1,200')
  expect(screen.getByRole('status')).toHaveTextContent(
    'Equity and P/L are not updating'
  )
})

it('shows a dash with no mark when there is nothing real to show', () => {
  render(<TraderMetrics {...props} accountFailed />)
  expect(cell('Account equity')).toBe('Account equity—')
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
})

it('marks realized P/L when its read failed, and keeps the last value', () => {
  render(
    <TraderMetrics
      {...props}
      account={account}
      accountFailed={false}
      stats={
        {
          total_trades: 3,
          total_pnl: 12.5,
          win_rate: 60,
          profit_factor: 1.5,
          sharpe_ratio: 1,
          max_drawdown_pct: 1,
          avg_win: 10,
          avg_loss: 5,
        } as ComponentProps<typeof TraderMetrics>['stats']
      }
      statsFailed
    />
  )
  expect(cell('Realized P/L')).toContain('+$12.5')
  expect(cell('Realized P/L')).toContain('Not updating')
})

const traded = {
  total_trades: 3,
  total_pnl: 20,
  total_fee: 2,
  win_rate: 60,
  profit_factor: 1.5,
  sharpe_ratio: 1,
  max_drawdown_pct: 1,
  avg_win: 10,
  avg_loss: 5,
} as ComponentProps<typeof TraderMetrics>['stats']
const costs = {
  total: 3,
  count: 5,
  unpriced_count: 0,
} as ComponentProps<typeof TraderMetrics>['costs']

it('takes model cost and fees off what the trader realized', () => {
  render(
    <TraderMetrics
      {...props}
      account={account}
      accountFailed={false}
      stats={traded}
      costs={costs}
    />
  )
  expect(cell('Model cost')).toContain('$3')
  expect(cell('Net after costs')).toContain('+$15')
})

it('marks a cost it could only partly price, and dashes one it could not read', () => {
  render(
    <TraderMetrics
      {...props}
      account={account}
      accountFailed={false}
      stats={traded}
      costs={{ ...costs!, unpriced_count: 2 }}
    />
  )
  expect(cell('Model cost')).toContain('≥ $3')
  expect(cell('Net after costs')).toContain('≤ +$15')
  cleanup()
  render(
    <TraderMetrics
      {...props}
      account={account}
      accountFailed={false}
      stats={traded}
      costsFailed
    />
  )
  expect(cell('Model cost')).toContain('—')
  expect(cell('Net after costs')).toContain('—')
})
