import { useEffect, useState } from 'react'
import type {
  AccountInfo,
  Position,
  TraderFullStats,
  TraderMetrics as StoredMetrics,
} from '@/types'
import { t, type Language } from '@/i18n'
import { FigureGrid, StatStrip } from '@/components/common/figures'
import { fmtCountdown, fmtPct, fmtTime, fmtUsd } from '@/utils/trader-format'

interface TraderMetricsProps {
  account?: AccountInfo
  /** The last read of the account failed, so what it holds is out of date. */
  accountFailed: boolean
  /** What the trader last stored, to show when the account has never been read. */
  stored?: StoredMetrics
  stats?: TraderFullStats
  positions?: Position[]
  running?: boolean
  nextCycleMs: number | null
  language: Language
}

// The countdown ticks on its own so the page around it does not re-render.
function NextCycle({ at }: { at: number | null }) {
  const [, setTick] = useState(0)
  useEffect(() => {
    const id = setInterval(() => setTick((n) => n + 1), 1000)
    return () => clearInterval(id)
  }, [])
  return <>{fmtCountdown(at)}</>
}

// Long and short exposure, margin use and leverage of the open book.
function exposure(positions: Position[], account?: AccountInfo) {
  let long = 0
  let short = 0
  let largest = 0
  let leverage = 0
  for (const p of positions) {
    const notional = Math.abs(p.quantity || 0) * (p.mark_price || p.entry_price)
    if (/long|buy/i.test(p.side)) long += notional
    else short += notional
    largest = Math.max(largest, notional)
    leverage += p.leverage || 0
  }
  const total = long + short
  return {
    long,
    short,
    margin: account?.margin_used_pct,
    leverage: positions.length ? leverage / positions.length : undefined,
    largest: total > 0 ? (largest / total) * 100 : undefined,
  }
}

// TraderMetrics is the headline readout: what it holds and what it has made first, then
// the figures behind them. A figure with no trades behind it is a dash, not a zero.
export function TraderMetrics({
  account,
  accountFailed,
  stored,
  stats,
  positions,
  running,
  nextCycleMs,
  language,
}: TraderMetricsProps) {
  const pnl = account?.total_pnl
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`traderDetail.${key}`, language, params)
  // A failed read keeps the last real values, marked. An account that has never been
  // read falls back to what the trader stored in its last cycle.
  const useStored = accountFailed && !account && stored?.equity != null
  const staleNote = !accountFailed
    ? undefined
    : account
      ? label('equityNotUpdating')
      : useStored
        ? label('equityAsOf', { time: fmtTime(stored?.equity_at) })
        : undefined
  const traded = stats && stats.total_trades > 0 ? stats : undefined
  const book = exposure(positions ?? [], account)
  const percent = (n?: number) => (n == null ? '—' : `${n.toFixed(1)}%`)
  return (
    <>
      <StatStrip
        stats={[
          {
            name: label('equity'),
            value: fmtUsd(useStored ? stored?.equity : account?.total_equity),
            note: staleNote && <span role="status">{staleNote}</span>,
          },
          {
            name: label('totalPnl'),
            value: useStored
              ? fmtPct(stored?.return_pct)
              : pnl == null
                ? '—'
                : fmtUsd(pnl, true),
            note:
              !useStored && pnl != null
                ? fmtPct(account?.total_pnl_pct)
                : undefined,
            gain: useStored ? stored?.return_pct : pnl,
          },
          {
            name: label('realizedPnl'),
            value: traded ? fmtUsd(traded.total_pnl, true) : '—',
            gain: traded?.total_pnl,
          },
          {
            name: label('maxDrawdown'),
            value: !traded
              ? '—'
              : traded.max_drawdown_pct >= 0.05
                ? `-${traded.max_drawdown_pct.toFixed(1)}%`
                : '0.0%',
          },
        ]}
      />
      <FigureGrid
        figures={[
          {
            name: label('profitFactor'),
            value: traded ? traded.profit_factor.toFixed(2) : '—',
          },
          {
            name: label('winRate'),
            value: traded ? `${traded.win_rate.toFixed(1)}%` : '—',
          },
          {
            name: label('trades'),
            value: stats ? stats.total_trades : '—',
          },
          {
            name: label('fees'),
            value: traded ? fmtUsd(traded.total_fee) : '—',
          },
          {
            name: label('sharpe'),
            value: traded ? traded.sharpe_ratio.toFixed(2) : '—',
          },
          {
            name: label('avgWinLoss'),
            value: traded
              ? `${traded.avg_win.toFixed(2)} / ${traded.avg_loss.toFixed(2)}`
              : '—',
          },
          { name: label('long'), value: fmtUsd(book.long) },
          { name: label('short'), value: fmtUsd(book.short) },
          { name: label('marginUsed'), value: percent(book.margin) },
          {
            name: label('avgLeverage'),
            value: book.leverage == null ? '—' : `${book.leverage.toFixed(1)}×`,
          },
          { name: label('largest'), value: percent(book.largest) },
          {
            name: label('nextCycle'),
            value: running ? <NextCycle at={nextCycleMs} /> : '—',
          },
        ]}
      />
    </>
  )
}
