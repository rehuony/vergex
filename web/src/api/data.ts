import type {
  SystemStatus,
  AccountInfo,
  Position,
  DecisionRecord,
  EquityHistory,
  EquityRange,
  OrderFill,
  Statistics,
  TraderFullStats,
  TraderOrder,
  HistoricalPosition,
  PositionHistoryResponse,
} from '@/types'
import { API_BASE, httpClient } from './helpers'

export type AICostPeriod = 'today' | 'week' | 'month' | 'all'

export interface AICostTotals {
  total: number
  count: number
  unpriced_count: number
  prompt_tokens: number
  completion_tokens: number
}

export interface AICostSummary extends AICostTotals {
  by_model: Record<string, AICostTotals>
}

// The exchange adapters return their own field names (positionAmt, entryPrice,
// unRealizedProfit). A missing value stays undefined so callers can refuse it.
export function normalizePosition(raw: Record<string, unknown>): Position {
  const number = (...keys: string[]) => {
    for (const key of keys) {
      const value = raw[key]
      if (value === undefined || value === null || value === '') continue
      const parsed = Number(value)
      if (Number.isFinite(parsed)) return parsed
    }
    return undefined
  }
  const signed = number('quantity', 'positionAmt', 'size')
  const quantity = signed === undefined ? undefined : Math.abs(signed)
  const entry = number('entry_price', 'entryPrice')
  const pnl = number('unrealized_pnl', 'unRealizedProfit')
  const leverage = number('leverage')
  const margin =
    quantity && entry && leverage ? (quantity * entry) / leverage : undefined
  return {
    ...raw,
    quantity,
    entry_price: entry,
    mark_price: number('mark_price', 'markPrice'),
    unrealized_pnl: pnl,
    unrealized_pnl_pct:
      number('unrealized_pnl_pct') ??
      (margin && pnl !== undefined ? (pnl / margin) * 100 : undefined),
    leverage,
    liquidation_price: number('liquidation_price', 'liquidationPrice'),
    margin_used: number('margin_used') ?? margin,
  } as Position
}

export const dataApi = {
  async getStatus(traderId?: string, silent?: boolean): Promise<SystemStatus> {
    const url = traderId
      ? `${API_BASE}/status?trader_id=${traderId}`
      : `${API_BASE}/status`
    const result = await httpClient.request<SystemStatus>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch system status')
    return result.data!
  },

  async getAccount(traderId?: string, silent?: boolean): Promise<AccountInfo> {
    const url = traderId
      ? `${API_BASE}/account?trader_id=${traderId}`
      : `${API_BASE}/account`
    const result = await httpClient.request<AccountInfo>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch account info')
    return result.data!
  },

  async getPositions(
    traderId?: string,
    silent?: boolean,
    exchangeId?: string
  ): Promise<Position[]> {
    const url = exchangeId
      ? `${API_BASE}/positions?exchange_id=${encodeURIComponent(exchangeId)}`
      : traderId
        ? `${API_BASE}/positions?trader_id=${encodeURIComponent(traderId)}`
        : `${API_BASE}/positions`
    const result = await httpClient.request<Record<string, unknown>[]>(url, {
      silent,
    })
    if (!result.success) throw new Error('Failed to fetch positions')
    return result.data!.map(normalizePosition)
  },

  async getDecisions(traderId?: string): Promise<DecisionRecord[]> {
    const url = traderId
      ? `${API_BASE}/decisions?trader_id=${traderId}`
      : `${API_BASE}/decisions`
    const result = await httpClient.get<DecisionRecord[]>(url)
    if (!result.success) throw new Error('Failed to fetch decision logs')
    return result.data!
  },

  async getDecisionSummaries(
    traderId: string,
    limit: number,
    before?: number,
    silent?: boolean
  ): Promise<DecisionRecord[]> {
    const params = new URLSearchParams({
      trader_id: traderId,
      limit: String(limit),
    })
    if (before) params.append('before', String(before))
    const result = await httpClient.request<DecisionRecord[]>(
      `${API_BASE}/decisions/summary?${params}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch decisions')
    return result.data ?? []
  },

  async getDecision(
    traderId: string,
    decisionId: number,
    silent?: boolean
  ): Promise<DecisionRecord> {
    const result = await httpClient.request<DecisionRecord>(
      `${API_BASE}/decisions/${decisionId}?trader_id=${traderId}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch decision')
    return result.data!
  },

  async getEquity(
    traderId: string,
    range: EquityRange,
    silent?: boolean
  ): Promise<EquityHistory> {
    const result = await httpClient.request<EquityHistory>(
      `${API_BASE}/equity?trader_id=${traderId}&range=${range}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch equity')
    return result.data!
  },

  async getOrders(
    traderId: string,
    limit: number,
    silent?: boolean
  ): Promise<TraderOrder[]> {
    const result = await httpClient.request<TraderOrder[]>(
      `${API_BASE}/orders?trader_id=${traderId}&limit=${limit}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch orders')
    return result.data ?? []
  },

  async getOrderFills(
    traderId: string,
    orderId: number,
    silent?: boolean
  ): Promise<OrderFill[]> {
    const result = await httpClient.request<OrderFill[]>(
      `${API_BASE}/orders/${orderId}/fills?trader_id=${traderId}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch order fills')
    return result.data ?? []
  },

  async getLatestDecisions(
    traderId?: string,
    limit: number = 5,
    silent?: boolean
  ): Promise<DecisionRecord[]> {
    const params = new URLSearchParams()
    if (traderId) {
      params.append('trader_id', traderId)
    }
    params.append('limit', limit.toString())

    const result = await httpClient.request<DecisionRecord[]>(
      `${API_BASE}/decisions/latest?${params}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch latest decisions')
    return result.data!
  },

  async getStatistics(
    traderId?: string,
    silent?: boolean
  ): Promise<Statistics> {
    const url = traderId
      ? `${API_BASE}/statistics?trader_id=${traderId}`
      : `${API_BASE}/statistics`
    const result = await httpClient.request<Statistics>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch statistics')
    return result.data!
  },

  async getFullStats(
    traderId?: string,
    silent?: boolean
  ): Promise<TraderFullStats> {
    const url = traderId
      ? `${API_BASE}/statistics/full?trader_id=${traderId}`
      : `${API_BASE}/statistics/full`
    const result = await httpClient.request<TraderFullStats>(url, { silent })
    if (!result.success) throw new Error('Failed to fetch full statistics')
    return result.data!
  },

  async getPositionHistory(
    traderId: string,
    limit: number = 100,
    silent?: boolean
  ): Promise<PositionHistoryResponse> {
    const result = await httpClient.request<PositionHistoryResponse>(
      `${API_BASE}/positions/history?trader_id=${traderId}&limit=${limit}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch position history')
    return result.data!
  },

  async getOlderPositions(
    traderId: string,
    before: number,
    limit: number
  ): Promise<HistoricalPosition[]> {
    const result = await httpClient.request<{
      positions: HistoricalPosition[]
    }>(
      `${API_BASE}/positions/history?trader_id=${traderId}&limit=${limit}&before=${before}`,
      { silent: true }
    )
    if (!result.success) throw new Error('Failed to fetch position history')
    return result.data?.positions ?? []
  },

  async getAICostSummary(
    period: AICostPeriod = 'today',
    silent?: boolean,
    traderId?: string
  ): Promise<AICostSummary> {
    const trader = traderId ? `&trader_id=${traderId}` : ''
    const result = await httpClient.request<AICostSummary>(
      `${API_BASE}/ai-costs/summary?period=${period}${trader}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch AI costs')
    return result.data!
  },
}
