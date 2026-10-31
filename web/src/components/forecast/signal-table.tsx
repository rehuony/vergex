import { useNavigate } from 'react-router-dom'
import type { ForecastInterval, ForecastSignal } from '@/api/forecast'
import { Panel, PanelState } from '@/components/common/panel'
import { buildForecastPath } from '@/constants/route-paths'
import { t, type Language } from '@/i18n/translations'
import { cn } from '@/utils/cn'
import {
  DirectionLabel,
  baseAsset,
  fmtPct,
  fmtPrice,
  fmtTime,
  scoreText,
} from './forecast-format'

const headClass =
  'bg-[var(--surface)] px-3 py-2 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-3 py-3 whitespace-nowrap'

// Strong calls first, in either direction; coins with no forecast yet last.
export function sortSignals(signals: ForecastSignal[]) {
  const strength = (signal: ForecastSignal) =>
    signal.record ? Math.abs(signal.record.display_prob - 0.5) : -1
  return [...signals].sort(
    (a, b) =>
      strength(b) - strength(a) ||
      a.tier - b.tier ||
      a.symbol.localeCompare(b.symbol)
  )
}

// The signal table. A phone shows the coin, price, direction and score at a
// glance; a row opens the coin.
export function SignalTable({
  signals,
  interval,
  language,
}: {
  signals: ForecastSignal[]
  interval: ForecastInterval
  language: Language
}) {
  const navigate = useNavigate()
  const label = (key: string) => t(`forecast.${key}`, language)
  const open = (symbol: string) => navigate(buildForecastPath(symbol, interval))
  return (
    <Panel>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-[13px] tabular-nums">
          <thead className="text-vergex-text-muted">
            <tr className="border-b border-[var(--line)]">
              <th className={headClass}>{label('coin')}</th>
              <th className={cn(headClass, 'text-right')}>{label('price')}</th>
              <th className={headClass}>{label('call')}</th>
              <th className={cn(headClass, 'text-right')}>{label('score')}</th>
              <th className={cn(headClass, 'text-right max-md:hidden')}>
                {label('confidence')}
              </th>
              <th className={cn(headClass, 'text-right max-md:hidden')}>
                {label('expectedMove')}
              </th>
              <th className={cn(headClass, 'max-lg:hidden')}>
                {label('regime')}
              </th>
              <th className={cn(headClass, 'text-right max-lg:hidden')}>
                {label('updated')}
              </th>
            </tr>
          </thead>
          <tbody>
            {signals.map((signal) => {
              const record = signal.record
              return (
                <tr
                  key={signal.symbol}
                  className="cursor-pointer border-b border-[var(--line)] last:border-b-0 hover:bg-vergex-bg"
                  onClick={() => open(signal.symbol)}
                >
                  <td className={cn(cellClass, 'font-medium text-vergex-text')}>
                    <button
                      type="button"
                      className="font-medium"
                      onClick={(event) => {
                        event.stopPropagation()
                        open(signal.symbol)
                      }}
                    >
                      {baseAsset(signal.symbol)}
                    </button>
                  </td>
                  <td className={cn(cellClass, 'text-right')}>
                    {fmtPrice(signal.price)}
                  </td>
                  {record ? (
                    <>
                      <td className={cellClass}>
                        <DirectionLabel
                          direction={record.direction}
                          language={language}
                        />
                      </td>
                      <td
                        className={cn(
                          cellClass,
                          'text-right font-medium',
                          signal.stale && 'text-vergex-text-muted'
                        )}
                        title={signal.stale ? label('stale') : undefined}
                      >
                        {scoreText(record.display_prob, record.calibrated)}
                      </td>
                      <td
                        className={cn(
                          cellClass,
                          'text-right text-vergex-text-muted max-md:hidden'
                        )}
                      >
                        {record.confidence}
                      </td>
                      <td
                        className={cn(
                          cellClass,
                          'text-right text-vergex-text-muted max-md:hidden'
                        )}
                      >
                        ±{fmtPct(record.expected_move_pct)}
                      </td>
                      <td
                        className={cn(
                          cellClass,
                          'text-vergex-text-muted max-lg:hidden'
                        )}
                      >
                        {label(`regimeName.${record.regime}`)}
                      </td>
                      <td
                        className={cn(
                          cellClass,
                          'text-right text-vergex-text-muted max-lg:hidden',
                          signal.stale && 'text-vergex-danger'
                        )}
                      >
                        {signal.stale
                          ? label('stale')
                          : fmtTime(record.open_time)}
                      </td>
                    </>
                  ) : (
                    <td
                      colSpan={6}
                      className={cn(cellClass, 'text-vergex-text-muted')}
                    >
                      {label(`pendingState.${signal.pending ?? 'waiting'}`)}
                    </td>
                  )}
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </Panel>
  )
}

export function SignalsEmpty({ children }: { children: React.ReactNode }) {
  return (
    <Panel>
      <PanelState role="status">{children}</PanelState>
    </Panel>
  )
}
