import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Download } from 'lucide-react'
import { api } from '@/api'
import {
  cellClass,
  headClass,
  numberClass,
  tableClass,
} from '@/components/common/figures'
import { PanelLoading, PanelState } from '@/components/common/panel'
import { Segmented } from '@/components/common/segmented'
import { SettingBack, SettingList } from '@/components/common/setting-row'
import { Button } from '@/components/ui/button'
import { t, type Language } from '@/i18n'
import type { Account } from '@/types'
import { cn } from '@/utils/cn'
import { accountOrdersToCsv, downloadCsv, fillsToCsv } from '@/utils/orders-csv'
import { baseLabel, fmtPx, fmtTime, fmtUsd } from '@/utils/trader-format'

// How many of the newest orders and fills it reads.
const LEDGER_ROWS = 500
const SOURCES = ['trader', 'futures', 'manual', 'external']

type View = 'orders' | 'fills'

// AccountLedger is everything the ledger knows of an account, whoever placed it: what came
// from traders and Futures, and manual orders and ones placed on the exchange by hand, which
// nothing else lists.
export function AccountLedger({
  account,
  language,
  onBack,
}: {
  account: Account
  language: Language
  onBack: () => void
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`connections.ledger.${key}`, language, params)
  const [view, setView] = useState<View>('orders')
  const orders = useQuery({
    queryKey: ['account-orders', account.id],
    queryFn: () => api.getAccountOrders(account.id, LEDGER_ROWS, true),
  })
  const fills = useQuery({
    queryKey: ['account-fills', account.id],
    queryFn: () => api.getAccountFills(account.id, LEDGER_ROWS, true),
  })
  const source = (name?: string) =>
    SOURCES.includes(name ?? '') ? label(`source.${name}`) : (name ?? '—')
  const summary = SOURCES.map((name) => ({
    name,
    orders: (orders.data ?? []).filter((row) => row.source === name).length,
    fills: (fills.data ?? []).filter((row) => row.source === name),
  })).filter((row) => row.orders > 0 || row.fills.length > 0)
  const current = view === 'orders' ? orders : fills
  const rows = current.data ?? []
  const name = account.name || account.venue_name

  return (
    <SettingList
      head={
        <SettingBack
          onBack={onBack}
          backLabel={t('back', language)}
          title={name}
        >
          <Segmented
            aria-label={label('title')}
            value={view}
            onChange={setView}
            options={[
              { value: 'orders' as const, label: label('orders') },
              { value: 'fills' as const, label: label('fills') },
            ]}
          />
        </SettingBack>
      }
      message={
        rows.length >= LEDGER_ROWS
          ? label('newest', { count: LEDGER_ROWS })
          : undefined
      }
      messageTone="muted"
      actions={
        <Button
          type="button"
          variant="secondary"
          size="sm"
          onClick={() =>
            downloadCsv(
              `${name}-${view}.csv`,
              view === 'orders'
                ? accountOrdersToCsv(orders.data ?? [])
                : fillsToCsv(fills.data ?? [])
            )
          }
        >
          <Download className="size-4" />
          {label('export')}
        </Button>
      }
    >
      <div className="flex flex-1 flex-col gap-4 py-3">
        {summary.length > 0 && (
          <table className={tableClass}>
            <thead>
              <tr>
                <th className={headClass}>{label('source.head')}</th>
                <th className={cn(headClass, 'w-20 text-right')}>
                  {label('orders')}
                </th>
                <th className={cn(headClass, 'w-20 text-right')}>
                  {label('fills')}
                </th>
                <th className={cn(headClass, 'w-28 text-right')}>
                  {label('realized')}
                </th>
              </tr>
            </thead>
            <tbody>
              {summary.map((row) => (
                <tr key={row.name}>
                  <td className={cn(cellClass, 'text-xs text-vergex-ink')}>
                    {source(row.name)}
                  </td>
                  <td className={cn(cellClass, numberClass)}>{row.orders}</td>
                  <td className={cn(cellClass, numberClass)}>
                    {row.fills.length}
                  </td>
                  <td className={cn(cellClass, numberClass)}>
                    {fmtUsd(
                      row.fills.reduce(
                        (sum, fill) => sum + fill.realized_pnl,
                        0
                      ),
                      true
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {current.error ? (
          <PanelState role="alert" className="flex-1">
            {label('failed')}
          </PanelState>
        ) : !current.data ? (
          <PanelLoading className="flex-1" />
        ) : rows.length === 0 ? (
          <PanelState role="status" className="flex-1">
            {label('empty')}
          </PanelState>
        ) : view === 'orders' ? (
          <table className={tableClass}>
            <thead>
              <tr>
                <th className={headClass}>{label('symbol')}</th>
                <th className={cn(headClass, 'w-20')}>
                  {label('source.head')}
                </th>
                <th className={cn(headClass, 'w-24 max-sm:hidden')}>
                  {label('action')}
                </th>
                <th className={cn(headClass, 'w-24 text-right')}>
                  {label('price')}
                </th>
                <th className={cn(headClass, 'w-20')}>{label('status')}</th>
                <th className={cn(headClass, 'w-28 max-sm:hidden')}>
                  {label('time')}
                </th>
              </tr>
            </thead>
            <tbody>
              {(orders.data ?? []).map((order) => (
                <tr key={order.id}>
                  <td className={cn(cellClass, 'font-medium text-vergex-ink')}>
                    {baseLabel(order.symbol)}
                  </td>
                  <td className={cn(cellClass, 'truncate text-xs')}>
                    {source(order.source)}
                  </td>
                  <td
                    className={cn(cellClass, 'truncate text-xs max-sm:hidden')}
                  >
                    {(order.order_action || order.side).toLowerCase()}
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
                      'mono text-xs text-vergex-muted max-sm:hidden'
                    )}
                  >
                    {fmtTime(order.created_at) || '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <table className={tableClass}>
            <thead>
              <tr>
                <th className={headClass}>{label('symbol')}</th>
                <th className={cn(headClass, 'w-20')}>
                  {label('source.head')}
                </th>
                <th className={cn(headClass, 'w-24 text-right max-sm:hidden')}>
                  {label('quantity')}
                </th>
                <th className={cn(headClass, 'w-24 text-right')}>
                  {label('price')}
                </th>
                <th className={cn(headClass, 'w-24 text-right')}>
                  {label('realized')}
                </th>
                <th className={cn(headClass, 'w-28 max-sm:hidden')}>
                  {label('time')}
                </th>
              </tr>
            </thead>
            <tbody>
              {(fills.data ?? []).map((fill) => (
                <tr key={fill.id}>
                  <td className={cn(cellClass, 'font-medium text-vergex-ink')}>
                    {baseLabel(fill.symbol)}
                  </td>
                  <td className={cn(cellClass, 'truncate text-xs')}>
                    {source(fill.source)}
                  </td>
                  <td className={cn(cellClass, numberClass, 'max-sm:hidden')}>
                    {fill.quantity}
                  </td>
                  <td className={cn(cellClass, numberClass)}>
                    {fmtPx(fill.price)}
                  </td>
                  <td className={cn(cellClass, numberClass)}>
                    {fmtUsd(fill.realized_pnl, true)}
                  </td>
                  <td
                    className={cn(
                      cellClass,
                      'mono text-xs text-vergex-muted max-sm:hidden'
                    )}
                  >
                    {fmtTime(fill.created_at) || '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </SettingList>
  )
}
