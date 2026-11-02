import { Fragment, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight } from 'lucide-react'
import { api } from '@/api'
import type {
  AgentCallFailure,
  AgentCostPeriod,
  AgentCostTotals,
} from '@/api/agent'
import {
  cellClass,
  headClass,
  numberClass,
  StatStrip,
  tableClass,
} from '@/components/common/figures'
import { PanelLoading, PanelState } from '@/components/common/panel'
import { Segmented } from '@/components/common/segmented'
import { SettingList } from '@/components/common/setting-row'
import { useLanguage } from '@/contexts/language-context'
import { useTraders } from '@/hooks/use-connections'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'
import { fmtTime } from '@/utils/trader-format'
import { Skeleton } from '@/components/ui/skeleton'
import { AppSelect } from '@/components/ui/select'

const PERIODS: { key: AgentCostPeriod; label: string }[] = [
  { key: 'today', label: 'periodToday' },
  { key: 'week', label: 'periodWeek' },
  { key: 'month', label: 'periodMonth' },
  { key: 'all', label: 'periodAll' },
]

const FEATURES = ['trader', 'futures', 'telegram', 'test']

const tokens = (n: number) => n.toLocaleString('en-US')
const megabytes = (bytes: number) =>
  `${(bytes / (1 << 20)).toLocaleString('en-US', { maximumFractionDigits: 1 })} MB`
const usd = (n: number) =>
  `$${n.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 4 })}`

function costLabel(row: AgentCostTotals, language: Language) {
  if (row.count > 0 && row.unpriced_count === row.count)
    return t('agentCost.unpriced', language)
  return row.unpriced_count > 0 ? `≥ ${usd(row.total)}` : usd(row.total)
}

// One table of calls by what made them, or by model, whose columns line up across both.
function CostTable({
  head,
  rows,
  language,
}: {
  head: string
  rows: [string, string, AgentCostTotals][]
  language: Language
}) {
  const label = (key: string) => t(`agentCost.${key}`, language)
  return (
    <table className={tableClass}>
      <thead>
        <tr>
          <th className={headClass}>{head}</th>
          <th className={cn(headClass, 'w-16 text-right')}>
            {label('callsShort')}
          </th>
          <th className={cn(headClass, 'w-16 text-right')}>
            {label('failedShort')}
          </th>
          <th className={cn(headClass, 'w-24 text-right max-sm:hidden')}>
            {label('input')}
          </th>
          <th className={cn(headClass, 'w-20 text-right max-sm:hidden')}>
            {label('output')}
          </th>
          <th className={cn(headClass, 'w-20 text-right')}>{label('cost')}</th>
        </tr>
      </thead>
      <tbody>
        {rows.map(([key, name, row]) => (
          <tr key={key}>
            <td className={cn(cellClass, 'text-vergex-ink')}>
              <span className="block truncate" title={name}>
                {name}
              </span>
            </td>
            <td className={cn(cellClass, numberClass)}>{tokens(row.count)}</td>
            <td
              className={cn(
                cellClass,
                numberClass,
                row.failed_count > 0 && 'text-vergex-danger'
              )}
            >
              {tokens(row.failed_count)}
            </td>
            <td className={cn(cellClass, numberClass, 'max-sm:hidden')}>
              {tokens(row.prompt_tokens)}
            </td>
            <td className={cn(cellClass, numberClass, 'max-sm:hidden')}>
              {tokens(row.completion_tokens)}
            </td>
            <td className={cn(cellClass, numberClass)}>
              {costLabel(row, language)}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

// What failed lately, on the same table as the calls: a line each, however long the provider's
// answer, with the whole of it under the row once pressed. A phone keeps only the first column,
// so the widths are on the columns, which then leave it all the room, even where an open row
// spans them, and what the other columns say is there too.
function RecentFailures({
  failures,
  language,
}: {
  failures: AgentCallFailure[]
  language: Language
}) {
  const label = (key: string) => t(`agentCost.${key}`, language)
  const [open, setOpen] = useState<number | null>(null)
  return (
    <table className={tableClass}>
      <colgroup>
        <col />
        <col className="w-20 max-sm:w-0" />
        <col className="w-36 max-sm:w-0" />
        <col className="w-28 max-sm:w-0" />
      </colgroup>
      <thead>
        <tr>
          <th className={headClass}>{label('recentFailures')}</th>
          <th className={cn(headClass, 'max-sm:hidden')}>
            {label('feature.name')}
          </th>
          <th className={cn(headClass, 'max-sm:hidden')}>{label('model')}</th>
          <th className={cn(headClass, 'text-right max-sm:hidden')}>
            {label('time')}
          </th>
        </tr>
      </thead>
      <tbody>
        {failures.map((failure) => {
          const expanded = open === failure.id
          const source = label(`feature.${failure.feature}`)
          const time = fmtTime(failure.created_at) || '—'
          return (
            <Fragment key={failure.id}>
              <tr
                className="cursor-pointer"
                onClick={() => setOpen(expanded ? null : failure.id)}
              >
                <td className={cn(cellClass, 'text-vergex-ink')}>
                  <button
                    type="button"
                    aria-expanded={expanded}
                    title={failure.error}
                    className="flex w-full items-center gap-2 text-left"
                  >
                    <ChevronRight
                      className={cn(
                        'size-4 shrink-0 text-vergex-muted transition-transform duration-(--dur-state) ease-(--ease-move)',
                        expanded && 'rotate-90'
                      )}
                      aria-hidden="true"
                    />
                    <span className="min-w-0 truncate">{failure.error}</span>
                  </button>
                </td>
                <td className={cn(cellClass, 'truncate text-xs max-sm:hidden')}>
                  {source}
                </td>
                <td
                  title={failure.model}
                  className={cn(cellClass, 'truncate text-xs max-sm:hidden')}
                >
                  {failure.model}
                </td>
                <td
                  className={cn(
                    cellClass,
                    'mono text-right text-xs text-vergex-muted max-sm:hidden'
                  )}
                >
                  {time}
                </td>
              </tr>
              {expanded && (
                <tr>
                  <td
                    colSpan={4}
                    className="border-b border-vergex-line bg-vergex-surface-muted px-4 py-2"
                  >
                    <p className="mb-1 text-xs text-vergex-muted sm:hidden">
                      {source} · {failure.model} · {time}
                    </p>
                    <p className="mono whitespace-pre-wrap text-xs leading-5 text-vergex-ink [overflow-wrap:anywhere]">
                      {failure.error}
                    </p>
                  </td>
                </tr>
              )}
            </Fragment>
          )
        })}
      </tbody>
    </table>
  )
}

// Local agent usage recorded from provider responses. Costs are estimates from
// public models.dev prices; models without a published price count tokens
// only, shown as "Unpriced" or a "≥" lower bound. A call that failed or was stopped has no
// tokens to count, though the provider may still have charged for it.
// What it holds fills the window's height so the empty state sits in the middle.
export function AgentCostPanel() {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`agentCost.${key}`, language, params)
  const [period, setPeriod] = useState<AgentCostPeriod>('today')
  const [traderId, setTraderId] = useState('')
  const { data: traders } = useTraders()
  const { data, error, isLoading } = useQuery({
    queryKey: ['agent-costs', period, traderId],
    queryFn: () => api.getAgentCostSummary(period, true, traderId || undefined),
  })
  const { data: storage, error: storageError } = useQuery({
    queryKey: ['agent-storage'],
    queryFn: () => api.getAgentStorage(true),
  })
  const models = Object.entries(data?.by_model ?? {}).sort(
    ([, a], [, b]) => b.total - a.total || b.count - a.count
  )
  const features = FEATURES.flatMap((feature) => {
    const row = data?.by_feature?.[feature]
    return row
      ? [
          [feature, label(`feature.${feature}`), row] as [
            string,
            string,
            AgentCostTotals,
          ],
        ]
      : []
  })
  const failures = data?.recent_errors ?? []

  return (
    <SettingList
      head={
        <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-center">
          <Segmented
            aria-label={label('period')}
            value={period}
            options={PERIODS.map((p) => ({
              value: p.key,
              label: label(p.label),
            }))}
            onChange={setPeriod}
          />
          <AppSelect
            aria-label={label('trader')}
            className="h-9 sm:ml-auto sm:w-48 max-sm:h-10"
            value={traderId}
            options={[
              { value: '', label: label('everything') },
              ...(traders ?? []).map((trader) => ({
                value: trader.trader_id,
                label: trader.trader_name,
              })),
            ]}
            onChange={setTraderId}
          />
        </div>
      }
      message={label('storage')}
      messageTone="muted"
      actions={
        <span className="flex h-8 items-center text-xs tabular-nums text-vergex-muted max-sm:h-10">
          {storage ? (
            megabytes(storage.bytes)
          ) : storageError ? (
            '—'
          ) : (
            <Skeleton className="w-12" />
          )}
        </span>
      }
    >
      <div className="flex flex-1 flex-col gap-4 py-4">
        {error ? (
          <PanelState role="alert" className="flex-1">
            {label('loadFailed')}
          </PanelState>
        ) : isLoading || !data ? (
          <PanelLoading className="flex-1" />
        ) : (
          <>
            <StatStrip
              className="border-t-0"
              stats={[
                {
                  name: label('calls'),
                  value: tokens(data.count),
                  note:
                    data.failed_count > 0
                      ? label('failedCount', { count: data.failed_count })
                      : undefined,
                },
                {
                  name: label('inputTokens'),
                  value: tokens(data.prompt_tokens),
                },
                {
                  name: label('outputTokens'),
                  value: tokens(data.completion_tokens),
                },
                {
                  name: label('estimatedCost'),
                  value: costLabel(data, language),
                },
              ]}
            />

            {data.count === 0 ? (
              <PanelState role="status" className="flex-1">
                {label('empty')}
              </PanelState>
            ) : (
              <>
                {!traderId && features.length > 0 && (
                  <CostTable
                    head={label('feature.name')}
                    rows={features}
                    language={language}
                  />
                )}
                <CostTable
                  head={label('model')}
                  rows={models.map(([model, row]) => [
                    model,
                    model || '—',
                    row,
                  ])}
                  language={language}
                />
                {failures.length > 0 && (
                  <RecentFailures failures={failures} language={language} />
                )}
              </>
            )}
          </>
        )}
      </div>
    </SettingList>
  )
}
