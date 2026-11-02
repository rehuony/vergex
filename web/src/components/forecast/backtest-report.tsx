import {
  ENOUGH_MOMENTS,
  tooFewMoments,
  type BacktestMetrics,
  type BacktestReport,
  type FactorEdge,
} from '@/api/forecast'
import {
  cellClass,
  FigureGrid,
  headClass,
  numberClass,
  StatStrip,
  tableClass,
} from '@/components/common/figures'
import { PanelState } from '@/components/common/panel'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'
import { fmtPct, fmtRate } from './forecast-format'
import {
  BreakdownTable,
  pointsAbove,
  type BreakdownRow,
} from './forecast-stats'

const pct = (value: number) => `${(value * 100).toFixed(1)}%`

// Returns are only known where a call was made.
const afterFees = (metrics: BacktestMetrics) =>
  metrics.signals > 0 ? fmtPct(metrics.avg_return_after_fee_pct, true) : '—'

const metricsRow = (name: string, metrics: BacktestMetrics): BreakdownRow => ({
  name,
  samples: metrics.samples,
  moments: metrics.moments || null,
  hit: metrics.hit_rate,
  chance: metrics.chance_rate,
  lift: metrics.lift,
  afterFees: metrics.signals > 0 ? metrics.avg_return_after_fee_pct : null,
})

const sortedRows = (group: Record<string, BacktestMetrics>) =>
  Object.entries(group).sort(([a], [b]) => a.localeCompare(b))

// The reasons a series can be skipped for, as the server words them. Anything else is
// passed on as it came.
const SKIP_REASONS: Record<string, string> = {
  'not enough history': 'skipNoHistory',
  'no horizon set': 'skipNoHorizon',
  'unsupported interval': 'skipBadInterval',
}

const correlationText = (edge: FactorEdge) => {
  const correlation = edge.correlation ?? 0
  return `${correlation > 0 ? '+' : ''}${correlation.toFixed(3)}`
}

// One run's result, led by its hit rate against chance and what it made after fees.
export function BacktestReportView({
  report,
  language,
}: {
  report: BacktestReport
  language: Language
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const { overall } = report
  const skippedItem = (item: string) => {
    const split = item.indexOf(': ')
    const reason = SKIP_REASONS[item.slice(split + 2)]
    return split > 0 && reason
      ? `${item.slice(0, split)}: ${label(reason)}`
      : item
  }
  const skipped =
    (report.skipped?.length ?? 0) > 0 ? (
      <p className="border-t border-vergex-line px-4 py-3 text-xs text-vergex-muted">
        {label('skippedSeries', {
          items: report.skipped!.map(skippedItem).join('; '),
        })}
      </p>
    ) : null

  if (overall.samples === 0) {
    return (
      <>
        <PanelState role="status">{label('noSamples')}</PanelState>
        {skipped}
      </>
    )
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
            value: afterFees(overall),
            note: label('perSignal'),
            gain:
              overall.signals > 0 && !tooFewMoments(overall.moments)
                ? overall.avg_return_after_fee_pct
                : null,
          },
          {
            name: label('samples'),
            value: overall.samples,
            note: [
              label('signalCount', { count: overall.signals }),
              ...(overall.moments
                ? [label('momentsCount', { count: overall.moments })]
                : []),
            ].join(' · '),
          },
        ]}
      />
      <FigureGrid
        figures={(
          [
            ['baseRate', pct(overall.baseline_up)],
            ['lift', pct(overall.lift)],
            ['moveCoverage', pct(overall.move_coverage)],
            ['brier', overall.brier.toFixed(4)],
            ['brierCandles', overall.brier_candles.toFixed(4)],
            ['derivativeCoverage', pct(overall.derivative_coverage)],
          ] as const
        ).map(([key, value]) => ({ name: label(key), value }))}
      />
      <BreakdownTable
        head={{
          samples: label('samples'),
          moments: label('momentsHead'),
          tooFew: label('tooFewMoments', { count: ENOUGH_MOMENTS }),
          hit: label('hitRate'),
          chance: label('chanceRate'),
          lift: label('lift'),
          afterFees: label('afterFeesShort'),
        }}
        groups={[
          {
            title: label('interval'),
            rows: sortedRows(report.by_interval).map(([name, metrics]) =>
              metricsRow(name, metrics)
            ),
          },
          {
            title: label('regime'),
            rows: sortedRows(report.by_regime).map(([name, metrics]) =>
              metricsRow(label(`regimeName.${name}`), metrics)
            ),
          },
          {
            title: label('coin'),
            rows: sortedRows(report.by_symbol).map(([name, metrics]) =>
              metricsRow(name, metrics)
            ),
          },
        ]}
      />
      <table className={cn(tableClass, 'mt-4')}>
        <thead>
          <tr>
            <th className={cn(headClass, 'border-t')}>{label('scoreRange')}</th>
            <th className={cn(headClass, 'w-20 border-t text-right')}>
              {label('samples')}
            </th>
            <th className={cn(headClass, 'w-24 border-t text-right')}>
              {label('averageSaid')}
            </th>
            <th className={cn(headClass, 'w-24 border-t text-right')}>
              {label('actuallyRose')}
            </th>
          </tr>
        </thead>
        <tbody>
          {(report.reliability ?? []).map((bucket) => (
            <tr key={bucket.from}>
              <td className={cn(cellClass, 'text-vergex-ink')}>
                {Math.round(bucket.from * 100)}–{Math.round(bucket.to * 100)}
              </td>
              <td className={cn(cellClass, numberClass)}>{bucket.samples}</td>
              <td className={cn(cellClass, numberClass)}>
                {pct(bucket.avg_prob)}
              </td>
              <td className={cn(cellClass, numberClass)}>
                {pct(bucket.up_rate)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {(report.factors?.length ?? 0) > 0 && (
        <table className={cn(tableClass, 'mt-4')}>
          <thead>
            <tr>
              <th className={cn(headClass, 'border-t')}>
                {label('factor.name')}
              </th>
              {(['trending', 'ranging'] as const).flatMap((regime) => [
                <th
                  key={regime}
                  className={cn(headClass, 'w-24 border-t text-right')}
                >
                  {label('agreedIn', {
                    regime: label(`regimeName.${regime}`),
                  })}
                </th>,
                <th
                  key={`${regime}-correlation`}
                  className={cn(
                    headClass,
                    'w-24 border-t text-right max-sm:hidden'
                  )}
                >
                  {label('correlationIn', {
                    regime: label(`regimeName.${regime}`),
                  })}
                </th>,
              ])}
            </tr>
          </thead>
          <tbody>
            {report.factors!.map((factor) => (
              <tr key={factor.name}>
                <td className={cn(cellClass, 'text-vergex-ink')}>
                  {label(`factor.${factor.name}`)}
                </td>
                {[factor.trending, factor.ranging].flatMap((edge, index) => [
                  <td key={index} className={cn(cellClass, numberClass)}>
                    {fmtRate(edge.agree_rate)}
                  </td>,
                  <td
                    key={`${index}-correlation`}
                    className={cn(cellClass, numberClass, 'max-sm:hidden')}
                  >
                    {correlationText(edge)}
                  </td>,
                ])}
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {skipped}
    </>
  )
}
