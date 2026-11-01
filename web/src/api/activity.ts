import { API_BASE, getAuthHeaders } from './helpers'

export type ActivityKind = 'agent' | 'orders' | 'funds' | 'market'

export type ActivityBlock =
  | 'market'
  | 'telegram'
  | 'traders'
  | 'frontier'
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

export interface ActivityMessage {
  type: 'snapshot' | 'events' | 'totals'
  seq: number
  at: number
  events: ActivityEvent[]
  totals: ActivityTotals
}

// Follows the activity stream until it ends or the signal aborts. It uses fetch
// because the stream needs the Authorization header, which EventSource cannot send.
export async function streamActivity(
  signal: AbortSignal,
  onMessage: (message: ActivityMessage) => void
): Promise<void> {
  const response = await fetch(`${API_BASE}/activity/stream`, {
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
