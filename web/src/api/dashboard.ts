import { API_BASE, getAuthHeaders, httpClient } from './helpers'

export interface OverviewAccount {
  id: string
  name: string
  venue: string
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
  running: boolean
  agent_model_id: string
  account_id: string
  kind?: string
}

export interface OverviewRun {
  id: string
  account_id: string
  symbol: string
  status: string
  updated_at: string
}

export interface OverviewPosition {
  account_id: string
  /** Empty when Futures manages the position. */
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
  account_id?: string
  plan_id?: string
}

export interface Overview {
  generated_at: string
  accounts: OverviewAccount[]
  models: OverviewModel[]
  traders: OverviewTrader[]
  runs: OverviewRun[]
  /** Absent when market data is unavailable. */
  market?: { coins: number; live: boolean; symbols: string[] }
  /** The forecast strategy in use. Absent when Forecast is unavailable. */
  forecast?: { name: string; coins: number; symbols: string[] }
  telegram_bound: boolean
  positions: OverviewPosition[]
  positions_partial: boolean
  realized_today: number | null
  attention: OverviewAttention[]
}

export const overviewApi = {
  async getOverview(silent?: boolean): Promise<Overview> {
    const result = await httpClient.request<Overview>(
      `${API_BASE}/dashboard/overview`,
      {
        silent,
      }
    )
    if (!result.success) throw new Error('Failed to fetch the overview')
    return result.data!
  },
}

export type ActivityKind = 'agent' | 'orders' | 'funds' | 'market'

export type ActivityBlock =
  | 'exchange'
  | 'market'
  | 'telegram'
  | 'traders'
  | 'futures'
  | 'forecast'
  | 'models'
  | 'accounts'
  | 'positions'
  | 'agent_cost'
  | 'fees'
  | 'other'

export interface ActivityEnd {
  block: ActivityBlock
  id?: string
  name?: string
}

export interface ActivityEvent {
  seq: number
  id: string
  /** Epoch milliseconds. */
  at: number
  kind: ActivityKind
  type:
    | 'agent_call'
    | 'agent_cost'
    | 'decision'
    | 'market_read'
    | 'forecast'
    | 'order'
    | 'plan_order'
    | 'fee'
    | 'margin'
    | 'realized'
  from: ActivityEnd
  to: ActivityEnd
  symbol?: string
  /** The coins a grouped event covers, when it names them. */
  symbols?: string[]
  action?: string
  quantity?: number
  tokens?: number
  /** Coins read, or actions decided. */
  items?: number
  /** In asset, or dollars when there is none. Fees and agent costs are positive. */
  amount?: number
  asset?: string
  status?: string
  detail?: string
  /** The decision record or plan it opens. */
  ref?: string
  trader_id?: string
}

export interface ActivityTotal {
  count: number
  tokens?: number
  items?: number
  amount?: number
}

/** Keyed by route: the source block, ">", the destination block. */
export type ActivityTotals = Record<
  string,
  { h1: ActivityTotal; h24: ActivityTotal }
>

/** What the market streams delivered in the feed's latest round. */
export interface ActivityStream {
  at: number
  messages: number
  topics: number
  connected: boolean
}

export interface ActivityMessage {
  type: 'snapshot' | 'events' | 'totals' | 'stream'
  seq: number
  at: number
  events: ActivityEvent[]
  totals: ActivityTotals
  stream?: ActivityStream
}

// Follows the activity stream until it ends or the signal aborts. It uses fetch
// because the stream needs the Authorization header, which EventSource cannot send.
export async function streamActivity(
  signal: AbortSignal,
  onMessage: (message: ActivityMessage) => void
): Promise<void> {
  const response = await fetch(`${API_BASE}/dashboard/events`, {
    headers: getAuthHeaders(),
    signal,
  })
  if (!response.ok || !response.body) {
    throw new Error('The activity stream could not be opened')
  }
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) return
    buffer += decoder.decode(value, { stream: true })
    let at: number
    while ((at = buffer.indexOf('\n\n')) >= 0) {
      const frame = buffer.slice(0, at)
      buffer = buffer.slice(at + 2)
      if (frame.startsWith('data: ')) {
        onMessage(JSON.parse(frame.slice(6)) as ActivityMessage)
      }
    }
  }
}
