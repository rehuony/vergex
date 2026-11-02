import { useEffect, useState, type RefObject } from 'react'
import { ArrowDown, ArrowUp, Minus } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import {
  FORECAST_INTERVALS,
  tooFewMoments,
  type ForecastInterval,
} from '@/api/forecast'
import type { FuturesChart } from '@/api/futures'
import { chartLineClass } from '@/components/market-chart/chart-legend'
import { Switch } from '@/components/ui/switch'
import { buildForecastPath } from '@/constants/route-paths'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { cn } from '@/utils/cn'

const STORAGE_KEY = 'vergex.futures.forecast'
const REFRESH_MS = 30000
const WAITING_REFRESH_MS = 5000

const NO_ROWS: never[] = []

function remembered() {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'on'
  } catch {
    return false
  }
}

// Whether the forecast is overlaid, remembered on this browser.
export function useForecastSwitch() {
  const [on, setOn] = useState(remembered)
  const toggle = (next: boolean) => {
    setOn(next)
    try {
      localStorage.setItem(STORAGE_KEY, next ? 'on' : 'off')
    } catch {
      /* Only a convenience: it just won't be remembered. */
    }
  }
  return [on, toggle] as const
}

export function ForecastSwitch({
  on,
  onChange,
}: {
  on: boolean
  onChange: (next: boolean) => void
}) {
  const { language } = useLanguage()
  const name = t('futures.forecast.toggle', language)
  return (
    <label className="flex h-8 items-center gap-2 whitespace-nowrap text-xs text-vergex-muted max-sm:h-10">
      {name}
      <Switch aria-label={name} checked={on} onChange={onChange} />
    </label>
  )
}

// forecastCoin names the Binance coin behind a market, since Forecast reads only
// Binance. An OKX USDT swap is the same coin; anything else has no forecast.
export function forecastCoin(venue: string, symbol: string): string | null {
  if (venue === 'binance') return symbol
  const swap = /^([A-Z0-9]+)-USDT-SWAP$/.exec(symbol)
  return swap ? `${swap[1]}USDT` : null
}

const percent = (rate: number | null) => `${((rate ?? 0) * 100).toFixed(1)}%`

// FuturesForecast overlays the coin's forecast on the market chart, off until asked
// for since this chart is for placing live trades. It reads the strategy in use from
// one read-only endpoint; choosing a strategy stays on the Forecast page. It extends the
// real candles, and answers far sooner than they do, so it is not asked for until they
// are on the chart: simulated candles never show on their own.
export function FuturesForecast({
  on,
  chart,
  venue,
  symbol,
  interval,
  scene,
  candlesReady,
  comparing,
}: {
  on: boolean
  chart: RefObject<FuturesChart | null>
  venue: string
  symbol: string
  interval: string
  scene: number
  candlesReady: boolean
  comparing: boolean
}) {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`futures.forecast.${key}`, language, params)

  const coin = forecastCoin(venue, symbol)
  const covered = FORECAST_INTERVALS.includes(interval as ForecastInterval)
  const wanted = on && candlesReady && !comparing && Boolean(coin) && covered
  const { data, error } = useQuery({
    queryKey: ['futures-forecast', coin, interval],
    queryFn: () => api.getForecastOverlay(coin as string, interval),
    enabled: wanted,
    refetchInterval: (query) =>
      query.state.data?.pending ? WAITING_REFRESH_MS : REFRESH_MS,
  })
  const rows = wanted && data ? data.projection : NO_ROWS

  useEffect(() => {
    chart.current?.setForecast(venue, symbol, interval, rows)
  }, [chart, venue, symbol, interval, scene, rows])

  const note =
    !on || !candlesReady || comparing || !interval
      ? null
      : !coin
        ? label('noMarket')
        : !covered
          ? label('shortInterval')
          : error
            ? label('failed')
            : data?.pending
              ? t(`forecast.pendingState.${data.pending}`, language)
              : null

  const score = data ? Math.round(data.display_prob * 100) : 0
  const Icon =
    data?.direction === 'up'
      ? ArrowUp
      : data?.direction === 'down'
        ? ArrowDown
        : Minus
  if (note) {
    return (
      <p
        role="status"
        className={cn(chartLineClass, 'pointer-events-none text-vergex-muted')}
      >
        {note}
      </p>
    )
  }
  if (!wanted || !data || !coin) return null
  // A percentage of a handful of calls on a page for placing trades reads as far more than
  // it is, so it waits until there is something behind it.
  const { samples, moments, hit_rate, chance_rate } = data.stats
  const record =
    samples === 0
      ? null
      : tooFewMoments(moments)
        ? label('fewCalls', { count: samples })
        : `${label('hitRate', { rate: percent(hit_rate), count: samples })} · ${label('byChance', { rate: percent(chance_rate) })}`
  return (
    <Link
      to={buildForecastPath(coin, interval)}
      title={record ?? undefined}
      className={cn(
        chartLineClass,
        'text-vergex-muted before:absolute before:inset-0 before:rounded-md hover:before:bg-vergex-ink-tint active:before:bg-vergex-ink/10'
      )}
    >
      <span className="whitespace-nowrap font-medium text-vergex-ink">
        <Icon
          aria-label={t(
            `forecast.direction.${data.direction ?? 'neutral'}`,
            language
          )}
          className={cn(
            'mr-1 inline size-3 align-[-2px]',
            data.direction === 'up' && 'text-vergex-success',
            data.direction === 'down' && 'text-vergex-danger'
          )}
        />
        {data.calibrated ? `${score}%` : label('score', { score })}
      </span>
      {record && <span className="whitespace-nowrap">{record}</span>}
      {venue !== 'binance' && (
        <span className="whitespace-nowrap">{label('fromBinance')}</span>
      )}
    </Link>
  )
}
