import { ROUTES } from '@/constants/route-paths'
import type { PageTab } from '@/components/common/page'
import { t, type Language } from '@/i18n/translations'

// Sections of the Forecast page.
export function forecastTabs(language: Language): PageTab[] {
  const others: string[] = [
    ROUTES.forecastStrategies,
    ROUTES.forecastBacktests,
    ROUTES.forecastSettings,
  ]
  return [
    {
      to: ROUTES.forecast,
      label: t('forecast.tabSignals', language),
      // Signals covers every coin's own page.
      isActive: (pathname) => !others.includes(pathname),
    },
    {
      to: ROUTES.forecastStrategies,
      label: t('forecast.tabStrategies', language),
    },
    {
      to: ROUTES.forecastBacktests,
      label: t('forecast.tabBacktests', language),
    },
    {
      to: ROUTES.forecastSettings,
      label: t('forecast.tabSettings', language),
    },
  ]
}
