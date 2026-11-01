import type {
  GridRiskInfo,
  LaunchPreflight,
  TraderInfo,
  TraderMetrics,
  TraderConfigData,
  CreateTraderRequest,
} from '@/types'
import { API_BASE, httpClient } from './helpers'
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

export const traderApi = {
  async getTraders(silent?: boolean): Promise<TraderInfo[]> {
    const result = await httpClient.request<TraderInfo[]>(
      `${API_BASE}/my-traders`,
      { silent }
    )
    if (!result.success) throw new Error('Failed to fetch trader list')
    return Array.isArray(result.data) ? result.data : []
  },

  async getTraderMetrics(silent?: boolean) {
    const result = await httpClient.request<Record<string, TraderMetrics>>(
      `${API_BASE}/my-traders/metrics`,
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

  async stopTrader(traderId: string): Promise<void> {
    const result = await httpClient.post(`${API_BASE}/traders/${traderId}/stop`)
    if (!result.success) throw new Error('Failed to stop trader')
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

  async getGridRiskInfo(traderId: string): Promise<GridRiskInfo> {
    const result = await httpClient.request<GridRiskInfo>(
      `${API_BASE}/traders/${traderId}/grid-risk`,
      { silent: true }
    )
    if (!result.success) throw new Error('Failed to fetch grid risk')
    return result.data!
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
    request: CreateTraderRequest
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
}
