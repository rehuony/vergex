import type { BacktestMetrics, BacktestReport } from '@/api/forecast'
import { Panel } from '@/components/common/panel'
import { t, type Language } from '@/i18n/translations'
import { cn } from '@/utils/cn'
import { fmtDate, fmtPct } from './forecast-format'

const headClass = 'px-2 py-2 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-2 py-2 whitespace-nowrap'

const pct = (value: number) => `${(value * 100).toFixed(1)}%`
export const hitRateText = (metrics: BacktestMetrics) =>
  metrics.hit_rate === null ? '—' : pct(metrics.hit_rate)

// Beating the base rate after fees is what would make signals worth anything.
export const hasEdge = (metrics: BacktestMetrics) =>
  metrics.lift > 0 && metrics.avg_return_after_fee_pct > 0

function MetricsTable({
  title,
  rows,
  language,
}: {
  title: string
  rows: [string, BacktestMetrics][]
  language: Language
}) {
  const label = (key: string) => t(`forecast.${key}`, language)
  return (
    <div className="min-w-0">
      <table className="w-full text-left text-[13px] tabular-nums">
        <thead className="text-vergex-text-muted">
          <tr className="border-b border-[var(--line)]">
            <th className={headClass}>{title}</th>
            <th className={cn(headClass, 'text-right max-sm:hidden')}>
              {label('samples')}
            </th>
            <th className={cn(headClass, 'text-right')}>{label('hitRate')}</th>
            <th className={cn(headClass, 'text-right')}>{label('baseRate')}</th>
            <th className={cn(headClass, 'text-right max-sm:hidden')}>
              {label('lift')}
            </th>
            <th className={cn(headClass, 'text-right')}>
              {label('afterFeesShort')}
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map(([name, metrics]) => (
            <tr
              key={name}
              className="border-b border-[var(--line)] last:border-b-0"
            >
              <td className={cn(cellClass, 'text-vergex-text')}>{name}</td>
              <td className={cn(cellClass, 'text-right max-sm:hidden')}>
                {metrics.samples}
              </td>
              <td className={cn(cellClass, 'text-right')}>
                {hitRateText(metrics)}
              </td>
              <td className={cn(cellClass, 'text-right')}>
                {pct(metrics.baseline_up)}
              </td>
              <td className={cn(cellClass, 'text-right max-sm:hidden')}>
                {pct(metrics.lift)}
              </td>
              <td
                className={cn(
                  cellClass,
                  'text-right',
                  metrics.avg_return_after_fee_pct > 0
                    ? 'text-vergex-success'
                    : 'text-vergex-danger'
                )}
              >
                {fmtPct(metrics.avg_return_after_fee_pct, true)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

const sortedRows = (group: Record<string, BacktestMetrics>) =>
  Object.entries(group).sort(([a], [b]) => a.localeCompare(b))

// One run's result, led by whether it beat simply knowing the base rate.
export function BacktestReportView({
  id,
  report,
  language,
}: {
  id: number
  report: BacktestReport
  language: Language
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const { overall } = report
  const edge = hasEdge(overall)
  return (
    <Panel
      title={label('reportTitle', { id })}
      meta={
        report.first_time
          ? `${fmtDate(report.first_time)} – ${fmtDate(report.last_time)}`
          : undefined
      }
    >
      <p
        className={cn(
          'border-t border-[var(--line)] px-4 py-3 text-sm font-medium',
          edge ? 'text-vergex-success' : 'text-vergex-text'
        )}
      >
        {label(edge ? 'verdictEdge' : 'verdictNoEdge')}
      </p>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-3 border-t border-[var(--line)] p-4 sm:grid-cols-4 xl:grid-cols-8">
        {(
          [
            ['samples', String(overall.samples)],
            ['signals', String(overall.signals)],
            ['hitRate', hitRateText(overall)],
            ['baseRate', pct(overall.baseline_up)],
            ['brier', overall.brier.toFixed(4)],
            ['lift', pct(overall.lift)],
            ['afterFees', fmtPct(overall.avg_return_after_fee_pct, true)],
            ['moveCoverage', pct(overall.move_coverage)],
          ] as const
        ).map(([key, value]) => (
          <div key={key} className="min-w-0">
            <dt className="truncate text-xs text-vergex-text-muted">
              {label(key)}
            </dt>
            <dd className="mt-0.5 text-sm font-medium tabular-nums text-vergex-text">
              {value}
            </dd>
          </div>
        ))}
      </dl>
      <div className="grid gap-4 border-t border-[var(--line)] p-4 xl:grid-cols-2">
        <MetricsTable
          title={label('interval')}
          rows={sortedRows(report.by_interval)}
          language={language}
        />
        <MetricsTable
          title={label('regime')}
          rows={sortedRows(report.by_regime).map(([name, metrics]) => [
            label(`regimeName.${name}`),
            metrics,
          ])}
          language={language}
        />
        <MetricsTable
          title={label('coin')}
          rows={sortedRows(report.by_symbol)}
          language={language}
        />
        <div className="min-w-0">
          <table className="w-full text-left text-[13px] tabular-nums">
            <thead className="text-vergex-text-muted">
              <tr className="border-b border-[var(--line)]">
                <th className={headClass}>{label('scoreRange')}</th>
                <th className={cn(headClass, 'text-right')}>
                  {label('samples')}
                </th>
                <th className={cn(headClass, 'text-right')}>
                  {label('averageSaid')}
                </th>
                <th className={cn(headClass, 'text-right')}>
                  {label('actuallyRose')}
                </th>
              </tr>
            </thead>
            <tbody>
              {(report.reliability ?? []).map((bucket) => (
                <tr
                  key={bucket.from}
                  className="border-b border-[var(--line)] last:border-b-0"
                >
                  <td className={cn(cellClass, 'text-vergex-text')}>
                    {Math.round(bucket.from * 100)}–
                    {Math.round(bucket.to * 100)}
                  </td>
                  <td className={cn(cellClass, 'text-right')}>
                    {bucket.samples}
                  </td>
                  <td className={cn(cellClass, 'text-right')}>
                    {pct(bucket.avg_prob)}
                  </td>
                  <td className={cn(cellClass, 'text-right')}>
                    {pct(bucket.up_rate)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      {(report.skipped?.length ?? 0) > 0 && (
        <p className="border-t border-[var(--line)] px-4 py-3 text-xs text-vergex-text-muted">
          {label('skippedSeries', { items: report.skipped!.join('; ') })}
        </p>
      )}
    </Panel>
  )
}
