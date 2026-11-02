import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { DecisionLog } from '@/components/trader/decision-log'
import type { DecisionRecord } from '@/types'

const mocks = vi.hoisted(() => ({ summaries: vi.fn() }))
vi.mock('@/api', () => ({ api: { getDecisionSummaries: mocks.summaries } }))

beforeEach(() => vi.clearAllMocks())
afterEach(cleanup)

const show = async (record: Partial<DecisionRecord>) => {
  mocks.summaries.mockResolvedValue([
    { id: 1, cycle_number: 7, timestamp: '2026-10-29T06:00:00Z', ...record },
  ])
  render(
    <MemoryRouter>
      <QueryWrapper>
        <DecisionLog traderId="t1" language="en" />
      </QueryWrapper>
    </MemoryRouter>
  )
  await waitFor(() => expect(screen.getByText('Cycle 7')).toBeInTheDocument())
}

it('marks success=true cycles as faults when their orders failed', async () => {
  await show({
    success: true,
    decisions: [
      {
        action: 'open_long',
        symbol: 'SOLUSDT',
        success: false,
        timestamp: '2026-08-15T12:00:00Z',
      } as DecisionRecord['decisions'][number],
    ],
  })
  expect(screen.getByText('Fault · 1 of 1 orders failed')).toBeInTheDocument()
  expect(screen.getByText('open_long SOL')).toBeInTheDocument()
})

it('formats protocol errors instead of showing raw bytes', async () => {
  const raw = 'request failed: malformed HTTP response "\\x00\\x00\\x12\\x04"'
  await show({
    success: false,
    decisions: [],
    error_message: raw,
    execution_log: [raw],
  })
  expect(screen.getByText('Fault')).toBeInTheDocument()
  expect(screen.getAllByText(/network protocol error/)).toHaveLength(2)
  expect(document.body.textContent).not.toContain('\\x00')
})

it('renders log tones as icons and strips the emoji the backend prefixes', async () => {
  await show({
    success: true,
    decisions: [],
    execution_log: [
      '🧊 BTCUSDT open_long blocked: cooldown',
      '❌ ETHUSDT open_short failed: insufficient margin',
      '✓ SOLUSDT close_long succeeded',
    ],
  })
  expect(screen.getByText('blocked: cooldown')).toBeInTheDocument()
  expect(screen.getByText('succeeded')).toBeInTheDocument()
  expect(document.body.textContent).not.toMatch(
    /[✓✗\p{Extended_Pictographic}]/u
  )
})

it('shows a dash for an action time that was never recorded', async () => {
  await show({
    timestamp: '0001-01-01T00:00:00Z',
    success: true,
    decisions: [],
  })
  expect(screen.getByText('—')).toBeInTheDocument()
  expect(document.body.textContent).not.toContain('1-01-01')
})
