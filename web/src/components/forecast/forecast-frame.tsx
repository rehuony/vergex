import type { ReactNode } from 'react'
import { Page } from '@/components/common/page'
import { t, type Language } from '@/i18n'
import { forecastTabs } from './forecast-tabs'
import { StrategySelect } from './strategy-select'

// What every Forecast page shares: its tabs, and the strategy in use beside them.
export function ForecastFrame({
  language,
  note,
  children,
}: {
  language: Language
  note?: ReactNode
  children: ReactNode
}) {
  return (
    <Page
      title={t('forecastNav', language)}
      tabs={forecastTabs(language)}
      actions={
        <>
          {note}
          <StrategySelect language={language} />
        </>
      }
    >
      {children}
    </Page>
  )
}
