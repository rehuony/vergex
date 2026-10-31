import { API_BASE, httpClient } from './helpers'

export type ForecastInterval = '15m' | '1h' | '4h' | '1d'
export const FORECAST_INTERVALS: ForecastInterval[] = ['15m', '1h', '4h', '1d']

export type ForecastDirection = 'up' | 'down' | 'neutral'

export interface ForecastRecord {
  id: number
  strategy_id: number
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
  stats: {
    hits: number
    misses: number
    // Judged records where price moved, and how many of them rose.
    moved: number
    rose: number
  }
  candles: ForecastCandle[]
  last_closed: number
  // The candle still open, while the live stream is delivering it.
  forming: ForecastCandle | null
  // The forecast drawn as candles: a restatement of the reading, not prices.
  projection: ForecastCandle[]
  indicators?: ForecastIndicators
}

// What another page's chart shows of a coin's forecast under the strategy in use.
export interface ForecastOverlay {
  symbol: string
  interval: string
  pending?: string
  direction?: ForecastDirection
  display_prob: number
  calibrated: boolean
  stats: ForecastDetail['stats']
  projection: ForecastCandle[]
}

export interface ForecastIndicatorPoint {
  time: number
  value: number
}

export interface ForecastIndicators {
  ema20: ForecastIndicatorPoint[]
  ema50: ForecastIndicatorPoint[]
  boll_upper: ForecastIndicatorPoint[]
  boll_middle: ForecastIndicatorPoint[]
  boll_lower: ForecastIndicatorPoint[]
  vwap: ForecastIndicatorPoint[]
  rsi: ForecastIndicatorPoint[]
}

export interface ForecastCandle {
  time: number
  open: number
  high: number
  low: number
  close: number
  volume: number
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

export interface ForecastStrategy {
  id: number
  name: string
  engine_version: number
  config: ForecastConfig
  created_at: number
}

export interface ForecastStrategies {
  active_strategy_id: number
  strategies: ForecastStrategy[]
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
  strategy_id: number
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
    return read<{
      strategy_id: number
      interval: ForecastInterval
      signals: ForecastSignal[]
    }>(
      `/forecast/signals?interval=${interval}`,
      'Failed to load forecast signals'
    )
  },

  getForecastDetail(
    symbol: string,
    interval: ForecastInterval,
    indicators = false
  ) {
    const extra = indicators ? '&indicators=1' : ''
    return read<ForecastDetail>(
      `/forecast/detail/${encodeURIComponent(symbol)}?interval=${interval}${extra}`,
      'Failed to load the forecast'
    )
  },

  getForecastOverlay(symbol: string, interval: string) {
    return read<ForecastOverlay>(
      `/forecast/overlay/${encodeURIComponent(symbol)}?interval=${interval}`,
      'Failed to load the forecast'
    )
  },

  getForecastStrategies() {
    return read<ForecastStrategies>(
      '/forecast/strategies',
      'Failed to load forecast strategies'
    )
  },

  async createForecastStrategy(
    config: ForecastConfig,
    name: string
  ): Promise<ForecastStrategy> {
    const result = await httpClient.post<ForecastStrategy>(
      `${API_BASE}/forecast/strategies`,
      { config, name }
    )
    if (!result.success)
      throw new Error(result.message || 'Failed to save the strategy')
    return result.data!
  },

  async activateForecastStrategy(id: number): Promise<void> {
    const result = await httpClient.post(
      `${API_BASE}/forecast/strategies/${id}/activate`
    )
    if (!result.success)
      throw new Error(result.message || 'Failed to use the strategy')
  },

  async deleteForecastStrategy(id: number): Promise<void> {
    const result = await httpClient.delete(
      `${API_BASE}/forecast/strategies/${id}`
    )
    if (!result.success)
      throw new Error(result.message || 'Failed to delete the strategy')
  },

  getForecastSettings() {
    return read<{ active_strategy_id: number; alert_threshold: number }>(
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
    strategy_id: number
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
