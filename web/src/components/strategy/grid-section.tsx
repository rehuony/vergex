import { t, type Language } from '@/i18n/translations'
import { Segmented } from '@/components/common/segmented'
import { SettingRow } from '@/components/common/setting-row'
import { Switch } from '@/components/ui/switch'
import type { GridStrategyConfig } from '@/types'
import { parseSymbols } from '@/utils/symbols'
import {
  FieldGrid,
  NumberField,
  fieldClass,
  switchRowClass,
} from '@/components/common/form-fields'

const DISTRIBUTIONS: GridStrategyConfig['distribution'][] = [
  'uniform',
  'gaussian',
  'pyramid',
]

// What must hold before a grid strategy can be saved, as a message for the
// action bar; empty when it is fine.
export function gridProblem(grid: GridStrategyConfig, language: Language) {
  const label = (key: string) => t(`strategy.grid.${key}`, language)
  if (!parseSymbols(grid.symbol).length) return label('needSymbol')
  if (grid.total_investment <= 0) return label('needInvestment')
  if (
    !grid.use_atr_bounds &&
    (grid.lower_price <= 0 || grid.upper_price <= grid.lower_price)
  ) {
    return label('needBounds')
  }
  return ''
}

// The whole of a grid strategy: the market and size, how the price range is
// set, and the limits that stop it.
export function GridSection({
  grid,
  language,
  onPatch,
}: {
  grid: GridStrategyConfig
  language: Language
  onPatch: (patch: Partial<GridStrategyConfig>) => void
}) {
  const label = (key: string) => t(`strategy.grid.${key}`, language)

  return (
    <FieldGrid>
      <SettingRow label={label('symbol')} htmlFor="grid-symbol">
        <input
          id="grid-symbol"
          className={fieldClass}
          value={grid.symbol}
          onChange={(event) =>
            onPatch({ symbol: event.target.value.toUpperCase() })
          }
          onBlur={(event) => {
            const [symbol] = parseSymbols(event.target.value)
            if (symbol) onPatch({ symbol })
          }}
        />
      </SettingRow>
      <SettingRow label={label('investment')} htmlFor="grid-investment">
        <NumberField
          id="grid-investment"
          value={grid.total_investment}
          min={1}
          step={100}
          suffix="USDT"
          onChange={(total_investment) => onPatch({ total_investment })}
        />
      </SettingRow>
      <SettingRow label={label('count')} htmlFor="grid-count">
        <NumberField
          id="grid-count"
          value={grid.grid_count}
          min={5}
          max={50}
          onChange={(grid_count) => onPatch({ grid_count })}
        />
      </SettingRow>
      <SettingRow label={label('leverage')} htmlFor="grid-leverage">
        <NumberField
          id="grid-leverage"
          value={grid.leverage}
          min={1}
          max={20}
          suffix="×"
          onChange={(leverage) => onPatch({ leverage })}
        />
      </SettingRow>
      <SettingRow label={label('distribution')}>
        <Segmented
          aria-label={label('distribution')}
          value={grid.distribution}
          options={DISTRIBUTIONS.map((value) => ({
            value,
            label: label(value),
          }))}
          onChange={(distribution) => onPatch({ distribution })}
        />
      </SettingRow>
      <SettingRow label={label('atrBounds')} className={switchRowClass}>
        <Switch
          aria-label={label('atrBounds')}
          checked={grid.use_atr_bounds}
          onChange={(use_atr_bounds) => onPatch({ use_atr_bounds })}
          className="my-2"
        />
      </SettingRow>
      {grid.use_atr_bounds ? (
        <SettingRow
          label={label('atrMultiplier')}
          htmlFor="grid-atr"
          className="xl:col-span-2"
        >
          <NumberField
            id="grid-atr"
            value={grid.atr_multiplier}
            min={0.5}
            max={10}
            step={0.1}
            suffix="× ATR"
            onChange={(atr_multiplier) => onPatch({ atr_multiplier })}
          />
        </SettingRow>
      ) : (
        <>
          <SettingRow label={label('upperPrice')} htmlFor="grid-upper">
            <NumberField
              id="grid-upper"
              value={grid.upper_price}
              min={0}
              suffix="USDT"
              onChange={(upper_price) => onPatch({ upper_price })}
            />
          </SettingRow>
          <SettingRow label={label('lowerPrice')} htmlFor="grid-lower">
            <NumberField
              id="grid-lower"
              value={grid.lower_price}
              min={0}
              suffix="USDT"
              onChange={(lower_price) => onPatch({ lower_price })}
            />
          </SettingRow>
        </>
      )}
      <SettingRow label={label('maxDrawdown')} htmlFor="grid-drawdown">
        <NumberField
          id="grid-drawdown"
          value={grid.max_drawdown_pct}
          min={1}
          max={100}
          suffix="%"
          onChange={(max_drawdown_pct) => onPatch({ max_drawdown_pct })}
        />
      </SettingRow>
      <SettingRow label={label('stopLoss')} htmlFor="grid-stop-loss">
        <NumberField
          id="grid-stop-loss"
          value={grid.stop_loss_pct}
          min={0.1}
          max={100}
          step={0.5}
          suffix="%"
          onChange={(stop_loss_pct) => onPatch({ stop_loss_pct })}
        />
      </SettingRow>
      <SettingRow label={label('dailyLoss')} htmlFor="grid-daily-loss">
        <NumberField
          id="grid-daily-loss"
          value={grid.daily_loss_limit_pct}
          min={1}
          max={100}
          suffix="%"
          onChange={(daily_loss_limit_pct) => onPatch({ daily_loss_limit_pct })}
        />
      </SettingRow>
      <SettingRow label={label('makerOnly')} className={switchRowClass}>
        <Switch
          aria-label={label('makerOnly')}
          checked={grid.use_maker_only}
          onChange={(use_maker_only) => onPatch({ use_maker_only })}
          className="my-2"
        />
      </SettingRow>
      <SettingRow label={label('directionAdjust')} className={switchRowClass}>
        <Switch
          aria-label={label('directionAdjust')}
          checked={Boolean(grid.enable_direction_adjust)}
          onChange={(enable_direction_adjust) =>
            onPatch({ enable_direction_adjust })
          }
          className="my-2"
        />
      </SettingRow>
      {grid.enable_direction_adjust && (
        <SettingRow label={label('biasRatio')} htmlFor="grid-bias">
          <NumberField
            id="grid-bias"
            value={Math.round((grid.direction_bias_ratio ?? 0.7) * 100)}
            min={50}
            max={100}
            suffix="%"
            onChange={(percent) =>
              onPatch({ direction_bias_ratio: percent / 100 })
            }
          />
        </SettingRow>
      )}
    </FieldGrid>
  )
}
