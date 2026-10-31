import { Search } from 'lucide-react'
import { dialogInputClass } from '@/components/common/dialog'
import { cn } from '@/utils/cn'

// The search box above a long list: filters as you type.
export function SearchField({
  label,
  value,
  onChange,
  className,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  className?: string
}) {
  return (
    <label className={cn('relative block', className)}>
      <Search
        className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-vergex-text-muted"
        aria-hidden="true"
      />
      <input
        aria-label={label}
        placeholder={label}
        className={cn(
          dialogInputClass,
          'h-9 pl-9 max-sm:h-10 max-sm:text-base'
        )}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  )
}
