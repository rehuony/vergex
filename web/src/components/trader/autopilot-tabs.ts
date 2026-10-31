import { t, type Language } from '@/i18n/translations'
import { ROUTES } from '@/constants/route-paths'
import type { PageTab } from '@/components/common/page'

// Sections of the Autopilot page: the traders and the strategies they run.
export function autopilotTabs(language: Language): PageTab[] {
  return [
    { to: ROUTES.autopilot, label: t('tradersTab', language) },
    { to: ROUTES.strategies, label: t('strategiesTab', language) },
  ]
}
