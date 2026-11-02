import { Fragment, useState } from 'react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ChevronRight, Download } from 'lucide-react'
import { api } from '@/api'
import { t, type Language } from '@/i18n'
import {
  cellClass,
  EmptyLine,
  headClass,
  numberClass,
  wideTableClass,
} from '@/components/common/figures'
import { PanelLoading, PanelState } from '@/components/common/panel'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/utils/cn'
import { pollEvery } from '@/utils/poll'
import { downloadCsv, ordersToCsv } from '@/utils/orders-csv'
import { baseLabel, fmtPx, fmtTime } from '@/utils/trader-format'

const PAGE_SIZE = 100

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
  const { data, error } = useQuery({
    queryKey: ['order-fills', traderId, orderId],
    queryFn: () => api.getOrderFills(traderId, orderId, true),
    refetchOnWindowFocus: false,
    retry: false,
  })
  if (error)
    return <p className="text-xs text-vergex-danger">{label('fillsFailed')}</p>
  if (!data) return <Skeleton className="h-3 w-40" />
  if (data.length === 0)
    return <p className="text-xs text-vergex-muted">{label('noFills')}</p>
  return (
    <ul className="flex flex-col gap-1 text-xs tabular-nums text-vergex-ink">
      {data.map((fill) => (
        <li key={fill.id} className="flex flex-wrap gap-x-4">
          <span className="text-vergex-muted">{fmtTime(fill.created_at)}</span>
          <span>
            {fill.quantity} @ {fmtPx(fill.price)}
          </span>
          <span className="text-vergex-muted">
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
  const { data: orders, error } = useQuery({
    queryKey: ['orders', traderId, limit],
    queryFn: () => api.getOrders(traderId, limit, true),
    ...pollEvery(30000),
    placeholderData: keepPreviousData,
  })
  if (!orders) {
    return error ? (
      <PanelState role="alert">{label('ordersFailed')}</PanelState>
    ) : (
      <PanelLoading />
    )
  }
  return (
    <>
      <table className={wideTableClass}>
        <thead className="sticky top-0 z-10">
          <tr>
            <th className={headClass}>{label('ordersHead')}</th>
            <th className={cn(headClass, 'w-24')}>{label('action')}</th>
            <th className={cn(headClass, 'w-16 max-xl:hidden')}>
              {label('type')}
            </th>
            <th className={cn(headClass, 'w-24 text-right max-sm:hidden')}>
              {label('filledQty')}
            </th>
            <th className={cn(headClass, 'w-20 text-right')}>
              {label('price')}
            </th>
            <th className={cn(headClass, 'w-16')}>{label('status')}</th>
            <th className={cn(headClass, 'w-32 max-md:hidden')}>
              {label('time')}
            </th>
            <th className={cn(headClass, 'w-20 text-right max-sm:w-12')}>
              {orders.length > 0 ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-6 px-1.5 text-[11px]"
                  title={label('exportCsv')}
                  onClick={() =>
                    downloadCsv(`${traderName}-orders.csv`, ordersToCsv(orders))
                  }
                >
                  <Download size={12} aria-hidden="true" />
                  <span className="max-xl:sr-only">{label('exportCsv')}</span>
                </Button>
              ) : (
                <span className="sr-only">{label('fills')}</span>
              )}
            </th>
          </tr>
        </thead>
        <tbody>
          {orders.map((order) => {
            const expanded = open === order.id
            return (
              <Fragment key={order.id}>
                <tr>
                  <td className={cn(cellClass, 'font-medium text-vergex-ink')}>
                    {baseLabel(order.symbol)}
                  </td>
                  <td className={cn(cellClass, 'truncate text-xs')}>
                    {(order.order_action || order.side).toLowerCase()}
                  </td>
                  <td className={cn(cellClass, 'text-xs max-xl:hidden')}>
                    {order.type.toLowerCase() || '—'}
                  </td>
                  <td className={cn(cellClass, numberClass, 'max-sm:hidden')}>
                    {order.filled_quantity} / {order.quantity}
                  </td>
                  <td className={cn(cellClass, numberClass)}>
                    {fmtPx(order.avg_fill_price || order.price)}
                  </td>
                  <td className={cn(cellClass, 'truncate text-xs')}>
                    {order.status.toLowerCase()}
                  </td>
                  <td
                    className={cn(
                      cellClass,
                      'mono text-xs text-vergex-muted max-md:hidden'
                    )}
                  >
                    {fmtTime(order.created_at) || '—'}
                  </td>
                  <td className={cn(cellClass, 'text-right')}>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-7 max-sm:size-10"
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
                    <td
                      colSpan={8}
                      className="border-b border-vergex-line bg-vergex-surface-muted px-4 py-2"
                    >
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
      {orders.length === 0 && (
        <EmptyLine role="status">{label('noOrders')}</EmptyLine>
      )}
      {orders.length >= limit && (
        <div className="flex justify-end px-4 py-2">
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
  )
}
