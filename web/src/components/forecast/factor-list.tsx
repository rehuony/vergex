import type { ForecastFactor } from '@/api/forecast'
import { Panel } from '@/components/common/panel'
import { t, type Language } from '@/i18n/translations'

// The factors behind a forecast, biggest influence first. Each bar runs from a
// centre line: left argues for down, right for up.
export function FactorList({
  factors,
  language,
}: {
  factors: ForecastFactor[]
  language: Language
}) {
  const largest = Math.max(
    0.01,
    ...factors.map((factor) => Math.abs(factor.contribution))
  )
  return (
    <Panel title={t('forecast.factors', language)}>
      <ul className="flex flex-col gap-3 px-4 pb-4">
        {factors.map((factor) => {
          const width = (Math.abs(factor.contribution) / largest) * 100
          const up = factor.contribution >= 0
          return (
            <li key={factor.name}>
              <div className="flex items-baseline justify-between gap-2 text-[13px]">
                <span className="truncate text-vergex-text">
                  {t(`forecast.factor.${factor.name}`, language)}
                </span>
                <span className="tabular-nums text-vergex-text-muted">
                  {factor.value > 0 ? '+' : ''}
                  {factor.value.toFixed(2)}
                </span>
              </div>
              <div
                className="mt-1 flex h-1 rounded-full bg-[var(--line)]"
                aria-hidden="true"
              >
                <div className="flex w-1/2 justify-end">
                  {!up && (
                    <div
                      className="h-1 rounded-l-full bg-vergex-danger"
                      style={{ width: `${width}%` }}
                    />
                  )}
                </div>
                <div className="w-1/2">
                  {up && (
                    <div
                      className="h-1 rounded-r-full bg-vergex-success"
                      style={{ width: `${width}%` }}
                    />
                  )}
                </div>
              </div>
            </li>
          )
        })}
      </ul>
    </Panel>
  )
}
