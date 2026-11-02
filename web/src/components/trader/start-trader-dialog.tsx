import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Check, Loader2, Minus, TriangleAlert, X } from 'lucide-react'
import { api } from '@/api'
import { ApiError } from '@/api/http-client'
import { Dialog } from '@/components/common/dialog'
import { PanelLoading, PanelState } from '@/components/common/panel'
import { SettingsLink } from '@/components/settings/settings-link'
import { Button } from '@/components/ui/button'
import { t, type Language } from '@/i18n'
import type { LaunchCheck, LaunchCheckStatus } from '@/types'
import { cn } from '@/utils/cn'
import { codedText } from '@/utils/request-errors'
import { haltReasonText } from './halt-reason'

const STATUS_ICON = {
  ok: { icon: Check, className: 'text-vergex-success' },
  failed: { icon: X, className: 'text-vergex-danger' },
  warning: { icon: TriangleAlert, className: 'text-vergex-warning' },
  skipped: { icon: Minus, className: 'text-vergex-muted' },
} satisfies Record<LaunchCheckStatus, { icon: typeof Check; className: string }>

function amount(check: LaunchCheck) {
  if (check.actual == null || !check.required) return null
  return `${check.actual.toFixed(2)} / ${check.required} ${check.asset ?? ''}`
}

// A check's wording comes from its code, an account's from what the account's state says,
// and a halt's from its reason. Anything else stays as the server said it.
function checkMessage(check: LaunchCheck, language: Language) {
  const message = check.message ?? ''
  if (check.id === 'halted') return haltReasonText(message, language)
  return codedText(
    'startTrader.code',
    check.code,
    codedText('accountError', check.code, message, language),
    language,
    check.params
  )
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
    (check.id === 'agent_model' || check.id.startsWith('account'))
  return (
    <li className="flex items-start gap-3 py-2.5">
      <Icon
        className={cn('mt-0.5 size-4 shrink-0', className)}
        aria-label={check.status}
      />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline justify-between gap-x-3">
          <span className="text-[13px] font-medium text-vergex-ink">
            {t(`startTrader.check.${check.id}`, language)}
          </span>
          {amount(check) && (
            <span className="text-xs tabular-nums text-vergex-muted">
              {amount(check)}
            </span>
          )}
        </div>
        {check.status !== 'ok' && check.message && (
          <p className="mt-0.5 text-xs break-words text-vergex-muted">
            {checkMessage(check, language)}
            {needsSettings && (
              <>
                {' '}
                <SettingsLink
                  section={check.id === 'agent_model' ? 'models' : 'accounts'}
                  className="text-vergex-ink underline"
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
  kind?: string
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
  kind,
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
    isFetching,
    refetch,
  } = useQuery({
    queryKey: ['trader-preflight', traderId],
    queryFn: () => api.getTraderPreflight(traderId),
    refetchOnWindowFocus: false,
    retry: false,
    staleTime: 0,
  })
  const [starting, setStarting] = useState(false)
  const [failure, setFailure] = useState('')

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
      void refetch()
    } finally {
      setStarting(false)
    }
  }

  const summary: [string, string][] = [
    [
      label('kind'),
      kind
        ? t(`traderDialog.kind${kind === 'grid' ? 'Grid' : 'Agent'}`, language)
        : '—',
    ],
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
              disabled={isFetching}
              onClick={() => void refetch()}
            >
              {label('retry')}
            </Button>
          ) : (
            <Button
              variant="primary"
              disabled={!preflight || starting || isFetching}
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
      <h3 className="text-[13px] font-medium text-vergex-muted">
        {label('summary')}
      </h3>
      <dl className="mt-1 divide-y divide-vergex-line">
        {summary.map(([name, value]) => (
          <div
            key={name}
            className="flex items-center gap-4 py-2.5 text-[13px]"
          >
            <dt className="w-32 shrink-0 text-vergex-muted">{name}</dt>
            <dd className="min-w-0 truncate text-vergex-ink" title={value}>
              {value}
            </dd>
          </div>
        ))}
      </dl>
      <h3 className="mt-4 text-[13px] font-medium text-vergex-muted">
        {label('checks')}
      </h3>
      {preflight ? (
        <ul className="mt-1 divide-y divide-vergex-line">
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
            disabled={isFetching}
            onClick={() => void refetch()}
          >
            {label('retry')}
          </Button>
        </PanelState>
      ) : (
        <PanelLoading />
      )}
    </Dialog>
  )
}
