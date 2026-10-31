import { API_BASE, httpClient } from './helpers'

export interface MarketDataSettings {
  enabled: boolean
  watchlist: string[]
  top_n: number
}

export interface MarketDataStatus {
  enabled: boolean
  series: number
  ready: number
  behind: number
  failing: number
  last_error?: string
  watch_symbols: number
  top_symbols: number
  limiter: {
    paused_until?: string
    reason?: string
    banned: boolean
    used_weight: number
  }
  backfill: {
    running: boolean
    current?: string
    done: number
    total: number
    error?: string
    finished_at?: number
  }
}

export interface BackfillRequest {
  symbols: string[]
  intervals: string[]
  months: number
  derivatives: boolean
}

export const marketDataApi = {
  async getMarketDataStatus(): Promise<MarketDataStatus> {
    const result = await httpClient.request<MarketDataStatus>(
      `${API_BASE}/market-data/status`,
      { silent: true }
    )
    if (!result.success) throw new Error('Failed to read market data status')
    return result.data!
  },

  async getMarketDataSettings(): Promise<MarketDataSettings> {
    const result = await httpClient.request<MarketDataSettings>(
      `${API_BASE}/market-data/settings`,
      { silent: true }
    )
    if (!result.success) throw new Error('Failed to read market data settings')
    return result.data!
  },

  async saveMarketDataSettings(
    settings: MarketDataSettings
  ): Promise<MarketDataSettings> {
    const result = await httpClient.put<MarketDataSettings>(
      `${API_BASE}/market-data/settings`,
      settings
    )
    if (!result.success)
      throw new Error(result.message || 'Failed to save market data settings')
    return result.data!
  },

  async startBackfill(request: BackfillRequest): Promise<void> {
    const result = await httpClient.post(
      `${API_BASE}/market-data/backfill`,
      request
    )
    if (!result.success)
      throw new Error(result.message || 'Failed to start the download')
  },
}
