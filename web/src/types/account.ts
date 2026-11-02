// A set of API keys on a venue.
export interface Account {
  id: string // UUID (empty for a venue that can be connected)
  venue: string // "binance" or "okx"
  name: string // User-defined account name
  venue_name: string // How the interface names the venue
  testnet?: boolean // On the venue's test environment; fixed when created
  has_api_key?: boolean
  has_secret_key?: boolean
  has_passphrase?: boolean
  apiKey?: string
  secretKey?: string
  passphrase?: string // OKX specific
}

export type AccountStatus =
  | 'ok'
  | 'missing_credentials'
  | 'invalid_credentials'
  | 'permission_denied'
  | 'unavailable'

// What an account's API key may do, as its venue reports it.
export interface KeyPermissions {
  read: boolean
  trade: boolean
  withdraw: boolean
  ip_restricted: boolean
}

export interface AccountState {
  account_id: string
  status: AccountStatus
  display_balance?: string
  asset?: string
  total_equity?: number
  available_balance?: number
  permissions?: KeyPermissions
  checked_at: string
  error_code?: string
  error_message?: string
}

export interface AccountStateResponse {
  states: Record<string, AccountState>
}

export interface CreateAccountRequest {
  venue: string // "binance" or "okx"
  name: string // User-defined account name
  api_key?: string
  secret_key?: string
  passphrase?: string
  testnet?: boolean
}

export interface UpdateAccountConfigRequest {
  accounts: {
    [key: string]: {
      api_key: string
      secret_key: string
      passphrase?: string
    }
  }
}
