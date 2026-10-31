import type { ReactNode } from 'react'
import type { Plan } from '@/api/planning'
import { Button } from '@/components/ui/button'
import { useLanguage } from '@/contexts/language-context'
import { t } from '@/i18n/translations'
import { ExecutionSchedule } from './execution-schedule'

interface FrontierChartPanelProps {
  plan: Plan | undefined
  comparing: number
  compareDisabled: boolean
  onFit: () => void
  onCompare: () => void
  onExitCompare: () => void
  // Drawn over the chart.
  overlay?: ReactNode
}

function Legend({
  className,
  children,
}: {
  className: string
  children: string
}) {
  return (
    <span className="flex items-center gap-1.5 whitespace-nowrap">
      <i className={`size-2 rounded-full ${className}`} aria-hidden="true" />
      {children}
    </span>
  )
}

// FrontierChartPanel frames the market chart, the main surface of the page. The
// chart itself mounts into #chart-container, with its timeframes on top.
export function FrontierChartPanel({
  plan,
  comparing,
  compareDisabled,
  onFit,
  onCompare,
  onExitCompare,
  overlay,
}: FrontierChartPanelProps) {
  const { language } = useLanguage()
  const label = (key: string) => t(`frontier.${key}`, language)
  return (
    <section
      id="frontier-chart"
      className="flex min-h-0 min-w-0 scroll-mt-20 flex-col overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface)] max-lg:h-[68dvh] max-lg:min-h-96"
    >
      <div className="relative min-h-0 flex-1">
        <div id="chart-container" className="h-full" />
        {overlay}
      </div>
      {plan && comparing < 2 && <ExecutionSchedule plan={plan} />}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-[var(--line)] px-3 py-2 text-xs text-vergex-text-muted">
        <Legend className="bg-[var(--warning)]">{label('entry')}</Legend>
        <Legend className="bg-vergex-danger">{label('stopLoss')}</Legend>
        <Legend className="bg-vergex-success">{label('takeProfit')}</Legend>
        <span className="ml-auto flex gap-2">
          <Button
            type="button"
            size="sm"
            className="h-8 px-2.5 text-xs max-sm:h-10"
            disabled={!plan}
            onClick={onFit}
          >
            {label('fitPlan')}
          </Button>
          {comparing > 1 ? (
            <Button
              type="button"
              size="sm"
              className="h-8 px-2.5 text-xs max-sm:h-10"
              onClick={onExitCompare}
            >
              {label('exitCompare')}
            </Button>
          ) : (
            <Button
              type="button"
              size="sm"
              className="h-8 px-2.5 text-xs max-sm:h-10"
              disabled={compareDisabled}
              onClick={onCompare}
            >
              {label('comparePlans')}
            </Button>
          )}
        </span>
      </div>
    </section>
  )
}
