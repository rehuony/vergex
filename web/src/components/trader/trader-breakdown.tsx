import { useMemo } from 'react'
import { t, type Language } from '@/i18n'
import type { HistoricalPosition, SymbolStats } from '@/types'
import {
  cellClass,
  EmptyLine,
  gainTone,
  groupHeadClass,
  headClass,
  numberClass,
  wideTableClass,
} from '@/components/common/figures'
import { cn } from '@/utils/cn'
import { baseLabel, fmtUsd } from '@/utils/trader-format'

interface Row {
  name: string
  trades: number
  winRate: number
  fees?: number
  net: number
}

interface Bucket {
  name: string
  trades: number
  wins: number
  fees: number
  net: number
}

const bucket = (name: string): Bucket => ({
  name,
  trades: 0,
  wins: 0,
  fees: 0,
  net: 0,
})

function add(row: Bucket, position: HistoricalPosition) {
  row.trades += 1
  row.net += position.realized_pnl || 0
  row.fees += position.fee || 0
  if ((position.realized_pnl || 0) > 0) row.wins += 1
}

const toRows = (buckets: Bucket[]): Row[] =>
  buckets
    .filter((row) => row.trades > 0)
    .map((row) => ({
      name: row.name,
      trades: row.trades,
      winRate: (100 * row.wins) / row.trades,
      fees: row.fees,
      net: row.net,
    }))

// TraderBreakdown shows where the money comes from, in one table whose columns line up
// from one way of grouping to the next: closed trades by symbol, and the latest of them by
// how long they were held and by side.
export function TraderBreakdown({
  symbols,
  positions,
  language,
}: {
  symbols?: SymbolStats[]
  positions?: HistoricalPosition[]
  language: Language
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`traderDetail.${key}`, language, params)
  const { holds, sides, sample } = useMemo(() => {
    const holds = ['<15m', '15–60m', '1–3h', '>3h'].map(bucket)
    const sides = [bucket(label('longSide')), bucket(label('shortSide'))]
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
    return { holds: toRows(holds), sides: toRows(sides), sample }
  }, [positions, language])

  const bySymbol = useMemo<Row[]>(
    () =>
      (symbols ?? [])
        .slice()
        .sort((a, b) => b.total_trades - a.total_trades)
        .map((row) => ({
          name: baseLabel(row.symbol),
          trades: row.total_trades,
          winRate: row.win_rate,
          net: row.total_pnl,
        })),
    [symbols]
  )

  const groups = [
    { title: label('bySymbol'), rows: bySymbol },
    {
      title: `${label('byHold')} · ${label('lastClosed', { count: sample })}`,
      rows: holds,
    },
    { title: label('bySide'), rows: sides },
  ].filter((group) => group.rows.length > 0)

  if (groups.length === 0)
    return <EmptyLine role="status">{label('noClosedTrades')}</EmptyLine>

  return (
    <table className={wideTableClass}>
      <thead className="sticky top-0 z-10">
        <tr>
          <th className={headClass}>{groups[0].title}</th>
          <th className={cn(headClass, 'w-20 text-right')}>
            {label('trades')}
          </th>
          <th className={cn(headClass, 'w-20 text-right')}>
            {label('winRate')}
          </th>
          <th className={cn(headClass, 'w-24 text-right max-sm:hidden')}>
            {label('fees')}
          </th>
          <th className={cn(headClass, 'w-28 text-right')}>{label('net')}</th>
        </tr>
      </thead>
      {groups.map((group, index) => (
        <tbody key={group.title}>
          {index > 0 && (
            <tr>
              <th scope="rowgroup" className={groupHeadClass}>
                {group.title}
              </th>
              <td className={headClass} />
              <td className={headClass} />
              <td className={cn(headClass, 'max-sm:hidden')} />
              <td className={headClass} />
            </tr>
          )}
          {group.rows.map((row) => (
            <tr key={row.name}>
              <td className={cn(cellClass, 'font-medium text-vergex-ink')}>
                {row.name}
              </td>
              <td className={cn(cellClass, numberClass)}>{row.trades}</td>
              <td className={cn(cellClass, numberClass)}>
                {row.winRate.toFixed(0)}%
              </td>
              <td className={cn(cellClass, numberClass, 'max-sm:hidden')}>
                {row.fees == null ? '—' : fmtUsd(row.fees)}
              </td>
              <td className={cn(cellClass, numberClass, gainTone(row.net))}>
                {fmtUsd(row.net, true)}
              </td>
            </tr>
          ))}
        </tbody>
      ))}
    </table>
  )
}
