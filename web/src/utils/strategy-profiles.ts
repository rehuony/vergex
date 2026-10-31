import type { AIStrategyConfig, StrategyConfig } from '@/types'

// Autopilot strategy settings the strategy page edits: candle choices, coin
// list limits and the risk profiles that set several limits at once.

export const TIMEFRAMES = ['1m', '3m', '5m', '15m', '1h', '4h']
export const MAX_COINS = 10
export const MAX_TIMEFRAMES = 4

export type Profile = 'careful' | 'balanced' | 'active'

export interface ProfileOption {
  value: Profile
  maxPositions: number
  leverage: number
  confidence: number
  timeframe: string
  bars: number
}

export const profileOptions: ProfileOption[] = [
  {
    value: 'careful',
    maxPositions: 1,
    leverage: 3,
    confidence: 82,
    timeframe: '1h',
    bars: 30,
  },
  // Same values as the server's default strategy (store.GetDefaultStrategyConfig),
  // so an untouched strategy shows as Balanced.
  {
    value: 'balanced',
    maxPositions: 3,
    leverage: 5,
    confidence: 75,
    timeframe: '5m',
    bars: 20,
  },
  {
    value: 'active',
    maxPositions: 5,
    leverage: 10,
    confidence: 68,
    timeframe: '5m',
    bars: 30,
  },
]

export function getAIConfig(config: StrategyConfig): AIStrategyConfig | null {
  if (config.ai_config) return config.ai_config
  if (config.coin_source && config.indicators && config.risk_control) {
    return {
      coin_source: config.coin_source,
      indicators: config.indicators,
      risk_control: config.risk_control,
      prompt_sections: config.prompt_sections,
      custom_prompt: config.custom_prompt,
    }
  }
  return null
}

export function profileFromConfig(ai: AIStrategyConfig | null): Profile | null {
  if (!ai) return null
  const match = profileOptions.find(
    (p) =>
      p.maxPositions === ai.risk_control.max_positions &&
      p.leverage === ai.risk_control.altcoin_max_leverage &&
      p.confidence === ai.risk_control.min_confidence &&
      p.timeframe === ai.indicators.klines.primary_timeframe
  )
  return match?.value ?? null
}

// withPrimaryTimeframe sets the main timeframe and keeps it among the fetched
// ones, dropping the oldest other choice when the list is full: the engine
// reads the main timeframe's data out of that list.
export function withPrimaryTimeframe(
  current: AIStrategyConfig,
  timeframe: string
): AIStrategyConfig {
  const selected = current.indicators.klines.selected_timeframes ?? []
  let next = selected
  if (selected.length > 0 && !selected.includes(timeframe)) {
    next = [...selected, timeframe]
    if (next.length > MAX_TIMEFRAMES) {
      const drop = next.findIndex((tf) => tf !== timeframe)
      next = next.filter((_, index) => index !== drop)
    }
  }
  return {
    ...current,
    indicators: {
      ...current.indicators,
      klines: {
        ...current.indicators.klines,
        primary_timeframe: timeframe,
        selected_timeframes: next,
      },
    },
  }
}

// applyProfile sets the candles and risk limits a profile stands for.
export function applyProfile(
  current: AIStrategyConfig,
  profile: ProfileOption
): AIStrategyConfig {
  const withTimeframe = withPrimaryTimeframe(current, profile.timeframe)
  return {
    ...withTimeframe,
    indicators: {
      ...withTimeframe.indicators,
      klines: {
        ...withTimeframe.indicators.klines,
        primary_count: profile.bars,
      },
    },
    risk_control: {
      ...current.risk_control,
      max_positions: profile.maxPositions,
      btc_eth_max_leverage: profile.leverage,
      altcoin_max_leverage: profile.leverage,
      min_confidence: profile.confidence,
    },
  }
}
