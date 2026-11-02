import { ChevronDown, ChevronUp } from 'lucide-react'
import { cn } from '@/utils/cn'

export interface ColumnSort<K extends string> {
  key: K
  ascending: boolean
}

// A table column heading that sorts by that column.
export function SortHeader<K extends string>({
  column,
  title,
  sort,
  onSort,
  className,
}: {
  column: K
  title: string
  sort: ColumnSort<K> | null
  onSort: (column: K) => void
  className?: string
}) {
  const active = sort?.key === column
  const Arrow = active && sort.ascending ? ChevronUp : ChevronDown
  return (
    <th
      className={className}
      aria-sort={
        active ? (sort.ascending ? 'ascending' : 'descending') : undefined
      }
    >
      <button
        type="button"
        className="inline-flex items-center gap-1 font-medium hover:text-vergex-ink"
        onClick={() => onSort(column)}
      >
        {/* The arrow keeps its place when hidden, so choosing a column moves nothing. */}
        <Arrow
          className={cn('size-3', !active && 'invisible')}
          aria-hidden="true"
        />
        {title}
      </button>
    </th>
  )
}
