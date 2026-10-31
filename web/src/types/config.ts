export interface AIModel {
  id: string
  name: string
  provider: string
  enabled: boolean
  has_api_key?: boolean
  apiKey?: string
  customApiUrl?: string
  customModelName?: string
}

export interface TelegramConfig {
  token_masked: string // Masked token like "123456:ABC***XYZ"
  is_bound: boolean // Whether a user has sent /start
  bound_chat_id?: number // The bound chat ID (if any)
  model_id?: string // AI model selected for Telegram replies
  alerts?: Record<string, boolean> // Each alert kind and whether it is on
}

export interface Exchange {
  id: string // UUID (empty for supported exchange templates)
  exchange_type: string // "binance" or "okx"
  account_name: string // User-defined account name
  name: string // Display name
  type: 'cex' | 'dex'
  enabled: boolean
  has_api_key?: boolean
  has_secret_key?: boolean
  has_passphrase?: boolean
  apiKey?: string
  secretKey?: string
  passphrase?: string // OKX specific
}

export type ExchangeAccountStatus =
  | 'ok'
  | 'disabled'
  | 'missing_credentials'
  | 'invalid_credentials'
  | 'permission_denied'
  | 'unavailable'

export interface ExchangeAccountState {
  exchange_id: string
  status: ExchangeAccountStatus
  display_balance?: string
  asset?: string
  total_equity?: number
  available_balance?: number
  checked_at: string
  error_code?: string
  error_message?: string
}

export interface ExchangeAccountStateResponse {
  states: Record<string, ExchangeAccountState>
}

export interface CreateExchangeRequest {
  exchange_type: string // "binance" or "okx"
  account_name: string // User-defined account name
  enabled: boolean
  api_key?: string
  secret_key?: string
  passphrase?: string
}

export interface CreateTraderRequest {
  name: string
  ai_model_id: string
  exchange_id: string
  strategy_id?: string // Strategy ID (new version, uses saved strategy config)
  scan_interval_minutes?: number
  is_cross_margin?: boolean
  // Fields below are kept for backward compatibility; new version uses strategy config
  btc_eth_leverage?: number
  altcoin_leverage?: number
  trading_symbols?: string
  custom_prompt?: string
  override_base_prompt?: boolean
  system_prompt_template?: string
  use_oi_top?: boolean
}

export interface UpdateModelConfigRequest {
  models: {
    [key: string]: {
      enabled: boolean
      api_key: string
      custom_api_url?: string
      custom_model_name?: string
    }
  }
}

export interface TestModelConfigRequest {
  model_id: string
  api_key: string
  custom_api_url: string
  custom_model_name: string
}

export interface ModelTest {
  ok: boolean
  model: string
  latency_ms?: number
  error?: string
}

export interface UpdateExchangeConfigRequest {
  exchanges: {
    [key: string]: {
      enabled: boolean
      api_key: string
      secret_key: string
      passphrase?: string
    }
  }
}
