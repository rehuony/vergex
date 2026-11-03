import { useEffect, useMemo, useRef, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { Link, useSearchParams } from 'react-router-dom'
import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { api } from '@/api'
import {
  FORECAST_INTERVALS,
  type ForecastIndicators,
  type ForecastInterval,
} from '@/api/forecast'
import { Panel, PanelLoading, PanelState } from '@/components/common/panel'
import { Segmented } from '@/components/common/segmented'
import {
  ChartOptionsMenu,
  useChartOptions,
  wantsIndicators,
  type ChartOptions,
} from '@/components/forecast/chart-options'
import { CoinList, sortCoins } from '@/components/forecast/coin-list'
import { FactorList } from '@/components/forecast/factor-list'
import { ForecastFrame } from '@/components/forecast/forecast-frame'
import { detailQuery } from '@/components/forecast/forecast-queries'
import { baseAsset } from '@/components/forecast/forecast-format'
import {
  ForecastCall,
  ForecastFigures,
} from '@/components/forecast/forecast-reading'
import { RecordList } from '@/components/forecast/record-list'
import { useLiveCandle } from '@/components/forecast/use-live-candle'
import { StrategySelect } from '@/components/forecast/strategy-select'
import { useWatchlist } from '@/components/forecast/use-watchlist'
import { LocalMarketChartView } from '@/components/market-chart/local-market-chart-view'
import type { ChartLines } from '@/components/market-chart/local-market-chart'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { ROUTES } from '@/constants/route-paths'
import { useAssistantPage } from '@/hooks/use-assistant-page'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'

const SIGNALS_REFRESH_MS = 15000
const WAITING_REFRESH_MS = 3000
const DETAIL_REFRESH_MS = 15000
const CHART_MARKERS = 40
const COIN_PREVIEW_MS = 100
const LAST_COIN_KEY = 'vergex.forecast.coin'
const DEFAULT_INTERVAL: ForecastInterval = '1h'
const LAST_INTERVAL_KEY = 'vergex.forecast.interval'

const lastCoin = () => {
  try {
    return localStorage.getItem(LAST_COIN_KEY)
  } catch {
    return null
  }
}

const lastInterval = () => {
  try {
    const saved = localStorage.getItem(LAST_INTERVAL_KEY) as ForecastInterval
    return FORECAST_INTERVALS.includes(saved) ? saved : DEFAULT_INTERVAL
  } catch {
    return DEFAULT_INTERVAL
  }
}

const rememberInterval = (interval: ForecastInterval) => {
  try {
    localStorage.setItem(LAST_INTERVAL_KEY, interval)
  } catch {
    /* Only a convenience: the next visit opens the default interval. */
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

// One page for every coin: the list to move between coins, the call above the chart,
// and what the call rests on below it. The address can name a coin and an interval, and
// without them the page opens the last ones looked at.
export function ForecastPage() {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const [params, setParams] = useSearchParams()
  const requested = params.get('interval') as ForecastInterval
  const interval = FORECAST_INTERVALS.includes(requested)
    ? requested
    : lastInterval()
  const asked = params.get('symbol')?.toUpperCase() ?? ''
  // The coin to show when the address names none. Once found it holds, so a refresh
  // of the list cannot change the coin under the reader.
  const [fallback, setFallback] = useState(lastCoin)
  const upper = asked || fallback || ''
  const [options, setOptions] = useChartOptions()
  const [listOpen, setListOpen] = useState(false)
  const { watchlist, toggle: toggleWatch } = useWatchlist(language)

  useEffect(() => rememberInterval(interval), [interval])
  useAssistantPage('forecast_signals', () => ({
    symbol: upper,
    interval,
    chart_options: options,
  }))

  const coins = useQuery({
    queryKey: ['forecast-coins', interval],
    queryFn: () => api.getForecastCoins(interval),
    refetchInterval: SIGNALS_REFRESH_MS,
    placeholderData: keepPreviousData,
  })
  const coinsLoading = coins.isLoading || coins.isPlaceholderData
  const rows = useMemo(
    () => (coins.data?.interval === interval ? coins.data.coins : []),
    [coins.data, interval]
  )

  useEffect(() => {
    if (asked) setFallback(asked)
    else if (!fallback && rows.length > 0) {
      setFallback(sortCoins(rows)[0].symbol)
    }
  }, [asked, fallback, rows])
  useEffect(() => {
    if (upper) rememberCoin(upper)
  }, [upper])

  const noneInUse = coins.data?.strategy_id === ''
  const withIndicators = wantsIndicators(options)
  const detail = useQuery({
    ...detailQuery(upper, interval, withIndicators),
    enabled: Boolean(upper) && !noneInUse,
    placeholderData: keepPreviousData,
    refetchInterval: (query) =>
      query.state.data?.last_closed === 0
        ? WAITING_REFRESH_MS
        : DETAIL_REFRESH_MS,
  })
  // A coin the pointer rests on is read ahead, so opening it shows it at once.
  const client = useQueryClient()
  const previewing = useRef<number>()
  const preview = (symbol: string | null) => {
    window.clearTimeout(previewing.current)
    if (!symbol || noneInUse) return
    previewing.current = window.setTimeout(() => {
      void client.prefetchQuery(detailQuery(symbol, interval, withIndicators))
    }, COIN_PREVIEW_MS)
  }
  useEffect(() => () => window.clearTimeout(previewing.current), [])

  // While another coin loads the chart keeps what it has, so nothing jumps.
  const shown = detail.data
  const current =
    shown?.symbol === upper && shown?.interval === interval ? shown : undefined
  const live = useLiveCandle(upper, interval)
  const row = rows.find((coin) => coin.symbol === upper)
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
  // The calls the statistics count: one per horizon, judged, with a direction.
  const markers = useMemo(
    () =>
      options.hits
        ? (shown?.signals ?? [])
            .filter(
              (signal) =>
                signal.independent &&
                (signal.outcome === 'hit' || signal.outcome === 'miss') &&
                signal.direction !== 'neutral'
            )
            .slice(0, CHART_MARKERS)
            .reverse()
            .map((signal) => ({
              time: signal.open_time / 1000,
              direction: signal.direction as 'up' | 'down',
              tone:
                signal.outcome === 'hit' ? ('up' as const) : ('down' as const),
            }))
        : [],
    [options.hits, shown?.signals]
  )
  const candles = useMemo(
    () =>
      shown?.forming
        ? [...shown.candles, shown.forming]
        : (shown?.candles ?? []),
    [shown?.candles, shown?.forming]
  )
  const forecast = useMemo(() => {
    const latest = shown?.latest
    if (!options.projection || !latest || latest.horizon <= 0) return null
    return {
      time: latest.open_time / 1000,
      step: (latest.target_time - latest.open_time) / 1000 / latest.horizon,
      close: latest.close,
      expectedMovePct: latest.expected_move_pct,
      horizon: latest.horizon,
      prob: latest.display_prob,
      direction: latest.direction,
    }
  }, [options.projection, shown?.latest])
  const lines = useMemo(
    () => chartLines(options, shown?.indicators),
    [options, shown?.indicators]
  )

  const nothingTracked =
    !upper && !coinsLoading && !coins.error && rows.length === 0

  const coinList = (onChosen?: () => void) => (
    <CoinList
      coins={rows}
      interval={interval}
      selected={upper}
      loading={coinsLoading}
      failed={Boolean(coins.error)}
      watchlist={watchlist}
      onToggleWatch={(next) => void toggleWatch(next)}
      onPreview={preview}
      onChosen={onChosen}
      language={language}
      className="h-full"
    />
  )

  return (
    <ForecastFrame
      language={language}
      actions={<StrategySelect language={language} />}
    >
      {noneInUse ? (
        <Panel>
          <PanelState role="status">
            {label('noStrategyInUse')}
            <Link
              to={ROUTES.forecastStrategies}
              className={buttonVariants({ variant: 'secondary', size: 'sm' })}
            >
              {label('chooseStrategy')}
            </Link>
          </PanelState>
        </Panel>
      ) : nothingTracked ? (
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
        <div className="grid min-w-0 items-start gap-4 lg:grid-cols-[320px_minmax(0,1fr)]">
          <Panel className="hidden overflow-hidden lg:sticky lg:top-[157px] lg:block lg:h-[max(320px,calc(100dvh-181px))]">
            {coinList()}
          </Panel>

          <div className="flex min-w-0 flex-col gap-4">
            <Panel className="flex min-w-0 flex-col overflow-hidden lg:h-[max(480px,calc(100dvh-181px))]">
              <div className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-b border-vergex-line p-2 lg:pl-4">
                <Popover open={listOpen} onOpenChange={setListOpen}>
                  <PopoverTrigger asChild>
                    <Button
                      type="button"
                      variant="select"
                      className="lg:hidden max-sm:h-10"
                    >
                      {baseAsset(upper) || label('coin')}
                      <ChevronDown
                        className="size-4 text-vergex-muted"
                        aria-hidden="true"
                      />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="h-[min(480px,70dvh)] w-[min(360px,calc(100vw-32px))] overflow-hidden p-0">
                    {coinList(() => setListOpen(false))}
                  </PopoverContent>
                </Popover>
                <ForecastCall
                  symbol={upper}
                  latest={current?.latest ?? row?.signal}
                  price={live?.close ?? current?.price ?? row?.price}
                  change={current?.change_24h_pct ?? row?.change_24h_pct}
                  stale={current?.stale ?? row?.stale}
                  starred={watchlist.includes(upper)}
                  onToggleWatch={(next) => void toggleWatch(next)}
                  language={language}
                />
                <div className="ml-auto flex items-center gap-2">
                  <Segmented
                    aria-label={label('interval')}
                    value={interval}
                    options={FORECAST_INTERVALS.map((value) => ({
                      value,
                      label: value,
                    }))}
                    onChange={(next) =>
                      setParams(
                        (current) => {
                          current.set('interval', next)
                          return current
                        },
                        { replace: true }
                      )
                    }
                  />
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
                <div className="h-[420px] min-h-0 lg:h-auto lg:flex-1">
                  <LocalMarketChartView
                    candles={candles}
                    levels={levels}
                    markers={markers}
                    forecast={forecast}
                    lines={lines}
                    live={current ? live : null}
                    fitFor={fitFor}
                    label={label('chartName', { symbol: upper, interval })}
                    failure={label('chartFailed')}
                    text={{
                      open: label('chart.open'),
                      high: label('chart.high'),
                      low: label('chart.low'),
                      close: label('chart.close'),
                      volume: label('chart.volume'),
                      zoomIn: label('chart.zoomIn'),
                      zoomOut: label('chart.zoomOut'),
                      reset: label('chart.reset'),
                    }}
                    className="h-full"
                  />
                </div>
              ) : shown ? (
                <PanelState
                  role="status"
                  className="h-[420px] lg:h-auto lg:flex-1"
                >
                  {label(`pendingState.${shown.pending ?? 'waiting'}`)}
                </PanelState>
              ) : (
                <PanelLoading className="h-[420px] lg:h-auto lg:flex-1" />
              )}
            </Panel>

            <ForecastFigures
              detail={current}
              price={live?.close ?? current?.price}
              language={language}
            />

            <div className="grid min-w-0 gap-4 xl:grid-cols-2">
              <FactorList
                result={current?.result}
                pending={
                  current
                    ? label(`pendingState.${current.pending ?? 'waiting'}`)
                    : undefined
                }
                language={language}
                className="flex h-[440px] flex-col overflow-hidden"
              />
              <RecordList
                signals={current?.signals ?? []}
                stats={current?.stats}
                language={language}
                className="flex h-[440px] flex-col overflow-hidden"
              />
            </div>
          </div>
        </div>
      )}
    </ForecastFrame>
  )
}
