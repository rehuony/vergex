import type { ReactNode } from 'react'
import { Star } from 'lucide-react'
import type { ForecastDetail, ForecastSignal } from '@/api/forecast'
import { Panel } from '@/components/common/panel'
import { Button } from '@/components/ui/button'
import { t, type Language } from '@/i18n'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/utils/cn'
import {
  DirectionLabel,
  baseAsset,
  fmtPct,
  fmtPrice,
  fmtRate,
  fmtTime,
  intervalMillis,
  scoreText,
} from './forecast-format'

function Figure({
  name,
  note,
  title,
  className,
  children,
}: {
  name: string
  // A smaller second value, such as a distance from the price.
  note?: ReactNode
  title?: string
  className?: string
  children: ReactNode
}) {
  return (
    <div className="min-w-0" title={title}>
      <dt className="truncate text-xs text-vergex-muted">{name}</dt>
      <dd
        className={cn(
          'mt-0.5 truncate font-mono text-[13px] font-medium tabular-nums text-vergex-ink',
          className
        )}
      >
        {children}
        {note && (
          <span className="ml-1.5 text-[11px] font-normal text-vergex-muted">
            {note}
          </span>
        )}
      </dd>
    </div>
  )
}

const distance = (level: number, price: number | null | undefined) =>
  price ? fmtPct((level / price - 1) * 100, true, 1) : undefined

// The call for the coin in view, in one line at the head of the chart, so the chart
// keeps the screen. While a coin loads, the coin list already knows what to show.
export function ForecastCall({
  symbol,
  latest,
  price,
  change,
  stale,
  starred,
  onToggleWatch,
  language,
}: {
  symbol: string
  latest: ForecastSignal | null | undefined
  price: number | null | undefined
  change: number | null | undefined
  stale: boolean | undefined
  starred: boolean
  onToggleWatch: (symbol: string) => void
  language: Language
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  // Nothing has come for this coin yet.
  const reading = latest === undefined && (price ?? null) === null
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 max-lg:contents">
      <span className="flex min-w-0 items-center gap-1">
        {/* Under `lg` the coin trigger beside this already names the coin. */}
        <span
          className="truncate text-sm font-medium text-vergex-ink max-lg:hidden"
          title={symbol}
        >
          {baseAsset(symbol) || '—'}
        </span>
        {symbol && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-pressed={starred}
            aria-label={label(starred ? 'removeWatch' : 'addWatch', {
              symbol: baseAsset(symbol),
            })}
            onClick={() => onToggleWatch(symbol)}
            className="size-6 max-lg:size-10"
          >
            <Star
              className={cn(
                'size-3.5',
                starred ? 'fill-current text-vergex-ink' : 'text-vergex-faint'
              )}
              aria-hidden="true"
            />
          </Button>
        )}
      </span>
      <span className="flex items-baseline gap-2 whitespace-nowrap">
        <span className="font-mono text-xl font-semibold tracking-tight tabular-nums text-vergex-ink">
          {reading ? (
            <Skeleton className="w-24" />
          ) : (
            fmtPrice(price ?? latest?.close)
          )}
        </span>
        <span
          title={label('change24h')}
          className={cn(
            'font-mono text-xs tabular-nums',
            change === null || change === undefined
              ? 'text-vergex-muted'
              : change >= 0
                ? 'text-vergex-success'
                : 'text-vergex-danger'
          )}
        >
          {reading ? <Skeleton className="w-12" /> : fmtPct(change ?? 0, true)}
        </span>
      </span>
      <span className="flex items-baseline gap-2 whitespace-nowrap">
        {latest ? (
          <DirectionLabel
            direction={latest.direction}
            language={language}
            className="self-center text-xs font-medium"
          />
        ) : (
          reading && <Skeleton className="h-3 w-8" />
        )}
        <span
          className={cn(
            'text-xl font-semibold tracking-tight tabular-nums',
            stale ? 'text-vergex-muted' : 'text-vergex-ink'
          )}
        >
          {latest ? (
            <>
              {latest.calibrated ? '' : `${label('scoreWord')} `}
              {scoreText(latest.display_prob, latest.calibrated)}
            </>
          ) : reading ? (
            <Skeleton className="w-14" />
          ) : (
            0
          )}
        </span>
      </span>
    </div>
  )
}

// What the call rests on and how this strategy's calls on the coin have done, under
// the chart. It holds its layout while a coin loads, so nothing below it moves.
export function ForecastFigures({
  detail,
  price,
  language,
}: {
  detail: ForecastDetail | undefined
  price: number | null | undefined
  language: Language
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const latest = detail?.latest
  const result = detail?.result
  const stats = detail?.stats
  const readings = result?.derivatives
  const shownPrice = price ?? latest?.close
  const closes = (time: number) =>
    latest ? fmtTime(time + intervalMillis[latest.interval]) : '—'
  // A figure being read is a bar, and one the call has nothing for is 0.
  const read = (value: ReactNode) => (detail ? value : <Skeleton />)
  return (
    <Panel>
      <dl className="grid grid-cols-[repeat(auto-fill,minmax(104px,1fr))] gap-x-3 gap-y-3 p-4">
        <Figure name={label('confidence')}>
          {read(latest?.confidence ?? 0)}
        </Figure>
        <Figure
          name={label('expectedMove')}
          note={
            latest ? label('horizonBars', { bars: latest.horizon }) : undefined
          }
          title={
            latest
              ? `${fmtPrice(latest.close * (1 - latest.expected_move_pct / 100))} – ${fmtPrice(latest.close * (1 + latest.expected_move_pct / 100))}`
              : undefined
          }
        >
          {read(`±${fmtPct(latest?.expected_move_pct ?? 0)}`)}
        </Figure>
        <Figure name={label('judgedAt')}>
          {read(latest ? closes(latest.target_time) : '—')}
        </Figure>
        <Figure
          name={label('support')}
          note={latest && distance(latest.support, shownPrice)}
        >
          {read(fmtPrice(latest?.support))}
        </Figure>
        <Figure
          name={label('resistance')}
          note={latest && distance(latest.resistance, shownPrice)}
        >
          {read(fmtPrice(latest?.resistance))}
        </Figure>
        <Figure
          name={label('regime')}
          note={
            result ? label('adxValue', { value: result.adx.toFixed(0) }) : ''
          }
        >
          {read(latest ? label(`regimeName.${latest.regime}`) : '—')}
        </Figure>
        <Figure
          name={label('hitRate')}
          note={
            stats ? label('callsCount', { count: stats.samples }) : undefined
          }
        >
          {read(fmtRate(stats?.hit_rate))}
        </Figure>
        <Figure name={label('chanceRate')}>
          {read(fmtRate(stats?.chance_rate))}
        </Figure>
        <Figure name={label('funding')}>
          {read(`${((readings?.funding ?? 0) * 100).toFixed(4)}%`)}
        </Figure>
        <Figure name={label('openInterest')}>
          {read(fmtPct(readings?.oi_change_pct ?? 0, true))}
        </Figure>
        <Figure name={label('longShort')}>
          {read(
            `z ${(readings?.long_short_z ?? 0) > 0 ? '+' : ''}${(readings?.long_short_z ?? 0).toFixed(2)}`
          )}
        </Figure>
        <Figure
          name={label('madeAt')}
          className={detail?.stale ? 'text-vergex-warning' : undefined}
          title={detail?.stale ? label('stale') : undefined}
        >
          {read(
            latest
              ? `${detail?.stale ? `${label('stale')} ` : ''}${closes(latest.open_time)}`
              : '—'
          )}
        </Figure>
      </dl>
    </Panel>
  )
}
