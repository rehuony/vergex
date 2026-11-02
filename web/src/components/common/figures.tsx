import type { ReactNode } from 'react'
import { cn } from '@/utils/cn'

// What a record is read from: a strip of the figures that matter, and tables whose columns
// line up from the first row to the last.

export interface Stat {
  name: string
  value: ReactNode
  note?: ReactNode
  // A gain or loss, in the market's colours.
  gain?: number | null
}

export const gainTone = (gain: number | null | undefined) =>
  !gain ? undefined : gain > 0 ? 'text-vergex-success' : 'text-vergex-danger'

// Hairlines between cells come from the gap showing the line colour behind them, so they
// meet however the cells wrap.
export function StatStrip({
  stats,
  small,
  className,
}: {
  stats: Stat[]
  small?: boolean
  className?: string
}) {
  return (
    <dl
      className={cn(
        'grid gap-px border-b border-vergex-line bg-vergex-line',
        stats.length === 3 ? 'grid-cols-3' : 'grid-cols-2 sm:grid-cols-4',
        className
      )}
    >
      {stats.map((stat) => (
        <div
          key={stat.name}
          className={cn(
            'min-w-0 bg-vergex-surface px-4',
            small ? 'py-2' : 'py-3'
          )}
          title={stat.name}
        >
          <dt className="truncate text-xs text-vergex-muted">{stat.name}</dt>
          {small ? (
            <dd className="mono mt-0.5 truncate text-[13px] text-vergex-ink">
              {stat.value}
              {stat.note !== undefined && (
                <span className="ml-1 font-sans text-[11px] text-vergex-muted">
                  {stat.note}
                </span>
              )}
            </dd>
          ) : (
            <>
              <dd
                className={cn(
                  'mono mt-1 truncate text-2xl leading-[1.2] tracking-[-0.02em] text-vergex-ink',
                  gainTone(stat.gain)
                )}
              >
                {stat.value}
              </dd>
              {stat.note !== undefined && (
                <dd className="mt-0.5 truncate text-[11px] tabular-nums text-vergex-muted">
                  {stat.note}
                </dd>
              )}
            </>
          )}
        </div>
      ))}
    </dl>
  )
}

export const headClass =
  'h-8 whitespace-nowrap border-b border-vergex-line bg-vergex-surface-muted px-2 py-0 text-[11px] font-medium text-vergex-muted first:pl-4 last:pr-4'
// A table's head row names its first group beside the column titles. A later group's name
// sticks over that one, so the row in view always names the group under it, while the
// column titles stay put.
export const groupHeadClass = cn(headClass, 'sticky top-0 z-20')
export const cellClass =
  'h-10 border-b border-vergex-line px-2 py-0 text-[13px] first:pl-4 last:pr-4'
export const numberClass = 'mono text-right text-xs'
export const tableClass =
  'w-full table-fixed border-separate border-spacing-0 text-left tabular-nums'
// For a table inside what scrolls sideways on a phone, whose columns would not fit.
export const wideTableClass = cn(tableClass, 'max-sm:min-w-[460px]')

export interface Figure {
  name: string
  value: ReactNode
  tone?: string
  title?: string
}

// The figures behind the ones that matter: 12 muted names over 13 mono values, in as many
// columns as fit.
export function FigureGrid({ figures }: { figures: Figure[] }) {
  return (
    <dl className="grid grid-cols-2 gap-x-4 gap-y-2 border-b border-vergex-line px-4 py-3 sm:grid-cols-3 lg:grid-cols-6">
      {figures.map((figure) => (
        <div
          key={figure.name}
          className="min-w-0"
          title={figure.title ?? figure.name}
        >
          <dt className="truncate text-xs text-vergex-muted">{figure.name}</dt>
          <dd
            className={cn(
              'mono mt-0.5 truncate text-[13px] text-vergex-ink',
              figure.tone
            )}
          >
            {figure.value}
          </dd>
        </div>
      ))}
    </dl>
  )
}

// What a table has instead of rows. It sits under the table rather than in it, since a
// cell spanning the columns would count the ones a phone hides.
export function EmptyLine({
  role,
  children,
}: {
  role?: 'alert' | 'status'
  children: ReactNode
}) {
  return (
    <p
      role={role}
      className="border-b border-vergex-line px-4 py-2.5 text-xs text-vergex-muted"
    >
      {children}
    </p>
  )
}
