import { Fragment, useState } from 'react'
import useSWR from 'swr'
import { ChevronRight, Download } from 'lucide-react'
import { api } from '@/api'
import { t, type Language } from '@/i18n/translations'
import type { TraderOrder } from '@/types'
import { Panel, PanelState } from '@/components/common/panel'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { downloadCsv, ordersToCsv } from './orders-csv'
import { baseLabel, fmtPx, fmtTime } from './trader-format'

const PAGE_SIZE = 100
const headClass =
  'sticky top-0 bg-[var(--surface)] px-2 py-2 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-2 py-2 whitespace-nowrap'

function OrderFills({
  traderId,
  orderId,
  language,
}: {
  traderId: string
  orderId: number
  language: Language
}) {
  const label = (key: string) => t(`traderDetail.${key}`, language)
  const { data, error } = useSWR(
    ['order-fills', traderId, orderId],
    () => api.getOrderFills(traderId, orderId, true),
    { revalidateOnFocus: false, shouldRetryOnError: false }
  )
  if (error)
    return <p className="text-xs text-vergex-danger">{label('fillsFailed')}</p>
  if (!data)
    return (
      <p className="text-xs text-vergex-text-muted">{t('loading', language)}</p>
    )
  if (data.length === 0)
    return <p className="text-xs text-vergex-text-muted">{label('noFills')}</p>
  return (
    <ul className="flex flex-col gap-1 text-xs tabular-nums text-vergex-text">
      {data.map((fill) => (
        <li key={fill.id} className="flex flex-wrap gap-x-4">
          <span className="text-vergex-text-muted">
            {fmtTime(fill.created_at)}
          </span>
          <span>
            {fill.quantity} @ {fmtPx(fill.price)}
          </span>
          <span className="text-vergex-text-muted">
            {label('fee')} {fill.commission} {fill.commission_asset}
          </span>
          {fill.realized_pnl !== 0 && (
            <span
              className={
                fill.realized_pnl > 0
                  ? 'text-vergex-success'
                  : 'text-vergex-danger'
              }
            >
              {fill.realized_pnl.toFixed(2)}
            </span>
          )}
        </li>
      ))}
    </ul>
  )
}

// TraderOrders is the trader's recorded orders, newest first. A row opens its
// fills; the export covers what is loaded.
export function TraderOrders({
  traderId,
  traderName,
  language,
}: {
  traderId: string
  traderName: string
  language: Language
}) {
  const label = (key: string) => t(`traderDetail.${key}`, language)
  const [limit, setLimit] = useState(PAGE_SIZE)
  const [open, setOpen] = useState<number | null>(null)
  const { data: orders, error } = useSWR<TraderOrder[]>(
    ['orders', traderId, limit],
    () => api.getOrders(traderId, limit, true),
    {
      refreshInterval: 30000,
      shouldRetryOnError: false,
      keepPreviousData: true,
    }
  )
  return (
    <Panel
      title={label('orders')}
      actions={
        orders && orders.length > 0 ? (
          <Button
            type="button"
            variant="secondary"
            size="sm"
            className="max-sm:h-10"
            onClick={() =>
              downloadCsv(`${traderName}-orders.csv`, ordersToCsv(orders))
            }
          >
            <Download size={14} aria-hidden="true" />
            {label('exportCsv')}
          </Button>
        ) : undefined
      }
    >
      {!orders ? (
        error ? (
          <PanelState role="alert">{label('ordersFailed')}</PanelState>
        ) : (
          <PanelState role="status">{t('loading', language)}</PanelState>
        )
      ) : orders.length === 0 ? (
        <PanelState role="status">{label('noOrders')}</PanelState>
      ) : (
        <>
          <ScrollArea className="max-h-[640px] px-2 pb-2">
            <table className="w-full text-left text-[13px] tabular-nums">
              <thead>
                <tr>
                  <th className={headClass}>{label('symbol')}</th>
                  <th className={headClass}>{label('action')}</th>
                  <th className={cn(headClass, 'max-sm:hidden')}>
                    {label('type')}
                  </th>
                  <th className={cn(headClass, 'text-right max-sm:hidden')}>
                    {label('filledQty')}
                  </th>
                  <th className={cn(headClass, 'text-right')}>
                    {label('price')}
                  </th>
                  <th className={headClass}>{label('status')}</th>
                  <th className={cn(headClass, 'max-sm:hidden')}>
                    {label('time')}
                  </th>
                  <th className={headClass}>
                    <span className="sr-only">{label('fills')}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {orders.map((order) => {
                  const expanded = open === order.id
                  return (
                    <Fragment key={order.id}>
                      <tr>
                        <td
                          className={cn(
                            cellClass,
                            'font-medium text-vergex-text'
                          )}
                        >
                          {baseLabel(order.symbol)}
                        </td>
                        <td className={cellClass}>
                          {(order.order_action || order.side).toLowerCase()}
                        </td>
                        <td className={cn(cellClass, 'max-sm:hidden')}>
                          {order.type.toLowerCase()}
                        </td>
                        <td
                          className={cn(cellClass, 'text-right max-sm:hidden')}
                        >
                          {order.filled_quantity} / {order.quantity}
                        </td>
                        <td className={cn(cellClass, 'text-right')}>
                          {fmtPx(order.avg_fill_price || order.price)}
                        </td>
                        <td className={cellClass}>
                          {order.status.toLowerCase()}
                        </td>
                        <td
                          className={cn(
                            cellClass,
                            'text-vergex-text-muted max-sm:hidden'
                          )}
                        >
                          {fmtTime(order.created_at) || '—'}
                        </td>
                        <td className={cn(cellClass, 'w-px text-right')}>
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            className="max-sm:size-10"
                            aria-expanded={expanded}
                            aria-label={label('fills')}
                            onClick={() => setOpen(expanded ? null : order.id)}
                          >
                            <ChevronRight
                              className={cn(
                                'size-4 transition-transform duration-(--dur-state) ease-(--ease-move)',
                                expanded && 'rotate-90'
                              )}
                              aria-hidden="true"
                            />
                          </Button>
                        </td>
                      </tr>
                      {expanded && (
                        <tr>
                          <td colSpan={8} className="px-2 pb-3">
                            <OrderFills
                              traderId={traderId}
                              orderId={order.id}
                              language={language}
                            />
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  )
                })}
              </tbody>
            </table>
          </ScrollArea>
          {orders.length >= limit && (
            <div className="flex justify-end border-t border-[var(--line)] px-4 py-2">
              <Button
                type="button"
                variant="secondary"
                size="sm"
                className="max-sm:h-10"
                onClick={() => setLimit(limit + PAGE_SIZE)}
              >
                {label('loadOlder')}
              </Button>
            </div>
          )}
        </>
      )}
    </Panel>
  )
}
