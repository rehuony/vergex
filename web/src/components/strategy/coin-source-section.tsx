import { t, type Language } from '@/i18n'
import { Segmented } from '@/components/common/segmented'
import { SettingRow } from '@/components/common/setting-row'
import { AppSelect } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type { AgentStrategyConfig, CoinSourceConfig } from '@/types'
import {
  COIN_SOURCE_TYPES,
  RANKED_SOURCES,
  coinSourceLabel,
} from '@/utils/coin-sources'
import { MAX_COINS } from '@/utils/strategy-profiles'
import {
  FieldGrid,
  NumberField,
  SymbolsField,
} from '@/components/common/form-fields'

const LIMITS = [1, 2, 3, 5, 8, 10]
const OI_WINDOWS = ['1h', '4h', '24h'] as const

// Where candidate coins come from: a static list, a ranking or a mix of them,
// plus the coins that are always left out.
export function CoinSourceSection({
  agent,
  language,
  onPatch,
}: {
  agent: AgentStrategyConfig
  language: Language
  onPatch: (patch: Partial<CoinSourceConfig>) => void
}) {
  const label = (key: string, params?: Record<string, string | number>) =>
    t(`strategy.${key}`, language, params)
  const source = agent.coin_source
  const sourceType = source.source_type || 'static'
  const isRanked = sourceType !== 'static'
  const usesOI =
    sourceType === 'oi_top' ||
    sourceType === 'oi_low' ||
    (sourceType === 'mixed' && Boolean(source.use_oi_top || source.use_oi_low))
  const removeLabel = (symbol: string) => label('removeSymbol', { symbol })

  return (
    <>
      <FieldGrid>
        <SettingRow label={label('coinSource')} htmlFor="strategy-source">
          <AppSelect
            id="strategy-source"
            className="h-9 max-sm:h-10"
            value={sourceType}
            options={COIN_SOURCE_TYPES.map((type) => ({
              value: type,
              label: coinSourceLabel(type, language),
            }))}
            onChange={(next) =>
              onPatch({
                source_type: next as CoinSourceConfig['source_type'],
              })
            }
          />
        </SettingRow>
        {isRanked && (
          <SettingRow label={label('minVolume')} htmlFor="strategy-min-volume">
            <NumberField
              id="strategy-min-volume"
              value={source.min_quote_volume ?? 0}
              min={0}
              step={1000000}
              suffix="USDT"
              onChange={(min_quote_volume) => onPatch({ min_quote_volume })}
            />
          </SettingRow>
        )}
        {RANKED_SOURCES.filter(
          (ranked) => sourceType === 'mixed' || sourceType === ranked.type
        ).map((ranked) => {
          const name = coinSourceLabel(ranked.type, language)
          return (
            <SettingRow key={ranked.type} label={name}>
              {sourceType === 'mixed' && (
                <Switch
                  aria-label={label('useSource', { source: name })}
                  checked={Boolean(source[ranked.enabledKey])}
                  onChange={(checked) =>
                    onPatch({ [ranked.enabledKey]: checked })
                  }
                  className="mr-2"
                />
              )}
              <AppSelect
                aria-label={label('sourceLimit', { source: name })}
                className="h-9 min-w-0 flex-1 max-sm:h-10"
                value={Number(source[ranked.limitKey]) || 3}
                options={LIMITS.map((n) => ({
                  value: n,
                  label: label('topN', { count: n }),
                }))}
                onChange={(next) =>
                  onPatch({ [ranked.limitKey]: Number(next) })
                }
              />
            </SettingRow>
          )
        })}
        {usesOI && (
          <>
            <SettingRow label={label('oiWindow')}>
              <Segmented
                aria-label={label('oiWindow')}
                value={source.oi_window || '4h'}
                options={OI_WINDOWS.map((value) => ({ value, label: value }))}
                onChange={(oi_window) => onPatch({ oi_window })}
              />
            </SettingRow>
            <SettingRow label={label('oiPool')} htmlFor="strategy-oi-pool">
              <NumberField
                id="strategy-oi-pool"
                value={source.oi_pool_size || 50}
                min={10}
                max={100}
                suffix={label('contracts')}
                onChange={(oi_pool_size) => onPatch({ oi_pool_size })}
              />
            </SettingRow>
          </>
        )}
      </FieldGrid>
      {(sourceType === 'static' || sourceType === 'mixed') && (
        <SettingRow
          label={label('staticCoins')}
          htmlFor="strategy-static-coins"
          className="border-t first:border-t"
        >
          <SymbolsField
            id="strategy-static-coins"
            values={source.static_coins || []}
            placeholder="BTCUSDT, ETHUSDT"
            max={MAX_COINS}
            limitMessage={label('coinLimit', { count: MAX_COINS })}
            addLabel={label('add')}
            removeLabel={removeLabel}
            onChange={(static_coins) => onPatch({ static_coins })}
          />
        </SettingRow>
      )}
      <SettingRow
        label={label('excludedCoins')}
        htmlFor="strategy-excluded-coins"
        className="border-t first:border-t"
      >
        <SymbolsField
          id="strategy-excluded-coins"
          values={source.excluded_coins || []}
          placeholder="DOGEUSDT"
          addLabel={label('add')}
          removeLabel={removeLabel}
          onChange={(excluded_coins) => onPatch({ excluded_coins })}
        />
      </SettingRow>
    </>
  )
}
