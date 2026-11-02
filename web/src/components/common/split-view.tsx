import { useEffect, useRef, type ReactNode } from 'react'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { MoreHorizontal, Plus, type LucideIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/utils/cn'

// A list beside what it opens, filling the screen down to the page's edge. Each side scrolls by itself, so the page does not, and the
// list's action and whatever the detail pins stay where they are. Where the two do not
// fit side by side, they follow each other in the page.
export function SplitView({
  rail,
  children,
}: {
  rail: ReactNode
  children: ReactNode
}) {
  return (
    <div className="flex min-w-0 flex-col gap-4 lg:grid lg:h-[max(440px,calc(100dvh-181px))] lg:grid-cols-[320px_minmax(0,1fr)]">
      {rail}
      {children}
    </div>
  )
}

// The list side: its entries on the page itself, with what adds one, if anything does,
// pinned underneath. The scrollbar floats in a gutter beside the entries, never over them.
export function SplitRail({
  label,
  addLabel,
  onAdd,
  top,
  children,
}: {
  label: string
  addLabel?: string
  onAdd?: () => void
  // Pinned above the list, such as a search box.
  top?: ReactNode
  children: ReactNode
}) {
  return (
    <nav aria-label={label} className="hidden min-h-0 flex-col pr-3 lg:flex">
      {top && <div className="shrink-0 pb-3">{top}</div>}
      <ScrollArea className="-mr-3 min-h-0 flex-1 pr-3">
        <ul className="space-y-0.5">{children}</ul>
      </ScrollArea>
      {onAdd && (
        <div className="shrink-0 border-t border-vergex-line pt-3">
          <Button
            type="button"
            variant="secondary"
            size="md"
            className="w-full"
            onClick={onAdd}
          >
            <Plus className="size-4" aria-hidden="true" />
            {addLabel}
          </Button>
        </div>
      )}
    </nav>
  )
}

export interface RailAction {
  label: string
  icon: LucideIcon
  onSelect: () => void
  danger?: boolean
  disabled?: boolean
  // It puts the cursor somewhere itself, so the menu should not take it back on closing.
  movesFocus?: boolean
}

const menuItemClass =
  'flex h-8 cursor-pointer select-none items-center gap-2 rounded-md px-2.5 text-sm text-vergex-ink outline-none data-[highlighted]:bg-vergex-surface-muted data-[disabled]:pointer-events-none data-[disabled]:opacity-45'

// What can be done to an entry, from a … button.
export function ActionMenu({
  label,
  actions,
  className,
}: {
  label: string
  actions: RailAction[]
  className?: string
}) {
  // What was chosen runs once the menu has closed, so it can take the focus.
  const chosen = useRef<RailAction | null>(null)
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={label}
          className={className}
        >
          <MoreHorizontal className="size-4" aria-hidden="true" />
        </Button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          onCloseAutoFocus={(event) => {
            const action = chosen.current
            chosen.current = null
            if (action?.movesFocus) event.preventDefault()
            action?.onSelect()
          }}
          className="z-[60] min-w-44 origin-[var(--radix-dropdown-menu-content-transform-origin)] animate-menu-in rounded-2xl border border-vergex-line bg-vergex-surface p-1.5 shadow-(--shadow-float)"
        >
          {actions.map((action) => (
            <DropdownMenu.Item
              key={action.label}
              disabled={action.disabled}
              className={cn(
                menuItemClass,
                action.danger && 'text-vergex-danger'
              )}
              onSelect={() => {
                chosen.current = action
              }}
            >
              <action.icon className="size-4" aria-hidden="true" />
              {action.label}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

// One entry in the list. The current one sits on the ink tint. Its menu shows on hover,
// and a double click on it edits the name in place.
export function RailItem({
  current,
  title,
  name,
  details,
  onOpen,
  rename,
  onStartRename,
  actions,
  actionsLabel,
}: {
  current: boolean
  title?: string
  name: ReactNode
  details?: ReactNode
  onOpen: () => void
  // The field that edits the name, while it is being edited.
  rename?: ReactNode
  onStartRename?: () => void
  actions?: RailAction[]
  actionsLabel?: string
}) {
  const row = useRef<HTMLLIElement>(null)
  useEffect(() => {
    if (current) row.current?.scrollIntoView?.({ block: 'nearest' })
  }, [current])

  // A button has its own weight and shaping for text. The row states them, so its text
  // does not change on turning to the field.
  const rowClass =
    'flex w-full min-w-0 flex-col gap-0.5 rounded-md px-3 py-2 text-left font-medium transition-colors [text-rendering:inherit]'
  return (
    <li ref={row} className="group relative isolate">
      {rename ? (
        <div className={rowClass}>
          <div className="flex min-w-0 text-[13px] font-medium text-vergex-ink">
            {rename}
          </div>
          {details}
        </div>
      ) : (
        <>
          <button
            type="button"
            aria-current={current ? 'true' : undefined}
            title={title}
            onClick={onOpen}
            onDoubleClick={onStartRename}
            className={cn(
              rowClass,
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-vergex-ink/25',
              current ? 'bg-vergex-ink-tint' : 'hover:bg-vergex-surface-muted'
            )}
          >
            <span
              className={cn(
                'truncate text-[13px] font-medium text-vergex-ink',
                actions && 'pr-6'
              )}
            >
              {name}
            </span>
            {details}
          </button>
          {actions && (
            <ActionMenu
              label={actionsLabel ?? ''}
              actions={actions}
              className={cn(
                'absolute right-1 top-1 size-7 transition-opacity focus-visible:opacity-100 group-hover:opacity-100 data-[state=open]:opacity-100 [@media(hover:none)]:opacity-100',
                !current && 'opacity-0'
              )}
            />
          )}
        </>
      )}
    </li>
  )
}
