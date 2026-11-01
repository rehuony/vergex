import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import useSWR from 'swr'
import { api } from '@/api'
import { FORECAST_INTERVALS, type BacktestRun } from '@/api/forecast'
import { SymbolsField } from '@/components/common/form-fields'
import { notify } from '@/components/common/notify'
import { Panel, PanelState } from '@/components/common/panel'
import { Segmented } from '@/components/common/segmented'
import {
  BacktestReportView,
  hitRateText,
} from '@/components/forecast/backtest-report'
import {
  baseAsset,
  fmtDate,
  fmtPct,
  fmtTime,
} from '@/components/forecast/forecast-format'
import { ForecastFrame } from '@/components/forecast/forecast-frame'
import { SettingActions, SettingRow } from '@/components/common/setting-row'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { AppSelect } from '@/components/ui/select'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { cn } from '@/utils/cn'

const RANGES = { '3': '3m', '6': '6m', '12': '1y', '24': '2y' } as const
type Range = keyof typeof RANGES

const headClass = 'px-3 py-2 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-3 py-2.5 whitespace-nowrap'

const pct = (value: number) => `${(value * 100).toFixed(1)}%`

export function ForecastBacktestsPage() {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const [params] = useSearchParams()

  const { data: strategies } = useSWR(
    'forecast-strategies',
    api.getForecastStrategies,
    { revalidateOnFocus: false }
  )
  const { data: settings } = useSWR(
    'market-data-settings',
    api.getMarketDataSettings,
    { revalidateOnFocus: false }
  )
  const { data: runs, mutate } = useSWR(
    'forecast-backtests',
    api.getForecastBacktests,
    {
      refreshInterval: (latest) =>
        latest?.backtests.some((run) => run.status === 'running') ? 2000 : 0,
    }
  )
  const { data: status, mutate: refreshStatus } = useSWR(
    'market-data-status',
    api.getMarketDataStatus,
    {
      refreshInterval: (latest) => (latest?.backfill.running ? 2000 : 15000),
    }
  )

  const [strategyId, setStrategyId] = useState(params.get('strategy') ?? '')
  const [symbols, setSymbols] = useState<string[] | null>(null)
  const [intervals, setIntervals] = useState<string[]>(['1h', '4h'])
  const [range, setRange] = useState<Range>('12')
  const [busy, setBusy] = useState(false)
  const [openRun, setOpenRun] = useState<number | null>(null)

  const chosenSymbols = symbols ?? settings?.watchlist.slice(0, 5) ?? []
  const strategy = strategyId || strategies?.active_strategy_id || ''

  const coverageKey =
    chosenSymbols.length && intervals.length
      ? ['forecast-coverage', chosenSymbols.join(), intervals.join()]
      : null
  const { data: coverage, mutate: refreshCoverage } = useSWR(coverageKey, () =>
    api.getHistoryCoverage(chosenSymbols, intervals)
  )

  // A finished download changes what is held locally.
  const backfilling = status?.backfill.running
  const backfilledAt = status?.backfill.finished_at
  useEffect(() => {
    if (backfilledAt) void refreshCoverage()
  }, [backfilledAt, refreshCoverage])

  const months = Number(range)
  const from = new Date()
  from.setMonth(from.getMonth() - months)
  const wanted = from.getTime()
  const missing = (coverage?.coverage ?? []).filter(
    (row) => row.candles === 0 || row.first > wanted
  )

  const toggleInterval = (interval: string) =>
    setIntervals((current) =>
      current.includes(interval)
        ? current.filter((item) => item !== interval)
        : [...current, interval]
    )

  const run = async () => {
    setBusy(true)
    try {
      const started = await api.startForecastBacktest({
        strategy_id: strategy,
        symbols: chosenSymbols,
        intervals,
        from: wanted,
        to: Date.now(),
      })
      setOpenRun(started.id)
      await mutate()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('runFailed'))
    } finally {
      setBusy(false)
    }
  }

  const download = async () => {
    setBusy(true)
    try {
      await api.startBackfill({
        symbols: chosenSymbols,
        intervals,
        months,
        derivatives: false,
      })
      notify.success(label('downloadStarted'))
      await refreshStatus()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('runFailed'))
    } finally {
      setBusy(false)
    }
  }

  const list: BacktestRun[] = runs?.backtests ?? []
  const strategyNames = Object.fromEntries(
    (strategies?.strategies ?? []).map((item) => [item.id, item.name])
  )
  const shown =
    list.find((item) => item.id === openRun) ??
    list.find((item) => item.status === 'finished')
  const running = list.some((item) => item.status === 'running')
  const message = backfilling
    ? label('downloading', {
        current: status?.backfill.current ?? '',
        done: status?.backfill.done ?? 0,
        total: status?.backfill.total ?? 0,
      })
    : missing.length > 0
      ? label('historyMissing', {
          date: fmtDate(wanted),
          count: missing.length,
        })
      : undefined

  return (
    <ForecastFrame language={language}>
      <div className="flex min-w-0 flex-col gap-4">
        <Panel title={label('newBacktest')}>
          <div className="border-t border-[var(--line)] px-4">
            <SettingRow label={label('strategy')} htmlFor="bt-strategy">
              <AppSelect
                id="bt-strategy"
                className="h-9 max-sm:h-10"
                value={strategy}
                options={(strategies?.strategies ?? []).map((item) => ({
                  value: item.id,
                  label: item.name,
                }))}
                onChange={(id) => setStrategyId(id)}
              />
            </SettingRow>
            <SettingRow label={label('coins')} htmlFor="bt-symbols">
              <SymbolsField
                id="bt-symbols"
                values={chosenSymbols}
                placeholder="BTCUSDT, ETHUSDT"
                max={50}
                limitMessage={label('coinLimit', { count: 50 })}
                addLabel={label('add')}
                removeLabel={(symbol) => label('remove', { symbol })}
                onChange={setSymbols}
              />
            </SettingRow>
            <SettingRow label={label('intervals')}>
              <div
                role="group"
                aria-label={label('intervals')}
                className="flex flex-wrap gap-1.5"
              >
                {FORECAST_INTERVALS.map((interval) => (
                  <Button
                    key={interval}
                    type="button"
                    variant="option"
                    size="sm"
                    aria-pressed={intervals.includes(interval)}
                    onClick={() => toggleInterval(interval)}
                    className="h-8 px-2.5 text-xs max-sm:h-10"
                  >
                    {interval}
                  </Button>
                ))}
              </div>
            </SettingRow>
            <SettingRow label={label('period')}>
              <Segmented
                aria-label={label('period')}
                value={range}
                options={(Object.keys(RANGES) as Range[]).map((value) => ({
                  value,
                  label: RANGES[value],
                }))}
                onChange={setRange}
              />
            </SettingRow>
            <SettingActions message={message} messageTone="muted">
              {missing.length > 0 && (
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  className="max-sm:h-10"
                  disabled={busy || backfilling}
                  onClick={() => void download()}
                >
                  {label('downloadHistory')}
                </Button>
              )}
              <Button
                type="button"
                variant="primary"
                size="sm"
                className="max-sm:h-10"
                disabled={
                  busy ||
                  running ||
                  !strategy ||
                  chosenSymbols.length === 0 ||
                  intervals.length === 0
                }
                onClick={() => void run()}
              >
                {running ? label('runningBacktest') : label('runBacktest')}
              </Button>
            </SettingActions>
          </div>
        </Panel>

        <Panel title={label('runs')}>
          {list.length === 0 ? (
            <PanelState role="status">{label('noRuns')}</PanelState>
          ) : (
            <ScrollArea className="max-h-72">
              <table className="w-full text-left text-[13px] tabular-nums">
                <thead className="text-vergex-text-muted">
                  <tr className="border-y border-[var(--line)]">
                    <th className={headClass}>{label('run')}</th>
                    <th className={cn(headClass, 'max-md:hidden')}>
                      {label('coins')}
                    </th>
                    <th className={cn(headClass, 'text-right')}>
                      {label('samples')}
                    </th>
                    <th className={cn(headClass, 'text-right')}>
                      {label('hitRate')}
                    </th>
                    <th className={cn(headClass, 'text-right max-sm:hidden')}>
                      {label('baseRate')}
                    </th>
                    <th className={cn(headClass, 'text-right')}>
                      {label('lift')}
                    </th>
                    <th className={cn(headClass, 'text-right max-sm:hidden')}>
                      {label('afterFees')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {list.map((item) => {
                    const metrics = item.report?.overall
                    return (
                      <tr
                        key={item.id}
                        aria-selected={shown?.id === item.id}
                        className="cursor-pointer border-b border-[var(--line)] last:border-b-0 hover:bg-vergex-bg aria-selected:bg-[var(--brand-soft)]"
                        onClick={() => setOpenRun(item.id)}
                      >
                        <td className={cellClass}>
                          <div className="font-medium text-vergex-text">
                            {strategyNames[item.strategy_id] ?? '—'}
                          </div>
                          <div className="text-xs text-vergex-text-muted">
                            {fmtTime(item.created_at)}
                          </div>
                        </td>
                        <td
                          className={cn(
                            cellClass,
                            'max-w-64 truncate text-vergex-text-muted max-md:hidden'
                          )}
                          title={item.options.symbols.join(', ')}
                        >
                          {item.options.symbols.map(baseAsset).join(', ')} ·{' '}
                          {item.options.intervals.join(', ')}
                        </td>
                        {item.status === 'finished' && metrics ? (
                          <>
                            <td className={cn(cellClass, 'text-right')}>
                              {metrics.samples}
                            </td>
                            <td className={cn(cellClass, 'text-right')}>
                              {hitRateText(metrics)}
                            </td>
                            <td
                              className={cn(
                                cellClass,
                                'text-right max-sm:hidden'
                              )}
                            >
                              {pct(metrics.baseline_up)}
                            </td>
                            <td className={cn(cellClass, 'text-right')}>
                              {pct(metrics.lift)}
                            </td>
                            <td
                              className={cn(
                                cellClass,
                                'text-right max-sm:hidden'
                              )}
                            >
                              {fmtPct(metrics.avg_return_after_fee_pct, true)}
                            </td>
                          </>
                        ) : (
                          <td
                            colSpan={5}
                            className={cn(
                              cellClass,
                              'text-right',
                              item.status === 'failed'
                                ? 'text-vergex-danger'
                                : 'text-vergex-text-muted'
                            )}
                            title={item.error}
                          >
                            {item.status === 'failed'
                              ? label('runFailedShort')
                              : label('runProgress', {
                                  done: item.done,
                                  total: item.total,
                                })}
                          </td>
                        )}
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </ScrollArea>
          )}
        </Panel>

        {shown?.report && (
          <BacktestReportView
            id={shown.id}
            report={shown.report}
            language={language}
          />
        )}
      </div>
    </ForecastFrame>
  )
}
