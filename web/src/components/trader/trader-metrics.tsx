import type { AgentCostSummary } from '@/api/agent'
import { useEffect, useState, type ReactNode } from 'react'
import type {
  AccountInfo,
  Position,
  TraderFullStats,
  TraderMetrics as StoredMetrics,
} from '@/types'
import { t, type Language } from '@/i18n'
import { FigureGrid, StatStrip } from '@/components/common/figures'
import { Skeleton } from '@/components/ui/skeleton'
import { fmtCountdown, fmtPct, fmtTime, fmtUsd } from '@/utils/trader-format'

interface TraderMetricsProps {
  account?: AccountInfo
  /** The last read of the account failed, so what it holds is out of date. */
  accountFailed: boolean
  /** What the trader last stored, to show when the account has never been read. */
  stored?: StoredMetrics
  statsFailed?: boolean
  stats?: TraderFullStats
  /** What its model calls have cost since it began. */
  costs?: AgentCostSummary
  costsFailed?: boolean
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
// the figures behind them. A figure being read is a bar, and one with no trades behind it
// is 0. A read that failed with nothing to fall back on stays a dash, as 0 would say the
// money is gone.
export function TraderMetrics({
  account,
  accountFailed,
  stored,
  stats,
  statsFailed,
  costs,
  costsFailed,
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
  const equity = useStored ? stored?.equity : account?.total_equity
  // A figure from the trader's record, being read or, with no trades behind it, 0.
  const traded = (
    value: (row: TraderFullStats) => ReactNode,
    size?: string
  ): ReactNode =>
    stats ? value(stats) : statsFailed ? '—' : <Skeleton className={size} />
  const book = exposure(positions ?? [], account)
  const held = (value: ReactNode) =>
    positions || accountFailed ? value : <Skeleton />
  const percent = (n?: number | null) => `${(n ?? 0).toFixed(1)}%`
  return (
    <>
      <StatStrip
        stats={[
          {
            name: label('equity'),
            value:
              equity != null ? (
                fmtUsd(equity)
              ) : accountFailed ? (
                '—'
              ) : (
                <Skeleton className="w-28" />
              ),
            note: staleNote && <span role="status">{staleNote}</span>,
          },
          {
            name: label('totalPnl'),
            value: useStored ? (
              fmtPct(stored?.return_pct)
            ) : pnl != null ? (
              fmtUsd(pnl, true)
            ) : accountFailed ? (
              '—'
            ) : (
              <Skeleton className="w-24" />
            ),
            note:
              !useStored && pnl != null
                ? fmtPct(account?.total_pnl_pct)
                : undefined,
            gain: useStored ? stored?.return_pct : pnl,
          },
          {
            name: label('realizedPnl'),
            value: traded((row) => fmtUsd(row.total_pnl, true), 'h-[1em] w-24'),
            note: statsFailed ? (
              <span role="status">{label('notUpdating')}</span>
            ) : undefined,
            gain: stats?.total_pnl,
          },
          {
            name: label('maxDrawdown'),
            value: traded(
              (row) =>
                row.max_drawdown_pct >= 0.05
                  ? `-${row.max_drawdown_pct.toFixed(1)}%`
                  : '0.0%',
              'h-[1em] w-20'
            ),
          },
        ]}
      />
      <FigureGrid
        figures={[
          {
            name: label('profitFactor'),
            value: traded((row) => row.profit_factor.toFixed(2)),
          },
          {
            name: label('winRate'),
            value: traded((row) => `${row.win_rate.toFixed(1)}%`),
          },
          {
            name: label('trades'),
            value: traded((row) => row.total_trades),
          },
          {
            name: label('fees'),
            value: traded((row) => fmtUsd(row.total_fee)),
          },
          {
            name: label('modelCost'),
            value: !costs ? (
              costsFailed ? (
                '—'
              ) : (
                <Skeleton />
              )
            ) : costs.count > 0 && costs.unpriced_count === costs.count ? (
              label('unpriced')
            ) : (
              `${costs.unpriced_count > 0 ? '≥ ' : ''}${fmtUsd(costs.total)}`
            ),
          },
          {
            name: label('netAfterCosts'),
            value: traded((row) =>
              costs ? (
                `${costs.unpriced_count > 0 ? '≤ ' : ''}${fmtUsd(row.total_pnl - row.total_fee - costs.total, true)}`
              ) : costsFailed ? (
                '—'
              ) : (
                <Skeleton />
              )
            ),
          },
          {
            name: label('sharpe'),
            value: traded((row) => row.sharpe_ratio.toFixed(2)),
          },
          {
            name: label('avgWinLoss'),
            value: traded(
              (row) => `${row.avg_win.toFixed(2)} / ${row.avg_loss.toFixed(2)}`
            ),
          },
          { name: label('long'), value: held(fmtUsd(book.long)) },
          { name: label('short'), value: held(fmtUsd(book.short)) },
          { name: label('marginUsed'), value: held(percent(book.margin)) },
          {
            name: label('avgLeverage'),
            value: held(`${(book.leverage ?? 0).toFixed(1)}×`),
          },
          { name: label('largest'), value: held(percent(book.largest)) },
          {
            name: label('nextCycle'),
            value: running ? <NextCycle at={nextCycleMs} /> : '—',
          },
        ]}
      />
    </>
  )
}
