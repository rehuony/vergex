import { useEffect, useMemo, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import useSWR from 'swr'
import { api } from '@/api'
import {
  FORECAST_INTERVALS,
  type ForecastDetail,
  type ForecastIndicators,
  type ForecastInterval,
} from '@/api/forecast'
import { Panel, PanelState } from '@/components/common/panel'
import { SectionTabs } from '@/components/common/section-tabs'
import { Segmented } from '@/components/common/segmented'
import {
  ChartOptionsMenu,
  useChartOptions,
  wantsIndicators,
  type ChartOptions,
} from '@/components/forecast/chart-options'
import { CoinList, sortSignals } from '@/components/forecast/coin-list'
import { FactorList } from '@/components/forecast/factor-list'
import { ForecastFrame } from '@/components/forecast/forecast-frame'
import { baseAsset } from '@/components/forecast/forecast-format'
import { ForecastReading } from '@/components/forecast/forecast-reading'
import { RecordList } from '@/components/forecast/record-list'
import { useLiveCandle } from '@/components/forecast/use-live-candle'
import { LocalMarketChartView } from '@/components/market-chart/local-market-chart-view'
import type { ChartLines } from '@/components/market-chart/local-market-chart'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { buildForecastPath, ROUTES } from '@/constants/route-paths'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'
import { cn } from '@/utils/cn'

const SIGNALS_REFRESH_MS = 15000
const WAITING_REFRESH_MS = 3000
const DETAIL_REFRESH_MS = 15000
const CHART_MARKERS = 40
const LAST_COIN_KEY = 'vergex.forecast.coin'
const DEFAULT_INTERVAL: ForecastInterval = '1h'

const lastCoin = () => {
  try {
    return localStorage.getItem(LAST_COIN_KEY)
  } catch {
    return null
  }
}

const rememberCoin = (symbol: string) => {
  try {
    localStorage.setItem(LAST_COIN_KEY, symbol)
  } catch {
    /* Only a convenience: the next visit opens the strongest call instead. */
  }
}

function chartLines(
  options: ChartOptions,
  indicators: ForecastIndicators | undefined
): ChartLines {
  if (!indicators) return {}
  return {
    ...(options.ema && { ema20: indicators.ema20, ema50: indicators.ema50 }),
    ...(options.bollinger && {
      bollUpper: indicators.boll_upper,
      bollMiddle: indicators.boll_middle,
      bollLower: indicators.boll_lower,
    }),
    ...(options.vwap && { vwap: indicators.vwap }),
    ...(options.rsi && { rsi: indicators.rsi }),
  }
}

type DetailTab = 'factors' | 'record'

// One page for every coin: the chart, the call beside it and the list to move
// between coins. /forecast opens the last coin looked at.
export function ForecastPage() {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const navigate = useNavigate()
  const { symbol = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const requested = params.get('interval') as ForecastInterval
  const interval = FORECAST_INTERVALS.includes(requested)
    ? requested
    : DEFAULT_INTERVAL
  const upper = symbol.toUpperCase()
  const [options, setOptions] = useChartOptions()
  const [tab, setTab] = useState<DetailTab>('factors')
  const [onProjection, setOnProjection] = useState(false)
  const [listOpen, setListOpen] = useState(false)

  const signals = useSWR(
    ['forecast-signals', interval],
    () => api.getForecastSignals(interval),
    { refreshInterval: SIGNALS_REFRESH_MS, keepPreviousData: true }
  )
  const rows = useMemo(
    () => (signals.data?.interval === interval ? signals.data.signals : []),
    [signals.data, interval]
  )

  useEffect(() => {
    if (upper) {
      rememberCoin(upper)
      return
    }
    const remembered = lastCoin()
    if (remembered) {
      navigate(buildForecastPath(remembered, interval), { replace: true })
    } else if (rows.length > 0) {
      navigate(buildForecastPath(sortSignals(rows)[0].symbol, interval), {
        replace: true,
      })
    }
  }, [upper, rows, interval, navigate])

  const withIndicators = wantsIndicators(options)
  const detail = useSWR(
    upper ? ['forecast-detail', upper, interval, withIndicators] : null,
    () => api.getForecastDetail(upper, interval, withIndicators),
    {
      keepPreviousData: true,
      refreshInterval: (latest: ForecastDetail | undefined) =>
        latest && latest.last_closed === 0
          ? WAITING_REFRESH_MS
          : DETAIL_REFRESH_MS,
    }
  )
  // While another coin loads the chart keeps what it has, so nothing jumps.
  const shown = detail.data
  const current =
    shown?.symbol === upper && shown?.interval === interval ? shown : undefined
  const live = useLiveCandle(upper, interval)
  const fitFor = shown ? `${shown.symbol}/${shown.interval}` : ''

  const levels = useMemo(
    () =>
      options.levels && shown?.result
        ? [
            {
              price: shown.result.resistance,
              title: label('resistance'),
              tone: 'down' as const,
            },
            {
              price: shown.result.support,
              title: label('support'),
              tone: 'up' as const,
            },
          ]
        : [],
    [
      options.levels,
      shown?.result?.resistance,
      shown?.result?.support,
      language,
    ]
  )
  const markers = useMemo(
    () =>
      options.hits
        ? (shown?.records ?? [])
            .filter(
              (record) => record.outcome === 'hit' || record.outcome === 'miss'
            )
            .slice(0, CHART_MARKERS)
            .reverse()
            .map((record) => ({
              time: record.open_time / 1000,
              tone:
                record.outcome === 'hit' ? ('up' as const) : ('down' as const),
              position:
                record.direction === 'up'
                  ? ('below' as const)
                  : ('above' as const),
            }))
        : [],
    [options.hits, shown?.records]
  )
  const candles = useMemo(
    () =>
      shown?.forming
        ? [...shown.candles, shown.forming]
        : (shown?.candles ?? []),
    [shown?.candles, shown?.forming]
  )
  const projection = useMemo(
    () => (options.projection ? (shown?.projection ?? []) : []),
    [options.projection, shown?.projection]
  )
  const lines = useMemo(
    () => chartLines(options, shown?.indicators),
    [options, shown?.indicators]
  )

  const nothingTracked =
    !upper && !signals.isLoading && !signals.error && rows.length === 0

  const coinList = (onChosen?: () => void) => (
    <CoinList
      signals={rows}
      interval={interval}
      selected={upper}
      loading={signals.isLoading}
      failed={Boolean(signals.error)}
      onChosen={onChosen}
      language={language}
      className="h-full"
    />
  )

  return (
    <ForecastFrame
      language={language}
      note={
        <span className="text-xs text-vergex-text-muted max-xl:hidden">
          {label('experimental')}
        </span>
      }
    >
      {nothingTracked ? (
        <Panel>
          <PanelState role="status">
            {label('noCoins')}
            <Link
              to={ROUTES.forecastSettings}
              className={buttonVariants({ variant: 'secondary', size: 'sm' })}
            >
              {label('openSettings')}
            </Link>
          </PanelState>
        </Panel>
      ) : (
        <>
          <div className="grid min-w-0 gap-3 lg:h-[max(520px,calc(100dvh-161px))] lg:grid-cols-[minmax(0,1fr)_380px]">
            <Panel className="flex min-h-0 min-w-0 flex-col overflow-hidden max-lg:order-2">
              <div className="flex shrink-0 items-center gap-2 border-b border-[var(--line)] p-2">
                <Popover open={listOpen} onOpenChange={setListOpen}>
                  <PopoverTrigger asChild>
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      className="h-9 lg:hidden max-sm:h-10"
                    >
                      {baseAsset(upper) || label('coin')}
                      <ChevronDown className="size-4" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="h-[min(420px,70dvh)] w-[min(340px,calc(100vw-32px))] overflow-hidden p-0">
                    {coinList(() => setListOpen(false))}
                  </PopoverContent>
                </Popover>
                <Segmented
                  aria-label={label('interval')}
                  value={interval}
                  options={FORECAST_INTERVALS.map((value) => ({
                    value,
                    label: value,
                  }))}
                  onChange={(next) =>
                    setParams({ interval: next }, { replace: true })
                  }
                />
                <div className="ml-auto">
                  <ChartOptionsMenu
                    options={options}
                    onChange={setOptions}
                    language={language}
                  />
                </div>
              </div>
              {detail.error && !shown ? (
                <PanelState
                  role="alert"
                  className="h-[420px] lg:h-auto lg:flex-1"
                >
                  {label('detailFailed')}
                </PanelState>
              ) : candles.length > 0 ? (
                <div className="relative h-[420px] min-h-0 lg:h-auto lg:flex-1">
                  <LocalMarketChartView
                    candles={candles}
                    levels={levels}
                    markers={markers}
                    projection={projection}
                    lines={lines}
                    live={current ? live : null}
                    fitFor={fitFor}
                    onProjectionHover={setOnProjection}
                    label={label('chartName', { symbol: upper, interval })}
                    failure={label('chartFailed')}
                    className="h-full"
                  />
                  {projection.length > 0 && (
                    <p
                      className={cn(
                        'pointer-events-none absolute left-3 top-2 z-10 max-w-[calc(100%-80px)] truncate text-xs',
                        onProjection
                          ? 'font-medium text-vergex-text'
                          : 'text-vergex-text-muted'
                      )}
                    >
                      {label('chart.projectionNote')}
                    </p>
                  )}
                </div>
              ) : (
                <PanelState
                  role="status"
                  className="h-[420px] lg:h-auto lg:flex-1"
                >
                  {shown
                    ? label(`pendingState.${shown.pending ?? 'waiting'}`)
                    : label('loading')}
                </PanelState>
              )}
            </Panel>

            <div className="flex min-h-0 min-w-0 flex-col gap-3 max-lg:order-1">
              <ForecastReading
                symbol={upper}
                detail={current}
                price={live?.close ?? current?.price}
                language={language}
              />
              <Panel className="min-h-0 flex-1 overflow-hidden max-lg:hidden">
                {coinList()}
              </Panel>
            </div>
          </div>

          <div className="mt-4">
            <SectionTabs
              tabs={[
                { id: 'factors', label: label('factors') },
                { id: 'record', label: label('record') },
              ]}
              value={tab}
              onChange={setTab}
              label={label('detailTabs')}
            />
            {tab === 'factors' ? (
              current?.result && current.result.factors.length > 0 ? (
                <FactorList
                  factors={current.result.factors}
                  language={language}
                />
              ) : (
                <Panel>
                  <PanelState role="status">
                    {current
                      ? label(`pendingState.${current.pending ?? 'waiting'}`)
                      : label('loading')}
                  </PanelState>
                </Panel>
              )
            ) : (
              <RecordList
                records={current?.records ?? []}
                hits={current?.stats.hits ?? 0}
                misses={current?.stats.misses ?? 0}
                language={language}
              />
            )}
          </div>
        </>
      )}
    </ForecastFrame>
  )
}
