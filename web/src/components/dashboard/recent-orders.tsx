import { useNavigate } from 'react-router-dom'
import type { Overview } from '@/api/overview'
import { Panel, PanelState } from '@/components/common/panel'
import { baseLabel, fmtPx, fmtTime } from '@/components/trader/trader-format'
import { ScrollArea } from '@/components/ui/scroll-area'
import { t, type Language } from '@/i18n/translations'
import { cn } from '@/utils/cn'
import { traderPath } from './overview-links'

const headClass =
  'sticky top-0 bg-[var(--surface)] px-2 py-2 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-2 py-2 whitespace-nowrap'

// RecentOrders shows the latest orders across traders and Frontier. A row opens
// the page that holds its full history.
export function RecentOrders({
  overview,
  language,
}: {
  overview: Overview
  language: Language
}) {
  const navigate = useNavigate()
  const label = (key: string) => t(`dashboard.${key}`, language)
  return (
    <Panel title={label('recentOrders')}>
      {overview.orders.length === 0 ? (
        <PanelState role="status">{label('noOrders')}</PanelState>
      ) : (
        <ScrollArea className="max-h-80 px-2 pb-2">
          <table className="w-full text-left text-[13px] tabular-nums">
            <thead>
              <tr>
                <th className={headClass}>{label('symbol')}</th>
                <th className={headClass}>{label('status')}</th>
                <th className={cn(headClass, 'text-right')}>
                  {label('price')}
                </th>
                <th className={cn(headClass, 'max-sm:hidden')}>
                  {label('time')}
                </th>
              </tr>
            </thead>
            <tbody>
              {overview.orders.map((order, index) => (
                <tr
                  key={`${order.trader_id ?? 'frontier'}-${order.symbol}-${order.created_at}-${index}`}
                  className="cursor-pointer"
                  onClick={() =>
                    navigate(
                      traderPath(overview.traders, order.trader_id, 'orders')
                    )
                  }
                >
                  <td className={cellClass}>
                    <div className="font-medium text-vergex-text">
                      {baseLabel(order.symbol)}
                    </div>
                    <div className="text-xs text-vergex-text-muted">
                      {order.action}
                    </div>
                  </td>
                  <td className={cn(cellClass, 'text-vergex-text-muted')}>
                    {order.status.toLowerCase()}
                  </td>
                  <td className={cn(cellClass, 'text-right')}>
                    {fmtPx(order.price)}
                  </td>
                  <td
                    className={cn(
                      cellClass,
                      'text-vergex-text-muted max-sm:hidden'
                    )}
                  >
                    {fmtTime(order.created_at) || '—'}
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
