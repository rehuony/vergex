import { useCallback } from 'react'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'

// The Futures copy, read from one group of futures.* translations. Stable
// across renders, so it is safe in the dependencies of polling callbacks.
export function useFuturesLabel(group: string) {
  const { language } = useLanguage()
  return useCallback(
    (key: string, params?: Record<string, string | number>) =>
      t(`futures.${group}.${key}`, language, params),
    [group, language]
  )
}
