import { expect, it } from 'vitest'
import type { AIStrategyConfig } from '@/types'
import {
  applyProfile,
  profileFromConfig,
  profileOptions,
  withPrimaryTimeframe,
} from '@/utils/strategy-profiles'

const ai = (selected: string[]): AIStrategyConfig => ({
  coin_source: { source_type: 'static' },
  indicators: {
    klines: {
      primary_timeframe: '5m',
      primary_count: 20,
      enable_multi_timeframe: false,
      selected_timeframes: selected,
    },
    enable_raw_klines: true,
    enable_ema: false,
    enable_macd: false,
    enable_rsi: false,
    enable_atr: false,
    enable_boll: false,
    enable_volume: true,
    enable_oi: true,
    enable_funding_rate: true,
  },
  risk_control: {
    max_positions: 3,
    btc_eth_max_leverage: 5,
    altcoin_max_leverage: 5,
    max_margin_usage: 0.9,
    min_position_size: 12,
    min_risk_reward_ratio: 3,
    min_confidence: 75,
  },
})

it('keeps the main timeframe among those fetched, within the limit', () => {
  const selected = (config: AIStrategyConfig) =>
    config.indicators.klines.selected_timeframes

  expect(selected(withPrimaryTimeframe(ai(['5m', '1h']), '4h'))).toEqual([
    '5m',
    '1h',
    '4h',
  ])
  expect(
    selected(withPrimaryTimeframe(ai(['1m', '5m', '15m', '1h']), '4h'))
  ).toEqual(['5m', '15m', '1h', '4h'])
  // An empty list means the engine derives the timeframes itself.
  expect(selected(withPrimaryTimeframe(ai([]), '4h'))).toEqual([])
})

it('recognises a config a profile produced', () => {
  const careful = profileOptions[0]
  const applied = applyProfile(ai(['5m']), careful)
  expect(profileFromConfig(applied)).toBe('careful')
  expect(applied.indicators.klines.selected_timeframes).toContain(
    careful.timeframe
  )
})
