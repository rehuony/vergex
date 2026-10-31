import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'

interface SegmentedProps<T extends string> {
  value: T
  options: { value: T; label: string }[]
  onChange: (value: T) => void
  'aria-label': string
  className?: string
}

// One choice out of a few, shown side by side. Each segment is a real button
// that reports its state with aria-pressed.
export function Segmented<T extends string>({
  value,
  options,
  onChange,
  'aria-label': ariaLabel,
  className,
}: SegmentedProps<T>) {
  return (
    <div
      role="group"
      aria-label={ariaLabel}
      className={cn(
        'inline-flex rounded-lg border border-[var(--line)] p-0.5',
        className
      )}
    >
      {options.map((option) => (
        <Button
          key={option.value}
          type="button"
          variant="segment"
          aria-pressed={value === option.value}
          onClick={() => onChange(option.value)}
          className="h-8 rounded-md px-2.5 text-xs max-sm:h-9"
        >
          {option.label}
        </Button>
      ))}
    </div>
  )
}
