import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import type { AgentConfig, TraderPreset } from '@/types'

// A trader's settings that stand for several limits at once: the starting points the
// server offers, and the candle choices and coin list limits of the editor.

export const TIMEFRAMES = ['1m', '3m', '5m', '15m', '1h', '4h']
export const MAX_COINS = 10
export const MAX_TIMEFRAMES = 4

// The order they read in, gentlest first; the server lists the default first.
const AGENT_PRESET_ORDER = ['careful', 'balanced', 'active']

// useAgentPresets reads the agent presets once and shares them.
export function useAgentPresets(): TraderPreset<AgentConfig>[] {
  const { data } = useQuery({
    queryKey: ['trader-presets', 'agent'],
    queryFn: () => api.getTraderPresets('agent'),
    refetchOnWindowFocus: false,
  })
  return [...((data ?? []) as TraderPreset<AgentConfig>[])].sort(
    (a, b) =>
      AGENT_PRESET_ORDER.indexOf(a.id) - AGENT_PRESET_ORDER.indexOf(b.id)
  )
}

// presetOfConfig names the preset a config matches on what a preset sets, or nothing.
export function presetOfConfig(
  agent: AgentConfig,
  presets: TraderPreset<AgentConfig>[]
): string {
  const match = presets.find(
    ({ config }) =>
      config.risk_control.max_positions === agent.risk_control.max_positions &&
      config.risk_control.altcoin_max_leverage ===
        agent.risk_control.altcoin_max_leverage &&
      config.risk_control.min_confidence ===
        agent.risk_control.min_confidence &&
      config.indicators.klines.primary_timeframe ===
        agent.indicators.klines.primary_timeframe
  )
  return match?.id ?? ''
}

// withPrimaryTimeframe sets the main timeframe and keeps it among the fetched
// ones, dropping the oldest other choice when the list is full: the engine
// reads the main timeframe's data out of that list.
export function withPrimaryTimeframe(
  current: AgentConfig,
  timeframe: string
): AgentConfig {
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

// applyPreset sets the candles and risk limits a preset stands for.
export function applyPreset(
  current: AgentConfig,
  preset: AgentConfig
): AgentConfig {
  const withTimeframe = withPrimaryTimeframe(
    current,
    preset.indicators.klines.primary_timeframe
  )
  return {
    ...withTimeframe,
    indicators: {
      ...withTimeframe.indicators,
      klines: {
        ...withTimeframe.indicators.klines,
        primary_count: preset.indicators.klines.primary_count,
      },
    },
    risk_control: {
      ...current.risk_control,
      max_positions: preset.risk_control.max_positions,
      btc_eth_max_leverage: preset.risk_control.btc_eth_max_leverage,
      altcoin_max_leverage: preset.risk_control.altcoin_max_leverage,
      min_confidence: preset.risk_control.min_confidence,
    },
  }
}
