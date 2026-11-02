import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { FORECAST_INTERVALS, type BacktestRun } from '@/api/forecast'
import { fieldClass, SymbolsField } from '@/components/common/form-fields'
import { notify } from '@/components/common/notify'
import { Panel, PanelLoading } from '@/components/common/panel'
import { Segmented } from '@/components/common/segmented'
import { SettingActions, SettingRow } from '@/components/common/setting-row'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { AppSelect } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { t, type Language } from '@/i18n'
import { fmtDate } from './forecast-format'
import { marketSettingsQuery, marketStatusQuery } from './forecast-queries'
import { useStrategies } from './strategy-select'

const RANGES = { '3': '3m', '6': '6m', '12': '1y', '24': '2y' } as const
type Range = keyof typeof RANGES | 'custom'

const MAX_COINS = 300
const MONTH_MS = 30 * 24 * 3600 * 1000

const dayText = (date: Date) => {
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000)
  return local.toISOString().slice(0, 10)
}

const monthsAgo = (months: number) => {
  const from = new Date()
  from.setMonth(from.getMonth() - months)
  return dayText(from)
}

// What to test, and the history it needs. Where local history is short it says so, and
// offers to fetch it, derivatives included: Binance's API keeps only about 30 days of
// those, so without them a long run tests mostly candle factors.
export function BacktestForm({
  initialStrategy,
  again,
  running,
  onStarted,
  language,
}: {
  initialStrategy: string | null
  // A run to start from, when running one again.
  again?: BacktestRun | null
  running: boolean
  onStarted: (run: BacktestRun) => void
  language: Language
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const { data: strategies } = useStrategies(language)
  const { data: settings } = useQuery(marketSettingsQuery)
  const { data: status, refetch: refreshStatus } = useQuery({
    ...marketStatusQuery,
    refetchInterval: (query) =>
      query.state.data?.backfill.running ? 2000 : 15000,
  })

  const [strategyId, setStrategyId] = useState(initialStrategy ?? '')
  const [name, setName] = useState(again?.name ?? '')
  const [symbols, setSymbols] = useState<string[] | null>(
    again?.options.symbols ?? null
  )
  const [intervals, setIntervals] = useState<string[]>(
    again?.options.intervals ?? ['1h', '4h']
  )
  const [range, setRange] = useState<Range>(again ? 'custom' : '12')
  const [from, setFrom] = useState(() =>
    again ? dayText(new Date(again.options.from)) : monthsAgo(12)
  )
  const [to, setTo] = useState(() =>
    dayText(again ? new Date(again.options.to) : new Date())
  )
  const [derivatives, setDerivatives] = useState(true)
  const [busy, setBusy] = useState(false)

  const chosenSymbols = symbols ?? settings?.watchlist.slice(0, 5) ?? []
  const usable = (strategies?.strategies ?? []).filter((item) => !item.retired)
  const strategy = usable.some((item) => item.id === strategyId)
    ? strategyId
    : strategies?.active_strategy_id || (usable[0]?.id ?? '')

  const { data: coverage, refetch: refreshCoverage } = useQuery({
    queryKey: ['forecast-coverage', chosenSymbols.join(), intervals.join()],
    queryFn: () => api.getHistoryCoverage(chosenSymbols, intervals),
    enabled: chosenSymbols.length > 0 && intervals.length > 0,
  })

  // A finished download changes what is held locally.
  const backfilling = status?.backfill.running
  const backfilledAt = status?.backfill.finished_at
  useEffect(() => {
    if (backfilledAt) void refreshCoverage()
  }, [backfilledAt, refreshCoverage])

  const fromMs = new Date(`${from}T00:00:00`).getTime()
  const today = dayText(new Date())
  const toMs = to >= today ? Date.now() : new Date(`${to}T23:59:59`).getTime()
  const validRange = Number.isFinite(fromMs) && toMs > fromMs
  const missing = (coverage?.coverage ?? []).filter(
    (row) => row.candles === 0 || row.first > fromMs
  )

  const chooseRange = (next: Range) => {
    setRange(next)
    if (next === 'custom') return
    setFrom(monthsAgo(Number(next)))
    setTo(today)
  }
  const toggleInterval = (interval: string) =>
    setIntervals((current) =>
      current.includes(interval)
        ? current.filter((item) => item !== interval)
        : [...current, interval]
    )

  const run = async () => {
    setBusy(true)
    try {
      onStarted(
        await api.startForecastBacktest({
          strategy_id: strategy,
          name: name.trim(),
          symbols: chosenSymbols,
          intervals,
          from: fromMs,
          to: toMs,
        })
      )
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
        months: Math.max(1, Math.ceil((Date.now() - fromMs) / MONTH_MS)),
        derivatives,
      })
      notify.success(label('downloadStarted'))
      await refreshStatus()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('runFailed'))
    } finally {
      setBusy(false)
    }
  }

  const message = backfilling
    ? label('downloading', {
        current: status?.backfill.current ?? '',
        done: status?.backfill.done ?? 0,
        total: status?.backfill.total ?? 0,
      })
    : !validRange
      ? label('badRange')
      : missing.length > 0
        ? label('historyMissing', {
            date: fmtDate(fromMs),
            count: missing.length,
          })
        : undefined

  return (
    <Panel
      title={label('newBacktest')}
      className="flex min-h-0 flex-1 flex-col overflow-hidden"
    >
      {!strategies ? (
        <PanelLoading className="flex-1" />
      ) : (
        <>
          <ScrollArea className="min-h-0 flex-1 border-t border-vergex-line">
            <div className="px-4">
              <SettingRow label={label('strategy')} htmlFor="bt-strategy">
                <AppSelect
                  id="bt-strategy"
                  className="h-9 min-w-0 flex-1 max-sm:h-10"
                  value={strategy}
                  options={usable.map((item) => ({
                    value: item.id,
                    label: item.name,
                  }))}
                  onChange={setStrategyId}
                />
              </SettingRow>
              <SettingRow label={label('strategyName')} htmlFor="bt-name">
                <input
                  id="bt-name"
                  className={fieldClass}
                  value={name}
                  maxLength={80}
                  placeholder={
                    usable.find((item) => item.id === strategy)?.name
                  }
                  onChange={(event) => setName(event.target.value)}
                />
              </SettingRow>
              <SettingRow label={label('coins')} htmlFor="bt-symbols" top>
                <SymbolsField
                  id="bt-symbols"
                  values={chosenSymbols}
                  placeholder="BTCUSDT, ETHUSDT"
                  max={MAX_COINS}
                  limitMessage={label('coinLimit', { count: MAX_COINS })}
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
                  options={[
                    ...(Object.keys(RANGES) as (keyof typeof RANGES)[]).map(
                      (value) => ({ value, label: RANGES[value] })
                    ),
                    { value: 'custom' as const, label: label('rangeCustom') },
                  ]}
                  onChange={chooseRange}
                />
                <div className="flex min-w-0 flex-[1_1_280px] items-center gap-2">
                  <input
                    type="date"
                    aria-label={label('periodFrom')}
                    className={fieldClass}
                    value={from}
                    max={to}
                    onChange={(event) => {
                      setRange('custom')
                      setFrom(event.target.value)
                    }}
                  />
                  <span className="shrink-0 text-xs text-vergex-muted">–</span>
                  <input
                    type="date"
                    aria-label={label('periodTo')}
                    className={fieldClass}
                    value={to}
                    min={from}
                    max={today}
                    onChange={(event) => {
                      setRange('custom')
                      setTo(event.target.value)
                    }}
                  />
                </div>
              </SettingRow>
              {missing.length > 0 && (
                <SettingRow
                  label={label('downloadDerivatives')}
                  className="max-sm:grid-cols-[minmax(0,1fr)_auto] max-sm:items-center"
                >
                  <Switch
                    aria-label={label('downloadDerivatives')}
                    checked={derivatives}
                    onChange={setDerivatives}
                  />
                </SettingRow>
              )}
            </div>
          </ScrollArea>
          <div className="shrink-0 bg-vergex-surface px-4">
            <SettingActions
              message={message}
              messageTone={!validRange ? 'danger' : 'muted'}
            >
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
                  !validRange ||
                  chosenSymbols.length === 0 ||
                  intervals.length === 0
                }
                onClick={() => void run()}
              >
                {running ? label('runningBacktest') : label('runBacktest')}
              </Button>
            </SettingActions>
          </div>
        </>
      )}
    </Panel>
  )
}
