import type { ForecastSignal } from '@/api/forecast'
import { Panel, PanelState } from '@/components/common/panel'
import { ScrollArea } from '@/components/ui/scroll-area'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'
import {
  DirectionLabel,
  fmtPct,
  fmtTime,
  intervalMillis,
  scoreText,
} from './forecast-format'

const headClass =
  'sticky top-0 z-10 bg-vergex-surface px-2 py-2 text-[11px] font-medium whitespace-nowrap'
const cellClass = 'px-2 py-2 whitespace-nowrap'

const outcomeTone = {
  hit: 'text-vergex-success',
  miss: 'text-vergex-danger',
  neutral: 'text-vergex-muted',
}

// This coin's recent forecasts and how each turned out.
export function RecordList({
  signals,
  hits,
  misses,
  language,
  className,
}: {
  signals: ForecastSignal[]
  hits: number
  misses: number
  language: Language
  className?: string
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const judged = hits + misses
  return (
    <Panel
      title={label('record')}
      meta={
        judged > 0
          ? label('hitSummary', {
              hits,
              judged,
              rate: ((hits / judged) * 100).toFixed(1),
            })
          : undefined
      }
      className={className}
    >
      {signals.length === 0 ? (
        <PanelState role="status" className="min-h-0 flex-1">
          {label('noRecords')}
        </PanelState>
      ) : (
        <ScrollArea className="min-h-0 flex-1 rounded-b-2xl px-2 pb-2">
          <table className="w-full text-left text-[13px] tabular-nums">
            <thead className="text-vergex-muted">
              <tr>
                <th className={headClass}>{label('time')}</th>
                <th className={headClass}>{label('call')}</th>
                <th className={cn(headClass, 'text-right')}>
                  {label('score')}
                </th>
                <th className={cn(headClass, 'text-right')}>
                  {label('expectedMove')}
                </th>
                <th className={cn(headClass, 'text-right')}>
                  {label('moved')}
                </th>
                <th className={headClass}>{label('outcome')}</th>
              </tr>
            </thead>
            <tbody>
              {signals.map((signal) => (
                <tr key={signal.id} className="border-t border-vergex-line">
                  <td
                    className={cn(
                      cellClass,
                      'font-mono text-xs text-vergex-muted'
                    )}
                  >
                    {fmtTime(
                      signal.open_time + intervalMillis[signal.interval]
                    )}
                  </td>
                  <td className={cellClass}>
                    <DirectionLabel
                      direction={signal.direction}
                      language={language}
                    />
                  </td>
                  <td className={cn(cellClass, 'text-right')}>
                    {scoreText(signal.display_prob, signal.calibrated)}
                  </td>
                  <td
                    className={cn(
                      cellClass,
                      'text-right font-mono text-xs text-vergex-muted'
                    )}
                  >
                    ±{fmtPct(signal.expected_move_pct)}
                  </td>
                  <td className={cn(cellClass, 'text-right font-mono text-xs')}>
                    {fmtPct(signal.actual_move_pct ?? 0, true)}
                  </td>
                  <td
                    className={cn(
                      cellClass,
                      signal.outcome
                        ? outcomeTone[signal.outcome]
                        : 'text-vergex-muted'
                    )}
                  >
                    {label(`outcomeName.${signal.outcome ?? signal.status}`)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </ScrollArea>
      )}
    </Panel>
  )
}
