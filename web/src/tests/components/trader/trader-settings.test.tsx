import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryWrapper } from '@/tests/query-wrapper'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { TraderSettings } from '@/components/trader/trader-settings'

const mocks = vi.hoisted(() => ({
  config: vi.fn(),
  update: vi.fn(),
  estimate: vi.fn(),
}))
vi.mock('@/api', () => ({
  api: {
    getTraderConfig: mocks.config,
    updateTrader: mocks.update,
    estimateTokens: mocks.estimate,
    getTraderPresets: vi.fn().mockResolvedValue([]),
  },
}))
vi.mock('@/hooks/use-connections', () => ({
  useModelConfigs: () => ({
    data: [
      { id: 'm1', name: 'First', enabled: true },
      { id: 'm2', name: 'Second', enabled: true },
    ],
  }),
  useAccounts: () => ({ data: [{ id: 'a1', name: 'Main', venue: 'binance' }] }),
}))

const agentConfig = {
  coin_source: { source_type: 'static', static_coins: ['BTCUSDT'] },
  indicators: {
    klines: {
      primary_timeframe: '5m',
      primary_count: 30,
      selected_timeframes: ['5m'],
    },
  },
  risk_control: {
    max_positions: 3,
    btc_eth_max_leverage: 5,
    altcoin_max_leverage: 5,
    max_margin_usage: 0.9,
    min_position_size: 12,
    min_risk_reward_ratio: 3,
    min_confidence: 75,
  },
  prompt_sections: {},
  custom_prompt: '',
}

const show = (running = false) =>
  render(
    <MemoryRouter>
      <QueryWrapper>
        <TraderSettings traderId="t1" running={running} language="en" />
      </QueryWrapper>
    </MemoryRouter>
  )

const openRisk = async () =>
  fireEvent.click(await screen.findByRole('button', { name: 'Risk' }))

const setMaxPositions = (value: string) =>
  fireEvent.change(screen.getByLabelText('Max positions'), {
    target: { value },
  })

beforeEach(() => {
  vi.clearAllMocks()
  mocks.config.mockResolvedValue({
    kind: 'agent',
    config: agentConfig,
    agent_model: 'm1',
    account_id: 'a1',
    scan_interval_minutes: 15,
    is_cross_margin: true,
  })
  mocks.update.mockResolvedValue(undefined)
  mocks.estimate.mockResolvedValue({ total: 1200 })
})
afterEach(cleanup)

it('saves the edited settings and nothing else about the trader', async () => {
  show()
  await openRisk()
  const save = screen.getByRole('button', { name: 'Save' })
  fireEvent.click(save)
  expect(mocks.update).not.toHaveBeenCalled()

  setMaxPositions('5')
  fireEvent.click(save)

  await waitFor(() => expect(mocks.update).toHaveBeenCalledTimes(1))
  const [id, body] = mocks.update.mock.calls[0]
  expect(id).toBe('t1')
  expect(Object.keys(body)).toEqual(['config'])
  expect(body.config.risk_control.max_positions).toBe(5)
})

it('keeps unsaved edits while moving between parts of the form', async () => {
  show()
  await openRisk()
  setMaxPositions('7')

  fireEvent.click(screen.getByRole('button', { name: 'Coins' }))
  fireEvent.click(screen.getByRole('button', { name: 'Risk' }))

  expect(screen.getByLabelText('Max positions')).toHaveValue(7)
})

it('says when an edit reaches a running trader', async () => {
  show(true)
  await openRisk()
  expect(
    screen.queryByText('A running trader picks this up on its next cycle.')
  ).toBeNull()

  setMaxPositions('5')
  expect(
    screen.getByText('A running trader picks this up on its next cycle.')
  ).toBeVisible()
})

it('changes what a stopped trader runs with, and only that', async () => {
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Basics' }))
  fireEvent.change(screen.getByLabelText('Scan interval'), {
    target: { value: '30' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(mocks.update).toHaveBeenCalledTimes(1))
  const [, body] = mocks.update.mock.calls[0]
  expect(body).toEqual({
    agent_model_id: 'm1',
    account_id: 'a1',
    scan_interval_minutes: 30,
    is_cross_margin: true,
  })
})

it('locks what a running trader runs with, and says why', async () => {
  show(true)
  fireEvent.click(await screen.findByRole('button', { name: 'Basics' }))
  expect(screen.getByLabelText('Scan interval')).toBeDisabled()
  expect(screen.getByText('Stop the trader to edit it')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Cross' })).toBeDisabled()
})
