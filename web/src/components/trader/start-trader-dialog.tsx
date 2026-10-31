import { useState } from 'react'
import useSWR from 'swr'
import { Check, Loader2, Minus, TriangleAlert, X } from 'lucide-react'
import { api } from '@/api'
import { ApiError } from '@/api/http-client'
import { Dialog } from '@/components/common/dialog'
import { PanelState } from '@/components/common/panel'
import { SettingsLink } from '@/components/settings/settings-link'
import { Button } from '@/components/ui/button'
import { t, type Language } from '@/i18n/translations'
import type { LaunchCheck, LaunchCheckStatus } from '@/types'
import { cn } from '@/utils/cn'

const STATUS_ICON = {
  ok: { icon: Check, className: 'text-vergex-success' },
  failed: { icon: X, className: 'text-vergex-danger' },
  warning: { icon: TriangleAlert, className: 'text-[var(--warning)]' },
  skipped: { icon: Minus, className: 'text-vergex-text-muted' },
} satisfies Record<LaunchCheckStatus, { icon: typeof Check; className: string }>

function amount(check: LaunchCheck) {
  if (check.actual == null || !check.required) return null
  return `${check.actual.toFixed(2)} / ${check.required} ${check.asset ?? ''}`
}

function CheckRow({
  check,
  language,
}: {
  check: LaunchCheck
  language: Language
}) {
  const { icon: Icon, className } = STATUS_ICON[check.status]
  const needsSettings =
    check.status === 'failed' &&
    (check.id === 'ai_model' || check.id.startsWith('exchange'))
  return (
    <li className="flex items-start gap-3 py-2.5">
      <Icon
        className={cn('mt-0.5 size-4 shrink-0', className)}
        aria-label={check.status}
      />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline justify-between gap-x-3">
          <span className="text-[13px] font-medium text-vergex-text">
            {t(`startTrader.check.${check.id}`, language)}
          </span>
          {amount(check) && (
            <span className="text-xs tabular-nums text-vergex-text-muted">
              {amount(check)}
            </span>
          )}
        </div>
        {check.status !== 'ok' && check.message && (
          <p className="mt-0.5 text-xs break-words text-vergex-text-muted">
            {check.message}
            {needsSettings && (
              <>
                {' '}
                <SettingsLink
                  section={check.id === 'ai_model' ? 'models' : 'exchanges'}
                  className="text-vergex-brand underline"
                >
                  {t('startTrader.settings', language)}
                </SettingsLink>
              </>
            )}
          </p>
        )}
      </div>
    </li>
  )
}

interface StartTraderDialogProps {
  traderId: string
  traderName: string
  strategy?: string
  model: string
  account: string
  scanMinutes?: number
  language: Language
  onStarted: () => void
  onClose: () => void
}

// Starting a trader begins live trading, so it always goes through this dialog:
// what will run and whether it is ready, with the trader's own preflight.
export function StartTraderDialog({
  traderId,
  traderName,
  strategy,
  model,
  account,
  scanMinutes,
  language,
  onStarted,
  onClose,
}: StartTraderDialogProps) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`startTrader.${key}`, language, params)
  const {
    data: preflight,
    error,
    isValidating,
    mutate,
  } = useSWR(
    ['trader-preflight', traderId],
    () => api.getTraderPreflight(traderId),
    {
      revalidateOnFocus: false,
      revalidateIfStale: true,
      shouldRetryOnError: false,
      dedupingInterval: 0,
    }
  )
  const [starting, setStarting] = useState(false)
  const [failure, setFailure] = useState('')
  const checking = isValidating && !preflight

  async function start() {
    setStarting(true)
    setFailure('')
    try {
      await api.startTrader(traderId)
      onStarted()
    } catch (err) {
      // A preflight rejection carries an actionable reason, such as a missing
      // model credential.
      setFailure(
        err instanceof ApiError &&
          err.errorKey === 'trader.start.preflight_failed'
          ? err.message
          : label('failed')
      )
      void mutate()
    } finally {
      setStarting(false)
    }
  }

  const summary: [string, string][] = [
    [label('strategy'), strategy || '—'],
    [label('model'), model || '—'],
    [label('account'), account || '—'],
    [
      label('interval'),
      scanMinutes ? label('everyMinutes', { minutes: scanMinutes }) : '—',
    ],
  ]

  return (
    <Dialog
      size="md"
      title={label('title', { name: traderName })}
      onClose={onClose}
      closeLabel={t('cancel', language)}
      footer={
        <div className="flex w-full items-center gap-3">
          <span
            role={failure ? 'alert' : undefined}
            className="min-w-0 flex-1 truncate text-xs text-vergex-danger"
            title={failure}
          >
            {failure}
          </span>
          <Button variant="secondary" onClick={onClose}>
            {t('cancel', language)}
          </Button>
          {preflight && !preflight.ready ? (
            <Button
              variant="secondary"
              disabled={isValidating}
              onClick={() => void mutate()}
            >
              {label('retry')}
            </Button>
          ) : (
            <Button
              variant="primary"
              disabled={!preflight || starting || isValidating}
              onClick={() => void start()}
            >
              {starting && (
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              )}
              {label('confirm')}
            </Button>
          )}
        </div>
      }
    >
      <h3 className="text-[13px] font-medium text-vergex-text-muted">
        {label('summary')}
      </h3>
      <dl className="mt-1 divide-y divide-[var(--line)]">
        {summary.map(([name, value]) => (
          <div
            key={name}
            className="flex items-center gap-4 py-2.5 text-[13px]"
          >
            <dt className="w-32 shrink-0 text-vergex-text-muted">{name}</dt>
            <dd className="min-w-0 truncate text-vergex-text" title={value}>
              {value}
            </dd>
          </div>
        ))}
      </dl>
      <h3 className="mt-4 text-[13px] font-medium text-vergex-text-muted">
        {label('checks')}
      </h3>
      {preflight ? (
        <ul className="mt-1 divide-y divide-[var(--line)]">
          {preflight.checks.map((check) => (
            <CheckRow key={check.id} check={check} language={language} />
          ))}
        </ul>
      ) : error ? (
        <PanelState role="alert">
          {label('checkFailed')}
          <Button
            variant="secondary"
            size="sm"
            disabled={isValidating}
            onClick={() => void mutate()}
          >
            {label('retry')}
          </Button>
        </PanelState>
      ) : (
        <PanelState role="status">
          {checking ? label('checking') : t('loading', language)}
        </PanelState>
      )}
    </Dialog>
  )
}
