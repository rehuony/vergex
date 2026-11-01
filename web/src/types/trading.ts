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
  stop_until: string
  last_reset_time: string
  agent_provider: string
  strategy_type?: 'agent_trading' | 'grid_trading'
  grid_symbol?: string
  /** Runtime health: true when the model failed repeatedly and no new positions open. */
  safe_mode?: boolean
  safe_mode_reason?: string
  trading_blocked?: boolean
  trading_error?: string
  trading_checked_at?: string
}

export interface AccountInfo {
  total_equity: number
  wallet_balance: number
  unrealized_profit: number // Unrealized PnL (official value from the exchange API)
  available_balance: number
  total_pnl: number
  total_pnl_pct: number
  initial_balance: number
  daily_pnl: number
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
  exchange_id?: string
  is_running?: boolean
  startup_warning?: string
  strategy_id?: string
  strategy_name?: string
}

// Trader Configuration Data for View Modal
export interface TraderConfigData {
  trader_id?: string
  trader_name: string
  agent_model: string
  exchange_id: string
  strategy_id?: string // Strategy ID
  strategy_name?: string // Strategy name
  is_cross_margin: boolean
  scan_interval_minutes: number
  initial_balance: number
  is_running: boolean
}

// Position History Types
export interface HistoricalPosition {
  id: number
  trader_id: string
  exchange_id: string
  exchange_type: string
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
