import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  Check,
  ChevronRight,
  TriangleAlert,
  X,
  type LucideIcon,
} from 'lucide-react'
import { api } from '@/api'
import { t, type Language } from '@/i18n'
import type { DecisionRecord } from '@/types'
import { PanelState } from '@/components/common/panel'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { pollEvery } from '@/utils/poll'
import { DecisionDetailDialog } from './decision-detail-dialog'
import {
  actionSide,
  cleanLog,
  cycleFault,
  fmtDateTime,
  logTone,
  tradeActions,
  type LogTone,
} from './decision-format'
import { baseLabel } from '@/utils/trader-format'

const PAGE_SIZE = 50

const TONE_CLASS: Record<LogTone, string> = {
  ok: 'text-vergex-success',
  warn: 'text-vergex-warning',
  risk: 'text-vergex-danger',
  info: 'text-vergex-muted',
}
const TONE_ICON: Record<LogTone, LucideIcon | null> = {
  ok: Check,
  warn: TriangleAlert,
  risk: X,
  info: null,
}

export function LogLine({ text }: { text: string }) {
  const tone = logTone(text)
  const Icon = TONE_ICON[tone]
  return (
    <li className={cn('flex gap-1.5 text-xs', TONE_CLASS[tone])}>
      <span className="mt-0.5 flex size-3 shrink-0 justify-center">
        {Icon && <Icon size={12} aria-hidden="true" />}
      </span>
      <span className="min-w-0 break-words">{cleanLog(text)}</span>
    </li>
  )
}

function Cycle({
  record,
  onOpen,
  language,
}: {
  record: DecisionRecord
  onOpen: () => void
  language: Language
}) {
  const { failed, fault } = cycleFault(record)
  const trades = tradeActions(record)
  const logs = record.execution_log ?? []
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`traderDetail.${key}`, language, params)
  return (
    <li className="border-t border-vergex-line first:border-t-0">
      <button
        type="button"
        onClick={onOpen}
        className="flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-vergex-surface-muted"
      >
        <span className="min-w-0 flex-1">
          <span className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <span className="text-[13px] font-medium text-vergex-ink">
              {label('cycleNumber', { cycle: record.cycle_number })}
            </span>
            <span
              className="whitespace-nowrap text-xs tabular-nums text-vergex-muted"
              title={label('localTime')}
            >
              {fmtDateTime(record.timestamp)}
            </span>
            {trades.map((action, index) => {
              const side = actionSide(action.action)
              return (
                <span
                  key={index}
                  className={cn(
                    'whitespace-nowrap text-xs font-medium',
                    side === 'long'
                      ? 'text-vergex-success'
                      : side === 'short'
                        ? 'text-vergex-danger'
                        : 'text-vergex-muted'
                  )}
                >
                  {action.action.toLowerCase()} {baseLabel(action.symbol)}
                </span>
              )
            })}
            {trades.length === 0 && (
              <span className="text-xs text-vergex-muted">
                {label('noAction')}
              </span>
            )}
            {fault && (
              <span className="whitespace-nowrap text-xs font-semibold text-vergex-danger">
                {failed > 0
                  ? label('faultCount', { failed, total: trades.length })
                  : label('fault')}
              </span>
            )}
          </span>
          {(logs.length > 0 || record.error_message) && (
            <ul className="mt-1.5 flex flex-col gap-0.5">
              {logs.map((line, index) => (
                <LogLine key={index} text={line} />
              ))}
              {record.error_message && <LogLine text={record.error_message} />}
            </ul>
          )}
        </span>
        <ChevronRight
          className="mt-0.5 size-4 shrink-0 text-vergex-muted"
          aria-hidden="true"
        />
      </button>
    </li>
  )
}

// DecisionLog is the trader's full decision history, newest first. Rows carry
// no prompts; opening one loads the rest.
export function DecisionLog({
  traderId,
  language,
}: {
  traderId: string
  language: Language
}) {
  const label = (key: string) => t(`traderDetail.${key}`, language)
  const { data: newest, error } = useQuery({
    queryKey: ['decision-summaries', traderId],
    queryFn: () =>
      api.getDecisionSummaries(traderId, PAGE_SIZE, undefined, true),
    ...pollEvery(30000),
  })
  const [older, setOlder] = useState<DecisionRecord[]>([])
  const [hasMore, setHasMore] = useState(true)
  const [loadingOlder, setLoadingOlder] = useState(false)
  const [olderFailed, setOlderFailed] = useState(false)
  // The Dashboard links straight to a decision.
  const [params, setParams] = useSearchParams()
  const [open, setOpen] = useState<number | null>(
    () => Number(params.get('decision')) || null
  )

  useEffect(() => {
    setOlder([])
    setHasMore(true)
  }, [traderId])

  const seen = new Set<number>()
  const records = [...(newest ?? []), ...older].filter(
    (record) => !seen.has(record.id) && seen.add(record.id)
  )

  async function loadOlder() {
    const last = records[records.length - 1]
    if (!last || loadingOlder) return
    setLoadingOlder(true)
    setOlderFailed(false)
    try {
      const page = await api.getDecisionSummaries(
        traderId,
        PAGE_SIZE,
        last.id,
        true
      )
      setOlder((current) => [...current, ...page])
      setHasMore(page.length === PAGE_SIZE)
    } catch {
      setOlderFailed(true)
    } finally {
      setLoadingOlder(false)
    }
  }

  return (
    <>
      {!newest ? (
        <PanelState role={error ? 'alert' : 'status'}>
          {error ? label('decisionsFailed') : t('loading', language)}
        </PanelState>
      ) : records.length === 0 ? (
        <PanelState role="status">{label('noDecisions')}</PanelState>
      ) : (
        <>
          <ul>
            {records.map((record) => (
              <Cycle
                key={record.id}
                record={record}
                language={language}
                onOpen={() => setOpen(record.id)}
              />
            ))}
          </ul>
          {hasMore && records.length >= PAGE_SIZE && (
            <div className="flex items-center justify-end gap-3 border-t border-vergex-line px-4 py-2">
              {olderFailed && (
                <span role="alert" className="text-xs text-vergex-danger">
                  {label('decisionsFailed')}
                </span>
              )}
              <Button
                type="button"
                variant="secondary"
                size="sm"
                className="max-sm:h-10"
                disabled={loadingOlder}
                onClick={() => void loadOlder()}
              >
                {loadingOlder ? t('loading', language) : label('loadOlder')}
              </Button>
            </div>
          )}
        </>
      )}
      {open !== null && (
        <DecisionDetailDialog
          traderId={traderId}
          decisionId={open}
          language={language}
          onClose={() => {
            setOpen(null)
            if (params.has('decision'))
              setParams(
                (current) => {
                  current.delete('decision')
                  return current
                },
                { replace: true }
              )
          }}
        />
      )}
    </>
  )
}
