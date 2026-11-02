import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api } from '@/api'
import { t, type Language } from '@/i18n'
import type { Position } from '@/types'
import { cn } from '@/utils/cn'
import {
  cellClass,
  EmptyLine,
  gainTone,
  headClass,
  numberClass,
  wideTableClass,
} from '@/components/common/figures'
import { confirmToast, notify } from '@/components/common/notify'
import { PanelState } from '@/components/common/panel'
import { Button } from '@/components/ui/button'
import { baseLabel, fmtPx, fmtUsd } from '@/utils/trader-format'

function sideOf(position: Position): 'LONG' | 'SHORT' {
  return /long|buy/i.test(position.side) ? 'LONG' : 'SHORT'
}

// OpenPositions is the trader's live book. Closing goes through the trader's
// close endpoint, one position at a time.
export function OpenPositions({
  traderId,
  positions,
  failed,
  language,
}: {
  traderId: string
  positions?: Position[]
  failed?: boolean
  language: Language
}) {
  const client = useQueryClient()
  const [closing, setClosing] = useState<string | null>(null)
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`traderDetail.${key}`, language, params)

  async function refresh() {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['trader-positions', traderId] }),
      client.invalidateQueries({ queryKey: ['trader-account', traderId] }),
    ])
  }

  async function closeOne(symbol: string, side: 'LONG' | 'SHORT') {
    if (closing) return
    const ok = await confirmToast(label('closeConfirm', { symbol, side }), {
      title: label('closePosition'),
      okText: label('close'),
      cancelText: t('cancel', language),
      tone: 'danger',
    })
    if (!ok) return
    setClosing(symbol)
    try {
      await api.closePosition(traderId, symbol, side)
      notify.success(label('closed', { symbol, side }))
      await refresh()
    } catch (err) {
      notify.error(err instanceof Error ? err.message : label('closeFailed'))
    } finally {
      setClosing(null)
    }
  }

  async function closeAll(open: Position[]) {
    if (closing || open.length === 0) return
    const ok = await confirmToast(
      label('closeAllConfirm', { count: open.length }),
      {
        title: label('closeAll'),
        okText: label('closeAll'),
        cancelText: t('cancel', language),
        tone: 'danger',
      }
    )
    if (!ok) return
    setClosing('__all__')
    let failures = 0
    // Sequential: parallel closes race on exchange nonces and rate limits.
    for (const position of open) {
      try {
        await api.closePosition(traderId, position.symbol, sideOf(position))
      } catch {
        failures++
      }
    }
    await refresh()
    if (failures === 0) notify.success(label('allClosed'))
    else
      notify.error(
        label('closesFailed', { failed: failures, total: open.length })
      )
    setClosing(null)
  }

  if (!positions) {
    return (
      <PanelState role={failed ? 'alert' : 'status'}>
        {failed ? label('positionsFailed') : t('loading', language)}
      </PanelState>
    )
  }

  return (
    <>
      <table className={wideTableClass}>
        <thead className="sticky top-0 z-10">
          <tr>
            <th className={headClass}>
              {label('openPositionsCount', { count: positions.length })}
            </th>
            <th className={cn(headClass, 'w-20')}>{label('side')}</th>
            <th className={cn(headClass, 'w-24 text-right max-sm:hidden')}>
              {label('entry')}
            </th>
            <th className={cn(headClass, 'w-24 text-right max-sm:hidden')}>
              {label('mark')}
            </th>
            <th className={cn(headClass, 'w-24 text-right max-sm:hidden')}>
              {label('size')}
            </th>
            <th className={cn(headClass, 'w-28 text-right')}>{label('pnl')}</th>
            <th className={cn(headClass, 'w-24 text-right')}>
              {positions.length > 0 ? (
                <Button
                  type="button"
                  variant="danger"
                  size="sm"
                  className="h-6 px-2 text-[11px]"
                  disabled={closing !== null}
                  onClick={() => void closeAll(positions)}
                >
                  {closing === '__all__' ? label('closing') : label('closeAll')}
                </Button>
              ) : (
                <span className="sr-only">{label('close')}</span>
              )}
            </th>
          </tr>
        </thead>
        <tbody>
          {positions.map((position) => {
            const side = sideOf(position)
            const notional =
              Math.abs(position.quantity ?? 0) *
              (position.mark_price || position.entry_price || 0)
            return (
              <tr key={`${position.symbol}-${side}`}>
                <td
                  className={cn(
                    cellClass,
                    'truncate font-medium text-vergex-ink'
                  )}
                  title={position.symbol}
                >
                  {baseLabel(position.symbol)}
                </td>
                <td
                  className={cn(
                    cellClass,
                    'text-xs',
                    side === 'LONG'
                      ? 'text-vergex-success'
                      : 'text-vergex-danger'
                  )}
                >
                  {side === 'LONG' ? label('longSide') : label('shortSide')}{' '}
                  <span className="mono">{position.leverage}×</span>
                </td>
                <td className={cn(cellClass, numberClass, 'max-sm:hidden')}>
                  {fmtPx(position.entry_price)}
                </td>
                <td className={cn(cellClass, numberClass, 'max-sm:hidden')}>
                  {fmtPx(position.mark_price)}
                </td>
                <td className={cn(cellClass, numberClass, 'max-sm:hidden')}>
                  {fmtUsd(notional)}
                </td>
                <td
                  className={cn(
                    cellClass,
                    numberClass,
                    gainTone(position.unrealized_pnl)
                  )}
                  title={`${(position.unrealized_pnl_pct ?? 0).toFixed(2)}%`}
                >
                  {fmtUsd(position.unrealized_pnl, true)}
                </td>
                <td className={cn(cellClass, 'text-right')}>
                  <Button
                    type="button"
                    variant="danger"
                    size="sm"
                    className="h-7 px-2.5 text-xs max-sm:h-10"
                    disabled={closing !== null}
                    aria-label={label('closeSymbol', {
                      symbol: position.symbol,
                    })}
                    onClick={() => void closeOne(position.symbol, side)}
                  >
                    {closing === position.symbol
                      ? label('closing')
                      : label('close')}
                  </Button>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      {positions.length === 0 && (
        <EmptyLine role="status">{label('noPositions')}</EmptyLine>
      )}
    </>
  )
}
