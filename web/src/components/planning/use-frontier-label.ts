import { useCallback } from 'react'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'

// Frontier's copy, read from one group of frontier.* translations. Stable
// across renders, so it is safe in the dependencies of polling callbacks.
export function useFrontierLabel(group: string) {
  const { language } = useLanguage()
  return useCallback(
    (key: string, params?: Record<string, string | number>) =>
      t(`frontier.${group}.${key}`, language, params),
    [group, language]
  )
}
