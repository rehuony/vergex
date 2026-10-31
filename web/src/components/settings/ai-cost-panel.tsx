import { useState } from 'react'
import useSWR from 'swr'
import { api } from '@/api'
import type { AICostPeriod, AICostTotals } from '@/api/data'
import { useLanguage } from '@/contexts/language-context'
import { useTraders } from '@/hooks/use-connections'
import { t, type Language } from '@/i18n/translations'
import { cn } from '@/utils/cn'
import { Button } from '@/components/ui/button'
import { AppSelect } from '@/components/ui/select'

const PERIODS: { key: AICostPeriod; label: string }[] = [
  { key: 'today', label: 'periodToday' },
  { key: 'week', label: 'periodWeek' },
  { key: 'month', label: 'periodMonth' },
  { key: 'all', label: 'periodAll' },
]

const tokens = (n: number) => n.toLocaleString('en-US')
const usd = (n: number) =>
  `$${n.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 4 })}`

function costLabel(row: AICostTotals, language: Language) {
  if (row.count > 0 && row.unpriced_count === row.count)
    return t('aiCost.unpriced', language)
  return row.unpriced_count > 0 ? `≥ ${usd(row.total)}` : usd(row.total)
}

// Local AI usage recorded from provider responses. Costs are estimates from
// public models.dev prices; models without a published price count tokens
// only, shown as "Unpriced" or a "≥" lower bound.
// It fills its container's height so the empty state sits in the middle.
export function AICostPanel() {
  const { language } = useLanguage()
  const label = (key: string) => t(`aiCost.${key}`, language)
  const [period, setPeriod] = useState<AICostPeriod>('today')
  const [traderId, setTraderId] = useState('')
  const { data: traders } = useTraders()
  const { data, error, isLoading } = useSWR(
    ['ai-costs', period, traderId],
    () => api.getAICostSummary(period, true, traderId || undefined)
  )
  const models = Object.entries(data?.by_model ?? {}).sort(
    ([, a], [, b]) => b.total - a.total || b.count - a.count
  )

  const centered =
    'flex flex-1 items-center justify-center py-6 text-center text-sm text-vergex-text-muted'

  return (
    <div className="flex flex-1 flex-col gap-4">
      <div className="flex flex-col gap-2 sm:flex-row">
        <div
          className="flex flex-1 gap-0.5 rounded-lg border border-[var(--line)] p-0.5"
          role="group"
          aria-label={label('period')}
        >
          {PERIODS.map((p) => (
            <Button
              variant="segment"
              size="sm"
              className="h-[30px] flex-1 rounded-md px-2 text-xs max-sm:h-9"
              key={p.key}
              onClick={() => setPeriod(p.key)}
              aria-pressed={period === p.key}
            >
              {label(p.label)}
            </Button>
          ))}
        </div>
        <AppSelect
          aria-label={label('trader')}
          className="h-9 sm:w-48 max-sm:h-10"
          value={traderId}
          options={[
            { value: '', label: label('allTraders') },
            ...(traders ?? []).map((trader) => ({
              value: trader.trader_id,
              label: trader.trader_name,
            })),
          ]}
          onChange={setTraderId}
        />
      </div>

      {error ? (
        <p role="alert" className={cn(centered, 'text-vergex-danger')}>
          {label('loadFailed')}
        </p>
      ) : isLoading || !data ? (
        <p className={centered}>{label('loading')}</p>
      ) : (
        <>
          <dl className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {[
              [label('calls'), tokens(data.count)],
              [label('inputTokens'), tokens(data.prompt_tokens)],
              [label('outputTokens'), tokens(data.completion_tokens)],
              [label('estimatedCost'), costLabel(data, language)],
            ].map(([label, value]) => (
              <div
                key={label}
                className="rounded-xl border border-[var(--line)] bg-vergex-bg px-3 py-2"
              >
                <dt className="text-[11px] text-vergex-text-muted">{label}</dt>
                <dd className="text-sm font-semibold text-vergex-text">
                  {value}
                </dd>
              </div>
            ))}
          </dl>

          {data.count === 0 ? (
            <p className={centered}>{label('empty')}</p>
          ) : (
            <table className="w-full text-left text-xs">
              <thead className="text-vergex-text-muted">
                <tr>
                  <th className="py-1.5 font-medium">{label('model')}</th>
                  <th className="py-1.5 text-right font-medium">
                    {label('callsShort')}
                  </th>
                  <th className="py-1.5 text-right font-medium">
                    {label('input')}
                  </th>
                  <th className="py-1.5 text-right font-medium">
                    {label('output')}
                  </th>
                  <th className="py-1.5 text-right font-medium">
                    {label('cost')}
                  </th>
                </tr>
              </thead>
              <tbody className="text-vergex-text">
                {models.map(([model, row]) => (
                  <tr key={model} className="border-t border-[var(--line)]">
                    <td className="py-1.5 pr-2 break-all">{model || '—'}</td>
                    <td className="py-1.5 text-right">{tokens(row.count)}</td>
                    <td className="py-1.5 text-right">
                      {tokens(row.prompt_tokens)}
                    </td>
                    <td className="py-1.5 text-right">
                      {tokens(row.completion_tokens)}
                    </td>
                    <td className="py-1.5 text-right">
                      {costLabel(row, language)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </div>
  )
}
