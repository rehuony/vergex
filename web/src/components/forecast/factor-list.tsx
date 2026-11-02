import type { ForecastResult } from '@/api/forecast'
import { Panel, PanelState } from '@/components/common/panel'
import { ScrollArea } from '@/components/ui/scroll-area'
import { t, type Language } from '@/i18n'

const headClass =
  'sticky top-0 z-10 bg-vergex-surface px-2 py-2 text-[11px] font-medium whitespace-nowrap'
const cellClass = 'px-2 py-2 whitespace-nowrap'

// The factors behind a forecast, biggest influence first, then those with no reading.
// Each bar runs from a centre line: left argues for down, right for up.
export function FactorList({
  result,
  pending,
  language,
  className,
}: {
  result: ForecastResult | null | undefined
  // What to say while there is nothing to list.
  pending: string
  language: Language
  className?: string
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`forecast.${key}`, language, params)
  const factors = result?.factors ?? []
  const largest = Math.max(
    0.01,
    ...factors.map((factor) => Math.abs(factor.contribution))
  )
  return (
    <Panel
      title={label('factors')}
      meta={
        result
          ? `${label(`regimeName.${result.regime}`)} · ${label('adxValue', { value: result.adx.toFixed(0) })}`
          : undefined
      }
      className={className}
    >
      {factors.length === 0 ? (
        <PanelState role="status" className="min-h-0 flex-1">
          {pending}
        </PanelState>
      ) : (
        <ScrollArea className="min-h-0 flex-1 rounded-b-2xl px-2 pb-2">
          <table className="w-full text-left text-[13px] tabular-nums">
            <thead className="text-vergex-muted">
              <tr>
                <th className={headClass}>{label('factor.name')}</th>
                <th className={`${headClass} text-right`}>
                  {label('reading')}
                </th>
                <th className={`${headClass} text-right`}>{label('weight')}</th>
                <th className={`${headClass} w-[40%]`}>{label('effect')}</th>
              </tr>
            </thead>
            <tbody>
              {factors.map((factor) => {
                const width = (Math.abs(factor.contribution) / largest) * 100
                const up = factor.contribution >= 0
                return (
                  <tr key={factor.name} className="border-t border-vergex-line">
                    <td className={`${cellClass} text-vergex-ink`}>
                      {label(`factor.${factor.name}`)}
                    </td>
                    <td className={`${cellClass} text-right font-mono text-xs`}>
                      {factor.value > 0 ? '+' : ''}
                      {factor.value.toFixed(2)}
                    </td>
                    <td
                      className={`${cellClass} text-right font-mono text-xs text-vergex-muted`}
                    >
                      {factor.weight.toFixed(1)}
                    </td>
                    <td className={cellClass}>
                      <div
                        className="flex h-1 rounded-full bg-vergex-line"
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
                    </td>
                  </tr>
                )
              })}
              {(result?.missing ?? []).map((name) => (
                <tr
                  key={name}
                  title={label('noReading')}
                  className="border-t border-vergex-line text-vergex-muted"
                >
                  <td className={cellClass}>{label(`factor.${name}`)}</td>
                  <td className={`${cellClass} text-right font-mono text-xs`}>
                    —
                  </td>
                  <td className={`${cellClass} text-right font-mono text-xs`}>
                    —
                  </td>
                  <td className={`${cellClass} text-xs`}>
                    {label('noReading')}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </ScrollArea>
      )}
    </Panel>
  )
}
