import { ChevronDown, TriangleAlert } from 'lucide-react'
import { SettingsLink } from '@/components/settings/settings-link'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { t, type Language } from '@/i18n'
import type { SystemStatus } from '@/types'
import { cn } from '@/utils/cn'
import { formatRequestError } from '@/utils/request-errors'
import { fmtTime } from '@/utils/trader-format'
import { haltReasonText, safeModeReasonText } from './halt-reason'

const badgeClass =
  'inline-flex shrink-0 items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium'

// TraderState says how a trader stands, in the one place beside its name. What the numbers
// alone would hide, a hard stop that has tripped, trading blocked by the exchange or safe
// mode after repeated model failures, takes that place and opens in full, so it never moves
// the page.
export function TraderState({
  running,
  status,
  language,
  onResume,
}: {
  running: boolean
  status?: SystemStatus
  language: Language
  onResume: () => void
}) {
  const label = (key: string, params?: Record<string, string>) =>
    t(`traderDetail.${key}`, language, params)
  const halted = status?.halted
  const blocked = Boolean(status?.trading_blocked)
  const safeMode = Boolean(status?.safe_mode)
  if (!halted && !blocked && !safeMode) {
    return (
      <span
        className={cn(
          badgeClass,
          running
            ? 'bg-vergex-success-bg text-vergex-success'
            : 'bg-vergex-surface-muted text-vergex-muted'
        )}
      >
        {running ? label('running') : label('stopped')}
      </span>
    )
  }
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          role="alert"
          className={cn(
            badgeClass,
            halted || blocked
              ? 'bg-vergex-danger-bg text-vergex-danger'
              : 'bg-vergex-warning-bg text-vergex-warning'
          )}
        >
          <TriangleAlert className="size-3" aria-hidden="true" />
          {label(halted ? 'halted' : blocked ? 'tradingBlocked' : 'safeMode')}
          <ChevronDown className="size-3" aria-hidden="true" />
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-[min(380px,calc(100vw-32px))] space-y-3 p-4 text-[13px] leading-relaxed text-vergex-ink"
      >
        {halted && (
          <div>
            <p className="font-medium text-vergex-danger">
              {label('haltedTitle')}
            </p>
            <p className="mt-1 break-words">
              {haltReasonText(halted.reason, language)}
            </p>
            <p className="mt-1 text-vergex-muted">
              {halted.until
                ? label('haltedUntil', { time: fmtTime(halted.until) })
                : label('haltedHeld')}
            </p>
            <Button
              type="button"
              size="sm"
              variant="secondary"
              className="mt-2"
              onClick={onResume}
            >
              {label('resume')}
            </Button>
          </div>
        )}
        {blocked && (
          <div>
            <p className="font-medium text-vergex-danger">
              {t('tradingBlockedTitle', language)}
            </p>
            <p className="mt-1 break-words">
              {formatRequestError(status?.trading_error)}
            </p>
            <p className="mt-1 text-vergex-muted">
              {t('tradingBlockedHint', language)}{' '}
              <SettingsLink section="accounts" className="underline">
                {t('connectionSettings', language)}
              </SettingsLink>
            </p>
          </div>
        )}
        {safeMode && (
          <div>
            <p className="font-medium text-vergex-warning">
              {t('safeModeTitle', language)}
            </p>
            {status?.safe_mode_reason && (
              <p className="mt-1 break-words">
                {safeModeReasonText(status.safe_mode_reason, language)}
              </p>
            )}
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}
