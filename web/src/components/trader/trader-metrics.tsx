import { useEffect, useState } from 'react'
import type { AccountInfo, Position, TraderFullStats } from '@/types'
import { t, type Language } from '@/i18n/translations'
import { cn } from '@/utils/cn'
import { Panel } from '@/components/common/panel'
import { fmtCountdown, fmtPct, fmtUsd } from './trader-format'

interface TraderMetricsProps {
  account?: AccountInfo
  stats?: TraderFullStats
  positions?: Position[]
  running?: boolean
  nextCycleMs: number | null
  language: Language
}

function trendClass(value?: number) {
  if (value == null) return 'text-vergex-text'
  return value >= 0 ? 'text-vergex-success' : 'text-vergex-danger'
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

// Long and short exposure, margin use and leverage of the open book, one line.
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

function Line({ label, value }: { label: string; value: string }) {
  return (
    <span className="whitespace-nowrap">
      <span className="text-vergex-text-muted">{label}</span>{' '}
      <span className="font-medium tabular-nums text-vergex-text">{value}</span>
    </span>
  )
}

// TraderMetrics is the headline readout: equity and P/L first, then exposure
// and trade totals as one line each.
export function TraderMetrics({
  account,
  stats,
  positions,
  running,
  nextCycleMs,
  language,
}: TraderMetricsProps) {
  const pnl = account?.total_pnl
  const label = (key: string) => t(`traderDetail.${key}`, language)
  const metrics = [
    {
      label: label('equity'),
      value: fmtUsd(account?.total_equity),
      className: 'max-sm:col-span-2',
    },
    {
      label: label('totalPnl'),
      value:
        pnl == null
          ? '—'
          : `${fmtUsd(pnl, true)} (${fmtPct(account?.total_pnl_pct)})`,
      tone: pnl,
    },
    {
      label: label('realizedPnl'),
      value: stats ? fmtUsd(stats.total_pnl, true) : '—',
      tone: stats?.total_pnl,
    },
    {
      label: label('profitFactor'),
      value: stats ? stats.profit_factor.toFixed(2) : '—',
    },
    {
      label: label('maxDrawdown'),
      value: !stats
        ? '—'
        : stats.max_drawdown_pct >= 0.05
          ? `-${stats.max_drawdown_pct.toFixed(1)}%`
          : '0.0%',
    },
  ]
  const book = exposure(positions ?? [], account)
  const percent = (n?: number) => (n == null ? '—' : `${n.toFixed(1)}%`)
  return (
    <Panel className="overflow-hidden">
      <dl className="grid grid-cols-2 gap-px bg-[var(--line)] sm:grid-cols-5">
        {metrics.map((m) => (
          <div
            key={m.label}
            className={cn('bg-[var(--surface)] px-4 py-3', m.className)}
          >
            <dt className="truncate text-xs text-vergex-text-muted">
              {m.label}
            </dt>
            <dd
              className={cn(
                'mt-1 truncate text-base font-semibold tabular-nums sm:text-xl',
                trendClass(m.tone)
              )}
              title={m.value}
            >
              {m.value}
            </dd>
          </div>
        ))}
      </dl>
      <div className="flex flex-col gap-1 border-t border-[var(--line)] px-4 py-3 text-xs">
        <div className="flex flex-wrap gap-x-5 gap-y-1">
          <Line label={label('long')} value={fmtUsd(book.long)} />
          <Line label={label('short')} value={fmtUsd(book.short)} />
          <Line label={label('marginUsed')} value={percent(book.margin)} />
          <Line
            label={label('avgLeverage')}
            value={book.leverage == null ? '—' : `${book.leverage.toFixed(1)}×`}
          />
          <Line label={label('largest')} value={percent(book.largest)} />
          {running && (
            <span className="whitespace-nowrap sm:ml-auto">
              <span className="text-vergex-text-muted">
                {label('nextCycle')}
              </span>{' '}
              <span className="font-medium tabular-nums text-vergex-text">
                <NextCycle at={nextCycleMs} />
              </span>
            </span>
          )}
        </div>
        {stats && (
          <div className="flex flex-wrap gap-x-5 gap-y-1">
            <Line label={label('trades')} value={String(stats.total_trades)} />
            <Line
              label={label('winRate')}
              value={`${stats.win_rate.toFixed(1)}%`}
            />
            <Line label={label('fees')} value={fmtUsd(stats.total_fee)} />
            <Line
              label={label('sharpe')}
              value={stats.sharpe_ratio.toFixed(2)}
            />
            <Line
              label={label('avgWinLoss')}
              value={`${stats.avg_win.toFixed(2)} / ${stats.avg_loss.toFixed(2)}`}
            />
          </div>
        )}
      </div>
    </Panel>
  )
}
