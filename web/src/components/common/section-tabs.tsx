import type { KeyboardEvent, ReactNode } from 'react'

export interface SectionTab<T extends string> {
  id: T
  label: ReactNode
}

// Tabs inside a page, looking like the page's own section tabs. Left and right
// arrow keys move between them. The caller shows the matching panel.
export function SectionTabs<T extends string>({
  tabs,
  value,
  onChange,
  label,
  className,
}: {
  tabs: SectionTab<T>[]
  value: T
  onChange: (id: T) => void
  label: string
  className?: string
}) {
  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    const step =
      event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : 0
    if (!step) return
    event.preventDefault()
    const index = tabs.findIndex((tab) => tab.id === value)
    const next = tabs[(index + step + tabs.length) % tabs.length]
    onChange(next.id)
    event.currentTarget.parentElement
      ?.querySelector<HTMLButtonElement>(`[data-tab="${next.id}"]`)
      ?.focus()
  }
  return (
    <div
      role="tablist"
      aria-label={label}
      className={`vergex-page-tabs ${className ?? ''}`}
    >
      {tabs.map((tab) => (
        <button
          key={tab.id}
          type="button"
          role="tab"
          data-tab={tab.id}
          aria-selected={value === tab.id}
          tabIndex={value === tab.id ? 0 : -1}
          onClick={() => onChange(tab.id)}
          onKeyDown={onKeyDown}
        >
          {tab.label}
        </button>
      ))}
    </div>
  )
}
