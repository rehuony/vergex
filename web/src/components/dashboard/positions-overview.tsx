import { useNavigate } from 'react-router-dom'
import type { Overview } from '@/api/dashboard'
import { PanelState } from '@/components/common/panel'
import { baseLabel, fmtUsd } from '@/utils/trader-format'
import { ScrollArea } from '@/components/ui/scroll-area'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'
import { traderPath } from './overview-links'

const headClass =
  'sticky top-0 bg-vergex-surface px-2 py-2 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-2 py-2 whitespace-nowrap'

// PositionsOverview lists every open position across accounts, filling what holds it. It
// is read-only: a row opens the page that manages that position, where it can be closed.
export function PositionsOverview({
  overview,
  language,
}: {
  overview: Overview
  language: Language
}) {
  const navigate = useNavigate()
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`dashboard.${key}`, language, params)
  const { positions, positions_partial: partial } = overview
  const holder = (traderId?: string) =>
    overview.traders.find((trader) => trader.trader_id === traderId)
      ?.trader_name ?? t('futuresNav', language)
  return (
    <div className="flex h-full min-h-0 flex-col">
      {positions.length > 0 && partial && (
        <p className="shrink-0 px-4 py-2 text-xs text-vergex-warning">
          {label('someAccountsUnread')}
        </p>
      )}
      {positions.length === 0 ? (
        <PanelState role={partial ? 'alert' : 'status'} className="flex-1">
          {partial ? label('positionsUnread') : label('noPositions')}
        </PanelState>
      ) : (
        <ScrollArea className="min-h-0 flex-1 px-2 pb-2">
          <table className="w-full text-left text-[13px] tabular-nums">
            <thead>
              <tr>
                <th className={headClass}>{label('symbol')}</th>
                <th className={cn(headClass, 'max-sm:hidden')}>
                  {label('heldBy')}
                </th>
                <th className={cn(headClass, 'text-right max-sm:hidden')}>
                  {label('size')}
                </th>
                <th className={cn(headClass, 'text-right')}>{label('pnl')}</th>
              </tr>
            </thead>
            <tbody>
              {positions.map((position) => {
                const win = position.unrealized_pnl >= 0
                const tone = win ? 'text-vergex-success' : 'text-vergex-danger'
                const path = traderPath(
                  overview.traders,
                  position.trader_id,
                  'positions'
                )
                return (
                  <tr
                    key={`${position.account_id}-${position.symbol}-${position.side}`}
                    className="cursor-pointer"
                    onClick={() => navigate(path)}
                  >
                    <td className={cellClass}>
                      <div className="font-medium text-vergex-ink">
                        {baseLabel(position.symbol)}
                      </div>
                      <div
                        className={cn(
                          'text-xs',
                          position.side === 'long'
                            ? 'text-vergex-success'
                            : 'text-vergex-danger'
                        )}
                      >
                        {label(position.side)} {position.leverage}×
                      </div>
                    </td>
                    <td
                      className={cn(
                        cellClass,
                        'max-w-40 truncate text-vergex-muted max-sm:hidden'
                      )}
                    >
                      {holder(position.trader_id)}
                    </td>
                    <td className={cn(cellClass, 'text-right max-sm:hidden')}>
                      {fmtUsd(
                        position.quantity *
                          (position.mark_price || position.entry_price)
                      )}
                    </td>
                    <td className={cn(cellClass, 'text-right', tone)}>
                      <div className="font-medium">
                        {fmtUsd(position.unrealized_pnl, true)}
                      </div>
                      <div className="text-xs">
                        {position.unrealized_pnl_pct.toFixed(2)}%
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </ScrollArea>
      )}
    </div>
  )
}
