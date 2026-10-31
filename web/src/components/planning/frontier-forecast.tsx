import { useEffect, useState, type RefObject } from 'react'
import { ArrowDown, ArrowUp, Minus } from 'lucide-react'
import { Link } from 'react-router-dom'
import useSWR from 'swr'
import { api } from '@/api'
import { FORECAST_INTERVALS, type ForecastInterval } from '@/api/forecast'
import type { PlanningChart } from '@/api/planning'
import { Switch } from '@/components/ui/switch'
import { buildForecastPath } from '@/constants/route-paths'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'
import { cn } from '@/utils/cn'

const STORAGE_KEY = 'vergex.frontier.forecast'
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

// forecastCoin names the Binance coin behind a market, since Forecast reads only
// Binance. An OKX USDT swap is the same coin; anything else has no forecast.
export function forecastCoin(venue: string, symbol: string): string | null {
  if (venue === 'binance') return symbol
  const swap = /^([A-Z0-9]+)-USDT-SWAP$/.exec(symbol)
  return swap ? `${swap[1]}USDT` : null
}

const percent = (part: number, whole: number) =>
  whole > 0 ? `${((part / whole) * 100).toFixed(1)}%` : '—'

// FrontierForecast overlays the coin's forecast on the market chart, off until asked
// for since this chart is for placing live trades. It reads the strategy in use from
// one read-only endpoint; choosing a strategy stays on the Forecast page.
export function FrontierForecast({
  chart,
  venue,
  symbol,
  interval,
  scene,
  comparing,
  hovering,
}: {
  chart: RefObject<PlanningChart | null>
  venue: string
  symbol: string
  interval: string
  scene: number
  comparing: boolean
  hovering: boolean
}) {
  const { language } = useLanguage()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`frontier.forecast.${key}`, language, params)
  const [on, setOn] = useState(remembered)

  const coin = forecastCoin(venue, symbol)
  const covered = FORECAST_INTERVALS.includes(interval as ForecastInterval)
  const wanted = on && !comparing && Boolean(coin) && covered
  const { data, error } = useSWR(
    wanted ? ['frontier-forecast', coin, interval] : null,
    () => api.getForecastOverlay(coin as string, interval),
    {
      refreshInterval: (latest) =>
        latest?.pending ? WAITING_REFRESH_MS : REFRESH_MS,
    }
  )
  const rows = wanted && data ? data.projection : NO_ROWS

  useEffect(() => {
    chart.current?.setForecast(venue, symbol, interval, rows)
  }, [chart, venue, symbol, interval, scene, rows])

  const toggle = (next: boolean) => {
    setOn(next)
    try {
      localStorage.setItem(STORAGE_KEY, next ? 'on' : 'off')
    } catch {
      /* Only a convenience: it just won't be remembered. */
    }
  }

  const note =
    !on || comparing || !interval
      ? null
      : !coin
        ? label('noMarket')
        : !covered
          ? label('shortInterval')
          : error
            ? label('failed')
            : !data
              ? label('loading')
              : data.pending
                ? t(`forecast.pendingState.${data.pending}`, language)
                : null

  const score = data ? Math.round(data.display_prob * 100) : 0
  const Icon =
    data?.direction === 'up'
      ? ArrowUp
      : data?.direction === 'down'
        ? ArrowDown
        : Minus
  return (
    <>
      <label className="absolute right-3 top-1.5 z-10 flex h-6 items-center gap-2 text-xs text-vergex-text-muted">
        {label('toggle')}
        <Switch aria-label={label('toggle')} checked={on} onChange={toggle} />
      </label>
      {note ? (
        <p
          role="status"
          className="pointer-events-none absolute left-3 top-10 z-10 rounded-md bg-[var(--surface)] px-2 py-1 text-xs text-vergex-text-muted"
        >
          {note}
        </p>
      ) : (
        wanted &&
        data &&
        coin && (
          <div className="pointer-events-none absolute left-3 top-10 z-10 max-w-[calc(100%-96px)] rounded-md bg-[var(--surface)] px-2 py-1 text-xs tabular-nums text-vergex-text-muted">
            <p className="sm:truncate">
              <Link
                to={buildForecastPath(coin, interval)}
                className="pointer-events-auto font-medium text-vergex-text hover:underline"
              >
                <Icon
                  aria-label={t(
                    `forecast.direction.${data.direction ?? 'neutral'}`,
                    language
                  )}
                  className={cn(
                    'mr-1 inline size-3.5 align-[-2px]',
                    data.direction === 'up' && 'text-vergex-success',
                    data.direction === 'down' && 'text-vergex-danger'
                  )}
                />
                {data.calibrated ? `${score}%` : label('score', { score })}
              </Link>
              {' · '}
              <span className="whitespace-nowrap">
                {label('hitRate', {
                  rate: percent(
                    data.stats.hits,
                    data.stats.hits + data.stats.misses
                  ),
                })}
              </span>
              {' · '}
              <span className="whitespace-nowrap">
                {label('roseAnyway', {
                  rate: percent(data.stats.rose, data.stats.moved),
                })}
              </span>
            </p>
            <p className={cn('sm:truncate', hovering && 'text-vergex-text')}>
              {label('notPrices')}
              {venue !== 'binance' && ` ${label('fromBinance')}`}
            </p>
          </div>
        )
      )}
    </>
  )
}
