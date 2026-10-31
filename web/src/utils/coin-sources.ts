import { t, type Language } from '@/i18n/translations'
import type { CoinSourceConfig } from '@/types'

export type CoinSourceType = CoinSourceConfig['source_type']

export type RankedSourceType = Exclude<CoinSourceType, 'static' | 'mixed'>

export const COIN_SOURCE_TYPES: CoinSourceType[] = [
  'static',
  'oi_top',
  'oi_low',
  'volume_top',
  'gainers',
  'losers',
  'mixed',
]

export const RANKED_SOURCES: Array<{
  type: RankedSourceType
  enabledKey: keyof CoinSourceConfig
  limitKey: keyof CoinSourceConfig
}> = [
  { type: 'oi_top', enabledKey: 'use_oi_top', limitKey: 'oi_top_limit' },
  { type: 'oi_low', enabledKey: 'use_oi_low', limitKey: 'oi_low_limit' },
  {
    type: 'volume_top',
    enabledKey: 'use_volume_top',
    limitKey: 'volume_top_limit',
  },
  { type: 'gainers', enabledKey: 'use_gainers', limitKey: 'gainers_limit' },
  { type: 'losers', enabledKey: 'use_losers', limitKey: 'losers_limit' },
]

export function coinSourceLabel(type: string, language: Language): string {
  if (!COIN_SOURCE_TYPES.includes(type as CoinSourceType)) return type || '—'
  return t(`strategy.source.${type}`, language)
}
