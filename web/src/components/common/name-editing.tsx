import { useRef, useState, type ReactNode } from 'react'
import { Check, Pencil, X } from 'lucide-react'
import { notify } from '@/components/common/notify'
import { Button } from '@/components/ui/button'
import { t, type Language } from '@/i18n'
import { cn } from '@/utils/cn'

// Edits a name where it stands, in the name's own font and in the same place. In a list
// the row itself turns into the field, with the buttons where its menu was. As a title
// the name's room is kept and the field floats over it. Either way its box takes no room,
// so nothing moves however long the name gets.
//
// Enter or the check saves, Escape or the cross keeps the old name, and leaving the field
// saves. An unchanged name saves nothing, and where a name can be empty, emptying it goes
// back to the placeholder. A name already in use keeps the field open to fix it, except on
// leaving, when the old name stays.
export function NameInput({
  value,
  label,
  placeholder,
  allowEmpty,
  isTaken,
  takenLabel,
  row,
  language,
  onSave,
  onCancel,
  className,
}: {
  value: string
  label: string
  placeholder?: string
  allowEmpty?: boolean
  isTaken?: (name: string) => boolean
  // What to say about a name already in use.
  takenLabel?: string
  row?: boolean
  language: Language
  onSave: (name: string) => void
  onCancel: () => void
  className?: string
}) {
  const [draft, setDraft] = useState(value)
  const [refused, setRefused] = useState(false)
  const field = useRef<HTMLInputElement>(null)
  const finished = useRef(false)

  const next = draft.trim()
  const changed = next !== value && (Boolean(next) || allowEmpty)
  const taken = changed && isTaken?.(next)
  const finish = (save: boolean) => {
    if (finished.current) return
    finished.current = true
    if (save && changed) onSave(next)
    else onCancel()
  }
  const confirm = () => {
    if (taken) {
      setRefused(true)
      notify.error(takenLabel ?? '')
      field.current?.focus()
    } else {
      finish(true)
    }
  }
  const leave = () => {
    if (finished.current) return
    if (taken) notify.error(takenLabel ?? '')
    finish(!taken)
  }

  const box = cn(
    'rounded-md border bg-vergex-surface',
    refused ? 'border-vergex-danger' : 'border-vergex-line-strong'
  )
  const input = (
    <input
      ref={field}
      autoFocus
      size={1}
      aria-label={label}
      aria-invalid={refused}
      placeholder={placeholder}
      value={draft}
      maxLength={80}
      onChange={(event) => {
        setDraft(event.target.value)
        setRefused(false)
      }}
      onFocus={(event) => event.target.select()}
      onBlur={leave}
      onKeyDown={(event) => {
        if (event.key === 'Enter') {
          event.preventDefault()
          confirm()
        } else if (event.key === 'Escape') {
          event.preventDefault()
          event.stopPropagation()
          finish(false)
        }
      }}
      className={cn(
        'w-full min-w-0 border-0 bg-transparent p-0 text-inherit outline-none [box-shadow:none] [font:inherit] [text-rendering:inherit] placeholder:text-vergex-faint',
        row ? 'pr-14' : 'col-start-1 row-start-1'
      )}
    />
  )
  // Pressing them keeps the focus in the field, so the cross is not taken for leaving it.
  const buttons = (size: string) => (
    <>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label={t('renameSave', language)}
        title={t('renameSave', language)}
        className={cn(size, 'text-vergex-ink')}
        onMouseDown={(event) => event.preventDefault()}
        onClick={confirm}
      >
        <Check className="size-4" aria-hidden="true" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label={t('renameCancel', language)}
        title={t('renameCancel', language)}
        className={size}
        onMouseDown={(event) => event.preventDefault()}
        onClick={() => finish(false)}
      >
        <X className="size-4" aria-hidden="true" />
      </Button>
    </>
  )

  if (row) {
    return (
      <>
        <span
          aria-hidden="true"
          className={cn(box, 'pointer-events-none absolute inset-0 -z-10')}
        />
        {input}
        <span className="absolute right-1 top-1 flex">{buttons('size-7')}</span>
      </>
    )
  }

  const sizer =
    'invisible col-start-1 row-start-1 overflow-hidden whitespace-pre'
  return (
    <span className={cn('relative z-10 inline-grid min-w-0', className)}>
      <span aria-hidden="true" className={sizer}>
        {value || placeholder}
      </span>
      <span
        className={cn(
          box,
          'absolute -left-1.5 top-1/2 flex h-7 w-max min-w-60 max-w-[min(480px,calc(100vw-56px))] -translate-y-1/2 items-center pl-[5px] pr-0.5 max-sm:h-11 max-sm:[font-size:16px]'
        )}
      >
        <span className="inline-grid min-w-0 flex-1">
          <span aria-hidden="true" className={sizer}>
            {draft || placeholder}
          </span>
          {input}
        </span>
        {buttons('size-6 max-sm:size-10')}
      </span>
    </span>
  )
}

// A detail panel's title: its name, whatever sits beside the name, and a pencil that shows
// when the title is hovered or reached by keyboard. While the name is edited, what follows
// it stays in place out of sight, so nothing moves and no word is left half covered.
export function EditableTitle({
  name,
  editing,
  field,
  renameLabel,
  onEdit,
  children,
}: {
  name: string
  editing: boolean
  // The field that edits the name, while it is being edited.
  field: ReactNode
  renameLabel: string
  // How to start editing the name; without it, there is no pencil.
  onEdit?: () => void
  children?: ReactNode
}) {
  return (
    <span className="group/title flex min-w-0 items-center gap-2">
      {editing ? (
        field
      ) : (
        <span className="max-w-64 truncate md:max-w-96" title={name}>
          {name}
        </span>
      )}
      <span
        className={cn(
          'flex shrink-0 items-center gap-2',
          editing && 'invisible'
        )}
      >
        {children}
        {onEdit && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={renameLabel}
            title={renameLabel}
            className="size-6 opacity-0 transition-opacity focus-visible:opacity-100 group-hover/title:opacity-100 [@media(hover:none)]:opacity-100 max-sm:size-10"
            onClick={onEdit}
          >
            <Pencil className="size-3.5" aria-hidden="true" />
          </Button>
        )}
      </span>
    </span>
  )
}
