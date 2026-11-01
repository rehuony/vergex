import * as SelectPrimitive from '@radix-ui/react-select'
import { Check, ChevronDown } from 'lucide-react'
import { cn } from '@/utils/cn'

export interface SelectOption {
  value: string | number
  label: string
  /** Language of the label when it differs from the page, e.g. "zh-CN". */
  lang?: string
}

interface AppSelectProps {
  value: string | number
  onChange: (value: string) => void
  options: SelectOption[]
  disabled?: boolean
  /** Classes for the trigger, e.g. its width or height. */
  className?: string
  id?: string
  'aria-label'?: string
}

// Radix reserves the empty string for "nothing selected", but options such as
// "Auto" use it as a real value.
const EMPTY = '__empty__'
const toItem = (value: string | number) =>
  String(value) === '' ? EMPTY : String(value)

/**
 * A dropdown styled like the account menu. Radix Select supplies keyboard
 * navigation, typeahead and the listbox semantics.
 */
export function AppSelect({
  value,
  onChange,
  options,
  disabled,
  className,
  id,
  'aria-label': ariaLabel,
}: AppSelectProps) {
  return (
    <SelectPrimitive.Root
      value={toItem(value)}
      onValueChange={(next) => onChange(next === EMPTY ? '' : next)}
      disabled={disabled}
    >
      <SelectPrimitive.Trigger
        id={id}
        aria-label={ariaLabel}
        className={cn(
          'flex h-10 w-full items-center justify-between gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 text-left text-sm text-vergex-text outline-none transition-colors hover:border-[var(--line-strong)] hover:duration-0 focus-visible:border-vergex-brand disabled:cursor-not-allowed disabled:opacity-50 data-[state=open]:border-vergex-brand',
          className
        )}
      >
        <span className="min-w-0 truncate">
          <SelectPrimitive.Value />
        </span>
        <SelectPrimitive.Icon asChild>
          <ChevronDown className="size-4 shrink-0 text-vergex-text-muted" />
        </SelectPrimitive.Icon>
      </SelectPrimitive.Trigger>
      <SelectPrimitive.Portal>
        <SelectPrimitive.Content
          position="popper"
          sideOffset={4}
          // Above the z-50 dialogs it is usually opened from.
          className="z-[60] max-h-[min(var(--radix-select-content-available-height),18rem)] min-w-[var(--radix-select-trigger-width)] origin-[var(--radix-select-content-transform-origin)] animate-menu-in overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface)] p-1.5 shadow-[var(--shadow-lg)]"
        >
          <SelectPrimitive.Viewport>
            {options.map((option) => (
              <SelectPrimitive.Item
                key={option.value}
                value={toItem(option.value)}
                lang={option.lang}
                className="flex h-9 cursor-pointer select-none items-center gap-2 rounded-lg px-2.5 text-sm text-vergex-text outline-none data-[disabled]:pointer-events-none data-[highlighted]:bg-vergex-bg data-[disabled]:opacity-50"
              >
                <SelectPrimitive.ItemText>
                  {option.label}
                </SelectPrimitive.ItemText>
                <SelectPrimitive.ItemIndicator className="ml-auto">
                  <Check
                    className="size-4 text-vergex-brand"
                    aria-hidden="true"
                  />
                </SelectPrimitive.ItemIndicator>
              </SelectPrimitive.Item>
            ))}
          </SelectPrimitive.Viewport>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  )
}
