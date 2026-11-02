import { expect, it } from 'vitest'
import type { AgentConfig } from '@/types'
import {
  applyPreset,
  presetOfConfig,
  withPrimaryTimeframe,
} from '@/utils/trader-presets'

const agent = (selected: string[]): AgentConfig => ({
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
  const selected = (config: AgentConfig) =>
    config.indicators.klines.selected_timeframes

  expect(selected(withPrimaryTimeframe(agent(['5m', '1h']), '4h'))).toEqual([
    '5m',
    '1h',
    '4h',
  ])
  expect(
    selected(withPrimaryTimeframe(agent(['1m', '5m', '15m', '1h']), '4h'))
  ).toEqual(['5m', '15m', '1h', '4h'])
  // An empty list means the engine derives the timeframes itself.
  expect(selected(withPrimaryTimeframe(agent([]), '4h'))).toEqual([])
})

it('recognises a config a preset produced', () => {
  const careful = withPrimaryTimeframe(agent(['15m']), '15m')
  careful.risk_control = {
    ...careful.risk_control,
    max_positions: 1,
    altcoin_max_leverage: 2,
    min_confidence: 80,
  }
  const presets = [{ id: 'careful', config: careful }]

  const applied = applyPreset(agent(['5m']), careful)
  expect(presetOfConfig(applied, presets)).toBe('careful')
  expect(applied.indicators.klines.selected_timeframes).toContain('15m')
  expect(presetOfConfig(agent(['5m']), presets)).toBe('')
})
