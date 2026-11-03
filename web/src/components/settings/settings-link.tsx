import type { ReactNode } from 'react'
import { useOpenSettings } from './use-open-settings'
import { SETTINGS_PARAM } from '@/constants/route-paths'
import type { SettingsSection } from './settings-sections'

/** A link that opens settings at a tab over the current page. */
export function SettingsLink({
  section,
  className,
  children,
}: {
  section: SettingsSection
  className?: string
  children: ReactNode
}) {
  const open = useOpenSettings()
  return (
    <a
      href={`?${SETTINGS_PARAM}=${section}`}
      className={className}
      onClick={(e) => {
        e.preventDefault()
        open(section)
      }}
    >
      {children}
    </a>
  )
}
