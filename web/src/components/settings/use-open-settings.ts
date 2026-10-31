import { useSearchParams } from 'react-router-dom'
import type { SettingsSection } from './settings-dialog'
import { SETTINGS_PARAM } from '@/constants/route-paths'

/**
 * Opens settings at a tab over the current page, or closes them with null.
 * They live in the URL, so a reload or a shared link reopens them, and the
 * page's own query parameters stay.
 */
export function useOpenSettings() {
  const [, setSearchParams] = useSearchParams()
  return (section: SettingsSection | null) =>
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        if (section) next.set(SETTINGS_PARAM, section)
        else next.delete(SETTINGS_PARAM)
        return next
      },
      { replace: true }
    )
}
