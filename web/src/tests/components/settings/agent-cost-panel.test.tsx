import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, expect, it, vi } from 'vitest'
import { AgentCostPanel } from '@/components/settings/agent-cost-panel'

const mocks = vi.hoisted(() => ({ summary: vi.fn() }))
vi.mock('@/api', () => ({
  api: {
    getAgentCostSummary: mocks.summary,
    getTraders: async () => [
      { trader_id: 't1', trader_name: 'Trend follower' },
    ],
  },
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
// A native select keeps the trader choice easy to drive here.
vi.mock('@/components/ui/select', () => ({
  AppSelect: ({
    value,
    options,
    onChange,
    'aria-label': label,
  }: {
    value: string
    options: { value: string; label: string }[]
    onChange: (value: string) => void
    'aria-label': string
  }) => (
    <select
      aria-label={label}
      value={value}
      onChange={(e) => onChange(e.target.value)}
    >
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  ),
}))
afterEach(() => {
  cleanup()
  mocks.summary.mockReset()
})

function show() {
  return render(
    <QueryWrapper>
      <AgentCostPanel />
    </QueryWrapper>
  )
}

it('shows tokens and estimated cost per model and flags unpriced calls', async () => {
  mocks.summary.mockResolvedValue({
    total: 0.0123,
    count: 3,
    unpriced_count: 1,
    prompt_tokens: 12000,
    completion_tokens: 800,
    by_model: {
      'deepseek-v3': {
        total: 0.0123,
        count: 2,
        unpriced_count: 0,
        prompt_tokens: 10000,
        completion_tokens: 600,
      },
      'my-local-model': {
        total: 0,
        count: 1,
        unpriced_count: 1,
        prompt_tokens: 2000,
        completion_tokens: 200,
      },
    },
  })
  show()
  expect(await screen.findByText('≥ $0.0123')).toBeVisible()
  expect(screen.getByText('12,000')).toBeVisible()
  expect(screen.getByText('Unpriced')).toBeVisible()
  expect(mocks.summary).toHaveBeenCalledWith('today', true, undefined)

  fireEvent.click(screen.getByRole('button', { name: '30 days' }))
  expect(mocks.summary).toHaveBeenLastCalledWith('month', true, undefined)
})

it('reports an empty period without inventing costs', async () => {
  mocks.summary.mockResolvedValue({
    total: 0,
    count: 0,
    unpriced_count: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    by_model: {},
  })
  show()
  expect(
    await screen.findByText('No model calls recorded in this period.')
  ).toBeVisible()
  expect(screen.getByText('$0.00')).toBeVisible()
})

it('narrows the summary to one trader', async () => {
  mocks.summary.mockResolvedValue({
    total: 0,
    count: 0,
    unpriced_count: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    by_model: {},
  })
  show()
  await screen.findByText('No model calls recorded in this period.')
  expect(mocks.summary).toHaveBeenLastCalledWith('today', true, undefined)

  await screen.findByRole('option', { name: 'Trend follower' })
  fireEvent.change(screen.getByLabelText('Trader'), {
    target: { value: 't1' },
  })
  await waitFor(() =>
    expect(mocks.summary).toHaveBeenLastCalledWith('today', true, 't1')
  )
})
