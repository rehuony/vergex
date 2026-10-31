import { ROUTES } from '@/constants/route-paths'
import type { PageTab } from '@/components/common/page'
import { t, type Language } from '@/i18n/translations'

// Sections of the Forecast page.
export function forecastTabs(language: Language): PageTab[] {
  return [
    { to: ROUTES.forecast, label: t('forecast.tabSignals', language) },
    {
      to: ROUTES.forecastStrategies,
      label: t('forecast.tabStrategies', language),
    },
    {
      to: ROUTES.forecastBacktests,
      label: t('forecast.tabBacktests', language),
    },
  ]
}
