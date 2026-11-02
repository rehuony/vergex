import type { ReactNode } from 'react'
import {
  cellClass,
  gainTone,
  groupHeadClass,
  headClass,
  numberClass,
  tableClass,
} from '@/components/common/figures'
import { cn } from '@/utils/cn'
import { fmtPct, fmtRate } from './forecast-format'

// How far a hit rate is above what chance would hit, in percentage points.
export const pointsAbove = (hit: number | null, chance: number | null) => {
  if (hit === null || chance === null) return '—'
  const points = (hit - chance) * 100
  return `${points > 0 ? '+' : ''}${points.toFixed(1)}`
}

// A hit rate as ink along a track, with a tick where chance would be. The cell beside it
// has the number, so this is left out for assistive technology.
function HitBar({
  hit,
  chance,
}: {
  hit: number | null
  chance: number | null
}) {
  if (hit === null) return null
  return (
    <span
      aria-hidden="true"
      className="relative block h-1.5 rounded-full bg-vergex-surface-muted"
    >
      <span
        className="absolute inset-y-0 left-0 rounded-full bg-vergex-ink"
        style={{ width: `${hit * 100}%` }}
      />
      {chance !== null && (
        <span
          className="absolute -inset-y-1 w-px bg-vergex-muted"
          style={{ left: `${chance * 100}%` }}
        />
      )}
    </span>
  )
}

export interface BreakdownRow {
  name: string
  // Beside the name, where there is room.
  note?: ReactNode
  samples: number | null
  hit: number | null
  chance: number | null
  lift?: number | null
  afterFees: number | null
}

export interface BreakdownGroup {
  title: string
  rows: BreakdownRow[]
}

export interface BreakdownHead {
  samples: string
  hit: string
  chance: string
  lift?: string
  afterFees: string
}

// The columns other than the name, so a group's heading row can leave the same ones out
// on a phone as the rows do.
const breakdownColumns = (head: BreakdownHead) => [
  { title: head.samples, className: 'w-16 max-sm:hidden' },
  { title: '', className: 'w-28 max-md:hidden' },
  { title: head.hit, className: 'w-16' },
  { title: head.chance, className: 'w-16' },
  ...(head.lift ? [{ title: head.lift, className: 'w-16 max-sm:hidden' }] : []),
  { title: head.afterFees, className: 'w-20' },
]

// One table for every way of grouping a record, whose columns line up from the first
// group to the last.
export function BreakdownTable({
  head,
  groups,
}: {
  head: BreakdownHead
  groups: BreakdownGroup[]
}) {
  const columns = breakdownColumns(head)
  const shown = groups.filter((group) => group.rows.length > 0)
  return (
    <table className={tableClass}>
      <thead className="sticky top-0 z-10">
        <tr>
          <th className={headClass}>{shown[0]?.title}</th>
          {columns.map((column, index) => (
            <th
              key={index}
              className={cn(headClass, 'text-right', column.className)}
            >
              {column.title}
            </th>
          ))}
        </tr>
      </thead>
      {shown.map((group, groupIndex) => (
        <tbody key={group.title}>
          {groupIndex > 0 && (
            <tr>
              <th scope="rowgroup" className={groupHeadClass}>
                {group.title}
              </th>
              {columns.map((column, index) => (
                <td key={index} className={cn(headClass, column.className)} />
              ))}
            </tr>
          )}
          {group.rows.map((row) => (
            <tr key={row.name}>
              <td className={cn(cellClass, 'text-vergex-ink')}>
                <span className="flex min-w-0 items-baseline gap-2">
                  <span className="truncate" title={row.name}>
                    {row.name}
                  </span>
                  {row.note && (
                    <span className="shrink-0 text-xs text-vergex-muted max-sm:hidden">
                      {row.note}
                    </span>
                  )}
                </span>
              </td>
              <td className={cn(cellClass, numberClass, 'max-sm:hidden')}>
                {row.samples ?? '—'}
              </td>
              <td className={cn(cellClass, 'max-md:hidden')}>
                <HitBar hit={row.hit} chance={row.chance} />
              </td>
              <td className={cn(cellClass, numberClass)}>{fmtRate(row.hit)}</td>
              <td className={cn(cellClass, numberClass)}>
                {fmtRate(row.chance)}
              </td>
              {head.lift && (
                <td className={cn(cellClass, numberClass, 'max-sm:hidden')}>
                  {row.lift === null || row.lift === undefined
                    ? '—'
                    : `${(row.lift * 100).toFixed(1)}%`}
                </td>
              )}
              <td
                className={cn(
                  cellClass,
                  numberClass,
                  gainTone(row.afterFees) ?? 'text-vergex-muted'
                )}
              >
                {row.afterFees === null ? '—' : fmtPct(row.afterFees, true)}
              </td>
            </tr>
          ))}
        </tbody>
      ))}
    </table>
  )
}
