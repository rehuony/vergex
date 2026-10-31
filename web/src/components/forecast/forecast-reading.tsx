import type { ReactNode } from 'react'
import type { ForecastDetail } from '@/api/forecast'
import { Panel } from '@/components/common/panel'
import { t, type Language } from '@/i18n/translations'
import {
  DirectionLabel,
  baseAsset,
  fmtPct,
  fmtPrice,
  fmtTime,
  scoreText,
} from './forecast-format'

function Reading({ name, children }: { name: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="truncate text-xs text-vergex-text-muted" title={name}>
        {name}
      </dt>
      <dd className="mt-0.5 truncate text-[13px] font-medium tabular-nums text-vergex-text">
        {children}
      </dd>
    </div>
  )
}

const percent = (part: number, whole: number) =>
  whole > 0 ? `${((part / whole) * 100).toFixed(1)}%` : '—'

// The call for the coin in view, and how this strategy's calls on it have done. The
// layout stays the same while a coin loads, so the list below it does not move.
export function ForecastReading({
  symbol,
  detail,
  price,
  language,
}: {
  symbol: string
  detail: ForecastDetail | undefined
  price: number | null | undefined
  language: Language
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const result = detail?.result
  const latest = detail?.latest
  const stats = detail?.stats
  const judged = stats ? stats.hits + stats.misses : 0
  return (
    <Panel className="shrink-0">
      <div className="flex items-start justify-between gap-3 px-4 pt-3">
        <div className="min-w-0">
          <p className="truncate text-xs text-vergex-text-muted">
            {baseAsset(symbol) || '—'}
          </p>
          <p className="mt-0.5 text-2xl font-semibold tabular-nums text-vergex-text">
            {fmtPrice(price ?? result?.close)}
          </p>
        </div>
        <div className="shrink-0 text-right">
          <p className="text-xs">
            {result ? (
              <DirectionLabel
                direction={result.direction}
                language={language}
                className="font-medium"
              />
            ) : (
              <span className="text-vergex-text-muted">—</span>
            )}
          </p>
          <p className="mt-0.5 whitespace-nowrap text-2xl font-semibold tabular-nums text-vergex-text">
            {latest ? (
              <>
                {latest.calibrated ? '' : `${label('scoreWord')} `}
                {scoreText(latest.display_prob, latest.calibrated)}
              </>
            ) : (
              '—'
            )}
          </p>
        </div>
      </div>
      <dl className="grid grid-cols-3 gap-x-3 gap-y-2 px-4 pb-3 pt-2">
        <Reading name={label('confidence')}>
          {result?.confidence ?? '—'}
        </Reading>
        <Reading name={label('expectedMove')}>
          {result ? `±${fmtPct(result.expected_move_pct)}` : '—'}
        </Reading>
        <Reading name={label('horizon')}>
          {result ? label('horizonBars', { bars: result.horizon }) : '—'}
        </Reading>
        <Reading name={label('support')}>{fmtPrice(result?.support)}</Reading>
        <Reading name={label('resistance')}>
          {fmtPrice(result?.resistance)}
        </Reading>
        <Reading name={label('regime')}>
          {result ? label(`regimeName.${result.regime}`) : '—'}
        </Reading>
        <Reading name={label('hitRate')}>
          {stats ? percent(stats.hits, judged) : '—'}
        </Reading>
        <Reading name={label('baseRate')}>
          {stats ? percent(stats.rose, stats.moved) : '—'}
        </Reading>
        <Reading name={label('updated')}>
          {latest ? fmtTime(latest.open_time) : '—'}
        </Reading>
      </dl>
    </Panel>
  )
}
