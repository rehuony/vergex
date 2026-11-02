import type { ReactNode } from 'react'
import { Segmented } from '@/components/common/segmented'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n'
import { resolutions } from './futures-chart/futures-chart-data'
import { format } from './futures-labels'
import { ForecastSwitch } from './futures-forecast'

interface FuturesChartPanelProps {
  // Picks the market the chart shows.
  market: ReactNode
  price: string
  resolution: string
  onResolution: (resolution: string) => void
  forecastOn: boolean
  onForecastChange: (next: boolean) => void
  // Drawn over the chart.
  overlay: ReactNode
}

const TIMEFRAMES = Object.entries(resolutions).map(([value, label]) => ({
  value,
  label,
}))

// FuturesChartPanel frames the market chart, the main surface of the page, with what it
// shows in its head. The chart itself mounts into #chart-container.
export function FuturesChartPanel({
  market,
  price,
  resolution,
  onResolution,
  forecastOn,
  onForecastChange,
  overlay,
}: FuturesChartPanelProps) {
  const { language } = useLanguage()
  return (
    <section
      id="futures-chart"
      className="flex min-h-0 min-w-0 scroll-mt-20 flex-col overflow-hidden rounded-2xl border border-vergex-line bg-vergex-surface max-lg:h-[68dvh] max-lg:min-h-96"
    >
      <div className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-b border-vergex-line p-2">
        {market}
        <span className="mono min-w-24 text-xl leading-[1.2] tracking-[-0.01em] text-vergex-ink">
          {price === '—' ? price : format(price)}
        </span>
        <div className="ml-auto flex items-center gap-3">
          <ForecastSwitch on={forecastOn} onChange={onForecastChange} />
          <Segmented
            aria-label={t('futures.chartTimeframe', language)}
            value={resolution}
            options={TIMEFRAMES}
            onChange={onResolution}
          />
        </div>
      </div>
      <div className="relative min-h-0 flex-1">
        <div id="chart-container" className="h-full" />
        {overlay}
      </div>
    </section>
  )
}
