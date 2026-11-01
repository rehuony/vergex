import { SettingsLink } from '@/components/settings/settings-link'
import { AppSelect } from '@/components/ui/select'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import type { Exchange } from '@/types'
import { accountLabel } from '@/utils/planning/frontier-accounts'
import type { AccountCheck } from '@/utils/planning/frontier-workspace'
import { MarketPicker } from './market-picker'

const LEVERAGES = [1, 2, 3, 5, 10, 20]

const money = (value: string | undefined) =>
  Number(value).toLocaleString('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })

interface FrontierToolbarProps {
  venue: string
  accounts: Exchange[]
  accountID: string
  accountCheck: AccountCheck | null
  symbol: string
  price: string
  leverage: number
  busy: boolean
  onVenueChange: (venue: string) => void
  onAccountChange: (id: string) => void
  onOpenMarket: (symbol: string) => void
  onLeverageChange: (leverage: number) => void
}

// FrontierToolbar picks what the chart and the assistant work on: the market,
// the exchange, the trading account and the sizing leverage. Accounts are added
// in settings.
export function FrontierToolbar({
  venue,
  accounts,
  accountID,
  accountCheck,
  symbol,
  price,
  leverage,
  busy,
  onVenueChange,
  onAccountChange,
  onOpenMarket,
  onLeverageChange,
}: FrontierToolbarProps) {
  const { language } = useLanguage()
  const label = (key: string) => t(`frontier.${key}`, language)
  return (
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <MarketPicker
        venue={venue}
        selected={symbol}
        price={price}
        disabled={busy}
        onSelect={onOpenMarket}
      />
      <AppSelect
        aria-label={label('marketVenue')}
        className="w-28"
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
        className="w-48 max-sm:w-auto max-sm:min-w-0 max-sm:flex-1"
        value={accountID}
        disabled={busy}
        onChange={onAccountChange}
        options={[
          { value: '', label: label('noAccount') },
          ...accounts
            .filter((account) => account.exchange_type === venue)
            .map((account) => ({
              value: account.id,
              label: accountLabel(account, accounts),
            })),
        ]}
      />
      <AppSelect
        aria-label={label('planningLeverage')}
        className="w-20"
        value={leverage}
        disabled={busy}
        onChange={(value) => onLeverageChange(Number(value))}
        options={LEVERAGES.map((value) => ({
          value,
          label: `${value}x`,
        }))}
      />
      <span className="ml-auto text-xs whitespace-nowrap text-vergex-text-muted max-sm:ml-0 max-sm:w-full">
        {!accountID ? (
          <SettingsLink section="exchanges" className="text-vergex-brand">
            {label('addAccount')}
          </SettingsLink>
        ) : accountCheck?.status === 'verified' ? (
          <span title={`${label('equity')} ${money(accountCheck.equity)} USDT`}>
            {label('available')}{' '}
            <span className="font-medium tabular-nums text-vergex-text">
              {money(accountCheck.available)} USDT
            </span>
          </span>
        ) : accountCheck?.status === 'unavailable' ? (
          <SettingsLink
            section="exchanges"
            className="text-vergex-danger underline"
          >
            {label('balanceUnavailable')}
          </SettingsLink>
        ) : (
          label('loadingBalance')
        )}
      </span>
    </div>
  )
}
