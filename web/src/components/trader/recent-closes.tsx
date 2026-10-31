import { useEffect, useState } from 'react'
import { api } from '@/api'
import { t, type Language } from '@/i18n/translations'
import type { HistoricalPosition } from '@/types'
import { cn } from '@/utils/cn'
import { Panel, PanelState } from '@/components/common/panel'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { baseLabel, fmtHold, fmtTime, fmtUsd } from '@/utils/trader-format'

const headClass =
  'sticky top-0 bg-[var(--surface)] px-2 py-2 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-2 py-2 whitespace-nowrap'

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
    <Panel title={label('recentCloses')}>
      {!trades ? (
        failed ? (
          <PanelState role="alert">{label('closedTradesFailed')}</PanelState>
        ) : (
          <PanelState role="status">{t('loading', language)}</PanelState>
        )
      ) : rows.length === 0 ? (
        <PanelState role="status">{label('noClosedTrades')}</PanelState>
      ) : (
        <>
          <ScrollArea className="max-h-96 px-2 pb-2">
            <table className="w-full text-left text-[13px] tabular-nums">
              <thead>
                <tr>
                  <th className={headClass}>{label('symbol')}</th>
                  <th className={headClass}>{label('side')}</th>
                  <th className={cn(headClass, 'text-right')}>
                    {label('hold')}
                  </th>
                  <th className={cn(headClass, 'max-sm:hidden')}>
                    {label('closedAt')}
                  </th>
                  <th className={cn(headClass, 'text-right')}>
                    {label('pnl')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((trade) => {
                  const long = /long/i.test(trade.side)
                  return (
                    <tr key={trade.id}>
                      <td
                        className={cn(
                          cellClass,
                          'font-medium text-vergex-text'
                        )}
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
          {hasMore && rows.length >= CLOSES_PAGE_SIZE && (
            <div className="flex items-center justify-end gap-3 border-t border-[var(--line)] px-4 py-2">
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
        </>
      )}
    </Panel>
  )
}
