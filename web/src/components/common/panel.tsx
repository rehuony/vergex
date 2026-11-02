import type { ReactNode } from 'react'
import { Skeleton } from '@/components/ui/skeleton'
import { activeLanguage, t } from '@/i18n'
import { cn } from '@/utils/cn'

interface PanelProps {
  title?: ReactNode
  /** Short context beside the title, such as a count. */
  meta?: ReactNode
  /** A status line that comes and goes, in the room the title row has anyway. */
  status?: ReactNode
  /** Controls at the end of the title row. */
  actions?: ReactNode
  /**
   * Keeps the title row on one line. The title gives way, so it should truncate
   * itself; the meta only shows where there is room for it.
   */
  singleLine?: boolean
  className?: string
  children: ReactNode
}

// One functional area of a page: a card with an optional title row.
export function Panel({
  title,
  meta,
  status,
  actions,
  singleLine,
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
        <header
          className={cn(
            'flex min-h-12 items-center gap-x-3 gap-y-2 px-4 py-2',
            !singleLine && 'flex-wrap'
          )}
        >
          {title && (
            <h2
              className={cn(
                'text-sm font-semibold text-vergex-ink',
                singleLine && 'min-w-0'
              )}
            >
              {title}
            </h2>
          )}
          {meta && (
            <span
              className={cn(
                'truncate text-xs text-vergex-muted',
                singleLine && 'shrink-0 max-sm:hidden'
              )}
            >
              {meta}
            </span>
          )}
          {status}
          {actions && (
            <div
              className={cn(
                'ml-auto flex items-center gap-2',
                singleLine && 'shrink-0'
              )}
            >
              {actions}
            </div>
          )}
        </header>
      )}
      {children}
    </section>
  )
}

// A region being read: bars where its rows will be, in the room its other states take.
export function PanelLoading({ className }: { className?: string }) {
  return (
    <div
      role="status"
      className={cn(
        'flex min-h-32 flex-col justify-center gap-3 px-4 py-8',
        className
      )}
    >
      <span className="sr-only">{t('loading', activeLanguage())}</span>
      <Skeleton className="h-3 w-full" />
      <Skeleton className="h-3 w-5/6" />
      <Skeleton className="h-3 w-2/3" />
    </div>
  )
}

// A centred empty or failed (role="alert") message inside a panel.
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
