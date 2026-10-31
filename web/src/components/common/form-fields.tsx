import { useEffect, useState, type ReactNode } from 'react'
import { X } from 'lucide-react'
import { dialogInputClass } from '@/components/common/dialog'
import { notify } from '@/components/common/notify'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { parseSymbols } from '@/utils/symbols'

// The controls setting rows share. Each is 36px tall (40px on phones) and
// holds its half-typed text itself, so a field can be cleared and retyped.

export const fieldClass = cn(
  dialogInputClass,
  'h-9 min-w-0 flex-1 max-sm:h-10 max-sm:text-base'
)

export const textAreaClass = cn(
  dialogInputClass,
  'h-auto max-h-96 min-h-32 w-full resize-none py-2 leading-relaxed field-sizing-content max-sm:text-base'
)

// For a row holding only a switch: on phones it stays beside its name.
export const switchRowClass =
  'max-sm:grid-cols-[minmax(0,1fr)_auto] max-sm:items-center'

// Rows in two columns where the page is wide enough, with the dividers meeting
// across them. A row left alone on the last line takes both columns.
export function FieldGrid({
  className,
  children,
}: {
  className?: string
  children: ReactNode
}) {
  return (
    <div
      className={cn(
        'grid xl:grid-cols-2 xl:gap-x-8 xl:[&>*:last-child:nth-child(odd)]:col-span-2 xl:[&>*:nth-child(2)]:border-t-0',
        className
      )}
    >
      {children}
    </div>
  )
}

function Suffix({ children }: { children?: ReactNode }) {
  if (!children) return null
  return (
    <span className="shrink-0 whitespace-nowrap text-xs text-vergex-text-muted">
      {children}
    </span>
  )
}

// A number kept within its range: out-of-range text is held until the field
// loses focus, then pulled back to the nearest limit.
export function NumberField({
  id,
  value,
  min,
  max,
  step,
  suffix,
  disabled,
  ariaLabel,
  onChange,
}: {
  id?: string
  value: number
  min: number
  max?: number
  step?: number
  suffix?: ReactNode
  disabled?: boolean
  ariaLabel?: string
  onChange: (value: number) => void
}) {
  const [draft, setDraft] = useState(String(value))
  useEffect(() => {
    setDraft((current) => (Number(current) === value ? current : String(value)))
  }, [value])
  const inRange = (n: number) => n >= min && (max === undefined || n <= max)

  return (
    <>
      <input
        id={id}
        aria-label={ariaLabel}
        type="number"
        inputMode="decimal"
        className={fieldClass}
        min={min}
        max={max}
        step={step}
        disabled={disabled}
        value={draft}
        onChange={(event) => {
          const raw = event.target.value
          setDraft(raw)
          const next = Number(raw)
          if (raw !== '' && inRange(next) && next !== value) onChange(next)
        }}
        onBlur={(event) => {
          const raw = event.target.value
          const next = Number(raw)
          if (raw === '' || !Number.isFinite(next)) {
            setDraft(String(value))
            return
          }
          const kept = Math.min(max ?? next, Math.max(min, next))
          setDraft(String(kept))
          if (kept !== value) onChange(kept)
        }}
      />
      <Suffix>{suffix}</Suffix>
    </>
  )
}

// A short list of whole numbers, typed as "20, 50". Empty text keeps the list.
export function PeriodsField({
  value,
  ariaLabel,
  onChange,
}: {
  value: number[]
  ariaLabel: string
  onChange: (value: number[]) => void
}) {
  const shown = value.join(', ')
  const [draft, setDraft] = useState(shown)
  useEffect(() => setDraft(shown), [shown])

  const commit = () => {
    const next = [
      ...new Set(
        draft
          .split(/[\s,]+/)
          .map(Number)
          .filter((n) => Number.isInteger(n) && n >= 2 && n <= 500)
      ),
    ]
    if (next.length === 0 || next.join(', ') === shown) {
      setDraft(shown)
      return
    }
    onChange(next)
  }

  return (
    <input
      aria-label={ariaLabel}
      inputMode="numeric"
      className={fieldClass}
      value={draft}
      onChange={(event) => setDraft(event.target.value)}
      onBlur={commit}
      onKeyDown={(event) => {
        if (event.key === 'Enter') commit()
      }}
    />
  )
}

// Symbols as removable chips with an input to add more.
export function SymbolsField({
  id,
  values,
  placeholder,
  max,
  addLabel,
  removeLabel,
  limitMessage,
  onChange,
}: {
  id?: string
  values: string[]
  placeholder: string
  max?: number
  addLabel: string
  removeLabel: (symbol: string) => string
  limitMessage?: string
  onChange: (values: string[]) => void
}) {
  const [draft, setDraft] = useState('')

  const add = () => {
    const next = [...new Set([...values, ...parseSymbols(draft)])]
    if (max !== undefined && next.length > max && limitMessage) {
      notify.warning(limitMessage)
    }
    onChange(max === undefined ? next : next.slice(0, max))
    setDraft('')
  }

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-2">
      {values.length > 0 && (
        <ul className="flex flex-wrap gap-1.5">
          {values.map((symbol) => (
            <li
              key={symbol}
              className="inline-flex h-7 items-center gap-1 rounded-md border border-[var(--line)] pl-2 pr-0.5 text-xs text-vergex-text max-sm:h-10"
            >
              {symbol}
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={removeLabel(symbol)}
                onClick={() =>
                  onChange(values.filter((value) => value !== symbol))
                }
                className="size-6 rounded-sm hover:text-vergex-danger max-sm:size-9"
              >
                <X className="size-3" />
              </Button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex items-center gap-2">
        <input
          id={id}
          className={fieldClass}
          placeholder={placeholder}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') add()
          }}
        />
        <Button
          type="button"
          variant="secondary"
          size="sm"
          disabled={!draft.trim()}
          onClick={add}
          className="max-sm:h-10"
        >
          {addLabel}
        </Button>
      </div>
    </div>
  )
}
