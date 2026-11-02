import { SettingsLink } from '@/components/settings/settings-link'
import { AppSelect } from '@/components/ui/select'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import type { Account } from '@/types'
import { accountLabel } from '@/utils/futures/futures-accounts'
import type { AccountCheck } from '@/utils/futures/futures-workspace'

const LEVERAGES = [1, 2, 3, 5, 10, 20]

const money = (value: string | undefined) =>
  Number(value).toLocaleString('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })

interface FuturesToolbarProps {
  venue: string
  // Until the connections are read, nothing is known about the accounts.
  loaded: boolean
  accounts: Account[]
  accountID: string
  accountCheck: AccountCheck | null
  leverage: number
  busy: boolean
  onVenueChange: (venue: string) => void
  onAccountChange: (id: string) => void
  onLeverageChange: (leverage: number) => void
}

// FuturesToolbar sits beside the page's tabs and picks what every tab works on: the
// exchange, the trading account and the sizing leverage. Accounts are added in settings.
export function FuturesToolbar({
  venue,
  loaded,
  accounts,
  accountID,
  accountCheck,
  leverage,
  busy,
  onVenueChange,
  onAccountChange,
  onLeverageChange,
}: FuturesToolbarProps) {
  const { language } = useLanguage()
  const label = (key: string) => t(`futures.${key}`, language)
  const choices = accounts.filter((account) => account.venue === venue)
  return (
    <>
      <span className="whitespace-nowrap text-xs text-vergex-muted max-sm:w-full">
        {!loaded ? null : !accountID ? (
          <SettingsLink section="accounts" className="text-vergex-ink">
            {label('addAccount')}
          </SettingsLink>
        ) : accountCheck?.status === 'verified' ? (
          <span title={`${label('equity')} ${money(accountCheck.equity)} USDT`}>
            {label('available')}{' '}
            <span className="mono text-vergex-ink">
              {money(accountCheck.available)} USDT
            </span>
          </span>
        ) : accountCheck?.status === 'unavailable' ? (
          <SettingsLink
            section="accounts"
            className="text-vergex-danger underline"
          >
            {label('balanceUnavailable')}
          </SettingsLink>
        ) : (
          label('loadingBalance')
        )}
      </span>
      <AppSelect
        aria-label={label('marketVenue')}
        className="w-28 max-sm:h-10"
        value={venue}
        disabled={busy}
        onChange={onVenueChange}
        options={[
          { value: 'binance', label: 'Binance' },
          { value: 'okx', label: 'OKX' },
        ]}
      />
      <AppSelect
        aria-label={label('tradingAccount')}
        className="w-48 max-sm:h-10 max-sm:w-auto max-sm:min-w-0 max-sm:flex-1"
        value={accountID}
        disabled={busy}
        onChange={onAccountChange}
        options={
          loaded && choices.length > 0
            ? choices.map((account) => ({
                value: account.id,
                label: accountLabel(account, accounts),
              }))
            : [{ value: '', label: loaded ? label('noAccount') : '—' }]
        }
      />
      <AppSelect
        aria-label={label('futuresLeverage')}
        className="w-20 max-sm:h-10"
        value={leverage}
        disabled={busy}
        onChange={(value) => onLeverageChange(Number(value))}
        options={LEVERAGES.map((value) => ({
          value,
          label: `${value}x`,
        }))}
      />
    </>
  )
}
