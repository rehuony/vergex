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
  expect(cell('Equity')).toBe('Equity$1,200')
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
})

it('falls back to the last stored equity, marked with its time, when the account was never read', () => {
  render(<TraderMetrics {...props} accountFailed stored={stored} />)
  expect(cell('Equity')).toContain('$1,072.65')
  expect(cell('Total P/L')).toContain('+7.26%')
  expect(screen.getByRole('status')).toHaveTextContent(
    /^Equity and P\/L as of \d/
  )
})

it('keeps what it last read and marks it when a later read fails', () => {
  render(
    <TraderMetrics {...props} account={account} accountFailed stored={stored} />
  )
  expect(cell('Equity')).toContain('$1,200')
  expect(screen.getByRole('status')).toHaveTextContent(
    'Equity and P/L are not updating'
  )
})

it('shows a dash with no mark when there is nothing real to show', () => {
  render(<TraderMetrics {...props} accountFailed />)
  expect(cell('Equity')).toBe('Equity—')
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
})
