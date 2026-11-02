import { testnetMark } from '@/constants/model-providers'
import type { Account } from '@/types'

// An account's name in a list of accounts, told apart from another of the same name.
export function accountName(account: Account, accounts: Account[] = []) {
  const name = account.name?.trim() || account.venue_name || account.venue
  const duplicate = accounts.some(
    (other) =>
      other.id !== account.id &&
      other.venue === account.venue &&
      (other.name?.trim() || other.venue_name || other.venue) === name
  )
  return duplicate ? `${name} · ${account.id.slice(0, 8)}` : name
}

// An account's name where what happens on it is at stake, so test money never reads as real.
export function accountLabel(account: Account, accounts: Account[] = []) {
  return accountName(account, accounts) + testnetMark(account)
}

function preferenceKey(venue: string) {
  return [
    'futures-account-v1',
    localStorage.getItem('user_id') || '',
    venue,
  ].join('/')
}

export function preferredAccount(accounts: Account[], venue: string): string {
  const matching = accounts.filter((account) => account.venue === venue)
  try {
    const saved: unknown = JSON.parse(
      localStorage.getItem(preferenceKey(venue)) || 'null'
    )
    if (matching.some((account) => account.id === saved)) return saved as string
  } catch {
    /* Storage may be unavailable. */
  }
  return matching[0]?.id || ''
}

export function rememberAccount(venue: string, id: string) {
  try {
    localStorage.setItem(preferenceKey(venue), JSON.stringify(id))
  } catch {
    /* Storage may be unavailable. */
  }
}

export const futuresVenues = ['binance', 'okx']
export const apiKeyVenues = ['binance', 'okx']
export function venueName(venue: string) {
  return (
    (
      {
        binance: 'Binance',
        okx: 'OKX',
      } as Record<string, string>
    )[venue] || venue
  )
}
export function defaultMarket(venue: string) {
  return venue === 'okx' ? 'BTC-USDT-SWAP' : 'BTCUSDT'
}
