import { t, type Language } from '@/i18n/translations'
import type { HistoricalPosition } from '@/types'
import { cn } from '@/utils/cn'
import { Panel, PanelState } from '@/components/common/panel'
import { ScrollArea } from '@/components/ui/scroll-area'
import { baseLabel, fmtHold, fmtTime, fmtUsd } from './trader-format'

const headClass =
  'sticky top-0 bg-[var(--surface)] px-2 py-2 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-2 py-2 whitespace-nowrap'

// RecentCloses lists the latest closed trades with side, hold time and P/L.
export function RecentCloses({
  trades,
  language,
}: {
  trades?: HistoricalPosition[]
  language: Language
}) {
  const label = (key: string) => t(`traderDetail.${key}`, language)
  return (
    <Panel title={label('recentCloses')}>
      {!trades ? (
        <PanelState role="status">{t('loading', language)}</PanelState>
      ) : trades.length === 0 ? (
        <PanelState role="status">{label('noClosedTrades')}</PanelState>
      ) : (
        <ScrollArea className="max-h-96 px-2 pb-2">
          <table className="w-full text-left text-[13px] tabular-nums">
            <thead>
              <tr>
                <th className={headClass}>{label('symbol')}</th>
                <th className={headClass}>{label('side')}</th>
                <th className={cn(headClass, 'text-right')}>{label('hold')}</th>
                <th className={cn(headClass, 'max-sm:hidden')}>
                  {label('closedAt')}
                </th>
                <th className={cn(headClass, 'text-right')}>{label('pnl')}</th>
              </tr>
            </thead>
            <tbody>
              {trades.map((trade) => {
                const long = /long/i.test(trade.side)
                return (
                  <tr key={trade.id}>
                    <td
                      className={cn(cellClass, 'font-medium text-vergex-text')}
                    >
                      {baseLabel(trade.symbol)}
                    </td>
                    <td
                      className={cn(
                        cellClass,
                        long ? 'text-vergex-success' : 'text-vergex-danger'
                      )}
                    >
                      {long ? label('longSide') : label('shortSide')}
                    </td>
                    <td className={cn(cellClass, 'text-right')}>
                      {fmtHold(trade.entry_time, trade.exit_time)}
                    </td>
                    <td
                      className={cn(
                        cellClass,
                        'text-vergex-text-muted max-sm:hidden'
                      )}
                    >
                      {fmtTime(trade.exit_time) || '—'}
                    </td>
                    <td
                      className={cn(
                        cellClass,
                        'text-right font-medium',
                        trade.realized_pnl >= 0
                          ? 'text-vergex-success'
                          : 'text-vergex-danger'
                      )}
                    >
                      {fmtUsd(trade.realized_pnl, true)}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </ScrollArea>
      )}
    </Panel>
  )
}
