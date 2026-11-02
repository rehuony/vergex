import { cn } from '@/utils/cn'

interface SwitchProps {
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  id?: string
  'aria-label'?: string
  className?: string
}

// An on/off control. The visible track is small, so a pseudo-element gives it
// the 40px touch target phones need without taking up layout space.
export function Switch({
  checked,
  onChange,
  disabled,
  id,
  'aria-label': ariaLabel,
  className,
}: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      id={id}
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cn(
        'relative inline-flex h-5 w-9 shrink-0 items-center rounded-full bg-vergex-line-strong transition-colors after:absolute after:-inset-2.5 after:content-[""] focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-vergex-ink/25 disabled:cursor-default disabled:opacity-45 aria-checked:bg-vergex-ink',
        className
      )}
    >
      <span
        className={cn(
          'size-4 rounded-full bg-vergex-surface transition-transform duration-[var(--dur-state)] ease-[var(--ease-move)]',
          checked ? 'translate-x-[18px]' : 'translate-x-0.5'
        )}
      />
    </button>
  )
}
