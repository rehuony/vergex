import { useEffect, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Page } from '@/components/common/page'
import { t, type Language } from '@/i18n'
import {
  backtestsQuery,
  forecastSettingsQuery,
  marketSettingsQuery,
  marketStatusQuery,
  performanceQuery,
  strategiesQuery,
} from './forecast-queries'
import { forecastTabs } from './forecast-tabs'

// What every Forecast page shares: its tabs, and what a page puts beside them. It enters
// once per tab: what a tab has open is in the address, and opening another coin, strategy
// or run moves nothing else. It also reads what the other tabs open on, and loads the
// chart's code, so going to any of them shows it at once.
export function ForecastFrame({
  language,
  actions,
  children,
}: {
  language: Language
  actions?: ReactNode
  children: ReactNode
}) {
  const client = useQueryClient()
  useEffect(() => {
    void client.prefetchQuery(backtestsQuery)
    void client.prefetchQuery(forecastSettingsQuery)
    void client.prefetchQuery(marketSettingsQuery)
    void client.prefetchQuery(marketStatusQuery)
    void client
      .fetchQuery(strategiesQuery)
      .then(({ active_strategy_id: inUse }) => {
        if (inUse) void client.prefetchQuery(performanceQuery(inUse))
      })
      .catch(() => {
        /* Only reading ahead: the page itself says when a read fails. */
      })
    void import('@/components/market-chart/local-market-chart').catch(() => {
      /* Only loading ahead: the chart itself says when it cannot load. */
    })
  }, [client])

  return (
    <Page
      title={t('forecastNav', language)}
      className="vergex-screen animate-page-in"
      tabs={forecastTabs(language)}
      actions={actions}
    >
      {children}
    </Page>
  )
}
