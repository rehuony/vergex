// What runs a trader. Set when it is created and never changes.
export type TraderKind = 'agent' | 'grid'

// A trader carries its kind and what that kind decides.
// A starting config the server offers. Its name comes from i18n by id.
export interface TraderPreset<Config = AgentConfig | GridConfig> {
  id: string
  config: Config
}

export interface CreateTraderRequest {
  name: string
  agent_model_id: string
  account_id: string
  kind: TraderKind
  preset?: string
  config?: Partial<AgentConfig> | Partial<GridConfig>
  scan_interval_minutes?: number
  is_cross_margin?: boolean
}

// Only what is sent changes. A config merges into the trader's by section.
export interface UpdateTraderRequest {
  name?: string
  agent_model_id?: string
  account_id?: string
  config?: AgentConfig | GridConfig
  scan_interval_minutes?: number
  is_cross_margin?: boolean
}

export interface SystemStatus {
  trader_id: string
  trader_name: string
  agent_model: string
  is_running: boolean
  start_time: string
  runtime_minutes: number
  call_count: number
  initial_balance: number
  scan_interval: string
  agent_provider: string
  kind?: TraderKind
  grid_symbol?: string
  /** Runtime health: true when the model failed repeatedly and no new positions open. */
  safe_mode?: boolean
  safe_mode_reason?: string
  trading_blocked?: boolean
  trading_error?: string
  trading_checked_at?: string
  /** A hard stop that has tripped: opens stop until it ends, or until it is resumed. */
  halted?: { at: string; reason: string; until?: string }
}

export interface AccountInfo {
  total_equity: number
  wallet_balance: number
  unrealized_profit: number // Unrealized PnL (official value from the exchange API)
  available_balance: number
  // By the ledger; absent when it could not be read.
  total_pnl?: number
  total_pnl_pct?: number
  initial_balance: number
  position_count: number
  margin_used: number
  margin_used_pct: number
}

export interface Position {
  symbol: string
  side: string
  entry_price: number
  mark_price: number
  quantity: number
  leverage: number
  unrealized_pnl: number
  unrealized_pnl_pct: number
  liquidation_price: number
  margin_used: number
  /** On the trader's account, but not its own in the ledger. */
  foreign?: boolean
}

export interface DecisionAction {
  action: string
  symbol: string
  quantity: number
  leverage: number
  price: number
  stop_loss?: number // Stop loss price
  take_profit?: number // Take profit price
  confidence?: number // agent confidence (0-100)
  reasoning?: string // Brief reasoning
  order_id: number
  timestamp: string
  success: boolean
  error?: string
  // A rule stopped it, and nothing reached the exchange.
  blocked?: boolean
}

export interface AccountSnapshot {
  total_balance: number
  available_balance: number
  total_unrealized_profit: number
  position_count: number
  margin_used_pct: number
}

export interface DecisionRecord {
  id: number
  timestamp: string
  cycle_number: number
  system_prompt: string
  input_prompt: string
  cot_trace: string
  raw_response?: string
  account_state: AccountSnapshot | null
  candidate_coins: string[]
  decisions: DecisionAction[]
  execution_log: string[]
  success: boolean
  error_message?: string
}

// Stored totals of one trader. A value is absent until something was recorded.
export interface TraderMetrics {
  equity?: number
  /** Unix seconds of the equity snapshot. */
  equity_at?: number
  return_pct?: number
  total_trades?: number
  win_rate?: number
  max_drawdown_pct?: number
}

export type LaunchCheckStatus = 'ok' | 'failed' | 'warning' | 'skipped'

export interface LaunchCheck {
  id: string
  status: LaunchCheckStatus
  code?: string
  message?: string
  // Fills in the interface's wording for the code.
  params?: Record<string, string>
  required?: number
  actual?: number
  asset?: string
}

export interface LaunchPreflight {
  ready: boolean
  checks: LaunchCheck[]
  min_trading_usdc: number
}

export type EquityRange = '24h' | '7d' | '30d' | 'all'

export interface EquityPoint {
  /** Unix seconds. */
  time: number
  equity: number
  unrealized_pnl: number
}

export interface EquityHistory {
  points: EquityPoint[]
  initial_balance: number
}

export interface TraderOrder {
  id: number
  // Who placed it: trader, futures, manual or external.
  source?: string
  symbol: string
  side: string
  position_side: string
  type: string
  quantity: number
  price: number
  status: string
  filled_quantity: number
  avg_fill_price: number
  commission: number
  commission_asset: string
  order_action: string
  /** Epoch milliseconds. */
  created_at: number
}

export interface OrderFill {
  id: number
  price: number
  quantity: number
  commission: number
  commission_asset: string
  realized_pnl: number
  /** Epoch milliseconds. */
  created_at: number
}

// A fill as an account's ledger lists it.
export interface AccountFill extends OrderFill {
  source: string
  symbol: string
  side: string
}

export interface Statistics {
  total_cycles: number
  successful_cycles: number
  failed_cycles: number
  total_open_positions: number
  total_close_positions: number
}

// Full trade-quality metrics from GET /api/statistics/full (store.TraderStats).
// Derived from CLOSED positions — the same numbers the agent sees each cycle.
export interface TraderFullStats {
  total_trades: number
  win_trades: number
  loss_trades: number
  win_rate: number // percentage, 0-100
  profit_factor: number
  sharpe_ratio: number
  total_pnl: number
  total_fee: number
  avg_win: number
  avg_loss: number
  /** Percent, not a fraction: 18.5 means -18.5% peak drawdown. */
  max_drawdown_pct: number
}

// Agent trading related types
export interface TraderInfo {
  trader_id: string
  trader_name: string
  agent_model: string
  account_id?: string
  is_running?: boolean
  startup_warning?: string
  kind?: TraderKind
  scan_interval_minutes?: number
}

// Trader Configuration Data for View Modal
export interface TraderConfigData {
  trader_id?: string
  trader_name: string
  agent_model: string
  account_id: string
  kind: TraderKind
  config: AgentConfig | GridConfig
  is_cross_margin: boolean
  scan_interval_minutes: number
  initial_balance: number
  is_running: boolean
}

// Position History Types
export interface HistoricalPosition {
  id: number
  trader_id: string
  account_id: string
  venue: string
  symbol: string
  side: string
  quantity: number
  entry_quantity: number
  entry_price: number
  /** Epoch milliseconds. */
  entry_time: number
  exit_price: number
  /** Epoch milliseconds. */
  exit_time: number
  realized_pnl: number
  fee: number
  leverage: number
  status: string
  close_reason: string
  updated_at: number
}

// Matches Go TraderStats struct exactly
export interface TraderStats {
  total_trades: number
  win_trades: number
  loss_trades: number
  win_rate: number
  profit_factor: number
  sharpe_ratio: number
  total_pnl: number
  total_fee: number
  avg_win: number
  avg_loss: number
  max_drawdown_pct: number
}

// Matches Go SymbolStats struct exactly
export interface SymbolStats {
  symbol: string
  total_trades: number
  win_trades: number
  win_rate: number
  total_pnl: number
  avg_pnl: number
  avg_hold_mins: number
}

// Matches Go DirectionStats struct exactly
export interface DirectionStats {
  side: string
  trade_count: number
  win_rate: number
  total_pnl: number
  avg_pnl: number
}

export interface PositionHistoryResponse {
  positions: HistoricalPosition[]
  stats: TraderStats | null
  symbol_stats: SymbolStats[]
  direction_stats: DirectionStats[]
}

// Grid Risk Information for frontend display
export interface GridRiskInfo {
  // Leverage info
  current_leverage: number
  effective_leverage: number
  recommended_leverage: number

  // Position info
  current_position: number
  max_position: number
  position_percent: number

  // Liquidation info
  liquidation_price: number
  liquidation_distance: number

  // Market state
  regime_level: string

  // Box state
  short_box_upper: number
  short_box_lower: number
  mid_box_upper: number
  mid_box_lower: number
  long_box_upper: number
  long_box_lower: number
  current_price: number

  // Breakout state
  breakout_level: string
  breakout_direction: string
}

export interface PromptSectionsConfig {
  role_definition?: string
  trading_frequency?: string
  entry_standards?: string
  decision_process?: string
}

export interface AgentConfig {
  coin_source: CoinSourceConfig
  indicators: IndicatorConfig
  custom_prompt?: string
  risk_control: RiskControlConfig
  prompt_sections?: PromptSectionsConfig
}

// What a grid trader decides by.
export interface GridConfig {
  // Trading pair (e.g., "BTCUSDT")
  symbol: string
  // Number of grid levels (5-50)
  grid_count: number
  // Total investment in USDT
  total_investment: number
  // Leverage (1-20)
  leverage: number
  // Upper price boundary (0 = auto-calculate from ATR)
  upper_price: number
  // Lower price boundary (0 = auto-calculate from ATR)
  lower_price: number
  // Use ATR to auto-calculate bounds
  use_atr_bounds: boolean
  // ATR multiplier for bound calculation (default 2.0)
  atr_multiplier: number
  // Position distribution: "uniform" | "gaussian" | "pyramid"
  distribution: 'uniform' | 'gaussian' | 'pyramid'
  // Maximum drawdown percentage before emergency exit
  max_drawdown_pct: number
  // Stop loss percentage per position
  stop_loss_pct: number
  // Daily loss limit percentage
  daily_loss_limit_pct: number
  // Use maker-only orders for lower fees
  use_maker_only: boolean
  // Enable automatic grid direction adjustment based on box breakouts
  enable_direction_adjust?: boolean
  // Direction bias ratio for long_bias/short_bias modes (default 0.7 = 70%/30%)
  direction_bias_ratio?: number
}

export interface CoinSourceConfig {
  source_type:
    | 'static'
    | 'mixed'
    | 'oi_top'
    | 'oi_low'
    | 'volume_top'
    | 'gainers'
    | 'losers'
  static_coins?: string[]
  excluded_coins?: string[] // List of excluded coins
  use_oi_top?: boolean
  oi_top_limit?: number
  use_oi_low?: boolean
  oi_low_limit?: number
  use_volume_top?: boolean
  volume_top_limit?: number
  use_gainers?: boolean
  gainers_limit?: number
  use_losers?: boolean
  losers_limit?: number
  oi_window?: '1h' | '4h' | '24h' // OI change window (default 4h)
  oi_pool_size?: number // Highest-volume symbols evaluated for OI change (default 50)
  min_quote_volume?: number // Minimum 24h quote volume (USDT) for ranking sources
}

export interface IndicatorConfig {
  klines: KlineConfig
  // Raw OHLCV kline data - required for agent analysis
  enable_raw_klines: boolean
  // Technical indicators (optional)
  enable_ema: boolean
  enable_macd: boolean
  enable_rsi: boolean
  enable_atr: boolean
  enable_boll: boolean
  enable_volume: boolean
  enable_oi: boolean
  enable_funding_rate: boolean
  ema_periods?: number[]
  rsi_periods?: number[]
  atr_periods?: number[]
  boll_periods?: number[]
}

export interface KlineConfig {
  primary_timeframe: string
  primary_count: number
  longer_timeframe?: string
  longer_count?: number
  enable_multi_timeframe: boolean
  // Added: support selecting multiple timeframes
  selected_timeframes?: string[]
}

export interface RiskControlConfig {
  // Max number of coins held simultaneously (CODE ENFORCED)
  max_positions: number

  // Trading Leverage - exchange leverage for opening positions (code enforced)
  btc_eth_max_leverage: number // BTC/ETH max exchange leverage
  altcoin_max_leverage: number // Altcoin max exchange leverage

  // Position Value Ratio - single position notional value / account equity (CODE ENFORCED)
  // Max position value = equity × this ratio
  btc_eth_max_position_value_ratio?: number // default: 5 (hard per-position cap)
  altcoin_max_position_value_ratio?: number // default: 5 (hard per-position cap)

  // Risk Parameters
  max_margin_usage: number // Max margin utilization, e.g. 0.9 = 90% (CODE ENFORCED)
  min_position_size: number // Min position size in USDT (CODE ENFORCED)
  min_risk_reward_ratio: number // Min reward to risk at the open price (code enforced)
  min_confidence: number // Min agent confidence to open position (code enforced)
  max_daily_loss_pct: number // Halt opens at this share lost today, 0 is off (code enforced)
  max_drawdown_pct: number // Halt opens this far below peak equity, 0 is off (code enforced)
}

// The server's prompt size estimate for a config (POST /traders/estimate-tokens).
export interface TokenEstimate {
  total: number
  breakdown: {
    system_prompt: number
    market_data: number
    fixed_overhead: number
  }
  model_limits: {
    name: string
    context_limit: number
    usage_pct: number
    level: 'ok' | 'warning' | 'danger'
  }[]
  suggestions: string[]
}

// A trial of a config on live public data (POST /traders/dry-run).
export interface DryRun {
  system_prompt: string
  user_prompt: string
  candidate_count: number
  candidates: { symbol: string; sources: string[] }[]
  agent_response?: string
  agent_error?: string
}
