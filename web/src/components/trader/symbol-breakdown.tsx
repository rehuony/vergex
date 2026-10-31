import { useMemo } from 'react'
import { t, type Language } from '@/i18n/translations'
import type { SymbolStats } from '@/types'
import { cn } from '@/utils/cn'
import { Panel, PanelState } from '@/components/common/panel'
import { baseLabel, fmtUsd } from './trader-format'

// SymbolBreakdown ranks the most traded symbols by closed trades, with win
// rate, P/L and a bar relative to the busiest symbol.
export function SymbolBreakdown({
  stats,
  language,
}: {
  stats?: SymbolStats[]
  language: Language
}) {
  const rows = useMemo(
    () =>
      (stats ?? [])
        .slice()
        .sort((a, b) => b.total_trades - a.total_trades)
        .slice(0, 6),
    [stats]
  )
  const busiest = rows.reduce((m, s) => Math.max(m, s.total_trades), 1)
  return (
    <Panel title={t('traderDetail.bySymbol', language)}>
      {rows.length === 0 ? (
        <PanelState role="status">
          {t('traderDetail.noClosedTrades', language)}
        </PanelState>
      ) : (
        <ul className="flex flex-col gap-3 px-4 pb-4">
          {rows.map((s) => (
            <li key={s.symbol}>
              <div className="flex items-baseline gap-2 text-[13px]">
                <span className="font-medium text-vergex-text">
                  {baseLabel(s.symbol)}
                </span>
                <span className="truncate text-xs text-vergex-text-muted">
                  {t('traderDetail.tradesAndWins', language, {
                    trades: s.total_trades,
                    wins: s.win_rate.toFixed(0),
                  })}
                </span>
                <span
                  className={cn(
                    'ml-auto font-medium tabular-nums',
                    s.total_pnl >= 0
                      ? 'text-vergex-success'
                      : 'text-vergex-danger'
                  )}
                >
                  {fmtUsd(s.total_pnl, true)}
                </span>
              </div>
              <div className="mt-1 h-1 rounded-full bg-[var(--line)]">
                <div
                  className={cn(
                    'h-1 rounded-full',
                    s.total_pnl >= 0 ? 'bg-vergex-success' : 'bg-vergex-danger'
                  )}
                  style={{ width: `${(s.total_trades / busiest) * 100}%` }}
                />
              </div>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  )
}
