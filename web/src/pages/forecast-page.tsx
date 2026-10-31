import { useMemo, useState } from 'react'
import { Search, Settings } from 'lucide-react'
import useSWR from 'swr'
import { api } from '@/api'
import { FORECAST_INTERVALS, type ForecastInterval } from '@/api/forecast'
import { dialogInputClass } from '@/components/common/dialog'
import { Page } from '@/components/common/page'
import { Panel, PanelState } from '@/components/common/panel'
import { Segmented } from '@/components/common/segmented'
import { forecastTabs } from '@/components/forecast/forecast-tabs'
import {
  SignalTable,
  SignalsEmpty,
  sortSignals,
} from '@/components/forecast/signal-table'
import { SettingsLink } from '@/components/settings/settings-link'
import { buttonVariants } from '@/components/ui/button'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'
import { cn } from '@/utils/cn'

const REFRESH_MS = 15000

export function ForecastPage() {
  const { language } = useLanguage()
  const label = (key: string) => t(`forecast.${key}`, language)
  const [interval, setInterval] = useState<ForecastInterval>('1h')
  const [search, setSearch] = useState('')
  const { data, error, isLoading } = useSWR(
    ['forecast-signals', interval],
    () => api.getForecastSignals(interval),
    { refreshInterval: REFRESH_MS, keepPreviousData: true }
  )

  const signals = useMemo(() => {
    const term = search.trim().toUpperCase()
    const rows = data?.signals ?? []
    return sortSignals(
      term ? rows.filter((row) => row.symbol.includes(term)) : rows
    )
  }, [data, search])

  return (
    <Page
      title={t('forecastNav', language)}
      description={label('description')}
      tabs={forecastTabs(language)}
      actions={
        <SettingsLink
          section="forecast"
          className={buttonVariants({ variant: 'secondary', size: 'md' })}
        >
          <Settings className="size-4" aria-hidden="true" />
          {label('settings')}
        </SettingsLink>
      }
    >
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <Segmented
          aria-label={label('interval')}
          value={interval}
          options={FORECAST_INTERVALS.map((value) => ({
            value,
            label: value,
          }))}
          onChange={setInterval}
        />
        <label className="relative ml-auto min-w-40 flex-1 sm:max-w-64">
          <Search
            className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-vergex-text-muted"
            aria-hidden="true"
          />
          <input
            aria-label={label('searchCoins')}
            placeholder={label('searchCoins')}
            className={cn(
              dialogInputClass,
              'h-9 pl-9 max-sm:h-10 max-sm:text-base'
            )}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </label>
      </div>
      {error && !data ? (
        <Panel>
          <PanelState role="alert">{label('loadFailed')}</PanelState>
        </Panel>
      ) : isLoading && !data ? (
        <SignalsEmpty>{label('loading')}</SignalsEmpty>
      ) : signals.length === 0 ? (
        <SignalsEmpty>
          {search.trim() ? label('noMatch') : label('noCoins')}
        </SignalsEmpty>
      ) : (
        <SignalTable
          signals={signals}
          interval={interval}
          language={language}
        />
      )}
    </Page>
  )
}
