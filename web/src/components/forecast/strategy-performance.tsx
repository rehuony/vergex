import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  ENOUGH_MOMENTS,
  tooFewMoments,
  type BacktestRun,
  type ForecastCalibrationStatus,
  type ForecastStats,
} from '@/api/forecast'
import {
  cellClass,
  headClass,
  numberClass,
  StatStrip,
  tableClass,
} from '@/components/common/figures'
import { PanelLoading, PanelState } from '@/components/common/panel'
import { buttonVariants } from '@/components/ui/button'
import { ROUTES } from '@/constants/route-paths'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'
import { fmtDate, fmtPct, fmtRate } from './forecast-format'
import { backtestsQuery, performanceQuery } from './forecast-queries'
import {
  BreakdownTable,
  pointsAbove,
  type BreakdownRow,
} from './forecast-stats'

const CONFIDENCE_BANDS = ['0', '1', '2', '3', '4']
const bandName = (band: string) =>
  band === '4' ? '80–100' : `${Number(band) * 20}–${Number(band) * 20 + 19}`

const statsRow = (name: string, stats: ForecastStats): BreakdownRow => ({
  name,
  samples: stats.samples,
  moments: stats.moments,
  hit: stats.hit_rate,
  chance: stats.chance_rate,
  afterFees: stats.avg_return_after_fee_pct,
})

// How a strategy has done live, set beside its latest backtest: what it is worth is
// whether live calls beat chance and hold up against what the backtest promised.
export function StrategyPerformance({
  strategyId,
  retired,
  inUse,
  language,
}: {
  strategyId: string
  retired: boolean
  // Curves are only tried for the strategy in use.
  inUse: boolean
  language: Language
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const { data, error } = useQuery(performanceQuery(strategyId))
  const { data: runs } = useQuery({
    ...backtestsQuery,
    refetchOnWindowFocus: false,
  })

  if (error) {
    return <PanelState role="alert">{label('performanceFailed')}</PanelState>
  }
  if (!data) {
    return <PanelLoading />
  }

  const shownAs = (row: ForecastCalibrationStatus) =>
    row.active && row.built_at
      ? label('probabilitySince', { date: fmtDate(row.built_at) })
      : row.moments < row.needed
        ? label('scoreWord')
        : label(inUse ? 'scoreFailedCheck' : 'scoreNotChecked')

  const latestRun: BacktestRun | undefined = runs?.backtests.find(
    (run) =>
      run.strategy_id === strategyId &&
      run.status === 'finished' &&
      (run.summary?.samples ?? 0) > 0
  )
  const summary = latestRun?.summary
  const { overall } = data
  const named = (
    group: Record<string, ForecastStats>,
    order: string[],
    name: (key: string) => string
  ) =>
    order.flatMap((key) =>
      group[key] ? [statsRow(name(key), group[key])] : []
    )

  const latestBacktest: BreakdownRow = {
    name: label('latestBacktest'),
    note: latestRun ? (
      fmtDate(latestRun.created_at)
    ) : retired ? undefined : (
      <Link
        to={`${ROUTES.forecastBacktests}?strategy=${strategyId}`}
        className={buttonVariants({
          variant: 'link',
          size: 'sm',
          className: 'h-auto px-0 text-xs',
        })}
      >
        {label('runOne')}
      </Link>
    ),
    samples: summary?.samples ?? null,
    moments: summary ? summary.moments || null : 0,
    hit: summary?.hit_rate ?? null,
    chance: summary?.chance_rate ?? null,
    afterFees:
      summary && summary.signals > 0 ? summary.avg_return_after_fee_pct : null,
  }

  return (
    <>
      <StatStrip
        stats={[
          {
            name: label('hitRate'),
            value: fmtRate(overall.hit_rate),
            note: `${label('chanceRate')} ${fmtRate(overall.chance_rate)}`,
          },
          {
            name: label('aboveChance'),
            value: pointsAbove(overall.hit_rate, overall.chance_rate),
            note: label('points'),
          },
          {
            name: label('afterFeesShort'),
            value:
              overall.avg_return_after_fee_pct === null
                ? '—'
                : fmtPct(overall.avg_return_after_fee_pct, true),
            note: label('perSignal'),
            gain: tooFewMoments(overall.moments)
              ? null
              : overall.avg_return_after_fee_pct,
          },
          {
            name: label('samples'),
            value: overall.samples,
            note: `${label('judgedCount', { count: overall.judged })} · ${label('momentsCount', { count: overall.moments })}`,
          },
        ]}
      />
      <BreakdownTable
        head={{
          samples: label('samples'),
          moments: label('momentsHead'),
          tooFew: label('tooFewMoments', { count: ENOUGH_MOMENTS }),
          hit: label('hitRate'),
          chance: label('chanceRate'),
          afterFees: label('afterFeesShort'),
        }}
        groups={[
          {
            title: label('source'),
            rows: [statsRow(label('live'), overall), latestBacktest],
          },
          {
            title: label('interval'),
            rows: named(
              data.by_interval,
              data.calibration.map((row) => row.interval),
              (key) => key
            ),
          },
          {
            title: label('regime'),
            rows: named(data.by_regime, ['trending', 'ranging'], (key) =>
              label(`regimeName.${key}`)
            ),
          },
          {
            title: label('call'),
            rows: named(data.by_direction, ['up', 'down'], (key) =>
              label(`direction.${key}`)
            ),
          },
          {
            title: label('confidence'),
            rows: named(data.by_confidence, CONFIDENCE_BANDS, bandName),
          },
        ]}
      />
      {overall.samples === 0 && (
        <PanelState role="status">{label('noPerformance')}</PanelState>
      )}
      <table className={cn(tableClass, 'mt-4')}>
        <thead>
          <tr>
            <th className={cn(headClass, 'border-t')}>
              {label('scoreShownAs')}
            </th>
            <th className={cn(headClass, 'border-t')}>{label('shownAs')}</th>
            <th className={cn(headClass, 'border-t text-right')}>
              {label('moments')}
            </th>
          </tr>
        </thead>
        <tbody>
          {data.calibration.map((row) => (
            <tr key={row.interval}>
              <td className={cn(cellClass, 'text-vergex-ink')}>
                {row.interval}
              </td>
              <td className={cellClass}>{shownAs(row)}</td>
              <td className={cn(cellClass, numberClass)}>
                {row.moments} / {row.needed}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}
