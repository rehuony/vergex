import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { Link, MemoryRouter } from 'react-router-dom'
import { SWRConfig } from 'swr'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { StrategiesPage } from '@/pages/strategies-page'
import type { Strategy } from '@/types'

const mocks = vi.hoisted(() => ({
  getStrategies: vi.fn(),
  updateStrategy: vi.fn(),
  duplicateStrategy: vi.fn(),
  estimate: vi.fn(),
  confirm: vi.fn(),
  getTraders: vi.fn(),
}))
vi.mock('@/contexts/auth-context', () => ({
  useAuth: () => ({ token: 'fixture' }),
}))
vi.mock('@/contexts/language-context', () => ({
  useLanguage: () => ({ language: 'en' }),
}))
vi.mock('@/api', () => ({
  api: {
    getStrategies: mocks.getStrategies,
    getTraders: mocks.getTraders,
    updateStrategy: mocks.updateStrategy,
    duplicateStrategy: mocks.duplicateStrategy,
    estimateStrategyTokens: mocks.estimate,
  },
}))
vi.mock('@/components/common/notify', () => ({
  notify: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
  confirmToast: mocks.confirm,
}))

const agentStrategy: Strategy = {
  id: 'agent',
  name: 'Trend',
  description: 'Old note',
  is_active: true,
  created_at: '',
  updated_at: '',
  config: {
    strategy_type: 'agent_trading',
    agent_config: {
      coin_source: { source_type: 'static', static_coins: ['BTCUSDT'] },
      indicators: {
        klines: {
          primary_timeframe: '5m',
          primary_count: 20,
          enable_multi_timeframe: false,
          selected_timeframes: ['5m'],
        },
        enable_raw_klines: true,
        enable_ema: false,
        enable_macd: false,
        enable_rsi: false,
        enable_atr: false,
        enable_boll: false,
        enable_volume: true,
        enable_oi: true,
        enable_funding_rate: true,
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
    },
  },
}

const gridStrategy: Strategy = {
  ...agentStrategy,
  id: 'grid',
  name: 'Range',
  description: '',
  is_active: false,
  config: {
    strategy_type: 'grid_trading',
    grid_config: {
      symbol: 'BTCUSDT',
      grid_count: 10,
      total_investment: 1000,
      leverage: 5,
      upper_price: 0,
      lower_price: 0,
      use_atr_bounds: true,
      atr_multiplier: 2,
      distribution: 'gaussian',
      max_drawdown_pct: 15,
      stop_loss_pct: 5,
      daily_loss_limit_pct: 10,
      use_maker_only: true,
    },
  },
}

const show = (entry = '/') =>
  render(
    <MemoryRouter initialEntries={[entry]}>
      <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
        <Link to="?settings=models">Open settings</Link>
        <StrategiesPage />
      </SWRConfig>
    </MemoryRouter>
  )

const save = () => screen.getByRole('button', { name: 'Save' })

beforeEach(() => {
  vi.clearAllMocks()
  mocks.getStrategies.mockResolvedValue([agentStrategy, gridStrategy])
  mocks.updateStrategy.mockResolvedValue({})
  mocks.estimate.mockResolvedValue({ total: 6434, model_limits: [] })
  mocks.getTraders.mockResolvedValue([])
})
afterEach(cleanup)

it('opens the active strategy and saves an edited limit', async () => {
  show()
  fireEvent.click(await screen.findByRole('tab', { name: 'Risk' }))
  expect(save()).toBeDisabled()

  fireEvent.change(screen.getByLabelText('Max positions'), {
    target: { value: '5' },
  })
  fireEvent.click(save())

  await waitFor(() => expect(mocks.updateStrategy).toHaveBeenCalled())
  const [id, body] = mocks.updateStrategy.mock.calls[0]
  expect(id).toBe('agent')
  expect(body.config.agent_config.risk_control.max_positions).toBe(5)
})

it('asks before leaving unsaved edits and stays when told to', async () => {
  mocks.confirm.mockResolvedValue(false)
  show()
  fireEvent.change(await screen.findByLabelText('Name'), {
    target: { value: 'Renamed' },
  })

  fireEvent.click(screen.getByRole('button', { name: /Range/ }))
  await waitFor(() => expect(mocks.confirm).toHaveBeenCalled())
  expect(screen.getByLabelText('Name')).toHaveValue('Renamed')
})

it('opens a linked strategy and keeps edits when the URL changes', async () => {
  show('/strategies?strategy=grid')
  const nameField = await screen.findByLabelText('Name')
  expect(nameField).toHaveValue('Range')
  fireEvent.change(nameField, { target: { value: 'Renamed' } })

  fireEvent.click(screen.getByRole('link', { name: 'Open settings' }))
  expect(mocks.getStrategies).toHaveBeenCalledTimes(1)
  expect(screen.getByLabelText('Name')).toHaveValue('Renamed')
})

it('keeps a grid strategy unsaved until its price range is valid', async () => {
  show()
  fireEvent.click(await screen.findByRole('button', { name: /Range/ }))
  fireEvent.click(
    await screen.findByRole('switch', { name: 'ATR price range' })
  )

  expect(
    screen.getByText('The upper price must be above a lower price above 0.')
  ).toBeVisible()
  expect(save()).toBeDisabled()

  fireEvent.change(screen.getByLabelText('Upper price'), {
    target: { value: '90000' },
  })
  fireEvent.change(screen.getByLabelText('Lower price'), {
    target: { value: '60000' },
  })
  expect(save()).toBeEnabled()
})

it('duplicates a saved strategy under a new name, but not unsaved edits', async () => {
  mocks.duplicateStrategy.mockResolvedValue({ id: 'copy' })
  show()
  const duplicate = await screen.findByRole('button', { name: 'Duplicate' })
  fireEvent.click(duplicate)
  await waitFor(() =>
    expect(mocks.duplicateStrategy).toHaveBeenCalledWith('agent', 'Trend copy')
  )
  await waitFor(() => expect(mocks.getStrategies).toHaveBeenCalledTimes(2))

  fireEvent.change(screen.getByLabelText('Name'), {
    target: { value: 'Renamed' },
  })
  expect(screen.getByRole('button', { name: 'Duplicate' })).toBeDisabled()
})

it('shows a retry when the list cannot load', async () => {
  mocks.getStrategies.mockRejectedValueOnce(new Error('down'))
  show()
  fireEvent.click(await screen.findByRole('button', { name: 'Retry' }))
  expect(await screen.findByLabelText('Name')).toHaveValue('Trend')
})

it('shows who uses a strategy, warns before changing running traders, and keeps it from deletion', async () => {
  mocks.getTraders.mockResolvedValue([
    {
      trader_id: 't1',
      trader_name: 'Runner',
      strategy_id: gridStrategy.id,
      is_running: true,
    },
  ])
  show()
  fireEvent.click(await screen.findByRole('button', { name: /Range/ }))

  expect(
    await screen.findByRole('link', { name: 'Runner' })
  ).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled()

  fireEvent.change(screen.getByLabelText('Name'), {
    target: { value: 'Range 2' },
  })
  expect(
    screen.getByText('Saving reaches running traders on their next cycle: 1.')
  ).toBeInTheDocument()
})
