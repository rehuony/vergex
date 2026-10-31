import { useMemo } from 'react'
import { t, type Language } from '@/i18n/translations'
import type { HistoricalPosition } from '@/types'
import { cn } from '@/utils/cn'
import { Panel, PanelState } from '@/components/common/panel'
import { fmtUsd } from './trader-format'

interface Bucket {
  label: string
  trades: number
  net: number
  fees: number
  wins: number
}

const bucket = (label: string): Bucket => ({
  label,
  trades: 0,
  net: 0,
  fees: 0,
  wins: 0,
})

function add(row: Bucket, position: HistoricalPosition) {
  row.trades += 1
  row.net += position.realized_pnl || 0
  row.fees += position.fee || 0
  if ((position.realized_pnl || 0) > 0) row.wins += 1
}

// EdgeProfile shows where the money comes from: recent closed trades split by
// how long they were held and by side, each with net P/L, fees and win rate.
export function EdgeProfile({
  positions,
  language,
}: {
  positions?: HistoricalPosition[]
  language: Language
}) {
  const { holds, sides, sample } = useMemo(() => {
    const holds = ['<15m', '15–60m', '1–3h', '>3h'].map(bucket)
    const sides = [
      bucket(t('traderDetail.longSide', language)),
      bucket(t('traderDetail.shortSide', language)),
    ]
    let sample = 0
    for (const position of positions ?? []) {
      if ((position.status || '').toUpperCase() !== 'CLOSED') continue
      const entry = Number(position.entry_time)
      const exit = Number(position.exit_time)
      if (!Number.isFinite(entry) || !Number.isFinite(exit) || exit <= entry)
        continue
      sample += 1
      const minutes = (exit - entry) / 60000
      add(
        holds[minutes < 15 ? 0 : minutes < 60 ? 1 : minutes < 180 ? 2 : 3],
        position
      )
      add(
        sides[(position.side || '').toLowerCase() === 'short' ? 1 : 0],
        position
      )
    }
    return { holds, sides, sample }
  }, [positions, language])

  const largest = Math.max(
    0.01,
    ...[...holds, ...sides].map((row) => Math.abs(row.net))
  )
  const row = (item: Bucket) => {
    const up = item.net >= 0
    return (
      <li key={item.label}>
        <div className="flex items-baseline gap-2 text-[13px]">
          <span className="w-14 shrink-0 font-medium text-vergex-text">
            {item.label}
          </span>
          <span className="truncate text-xs text-vergex-text-muted">
            {item.trades > 0
              ? t('traderDetail.tradesWinsFees', language, {
                  trades: item.trades,
                  wins: ((100 * item.wins) / item.trades).toFixed(0),
                  fees: item.fees.toFixed(2),
                })
              : '—'}
          </span>
          <span
            className={cn(
              'ml-auto font-medium tabular-nums',
              item.trades === 0
                ? 'text-vergex-text-muted'
                : up
                  ? 'text-vergex-success'
                  : 'text-vergex-danger'
            )}
          >
            {item.trades > 0 ? fmtUsd(item.net, true) : '—'}
          </span>
        </div>
        {/* A bar either side of a centre line: losses left, gains right. */}
        <div className="mt-1 flex h-1 rounded-full bg-[var(--line)]">
          <div className="flex w-1/2 justify-end">
            {!up && (
              <div
                className="h-1 rounded-l-full bg-vergex-danger"
                style={{ width: `${(Math.abs(item.net) / largest) * 100}%` }}
              />
            )}
          </div>
          <div className="w-1/2">
            {up && item.net > 0 && (
              <div
                className="h-1 rounded-r-full bg-vergex-success"
                style={{ width: `${(item.net / largest) * 100}%` }}
              />
            )}
          </div>
        </div>
      </li>
    )
  }

  return (
    <Panel
      title={t('traderDetail.holdAndSide', language)}
      meta={
        sample > 0
          ? t('traderDetail.lastClosed', language, { count: sample })
          : undefined
      }
    >
      {sample === 0 ? (
        <PanelState role="status">
          {t('traderDetail.noClosedTrades', language)}
        </PanelState>
      ) : (
        <ul className="flex flex-col gap-3 px-4 pb-4">
          {holds.map(row)}
          <li className="h-px bg-[var(--line)]" aria-hidden="true" />
          {sides.map(row)}
        </ul>
      )}
    </Panel>
  )
}
