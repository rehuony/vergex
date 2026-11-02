import { useEffect, useState } from 'react'
import { api } from '@/api'
import { t, type Language } from '@/i18n'
import type { HistoricalPosition } from '@/types'
import { cn } from '@/utils/cn'
import {
  cellClass,
  EmptyLine,
  gainTone,
  headClass,
  numberClass,
  wideTableClass,
} from '@/components/common/figures'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { baseLabel, fmtHold, fmtTime, fmtUsd } from '@/utils/trader-format'

/** How many closed trades the page reads first, and each older page after that. */
export const CLOSES_PAGE_SIZE = 50

// RecentCloses lists closed trades with side, hold time and P/L, newest first. The
// latest come from the page's own refresh; older ones load on request.
export function RecentCloses({
  traderId,
  trades,
  failed,
  language,
}: {
  traderId: string
  trades?: HistoricalPosition[]
  failed: boolean
  language: Language
}) {
  const label = (key: string) => t(`traderDetail.${key}`, language)
  const [older, setOlder] = useState<HistoricalPosition[]>([])
  const [hasMore, setHasMore] = useState(true)
  const [loadingOlder, setLoadingOlder] = useState(false)
  const [olderFailed, setOlderFailed] = useState(false)

  useEffect(() => {
    setOlder([])
    setHasMore(true)
  }, [traderId])

  // A refresh can bring a page's first rows into the latest ones.
  const seen = new Set<number>()
  const rows = [...(trades ?? []), ...older].filter(
    (trade) => !seen.has(trade.id) && seen.add(trade.id)
  )

  async function loadOlder() {
    const last = rows[rows.length - 1]
    if (!last || loadingOlder) return
    setLoadingOlder(true)
    setOlderFailed(false)
    try {
      const page = await api.getOlderPositions(
        traderId,
        last.id,
        CLOSES_PAGE_SIZE
      )
      setOlder((current) => [...current, ...page])
      setHasMore(page.length === CLOSES_PAGE_SIZE)
    } catch {
      setOlderFailed(true)
    } finally {
      setLoadingOlder(false)
    }
  }

  return (
    <div className="mt-4">
      <table className={wideTableClass}>
        <thead>
          <tr>
            <th className={cn(headClass, 'border-t')}>
              {label('closedTradesHead')}
            </th>
            <th className={cn(headClass, 'w-20 border-t')}>{label('side')}</th>
            <th className={cn(headClass, 'w-24 border-t text-right')}>
              {label('hold')}
            </th>
            <th className={cn(headClass, 'w-40 border-t max-sm:hidden')}>
              {label('closedAt')}
            </th>
            <th className={cn(headClass, 'w-28 border-t text-right')}>
              {label('pnl')}
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map((trade) => {
            const long = /long/i.test(trade.side)
            return (
              <tr key={trade.id}>
                <td className={cn(cellClass, 'font-medium text-vergex-ink')}>
                  {baseLabel(trade.symbol)}
                </td>
                <td
                  className={cn(
                    cellClass,
                    'text-xs',
                    long ? 'text-vergex-success' : 'text-vergex-danger'
                  )}
                >
                  {long ? label('longSide') : label('shortSide')}
                </td>
                <td className={cn(cellClass, numberClass)}>
                  {fmtHold(trade.entry_time, trade.exit_time)}
                </td>
                <td
                  className={cn(
                    cellClass,
                    'mono text-xs text-vergex-muted max-sm:hidden'
                  )}
                >
                  {fmtTime(trade.exit_time) || '—'}
                </td>
                <td
                  className={cn(
                    cellClass,
                    numberClass,
                    gainTone(trade.realized_pnl)
                  )}
                >
                  {fmtUsd(trade.realized_pnl, true)}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      {!trades ? (
        <EmptyLine role={failed ? 'alert' : 'status'}>
          {failed ? label('closedTradesFailed') : <Skeleton className="w-40" />}
        </EmptyLine>
      ) : (
        rows.length === 0 && (
          <EmptyLine role="status">{label('noClosedTrades')}</EmptyLine>
        )
      )}
      {hasMore && rows.length >= CLOSES_PAGE_SIZE && (
        <div className="flex items-center justify-end gap-3 px-4 py-2">
          {olderFailed && (
            <span role="alert" className="text-xs text-vergex-danger">
              {label('closedTradesFailed')}
            </span>
          )}
          <Button
            type="button"
            variant="secondary"
            size="sm"
            className="max-sm:h-10"
            disabled={loadingOlder}
            onClick={() => void loadOlder()}
          >
            {loadingOlder ? t('loading', language) : label('loadOlder')}
          </Button>
        </div>
      )}
    </div>
  )
}
