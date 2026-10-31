import type { SystemStatus } from '@/types'
import { t, type Language } from '@/i18n/translations'
import { formatRequestError } from '@/utils/request-errors'
import { SettingsLink } from '@/components/settings/settings-link'

const dangerClass =
  'rounded-xl border border-vergex-danger/30 bg-vergex-danger/10 px-4 py-3 text-sm text-vergex-danger'

// TraderBanners explains a run state the numbers alone would hide: trading
// blocked by the exchange, safe mode after repeated AI failures, or a fresh
// trader that has not decided yet.
export function TraderBanners({
  status,
  language,
}: {
  status?: SystemStatus
  language: Language
}) {
  const firstRun =
    status?.is_running &&
    (status.call_count ?? 0) <= 1 &&
    !status.safe_mode &&
    !status.trading_blocked &&
    status.trading_checked_at
  if (!status?.trading_blocked && !status?.safe_mode && !firstRun) return null
  return (
    <div className="flex flex-col gap-2">
      {status?.trading_blocked && (
        <div role="alert" className={dangerClass}>
          <p className="font-semibold">{t('tradingBlockedTitle', language)}</p>
          <p className="mt-1 break-words">
            {formatRequestError(status.trading_error)}
          </p>
          <p className="mt-1">
            {t('tradingBlockedHint', language)}{' '}
            <SettingsLink section="exchanges" className="underline">
              {t('connectionSettings', language)}
            </SettingsLink>
          </p>
        </div>
      )}
      {status?.safe_mode && (
        <div role="alert" className={dangerClass}>
          <p className="font-semibold">{t('safeModeTitle', language)}</p>
          {status.safe_mode_reason && (
            <p className="mt-1 break-words">{status.safe_mode_reason}</p>
          )}
        </div>
      )}
      {firstRun && (
        <div className="rounded-xl border border-vergex-success/30 bg-vergex-success/10 px-4 py-3 text-sm text-vergex-text">
          <span className="font-semibold text-vergex-success">
            {t('traderLiveTitle', language)}
          </span>{' '}
          {t('traderLiveHint', language)}
        </div>
      )}
    </div>
  )
}
