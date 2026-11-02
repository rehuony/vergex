import type { ReactNode } from 'react'
import { cn } from '@/utils/cn'

interface PanelProps {
  title?: ReactNode
  /** Short context beside the title, such as a count. */
  meta?: ReactNode
  /** Controls at the end of the title row. */
  actions?: ReactNode
  className?: string
  children: ReactNode
}

// One functional area of a page: a card with an optional title row.
export function Panel({
  title,
  meta,
  actions,
  className,
  children,
}: PanelProps) {
  return (
    <section
      className={cn(
        'min-w-0 rounded-2xl border border-vergex-line bg-vergex-surface',
        className
      )}
    >
      {(title || actions) && (
        <header className="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-2 px-4 py-2">
          {title && (
            <h2 className="text-sm font-semibold text-vergex-ink">{title}</h2>
          )}
          {meta && (
            <span className="truncate text-xs text-vergex-muted">{meta}</span>
          )}
          {actions && (
            <div className="ml-auto flex items-center gap-2">{actions}</div>
          )}
        </header>
      )}
      {children}
    </section>
  )
}

// A centred loading, empty or failed (role="alert") message inside a panel.
export function PanelState({
  role,
  className,
  children,
}: {
  role?: 'alert' | 'status'
  className?: string
  children: ReactNode
}) {
  return (
    <div
      role={role}
      className={cn(
        'flex min-h-32 flex-col items-center justify-center gap-2 px-4 py-8 text-center text-sm',
        role === 'alert' ? 'text-vergex-danger' : 'text-vergex-muted',
        className
      )}
    >
      {children}
    </div>
  )
}
