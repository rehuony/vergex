import { API_BASE, httpClient } from './helpers'

export interface OverviewAccount {
  id: string
  name: string
  exchange_type: string
  status: string
  asset?: string
  total_equity?: number
  error_message?: string
}

export interface OverviewModel {
  id: string
  name: string
  provider: string
  model?: string
  host?: string
}

export interface OverviewTrader {
  trader_id: string
  trader_name: string
  is_running: boolean
  agent_model_id: string
  exchange_id: string
  strategy_name?: string
}

export interface OverviewRun {
  id: string
  exchange_id: string
  symbol: string
  state: string
  updated_at: string
}

export interface OverviewPosition {
  exchange_id: string
  /** Empty when Frontier manages the position. */
  trader_id?: string
  symbol: string
  side: 'long' | 'short'
  quantity: number
  entry_price: number
  mark_price: number
  unrealized_pnl: number
  unrealized_pnl_pct: number
  leverage: number
}

export interface OverviewAttention {
  kind: string
  name?: string
  detail?: string
  trader_id?: string
  exchange_id?: string
  plan_id?: string
}

export interface Overview {
  generated_at: string
  accounts: OverviewAccount[]
  models: OverviewModel[]
  traders: OverviewTrader[]
  runs: OverviewRun[]
  /** Absent when market data is unavailable. */
  market?: { coins: number; live: boolean }
  /** The forecast strategy in use. Absent when Forecast is unavailable. */
  forecast?: { name: string; coins: number }
  telegram_bound: boolean
  positions: OverviewPosition[]
  positions_partial: boolean
  realized_today: number | null
  attention: OverviewAttention[]
}

export const overviewApi = {
  async getOverview(silent?: boolean): Promise<Overview> {
    const result = await httpClient.request<Overview>(`${API_BASE}/overview`, {
      silent,
    })
    if (!result.success) throw new Error('Failed to fetch the overview')
    return result.data!
  },
}
