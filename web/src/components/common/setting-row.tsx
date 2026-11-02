import type { ReactNode } from 'react'
import { ChevronLeft } from 'lucide-react'
import { cn } from '@/utils/cn'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'

// The settings tabs share one layout:
// - rows: the name in a 160px column, the control filling the rest, every
//   control 36px tall, rows split by a line, no help text under them;
// - a group that is submitted together ends with SettingActions.
// Nothing appears between rows on input, so the dialog never shifts.

// One labelled row: the name on the left, the control on the right.
export function SettingRow({
  label,
  htmlFor,
  top,
  className,
  children,
}: {
  label: string
  /** Id of the control the label names; omit when the row has no single one. */
  htmlFor?: string
  /** Lines the name up with a tall control's first line, not its middle. */
  top?: boolean
  className?: string
  children: ReactNode
}) {
  const Label = htmlFor ? 'label' : 'p'
  return (
    <div
      className={cn(
        'grid gap-2 border-t border-vergex-line py-3 first:border-t-0 sm:grid-cols-[160px_minmax(0,1fr)] sm:gap-6',
        top ? 'sm:items-start' : 'sm:items-center',
        className
      )}
    >
      <Label
        htmlFor={htmlFor}
        className={cn(
          'text-sm font-medium text-vergex-ink',
          top && 'sm:leading-9'
        )}
      >
        {label}
      </Label>
      <div className="flex min-h-9 min-w-0 flex-wrap items-center gap-2">
        {children}
      </div>
    </div>
  )
}

// One entry in a list, such as a saved connection: its icon, name and status
// badge on the left, its buttons on the right.
export function SettingItem({
  icon,
  title,
  badge,
  detail,
  children,
}: {
  icon: ReactNode
  title: ReactNode
  /** A SettingBadge shown right after the name. */
  badge?: ReactNode
  detail?: ReactNode
  children?: ReactNode
}) {
  return (
    <div className="flex items-center gap-3 border-t border-vergex-line py-3 first:border-t-0">
      <span className="grid size-9 shrink-0 place-items-center rounded-md border border-vergex-line bg-vergex-surface text-vergex-muted">
        {icon}
      </span>
      <div className="min-w-0 flex-1">
        {/* Where the name and badge do not fit, the badge drops below. */}
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <p className="max-w-full truncate text-sm font-medium text-vergex-ink">
            {title}
          </p>
          {badge}
        </div>
        {detail && (
          <p className="mt-1 truncate text-xs text-vergex-muted">{detail}</p>
        )}
      </div>
      {children}
    </div>
  )
}

// A window of one scrolling region, which is every window in Settings: what it holds scrolls
// between its head, which stays at the top, and the action bar, which stays at the bottom in
// the same place on every one, however long what it holds gets. A form goes around this, so
// its buttons submit it.
export function SettingList({
  head,
  children,
  actions,
  message,
  messageTone,
  messageId,
}: {
  head?: ReactNode
  children: ReactNode
  /** A window with nothing to do has no bar. */
  actions?: ReactNode
  message?: ReactNode
  messageTone?: keyof typeof messageTones
  messageId?: string
}) {
  return (
    <div className="flex h-full flex-col">
      {head && <div className="border-b border-vergex-line">{head}</div>}
      <ScrollArea
        className="-mx-6 min-h-0 flex-1 px-6"
        viewportClassName="flex flex-col"
      >
        {children}
      </ScrollArea>
      {actions && (
        <SettingActions
          message={message}
          messageTone={messageTone}
          messageId={messageId}
        >
          {actions}
        </SettingActions>
      )}
    </div>
  )
}

// Fills the rest of the tab with one centred message: loading, empty or a
// failure (role="alert").
export function SettingState({
  role,
  children,
}: {
  role?: 'alert' | 'status'
  children: ReactNode
}) {
  return (
    <div
      role={role}
      className={cn(
        'flex flex-1 flex-col items-center justify-center gap-2 py-10 text-center text-sm',
        role === 'alert' ? 'text-vergex-danger' : 'text-vergex-muted'
      )}
    >
      {children}
    </div>
  )
}

const badgeTones = {
  success: 'bg-vergex-success/10 text-vergex-success',
  danger: 'bg-vergex-danger/10 text-vergex-danger',
  warning: 'bg-vergex-warning/10 text-vergex-warning',
  muted: 'bg-vergex-surface-muted text-vergex-muted',
}

export function SettingBadge({
  tone = 'muted',
  children,
}: {
  tone?: keyof typeof badgeTones
  children: ReactNode
}) {
  return (
    <span
      className={cn(
        'shrink-0 whitespace-nowrap rounded-md px-2 py-0.5 text-xs font-medium',
        badgeTones[tone]
      )}
    >
      {children}
    </span>
  )
}

// Heads a view opened from a list: back to the list, then what it is about, with its state
// and what can be opened from it.
export function SettingBack({
  onBack,
  backLabel,
  icon,
  title,
  badge,
  detail,
  children,
}: {
  onBack: () => void
  backLabel: string
  icon?: ReactNode
  title: ReactNode
  /** A SettingBadge shown right after the name. */
  badge?: ReactNode
  detail?: ReactNode
  /** A link or button on the right, such as where to get a key. */
  children?: ReactNode
}) {
  return (
    <div className="flex items-center gap-3 py-3">
      <Button
        type="button"
        variant="ghost"
        size="icon"
        onClick={onBack}
        aria-label={backLabel}
        className="-ml-2 size-8 shrink-0"
      >
        <ChevronLeft className="size-4" />
      </Button>
      {icon && (
        <span className="grid size-9 shrink-0 place-items-center rounded-md border border-vergex-line bg-vergex-surface text-vergex-muted">
          {icon}
        </span>
      )}
      <div className="min-w-0 flex-1">
        {/* Where the name and badge do not fit, the badge drops below. */}
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <h3 className="max-w-full truncate text-sm font-medium text-vergex-ink">
            {title}
          </h3>
          {badge}
        </div>
        {detail && (
          <p className="mt-1 truncate text-xs text-vergex-muted">{detail}</p>
        )}
      </div>
      {children}
    </div>
  )
}

const messageTones = {
  danger: 'text-vergex-danger',
  success: 'text-vergex-success',
  warning: 'text-vergex-warning',
  muted: 'text-vergex-muted',
}

/**
 * The bar that ends a group submitted together: its buttons on the right and,
 * on the left, a one-line message such as a validation error. The bar is as
 * tall as its buttons whether or not the message shows, so nothing moves.
 */
export function SettingActions({
  message,
  messageTone = 'danger',
  messageId,
  children,
}: {
  message?: ReactNode
  /** Errors are red; a passed check is green, a caution amber, progress muted. */
  messageTone?: keyof typeof messageTones
  /** For the fields the message is about to point aria-describedby at. */
  messageId?: string
  children: ReactNode
}) {
  return (
    <div className="flex items-center gap-2 border-t border-vergex-line py-3 max-sm:[&>a]:h-10 max-sm:[&>button]:h-10">
      <p
        id={messageId}
        aria-live="polite"
        title={typeof message === 'string' ? message : undefined}
        className={cn(
          'mr-auto min-w-0 truncate text-xs',
          messageTones[messageTone]
        )}
      >
        {message}
      </p>
      {children}
    </div>
  )
}
