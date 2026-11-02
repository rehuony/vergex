import { t, type Language } from '@/i18n'
import { Segmented } from '@/components/common/segmented'
import { SettingRow } from '@/components/common/setting-row'
import type { AgentConfig, RiskControlConfig } from '@/types'
import {
  applyPreset,
  presetOfConfig,
  useAgentPresets,
} from '@/utils/trader-presets'
import { FieldGrid, NumberField } from '@/components/common/form-fields'

// A preset and the individual limits it sets. The ranges match the limits the
// server holds them to (kind.RiskControl).
export function RiskProfileSection({
  agent,
  language,
  onPatchAgent,
}: {
  agent: AgentConfig
  language: Language
  onPatchAgent: (patch: (agent: AgentConfig) => AgentConfig) => void
}) {
  const label = (key: string) => t(`traderSettings.${key}`, language)
  const risk = agent.risk_control
  const patch = (next: Partial<RiskControlConfig>) =>
    onPatchAgent((current) => ({
      ...current,
      risk_control: { ...current.risk_control, ...next },
    }))
  const equity = label('timesEquity')
  const presets = useAgentPresets()

  return (
    <FieldGrid>
      <SettingRow label={label('riskProfile')}>
        <Segmented<string>
          aria-label={label('riskProfile')}
          value={presetOfConfig(agent, presets)}
          options={presets.map((preset) => ({
            value: preset.id,
            label: label(`profile.${preset.id}`),
          }))}
          onChange={(value) => {
            const preset = presets.find((p) => p.id === value)
            if (preset) onPatchAgent((c) => applyPreset(c, preset.config))
          }}
        />
      </SettingRow>
      <SettingRow label={label('maxPositions')} htmlFor="risk-max-positions">
        <NumberField
          id="risk-max-positions"
          value={risk.max_positions}
          min={1}
          max={8}
          onChange={(max_positions) => patch({ max_positions })}
        />
      </SettingRow>
      <SettingRow label={label('btcEthLeverage')} htmlFor="risk-btc-leverage">
        <NumberField
          id="risk-btc-leverage"
          value={risk.btc_eth_max_leverage}
          min={1}
          max={20}
          suffix="×"
          onChange={(btc_eth_max_leverage) => patch({ btc_eth_max_leverage })}
        />
      </SettingRow>
      <SettingRow label={label('altLeverage')} htmlFor="risk-alt-leverage">
        <NumberField
          id="risk-alt-leverage"
          value={risk.altcoin_max_leverage}
          min={1}
          max={20}
          suffix="×"
          onChange={(altcoin_max_leverage) => patch({ altcoin_max_leverage })}
        />
      </SettingRow>
      <SettingRow label={label('btcEthCap')} htmlFor="risk-btc-cap">
        <NumberField
          id="risk-btc-cap"
          value={risk.btc_eth_max_position_value_ratio ?? 5}
          min={0.5}
          max={10}
          step={0.5}
          suffix={equity}
          onChange={(btc_eth_max_position_value_ratio) =>
            patch({ btc_eth_max_position_value_ratio })
          }
        />
      </SettingRow>
      <SettingRow label={label('altCap')} htmlFor="risk-alt-cap">
        <NumberField
          id="risk-alt-cap"
          value={risk.altcoin_max_position_value_ratio ?? 5}
          min={0.5}
          max={10}
          step={0.5}
          suffix={equity}
          onChange={(altcoin_max_position_value_ratio) =>
            patch({ altcoin_max_position_value_ratio })
          }
        />
      </SettingRow>
      <SettingRow label={label('marginUsage')} htmlFor="risk-margin-usage">
        <NumberField
          id="risk-margin-usage"
          value={Math.round(risk.max_margin_usage * 100)}
          min={10}
          max={100}
          suffix="%"
          onChange={(percent) => patch({ max_margin_usage: percent / 100 })}
        />
      </SettingRow>
      <SettingRow label={label('minPosition')} htmlFor="risk-min-position">
        <NumberField
          id="risk-min-position"
          value={risk.min_position_size}
          min={10}
          max={1000}
          suffix="USDT"
          onChange={(min_position_size) => patch({ min_position_size })}
        />
      </SettingRow>
      <SettingRow label={label('minRiskReward')} htmlFor="risk-min-rr">
        <NumberField
          id="risk-min-rr"
          value={risk.min_risk_reward_ratio}
          min={1}
          max={10}
          step={0.1}
          onChange={(min_risk_reward_ratio) => patch({ min_risk_reward_ratio })}
        />
      </SettingRow>
      <SettingRow label={label('minConfidence')} htmlFor="risk-min-confidence">
        <NumberField
          id="risk-min-confidence"
          value={risk.min_confidence}
          min={50}
          max={100}
          onChange={(min_confidence) => patch({ min_confidence })}
        />
      </SettingRow>
    </FieldGrid>
  )
}
