import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { Check, ChevronDown } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'

// A model's name: a field that takes any name, with what the provider lists a click away.
// There is one value and it lives in the field, so a model from the list replaces what was
// typed, typing replaces what was picked, and there is no mode to go into or come back from.
export function ModelField({
  id,
  value,
  placeholder,
  names,
  listing,
  className,
  language,
  onChange,
}: {
  id: string
  value: string
  placeholder: string
  // What the provider lists.
  names: string[]
  // That list is still on its way.
  listing: boolean
  className: string
  language: Language
  onChange: (name: string) => void
}) {
  const [open, setOpen] = useState(false)
  // By name, so it stays on its model when the list arrives while open.
  const [activeName, setActiveName] = useState('')
  const active = Math.max(names.indexOf(activeName), 0)
  const field = useRef<HTMLInputElement>(null)
  const activeRow = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (open) activeRow.current?.scrollIntoView?.({ block: 'nearest' })
  }, [open, active])

  function show(next: boolean) {
    if (next) setActiveName(value.trim())
    setOpen(next)
  }

  function choose(name: string) {
    setOpen(false)
    onChange(name)
    field.current?.focus()
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      if (!open) show(true)
      else if (names.length > 0) {
        const step = event.key === 'ArrowDown' ? 1 : -1
        setActiveName(names[(active + step + names.length) % names.length])
      }
    } else if (event.key === 'Enter' && open && names[active]) {
      // With the list closed, Enter saves the form like in any other field.
      event.preventDefault()
      choose(names[active])
    }
  }

  return (
    <Popover open={open} onOpenChange={show}>
      <PopoverAnchor asChild>
        <div className="relative w-full">
          <input
            ref={field}
            id={id}
            type="text"
            role="combobox"
            aria-expanded={open}
            aria-controls={`${id}-list`}
            aria-autocomplete="none"
            aria-activedescendant={open ? `${id}-row-${active}` : undefined}
            value={value}
            placeholder={placeholder}
            onChange={(event) => {
              setOpen(false)
              onChange(event.target.value)
            }}
            onKeyDown={onKeyDown}
            className={cn(className, 'pr-10')}
          />
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t('connections.modelList', language)}
              title={t('connections.modelList', language)}
              className="absolute right-0 top-1/2 size-9 -translate-y-1/2 rounded-md hover:bg-transparent max-sm:size-10"
            >
              <ChevronDown className="size-4" aria-hidden="true" />
            </Button>
          </PopoverTrigger>
        </div>
      </PopoverAnchor>
      <PopoverContent
        className="w-[var(--radix-popover-trigger-width)] p-1.5"
        // The cursor stays in the field, where the arrow keys move through the list.
        onOpenAutoFocus={(event) => {
          event.preventDefault()
          field.current?.focus()
        }}
        onFocusOutside={(event) => {
          if (event.target === field.current) event.preventDefault()
        }}
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        <ScrollArea className="max-h-60">
          <ul id={`${id}-list`} role="listbox">
            {names.map((name, index) => (
              <li key={name} role="presentation">
                <button
                  id={`${id}-row-${index}`}
                  ref={index === active ? activeRow : undefined}
                  type="button"
                  role="option"
                  aria-selected={name === value.trim()}
                  tabIndex={-1}
                  title={name}
                  onMouseEnter={() => setActiveName(name)}
                  onClick={() => choose(name)}
                  className={cn(
                    'flex h-8 w-full items-center justify-between gap-2 rounded-md px-2 text-left text-sm text-vergex-ink outline-none max-sm:h-10',
                    index === active && 'bg-vergex-surface-muted'
                  )}
                >
                  <span className="min-w-0 truncate">{name}</span>
                  {name === value.trim() && (
                    <Check className="size-4 shrink-0" aria-hidden="true" />
                  )}
                </button>
              </li>
            ))}
            {listing && (
              <li role="presentation" className="flex h-8 items-center px-2">
                <Skeleton className="h-3 w-2/3" />
              </li>
            )}
          </ul>
        </ScrollArea>
      </PopoverContent>
    </Popover>
  )
}
