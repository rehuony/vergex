import { API_BASE, httpClient } from './helpers'

export type ForecastInterval = '15m' | '1h' | '4h' | '1d'
export const FORECAST_INTERVALS: ForecastInterval[] = ['15m', '1h', '4h', '1d']

export type ForecastDirection = 'up' | 'down' | 'neutral'

export interface ForecastRecord {
  id: number
  version_id: number
  series_id: number
  open_time: number
  symbol: string
  interval: ForecastInterval
  close: number
  prob_candles: number
  prob_full: number
  display_prob: number
  calibrated: boolean
  direction: ForecastDirection
  confidence: number
  expected_move_pct: number
  support: number
  resistance: number
  regime: string
  horizon: number
  target_time: number
  status: 'open' | 'verified' | 'void'
  outcome?: 'hit' | 'miss' | 'neutral'
  target_close?: number
  actual_move_pct?: number
}

export interface ForecastSignal {
  symbol: string
  interval: ForecastInterval
  tier: number
  price: number | null
  record: ForecastRecord | null
  stale: boolean
  pending?: string
}

export interface ForecastFactor {
  name: string
  value: number
  weight: number
  contribution: number
}

export interface ForecastResult {
  close: number
  prob_candles: number
  prob_full: number
  direction: ForecastDirection
  confidence: number
  expected_move_pct: number
  support: number
  resistance: number
  regime: string
  adx: number
  horizon: number
  has_derivatives: boolean
  factors: ForecastFactor[]
}

export interface ForecastDetail {
  symbol: string
  interval: ForecastInterval
  price: number | null
  ready: boolean
  pending?: string
  result: ForecastResult | null
  latest: ForecastRecord | null
  records: ForecastRecord[]
  hits: number
  misses: number
  candles: {
    time: number
    open: number
    high: number
    low: number
    close: number
    volume: number
  }[]
  last_closed: number
}

export interface FactorConfig {
  enabled: boolean
  trending: number
  ranging: number
}

export interface ForecastConfig {
  scorer: string
  factors: Record<string, FactorConfig>
  adx_threshold: number
  slope: number
  direction_threshold: number
  horizons: Record<string, number>
}

export interface ForecastVersion {
  id: number
  note: string
  engine_version: number
  config: ForecastConfig
  created_at: number
}

export interface ForecastVersions {
  active_version_id: number
  versions: ForecastVersion[]
  default_config: ForecastConfig
}

export interface BacktestMetrics {
  samples: number
  signals: number
  hits: number
  misses: number
  hit_rate: number | null
  baseline_up: number
  brier: number
  baseline_brier: number
  lift: number
  avg_return_pct: number
  avg_return_after_fee_pct: number
  move_coverage: number
  derivative_coverage: number
}

export interface BacktestReport {
  fee_pct: number
  overall: BacktestMetrics
  by_interval: Record<string, BacktestMetrics>
  by_regime: Record<string, BacktestMetrics>
  by_symbol: Record<string, BacktestMetrics>
  reliability:
    | {
        from: number
        to: number
        samples: number
        avg_prob: number
        up_rate: number
      }[]
    | null
  first_time: number
  last_time: number
  skipped?: string[] | null
}

export interface BacktestRun {
  id: number
  version_id: number
  options: {
    symbols: string[]
    intervals: string[]
    from: number
    to: number
  }
  status: 'running' | 'finished' | 'failed'
  done: number
  total: number
  report?: BacktestReport
  error?: string
  created_at: number
  finished_at?: number
}

export interface HistoryCoverage {
  symbol: string
  interval: string
  candles: number
  first: number
  last: number
}

async function read<T>(path: string, failure: string): Promise<T> {
  const result = await httpClient.request<T>(`${API_BASE}${path}`, {
    silent: true,
  })
  if (!result.success) throw new Error(result.message || failure)
  return result.data!
}

export const forecastApi = {
  getForecastSignals(interval: ForecastInterval) {
    return read<{ version_id: number; signals: ForecastSignal[] }>(
      `/forecast/signals?interval=${interval}`,
      'Failed to load forecast signals'
    )
  },

  getForecastDetail(symbol: string, interval: ForecastInterval) {
    return read<ForecastDetail>(
      `/forecast/detail/${encodeURIComponent(symbol)}?interval=${interval}`,
      'Failed to load the forecast'
    )
  },

  getForecastVersions() {
    return read<ForecastVersions>(
      '/forecast/versions',
      'Failed to load forecast versions'
    )
  },

  async createForecastVersion(
    config: ForecastConfig,
    note: string
  ): Promise<ForecastVersion> {
    const result = await httpClient.post<ForecastVersion>(
      `${API_BASE}/forecast/versions`,
      { config, note }
    )
    if (!result.success)
      throw new Error(result.message || 'Failed to save the version')
    return result.data!
  },

  async activateForecastVersion(id: number): Promise<void> {
    const result = await httpClient.post(
      `${API_BASE}/forecast/versions/${id}/activate`
    )
    if (!result.success)
      throw new Error(result.message || 'Failed to use the version')
  },

  getForecastSettings() {
    return read<{ active_version_id: number; alert_threshold: number }>(
      '/forecast/settings',
      'Failed to load forecast settings'
    )
  },

  async saveForecastSettings(alertThreshold: number): Promise<void> {
    const result = await httpClient.put(`${API_BASE}/forecast/settings`, {
      alert_threshold: alertThreshold,
    })
    if (!result.success)
      throw new Error(result.message || 'Failed to save forecast settings')
  },

  getHistoryCoverage(symbols: string[], intervals: string[]) {
    const query = new URLSearchParams({
      symbols: symbols.join(','),
      intervals: intervals.join(','),
    })
    return read<{ coverage: HistoryCoverage[] }>(
      `/forecast/coverage?${query}`,
      'Failed to read local history'
    )
  },

  getForecastBacktests() {
    return read<{ backtests: BacktestRun[] }>(
      '/forecast/backtests',
      'Failed to load backtests'
    )
  },

  async startForecastBacktest(request: {
    version_id: number
    symbols: string[]
    intervals: string[]
    from: number
    to: number
  }): Promise<BacktestRun> {
    const result = await httpClient.post<BacktestRun>(
      `${API_BASE}/forecast/backtests`,
      request
    )
    if (!result.success)
      throw new Error(result.message || 'Failed to start the backtest')
    return result.data!
  },
}
