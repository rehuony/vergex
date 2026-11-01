import { useState } from 'react'
import { mutate } from 'swr'
import { api } from '@/api'
import { t, type Language } from '@/i18n'
import type { Position } from '@/types'
import { cn } from '@/utils/cn'
import { confirmToast, notify } from '@/components/common/notify'
import { Panel, PanelState } from '@/components/common/panel'
import { Button } from '@/components/ui/button'
import { baseLabel, fmtPx, fmtUsd } from '@/utils/trader-format'

const headClass = 'px-2 py-2 text-xs font-medium whitespace-nowrap'
const cellClass = 'px-2 py-2 whitespace-nowrap'

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
  const [closing, setClosing] = useState<string | null>(null)
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`traderDetail.${key}`, language, params)

  async function refresh() {
    await Promise.all([
      mutate(`positions-${traderId}`),
      mutate(`account-${traderId}`),
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

  return (
    <Panel
      title={label('openPositions')}
      meta={
        positions ? label('openCount', { count: positions.length }) : undefined
      }
      actions={
        positions && positions.length > 0 ? (
          <Button
            type="button"
            variant="danger"
            size="sm"
            className="max-sm:h-10"
            disabled={closing !== null}
            onClick={() => void closeAll(positions)}
          >
            {closing === '__all__' ? label('closing') : label('closeAll')}
          </Button>
        ) : undefined
      }
    >
      {!positions ? (
        failed ? (
          <PanelState role="alert">{label('positionsFailed')}</PanelState>
        ) : (
          <PanelState role="status">{t('loading', language)}</PanelState>
        )
      ) : positions.length === 0 ? (
        <PanelState role="status">{label('noPositions')}</PanelState>
      ) : (
        <div className="px-2 pb-2">
          <table className="w-full text-left text-[13px] tabular-nums">
            <thead>
              <tr>
                <th className={headClass}>{label('symbol')}</th>
                <th className={cn(headClass, 'text-right max-sm:hidden')}>
                  {label('entry')}
                </th>
                <th className={cn(headClass, 'text-right max-sm:hidden')}>
                  {label('mark')}
                </th>
                <th className={cn(headClass, 'text-right max-sm:hidden')}>
                  {label('size')}
                </th>
                <th className={cn(headClass, 'text-right')}>{label('pnl')}</th>
                <th className={headClass}>
                  <span className="sr-only">{label('close')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {positions.map((position) => {
                const side = sideOf(position)
                const win = (position.unrealized_pnl ?? 0) >= 0
                const tone = win ? 'text-vergex-success' : 'text-vergex-danger'
                const notional =
                  Math.abs(position.quantity ?? 0) *
                  (position.mark_price || position.entry_price || 0)
                return (
                  <tr key={`${position.symbol}-${side}`}>
                    <td className={cellClass}>
                      <div className="font-medium text-vergex-text">
                        {baseLabel(position.symbol)}
                      </div>
                      <div
                        className={cn(
                          'text-xs',
                          side === 'LONG'
                            ? 'text-vergex-success'
                            : 'text-vergex-danger'
                        )}
                      >
                        {side === 'LONG'
                          ? label('longSide')
                          : label('shortSide')}{' '}
                        {position.leverage}×
                      </div>
                    </td>
                    <td className={cn(cellClass, 'text-right max-sm:hidden')}>
                      {fmtPx(position.entry_price)}
                    </td>
                    <td className={cn(cellClass, 'text-right max-sm:hidden')}>
                      {fmtPx(position.mark_price)}
                    </td>
                    <td className={cn(cellClass, 'text-right max-sm:hidden')}>
                      {fmtUsd(notional)}
                    </td>
                    <td className={cn(cellClass, 'text-right', tone)}>
                      <div className="font-medium">
                        {fmtUsd(position.unrealized_pnl, true)}
                      </div>
                      <div className="text-xs">
                        {(position.unrealized_pnl_pct ?? 0).toFixed(2)}%
                      </div>
                    </td>
                    <td className={cn(cellClass, 'w-px text-right')}>
                      <Button
                        type="button"
                        variant="danger"
                        size="sm"
                        className="max-sm:h-10"
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
        </div>
      )}
    </Panel>
  )
}
