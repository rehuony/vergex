import type { ForecastRecord } from '@/api/forecast'
import { Panel, PanelState } from '@/components/common/panel'
import { ScrollArea } from '@/components/ui/scroll-area'
import { t, type Language } from '@/i18n/translations'
import { cn } from '@/utils/cn'
import { DirectionLabel, fmtPct, fmtTime, scoreText } from './forecast-format'

const headClass =
  'sticky top-0 bg-[var(--surface)] px-2 py-2 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-2 py-2 whitespace-nowrap'

const outcomeTone = {
  hit: 'text-vergex-success',
  miss: 'text-vergex-danger',
  neutral: 'text-vergex-text-muted',
}

// This coin's recent forecasts and how each turned out.
export function RecordList({
  records,
  hits,
  misses,
  language,
}: {
  records: ForecastRecord[]
  hits: number
  misses: number
  language: Language
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
    >
      {records.length === 0 ? (
        <PanelState role="status">{label('noRecords')}</PanelState>
      ) : (
        <ScrollArea className="max-h-80 px-2 pb-2">
          <table className="w-full text-left text-[13px] tabular-nums">
            <thead className="text-vergex-text-muted">
              <tr>
                <th className={headClass}>{label('time')}</th>
                <th className={headClass}>{label('call')}</th>
                <th className={cn(headClass, 'text-right')}>
                  {label('score')}
                </th>
                <th className={cn(headClass, 'text-right')}>
                  {label('moved')}
                </th>
                <th className={headClass}>{label('outcome')}</th>
              </tr>
            </thead>
            <tbody>
              {records.map((record) => (
                <tr key={record.id} className="border-t border-[var(--line)]">
                  <td className={cn(cellClass, 'text-vergex-text-muted')}>
                    {fmtTime(record.open_time)}
                  </td>
                  <td className={cellClass}>
                    <DirectionLabel
                      direction={record.direction}
                      language={language}
                    />
                  </td>
                  <td className={cn(cellClass, 'text-right')}>
                    {scoreText(record.display_prob, record.calibrated)}
                  </td>
                  <td className={cn(cellClass, 'text-right')}>
                    {record.status === 'verified'
                      ? fmtPct(record.actual_move_pct ?? 0, true)
                      : '—'}
                  </td>
                  <td
                    className={cn(
                      cellClass,
                      record.outcome
                        ? outcomeTone[record.outcome]
                        : 'text-vergex-text-muted'
                    )}
                  >
                    {label(`outcomeName.${record.outcome ?? record.status}`)}
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
