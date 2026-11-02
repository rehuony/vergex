import type {
  AccountInfo,
  AgentConfig,
  CreateTraderRequest,
  DecisionRecord,
  DryRun,
  EquityHistory,
  EquityRange,
  GridConfig,
  GridRiskInfo,
  HistoricalPosition,
  LaunchPreflight,
  OrderFill,
  Position,
  PositionHistoryResponse,
  Statistics,
  SystemStatus,
  TokenEstimate,
  TraderConfigData,
  TraderFullStats,
  TraderInfo,
  TraderKind,
  TraderMetrics,
  TraderOrder,
  TraderPreset,
  UpdateTraderRequest,
} from '@/types'
import { API_BASE, httpClient, serverError } from './helpers'
import { ApiError } from './http-client'

// Create/update/start legitimately run long: stopping a live trader waits for
// its in-flight cycle and monitors, and creation probes the exchange (~35s
// worst case observed). The default 30s axios timeout aborts mid-operation and
// reports a false failure, so these calls get their own generous ceiling.
const TRADER_LIFECYCLE_TIMEOUT_MS = 120_000

function throwApiError(
  message: string,
  errorKey?: string,
  errorParams?: Record<string, string>,
  statusCode?: number,
  errorData?: Record<string, any>
): never {
  throw new ApiError(message, errorKey, errorParams, statusCode, errorData)
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

export const traderApi = {
  async getTraders(silent?: boolean): Promise<TraderInfo[]> {
    const result = await httpClient.request<TraderInfo[]>(
      `${API_BASE}/traders`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch trader list')
    return Array.isArray(result.data) ? result.data : []
  },

  async getTraderMetrics(silent?: boolean) {
    const result = await httpClient.request<Record<string, TraderMetrics>>(
      `${API_BASE}/traders/metrics`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch trader metrics')
    return result.data ?? {}
  },

  async getTraderPreflight(traderId: string): Promise<LaunchPreflight> {
    const result = await httpClient.request<LaunchPreflight>(
      `${API_BASE}/traders/${traderId}/preflight`,
      { silent: true, timeout: TRADER_LIFECYCLE_TIMEOUT_MS }
    )
    if (!result.success) throw new Error('Failed to check the trader')
    return result.data!
  },

  async createTrader(request: CreateTraderRequest): Promise<TraderInfo> {
    const result = await httpClient.request<TraderInfo>(`${API_BASE}/traders`, {
      method: 'POST',
      data: request,
      timeout: TRADER_LIFECYCLE_TIMEOUT_MS,
    })
    if (!result.success) {
      throwApiError(
        result.message || 'Failed to create trader',
        result.errorKey,
        result.errorParams,
        result.statusCode
      )
    }
    return result.data!
  },

  async deleteTrader(traderId: string): Promise<void> {
    const result = await httpClient.delete(`${API_BASE}/traders/${traderId}`)
    if (!result.success) throw new Error('Failed to delete trader')
  },

  async startTrader(traderId: string): Promise<void> {
    const result = await httpClient.request(
      `${API_BASE}/traders/${traderId}/start`,
      { method: 'POST', timeout: TRADER_LIFECYCLE_TIMEOUT_MS }
    )
    if (!result.success) {
      throwApiError(
        result.message || 'Failed to start trader',
        result.errorKey,
        result.errorParams,
        result.statusCode,
        result.errorData
      )
    }
  },

  /** Says whether a grid's orders could not be cancelled as it stopped. */
  async stopTrader(traderId: string): Promise<{ ordersLeft: boolean }> {
    const result = await httpClient.post<{ warning?: string }>(
      `${API_BASE}/traders/${traderId}/stop`
    )
    if (!result.success) throw new Error('Failed to stop trader')
    return { ordersLeft: result.data?.warning === 'orders_left' }
  },

  async resumeTrader(traderId: string): Promise<void> {
    const result = await httpClient.post(
      `${API_BASE}/traders/${traderId}/resume`
    )
    if (!result.success) throw new Error('Failed to resume trader')
  },

  async closePosition(
    traderId: string,
    symbol: string,
    side: string
  ): Promise<{ message: string }> {
    const result = await httpClient.post<{ message: string }>(
      `${API_BASE}/traders/${traderId}/close-position`,
      { symbol, side }
    )
    if (!result.success) throw new Error('Failed to close position')
    return result.data!
  },

  async getGridRiskInfo(traderId: string): Promise<GridRiskInfo | null> {
    const result = await httpClient.request<GridRiskInfo | null>(
      `${API_BASE}/traders/${traderId}/grid-risk`,
      { silent: true }
    )
    if (!result.success) throw new Error('Failed to fetch grid risk')
    return result.data ?? null
  },

  async getTraderConfig(
    traderId: string,
    silent?: boolean
  ): Promise<TraderConfigData> {
    const result = await httpClient.request<TraderConfigData>(
      `${API_BASE}/traders/${traderId}/config`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch trader config')
    return result.data!
  },

  async updateTrader(
    traderId: string,
    request: UpdateTraderRequest
  ): Promise<TraderInfo> {
    const result = await httpClient.request<TraderInfo>(
      `${API_BASE}/traders/${traderId}`,
      { method: 'PUT', data: request, timeout: TRADER_LIFECYCLE_TIMEOUT_MS }
    )
    if (!result.success) {
      throwApiError(
        result.message || 'Failed to update trader',
        result.errorKey,
        result.errorParams,
        result.statusCode
      )
    }
    return result.data!
  },

  async getTraderPresets(kind: TraderKind): Promise<TraderPreset[]> {
    const result = await httpClient.request<{ presets: TraderPreset[] }>(
      `${API_BASE}/traders/presets?kind=${kind}`,
      { silent: true }
    )
    if (!result.success) throw new Error('Failed to fetch presets')
    return result.data?.presets ?? []
  },

  async duplicateTrader(traderId: string, name: string): Promise<TraderInfo> {
    const result = await httpClient.post<TraderInfo>(
      `${API_BASE}/traders/${traderId}/duplicate`,
      { name }
    )
    if (!result.success)
      throw new Error(result.message || 'Failed to duplicate trader')
    return result.data!
  },

  async estimateTokens(config: AgentConfig): Promise<TokenEstimate> {
    const result = await httpClient.request<TokenEstimate>(
      `${API_BASE}/traders/estimate-tokens`,
      { method: 'POST', data: { config }, silent: true }
    )
    if (!result.success) throw new Error('Failed to estimate tokens')
    return result.data!
  },

  // Builds the prompts a config would send and, with a model, sends them. It
  // never places an order.
  async dryRun(body: {
    kind: TraderKind
    config: AgentConfig | GridConfig
    agent_model_id?: string
  }): Promise<DryRun> {
    const result = await httpClient.request<DryRun>(
      `${API_BASE}/traders/dry-run`,
      { method: 'POST', data: body, timeout: TRADER_LIFECYCLE_TIMEOUT_MS }
    )
    if (!result.success)
      throw new Error(result.message || 'The dry run did not work')
    return result.data!
  },

  async getStatus(traderId: string, silent?: boolean): Promise<SystemStatus> {
    const result = await httpClient.request<SystemStatus>(
      `${API_BASE}/traders/${traderId}/status`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch system status')
    return result.data!
  },

  async getAccount(traderId: string, silent?: boolean): Promise<AccountInfo> {
    const result = await httpClient.request<AccountInfo>(
      `${API_BASE}/traders/${traderId}/account`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch account info')
    return result.data!
  },

  async getPositions(traderId: string, silent?: boolean): Promise<Position[]> {
    const result = await httpClient.request<Record<string, unknown>[]>(
      `${API_BASE}/traders/${traderId}/open-positions`,
      { silent }
    )
    if (!result.success) throw serverError(result, 'Failed to fetch positions')
    return result.data!.map(normalizePosition)
  },

  async getDecisions(traderId: string): Promise<DecisionRecord[]> {
    const result = await httpClient.get<DecisionRecord[]>(
      `${API_BASE}/traders/${traderId}/decisions`
    )
    if (!result.success) throw new Error('Failed to fetch decision logs')
    return result.data!
  },

  async getDecisionSummaries(
    traderId: string,
    limit: number,
    before?: number,
    silent?: boolean
  ): Promise<DecisionRecord[]> {
    const params = new URLSearchParams({ limit: String(limit) })
    if (before) params.append('before', String(before))
    const result = await httpClient.request<DecisionRecord[]>(
      `${API_BASE}/traders/${traderId}/decisions/summary?${params}`,
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
      `${API_BASE}/traders/${traderId}/decisions/${decisionId}`,
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
      `${API_BASE}/traders/${traderId}/equity?range=${range}`,
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
      `${API_BASE}/traders/${traderId}/orders?limit=${limit}`,
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
      `${API_BASE}/traders/${traderId}/orders/${orderId}/fills`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch order fills')
    return result.data ?? []
  },

  async getLatestDecisions(
    traderId: string,
    limit: number = 5,
    silent?: boolean
  ): Promise<DecisionRecord[]> {
    const result = await httpClient.request<DecisionRecord[]>(
      `${API_BASE}/traders/${traderId}/decisions/latest?limit=${limit}`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch latest decisions')
    return result.data!
  },

  async getStatistics(traderId: string, silent?: boolean): Promise<Statistics> {
    const result = await httpClient.request<Statistics>(
      `${API_BASE}/traders/${traderId}/statistics`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch statistics')
    return result.data!
  },

  async getFullStats(
    traderId: string,
    silent?: boolean
  ): Promise<TraderFullStats> {
    const result = await httpClient.request<TraderFullStats>(
      `${API_BASE}/traders/${traderId}/statistics/full`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch full statistics')
    return result.data!
  },

  async getPositionHistory(
    traderId: string,
    limit: number = 100,
    silent?: boolean
  ): Promise<PositionHistoryResponse> {
    const result = await httpClient.request<PositionHistoryResponse>(
      `${API_BASE}/traders/${traderId}/positions?limit=${limit}`,
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
      `${API_BASE}/traders/${traderId}/positions?limit=${limit}&before=${before}`,
      { silent: true }
    )
    if (!result.success) throw new Error('Failed to fetch position history')
    return result.data?.positions ?? []
  },
}
