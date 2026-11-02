import { Fragment, useEffect, useId, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Check, ChevronLeft, X } from 'lucide-react'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { lockPageScroll } from '@/utils/page-scroll'
import { Button } from '@/components/ui/button'

// Shared visual contract for the configuration dialogs: one backdrop, one
// surface, one header/body/footer rhythm, Escape and backdrop to close.

export const dialogInputClass =
  'h-9 w-full rounded-md border border-vergex-line bg-vergex-surface px-3 text-sm text-vergex-ink placeholder:text-vergex-faint transition-colors hover:border-vergex-line-strong hover:duration-0 focus:border-vergex-ink/40 focus:bg-vergex-surface focus:outline-none disabled:bg-vergex-surface-muted disabled:text-vergex-muted'

const sizes = {
  sm: 'max-w-md',
  md: 'max-w-xl',
  lg: 'max-w-2xl',
  xl: 'max-w-4xl',
}

interface DialogProps {
  title: ReactNode
  description?: ReactNode
  onClose: () => void
  onBack?: () => void
  icon?: ReactNode
  actions?: ReactNode
  footer?: ReactNode
  size?: keyof typeof sizes
  closeLabel?: string
  /** Replaces the body's default padding and scrolling, e.g. for a sidebar. */
  bodyClassName?: string
  children: ReactNode
}

export function Dialog({
  title,
  description,
  onClose,
  onBack,
  icon,
  actions,
  footer,
  size = 'lg',
  closeLabel = 'Close',
  bodyClassName = 'min-h-0 flex-1 overflow-y-auto px-6 py-5',
  children,
}: DialogProps) {
  const { language } = useLanguage()
  const titleId = useId()
  const panel = useRef<HTMLDivElement>(null)

  // Hold the page still while open, and hand focus back on close.
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    const unlock = lockPageScroll()
    panel.current?.focus()
    return () => {
      unlock()
      previous?.focus()
    }
  }, [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // Only the topmost dialog reacts when dialogs are stacked.
      const open = document.querySelectorAll('[role="dialog"]')
      const box = panel.current
      if (!box || open[open.length - 1] !== box) return
      // A dropdown or menu inside the dialog marks the Escape it used to close.
      if (e.key === 'Escape' && !e.defaultPrevented) onClose()
      if (e.key !== 'Tab') return
      // Keep Tab inside the dialog.
      const items = box.querySelectorAll<HTMLElement>(
        'button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), summary'
      )
      const first = items[0]
      const last = items[items.length - 1]
      if (!first) return
      const at = document.activeElement
      if (e.shiftKey && (at === first || at === box)) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && at === last) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  // Portal to <body> so a transformed ancestor (page enter animations) cannot
  // become the containing block of the fixed backdrop.
  return createPortal(
    <div
      className="fixed inset-0 z-50 flex animate-overlay-in items-start justify-center overflow-y-auto bg-vergex-scrim p-4 sm:items-center"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className={`my-6 flex max-h-[calc(100vh-3rem)] w-full animate-dialog-in flex-col rounded-2xl border border-vergex-line bg-vergex-surface shadow-(--shadow-overlay) outline-none ${sizes[size]}`}
      >
        <header className="flex items-start gap-3 border-b border-vergex-line px-6 py-4">
          {onBack && (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={onBack}
              aria-label={t('back', language)}
              className="-ml-2 mt-0.5 size-8"
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
          )}
          {icon && <div className="mt-0.5 shrink-0">{icon}</div>}
          <div className="min-w-0 flex-1">
            <h2
              id={titleId}
              className="text-base font-semibold leading-snug text-vergex-ink"
            >
              {title}
            </h2>
            {description && (
              <div className="mt-0.5 text-[13px] text-vergex-muted">
                {description}
              </div>
            )}
          </div>
          <div className="flex shrink-0 items-center gap-1">
            {actions}
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={onClose}
              aria-label={closeLabel}
              className="size-8"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        </header>
        <div className={bodyClassName}>{children}</div>
        {footer && (
          <footer className="flex items-center justify-end gap-2 border-t border-vergex-line px-6 py-4">
            {footer}
          </footer>
        )}
      </div>
    </div>,
    document.body
  )
}

export function DialogSteps({
  labels,
  current,
}: {
  labels: string[]
  current: number
}) {
  return (
    <ol className="flex items-center gap-2 text-xs">
      {labels.map((label, index) => {
        const done = index < current
        const active = index === current
        return (
          <Fragment key={label}>
            {index > 0 && (
              <li
                aria-hidden
                className={`h-px w-6 ${done || active ? 'bg-vergex-ink' : 'bg-vergex-line'}`}
              />
            )}
            <li
              aria-current={active ? 'step' : undefined}
              className={`flex items-center gap-1.5 whitespace-nowrap ${active ? 'font-medium text-vergex-ink' : 'text-vergex-muted'}`}
            >
              <span
                className={`flex h-5 w-5 items-center justify-center rounded-full text-[11px] font-semibold ${
                  done
                    ? 'bg-vergex-ink-tint text-vergex-ink'
                    : active
                      ? 'bg-vergex-ink text-vergex-on-ink'
                      : 'bg-vergex-surface-muted text-vergex-muted'
                }`}
              >
                {done ? <Check className="h-3 w-3" /> : index + 1}
              </span>
              {/* A phone has room to name only the step it is on. */}
              <span className={active ? undefined : 'max-sm:sr-only'}>
                {label}
              </span>
            </li>
          </Fragment>
        )
      })}
    </ol>
  )
}

export function DialogField({
  label,
  hint,
  htmlFor,
  required,
  children,
}: {
  label: ReactNode
  hint?: ReactNode
  htmlFor?: string
  required?: boolean
  children: ReactNode
}) {
  return (
    <div className="space-y-1.5">
      <label
        htmlFor={htmlFor}
        className="block text-[13px] font-medium text-vergex-ink"
      >
        {label}
        {required && <span className="ml-0.5 text-vergex-danger">*</span>}
      </label>
      {children}
      {hint && <p className="text-xs text-vergex-muted">{hint}</p>}
    </div>
  )
}

export function DialogNote({
  tone = 'info',
  children,
}: {
  tone?: 'info' | 'success' | 'warning'
  children: ReactNode
}) {
  const tones = {
    info: 'border-vergex-line bg-vergex-surface-muted text-vergex-ink',
    success:
      'border-vergex-success-border bg-vergex-success-bg text-vergex-success',
    warning:
      'border-vergex-warning-border bg-vergex-warning-bg text-vergex-warning',
  }
  return (
    <div
      className={`rounded-md border px-4 py-3 text-[13px] leading-relaxed ${tones[tone]}`}
    >
      {children}
    </div>
  )
}
