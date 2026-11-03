import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { DecisionLog } from '@/components/trader/decision-log'
import { actionName, logText } from '@/components/trader/decision-format'
import type { DecisionRecord } from '@/types'

const mocks = vi.hoisted(() => ({ summaries: vi.fn() }))
vi.mock('@/api', () => ({ api: { getDecisionSummaries: mocks.summaries } }))

beforeEach(() => vi.clearAllMocks())
afterEach(cleanup)

const show = async (record: Partial<DecisionRecord>) => {
  mocks.summaries.mockResolvedValue([
    { id: 1, seq: 7, timestamp: '2026-10-29T06:00:00Z', ...record },
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

it('says a rule stopped an order without calling it a fault', async () => {
  await show({
    success: true,
    decisions: [
      {
        action: 'open_long',
        symbol: 'SOLUSDT',
        success: false,
        blocked: true,
        timestamp: '2026-08-15T12:00:00Z',
      } as DecisionRecord['decisions'][number],
    ],
  })
  expect(screen.getByText('1 stopped by a rule')).toBeInTheDocument()
  expect(screen.queryByText(/Fault/)).toBeNull()
})

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
  expect(screen.getByText('Open long SOL')).toBeInTheDocument()
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
  expect(screen.getByText('Stopped by a rule: cooldown')).toBeInTheDocument()
  expect(screen.getByText('Succeeded')).toBeInTheDocument()
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

it('reads the log in the language in use, and leaves what it has no words for', () => {
  const cases: [string, string][] = [
    ['model call duration: 3114 ms', '模型调用耗时 3114 ms'],
    ['✓ SOLUSDT open_long succeeded', '成功'],
    ['🧊 SOLUSDT open_long blocked: halted', '被规则拦下：halted'],
    [
      '1 of 3 trade actions failed or were blocked',
      '3 笔交易中有 1 笔失败或被拦下',
    ],
    ['Failed to get agent decision: boom', '没有拿到模型的决定：boom'],
    ['something the server just said', 'something the server just said'],
  ]
  for (const [raw, want] of cases) expect(logText(raw, 'zh-CN')).toBe(want)
  expect(actionName('OPEN_SHORT', 'zh-CN')).toBe('开空')
  expect(actionName('something_new', 'zh-CN')).toBe('something_new')
})
