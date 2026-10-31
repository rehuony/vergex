import { t, type Language } from '@/i18n/translations'
import { Segmented } from '@/components/common/segmented'
import { SettingRow } from '@/components/common/setting-row'
import { Button } from '@/components/ui/button'
import { AppSelect } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type { AIStrategyConfig, IndicatorConfig, KlineConfig } from '@/types'
import {
  MAX_TIMEFRAMES,
  TIMEFRAMES,
  withPrimaryTimeframe,
} from '@/utils/strategy-profiles'
import {
  FieldGrid,
  NumberField,
  PeriodsField,
  switchRowClass,
} from '@/components/common/form-fields'

const INDICATORS: Array<{
  name: string
  enabled: keyof IndicatorConfig
  periods?: 'ema_periods' | 'rsi_periods' | 'atr_periods' | 'boll_periods'
}> = [
  { name: 'ema', enabled: 'enable_ema', periods: 'ema_periods' },
  { name: 'macd', enabled: 'enable_macd' },
  { name: 'rsi', enabled: 'enable_rsi', periods: 'rsi_periods' },
  { name: 'atr', enabled: 'enable_atr', periods: 'atr_periods' },
  { name: 'boll', enabled: 'enable_boll', periods: 'boll_periods' },
  { name: 'volume', enabled: 'enable_volume' },
  { name: 'oi', enabled: 'enable_oi' },
  { name: 'funding', enabled: 'enable_funding_rate' },
]

// The candles the model reads and the indicators worked out from them.
export function MarketDataSection({
  ai,
  language,
  onPatchAI,
}: {
  ai: AIStrategyConfig
  language: Language
  onPatchAI: (patch: (ai: AIStrategyConfig) => AIStrategyConfig) => void
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`strategy.${key}`, language, params)
  const klines = ai.indicators.klines
  const selected = klines.selected_timeframes ?? []
  const patchIndicators = (next: Partial<IndicatorConfig>) =>
    onPatchAI((current) => ({
      ...current,
      indicators: { ...current.indicators, ...next },
    }))
  const patchKlines = (next: Partial<KlineConfig>) =>
    onPatchAI((current) => ({
      ...current,
      indicators: {
        ...current.indicators,
        klines: { ...current.indicators.klines, ...next },
      },
    }))
  const toggleTimeframe = (timeframe: string) => {
    if (timeframe === klines.primary_timeframe) return
    patchKlines({
      selected_timeframes: selected.includes(timeframe)
        ? selected.filter((tf) => tf !== timeframe)
        : [...selected, timeframe],
    })
  }

  return (
    <>
      <FieldGrid>
        <SettingRow label={label('primaryTimeframe')}>
          <Segmented
            aria-label={label('primaryTimeframe')}
            value={klines.primary_timeframe}
            options={TIMEFRAMES.map((value) => ({ value, label: value }))}
            onChange={(timeframe) =>
              onPatchAI((current) => withPrimaryTimeframe(current, timeframe))
            }
          />
        </SettingRow>
        <SettingRow label={label('bars')} htmlFor="strategy-bars">
          <NumberField
            id="strategy-bars"
            value={klines.primary_count}
            min={10}
            max={30}
            onChange={(primary_count) => patchKlines({ primary_count })}
          />
        </SettingRow>
        <SettingRow label={label('alsoFetch')}>
          <div
            role="group"
            aria-label={label('alsoFetch')}
            className="flex flex-wrap gap-1.5"
          >
            {TIMEFRAMES.map((timeframe) => {
              const on =
                timeframe === klines.primary_timeframe ||
                selected.includes(timeframe)
              return (
                <Button
                  key={timeframe}
                  type="button"
                  variant="option"
                  size="sm"
                  aria-pressed={on}
                  disabled={
                    timeframe === klines.primary_timeframe ||
                    (!on && selected.length >= MAX_TIMEFRAMES)
                  }
                  onClick={() => toggleTimeframe(timeframe)}
                  className="h-8 px-2.5 text-xs max-sm:h-10"
                >
                  {timeframe}
                </Button>
              )
            })}
          </div>
        </SettingRow>
        <SettingRow label={label('longerContext')}>
          <Switch
            aria-label={label('longerContext')}
            checked={klines.enable_multi_timeframe}
            onChange={(enable_multi_timeframe) =>
              patchKlines({ enable_multi_timeframe })
            }
          />
          {klines.enable_multi_timeframe && (
            <div className="ml-2 flex min-w-0 flex-1 items-center gap-2">
              <AppSelect
                aria-label={label('longerTimeframe')}
                className="h-9 min-w-0 flex-1 max-sm:h-10"
                value={klines.longer_timeframe || '4h'}
                options={TIMEFRAMES.map((value) => ({ value, label: value }))}
                onChange={(longer_timeframe) =>
                  patchKlines({ longer_timeframe })
                }
              />
            </div>
          )}
        </SettingRow>
      </FieldGrid>
      <FieldGrid className="border-t border-[var(--line)]">
        {INDICATORS.map(({ name, enabled, periods }) => {
          const on = Boolean(ai.indicators[enabled])
          const title = label(`indicator.${name}`)
          return (
            <SettingRow
              key={name}
              label={title}
              className={on && periods ? undefined : switchRowClass}
            >
              <Switch
                aria-label={title}
                checked={on}
                onChange={(checked) => patchIndicators({ [enabled]: checked })}
                className="my-2"
              />
              {on && periods && (
                <div className="ml-2 flex min-w-0 flex-1 items-center gap-2">
                  <PeriodsField
                    ariaLabel={label('indicatorPeriods', { name: title })}
                    value={ai.indicators[periods] ?? []}
                    onChange={(next) => patchIndicators({ [periods]: next })}
                  />
                </div>
              )}
            </SettingRow>
          )
        })}
      </FieldGrid>
    </>
  )
}
